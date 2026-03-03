# Label-Driven YurtHub 安装与卸载

|          title          | authors     | reviewers | creation-date | last-updated | status |
|:-----------------------:|-------------| --------- |---------------| ------------ | ------ |
| Label-Driven YurtHub 安装与卸载 | @Vacantlot-07734 |           | 2026-03-03    |              | Draft  |

<!-- TOC -->
* [Label-Driven YurtHub 安装与卸载](#label-driven-yurthub-安装与卸载)
    * [Summary](#summary)
    * [Motivation](#motivation)
        * [Goals](#goals)
        * [Non-Goals/Future Work](#non-goals)
    * [Proposal](#proposal)
      * [总体架构设计](#总体架构设计)
      * [YurtHubInstallController 核心控制逻辑](#yurthubinstallcontroller-核心控制逻辑)
      * [Node-Servant 安装与卸载逻辑扩展](#node-servant-安装与卸载逻辑扩展)
    * [Implementation History](#implementation-history)
<!-- TOC -->

## Summary
OpenYurt 当前边缘节点的核心组件 **YurtHub** 主要是通过社区提供的 `yurtadm` 工具在节点加入集群时作为 systemd service 安装部署的。虽然将存量标准的 Kubernetes 节点改造为 OpenYurt 节点可以通过手动配置并拉起 YurtHub（例如手动添加 `/etc/kubernetes/manifests/yurt-hub.yaml` 以 StaticPod 模式启动等）来实现，但这种强入侵且繁琐的操作对在大规模集群中实现存量节点的平滑迁移及自动化生命周期管理造成了不小的阻碍。

本提案旨在引入一种基于 **Label-Driven** 的声明式自动化部署机制。用户只需在对应的 Kubernetes Node 上打上特定 Label（例如 `openyurt.io/is-edge-worker=true` 且存在有效的 NodePool 标签），OpenYurt 的控制平面（通过一个新增的 Controller）将自动侦听此类变更，并通过下发特权 Job 调度 `node-servant`，以 Systemd Service 模式在对应的 Node 上自动安装 YurtHub。反之，当移除该 Label 时，即自动进行资源清理并卸载 YurtHub。

## Motivation

当前，YurtHub 作为一个透明代理运行在所有边缘节点的系统组件（如 kubelet, CNI, CoreDNS, kube-proxy）与 Kubernetes API Server 之间。然而在实际落地中，用户往往期望将已有的标准 Kubernetes 节点平滑无缝地接入到 OpenYurt 控制平面。如果依赖于人工进入节点配置环境（如编写 StaticPod 配置或配置 systemd），不仅极大增加了运维成本，还容易因配置错误导致服务中断。

为了提升用户体验、降低接入成本并增强框架的灵活性，社区希望能够支持基于 Label 驱动的按需自动化安装与卸载：
1. **自动安装**：当集群中任意 Node 被添加边缘属性标签（如 `openyurt.io/is-edge-worker=true`）后，YurtHub 系统组件应当自动被拉起并在该节点启动。
2. **优雅卸载**：当移除该标签后，YurtHub 能够被安全有序地停止、禁用并清理掉相关依赖配置，避免环境污染。

此功能将极大简化将边缘环境下的 Kubernetes 节点迁移到 OpenYurt 生态的难度。

### Goals
1. **设计 Controller**：设计并实现一个监听 Node Label 的 Operator / Controller（即 `YurtHubInstallController`），触发并在目标边缘节点管理 YurtHub 生命周期。
2. **实现特权级安装卸载操作**：
   - 依赖现有的 `node-servant` 组件，新增并补全针对 Systemd 制式 YurtHub 二进制的安装与卸载能力。
   - 实现 Kubelet 流量代理配置在部署前后的接管和回撤。
   - 确保安装流程具备幂等性（Idempotency）以及在出错时的自动重试与优雅退出能力。

### Non-Goals/Future Work
1. 本提案暂仅聚焦于 **YurtHub 组件的部署与生命周期管理**，暂不涉及对其他 OpenYurt 系统核心组件（如 raven-agent，yurt-manager 等）下发部署逻辑的改造。
2. 当前方案下暂不支持由 Controller 动态生成与下发节点纳管所需的 Bootstrap Token，部署期将依赖于外部配置或手动指定固定的有效 Token。在未来规划中，可以考虑深度对接 Kubernetes API 自动下发 Token，从而进一步提升安全性和可用性。

## Proposal

### 总体架构设计
本提案采用 **Controller + Job** 方案实现基于 Label 触发的任务下发机制。

- **控制面（YurtManager）**：新增 `YurtHubInstallController`，监听所有带有特定 Label 及对应 Annotation 更新的 Node 事件。当需要进行状态改变时，创建下发位于 `kube-system` Namespace 的 `node-servant-install/uninstall` Job。
- **目标节点（Node）**：部署的 Job 携带了 NodeSelector/NodeName 指向目标 Node，并在节点上以 HostNetwork/HostPID 启动一个 privileged 权限的 `node-servant` 容器，来挂载宿主机文件系统直接写入 /usr/local/bin 、systemd service unit 和相关配置后将其作为宿主机的 service 拉起运行。

### YurtHubInstallController 核心控制逻辑

#### 接入链路与 RBAC
Controller 通过现有 `yurt-manager` 注册链路接入控制面：
1. 在 `cmd/yurt-manager/names/controller_names.go` 中新增 controller 名称与别名。
2. 在 `pkg/yurtmanager/controller/apis/config/types.go` 与 `cmd/yurt-manager/app/options/...` 中新增配置和参数定义。
3. 在 `pkg/yurtmanager/controller/base/controller.go` 中注册 controller initializer。

Controller 需要具备以下核心控制权限：
- **Node (corev1)**: `get`, `list`, `watch`, `update`, `patch`
- **Job (batchv1)**: `get`, `list`, `watch`, `create`, `update`, `patch`, `delete`

**事件侦听（Watch）策略**：
1. **Watch Node**：
   - 监听含有特定身份标签 `openyurt.io/is-edge-worker=true` 添加或移除的 Node 变更事件。
   - 监听部署状态相关注解变更，包括：
     - `openyurt.io/yurthub-install-status`
     - `openyurt.io/yurthub-install-retry-count`
     - `openyurt.io/yurthub-install-active-job`
     - `openyurt.io/yurthub-install-locked`
2. **Watch Job**：
   - 侦听含有标签 `openyurt.io/yurthub-install-node=<NodeName>` 的安装/卸载 Job。
   - Job 完成后的成功/失败结果反向映射回 Node Reconcile 以触发状态演进。

#### 状态机流转设计（可执行规则）
为了确保部署容错性和可回溯性，在 Node annotation 上记录状态与执行元数据。

**状态与元数据注解**：
- `openyurt.io/yurthub-install-status`: `installing`、`installed`、`uninstalling`、`uninstalled`、`failed`
- `openyurt.io/yurthub-install-last-action`: `install` 或 `uninstall`
- `openyurt.io/yurthub-install-active-job`: 当前活动 Job 名称
- `openyurt.io/yurthub-install-retry-count`: 重试计数
- `openyurt.io/yurthub-install-locked`: `true` 或 `false`
- `openyurt.io/yurthub-install-lock-reason`: 例如 `max-retries-exceeded`

**流转规则（State Machine）**：
1. **前置校验与锁定门禁**：
   - 若 `openyurt.io/yurthub-install-locked=true`，不再创建新 Job。
   - 解锁为人工操作：排障后由运维清理 `locked/lock-reason` 并重置 `retry-count`。
   - 若 `wantInstall=true` 但缺失 `apps.openyurt.io/nodepool`，标记 `status=failed`、`last-action=install` 并记录失败原因；不创建安装 Job，直到 NodePool 标签补齐。
2. **活动 Job 处理**：
   - 若存在 `active-job` 且 Job 仍在运行，则 Requeue 等待。
   - 若 Job 成功：
     - `last-action=install` -> `status=installed`
     - `last-action=uninstall` -> `status=uninstalled`
     - 清理 `active-job` 并重置 `retry-count`
   - 若 Job 失败：
     - 设置 `status=failed`
     - 清理 `active-job`
     - 递增 `retry-count`
     - 若 `retry-count >= maxRetry`（默认 3），设置 `locked=true` 与 `lock-reason=max-retries-exceeded`
3. **期望态收敛（仅在无活动 Job 且未锁定时）**：
   - 若 `wantInstall=true` 且 `status!=installed`，创建 Install Job，并设置 `status=installing`、`last-action=install`、`active-job=<jobName>`。
   - 若 `wantInstall=false` 且 `status` 不为 `""`/`uninstalled`，创建 Uninstall Job，并设置 `status=uninstalling`、`last-action=uninstall`、`active-job=<jobName>`。
4. **失败退避与重试**：
   - 仅当 `status=failed`、`locked=false` 且 `retry-count < maxRetry` 时触发重试。
   - 使用有界退避，例如 `30s -> 60s -> 120s`。
5. **标签快速反复变更（Flapping）**：
   - 运行中的 Job 不被中断。
   - 当标签在 `installing`/`uninstalling` 期间变化时，先等待当前 Job 结束，再由下一轮 Reconcile 按最新期望态收敛。
   - 每个 Node 始终最多跟踪一个 `active-job`，保证串行执行。

### Node-Servant 安装与卸载逻辑扩展
使用 `node-servant` 作为边缘操作执行者，提供 `install` 和 `uninstall` CLI 支持。
**安装流程**：
在容器内挂载宿主机根目录（`/`）至 `/openyurt`，并将各类资源写入其宿主路径：
1. 下载指定版本的 `yurthub` 二进制。
2. 构造 `bootstrap-hub.conf` 文件注入鉴权参数。
3. 渲染配置并生成 `yurthub.service` 及其 `10-yurthub.conf` 参数注入的 Systemd Unit，写入宿主机。
4. 在特权容器下调用 `nsenter --mount=/proc/1/ns/mnt -- systemctl daemon-reload`，随后执行 `enable yurthub.service` 与 `start yurthub.service`，对宿主服务层进行管理。
5. 转移节点 Kubelet 配置中的 Server API 重定向至 `127.0.0.1:10261`，即把发往 APIServer 的所有请求强行路由至 YurtHub 中。

**卸载流程**：
1. 最优先反向恢复 Kubelet 中的 ApiServer 代理地址配置。
2. 同样通过 `nsenter` 在宿主机直接停止并 Disable 对应 Service。如果遇到服务本身就 `not loaded` 或者 `not found` 的系统提示则直接跳过该阶段抛错。
3. 清除此前通过 `node-servant` 创建落盘的各类资源路径。
4. 根据 `--clean-data` 可信指令来决定是否一并移除 YurtHub 工作状态产生的历史残留资源与缓存。

## Implementation History
- [x] 03/03/2026: Proposed idea based on implementation document. 
