# openyurtio/openyurt

<div align="center">

<img src="docs/img/OpenYurt.png" width="400" height="94"><br/>

[![Version](https://img.shields.io/badge/OpenYurt-v0.6.0-orange)](CHANGELOG.md)
[![License](https://img.shields.io/badge/license-Apache%202-4EB1BA.svg)](https://www.apache.org/licenses/LICENSE-2.0.html)
[![Go Report Card](https://goreportcard.com/badge/github.com/openyurtio/openyurt)](https://goreportcard.com/report/github.com/openyurtio/openyurt)

</div>

[English](./README.md) | 简体中文

|![notification](docs/img/bell-outline-badge.svg) What is NEW!|
|------------------|
| 最新发布：2022-01-11  OpenYurt v0.6.0 请查看 [CHANGELOG](CHANGELOG.md) 来获得更多更新细节.|
| 第一个发布：2020-05-29 OpenYurt v0.1.0-beta.1 |

OpenYurt (官网: <https://openyurt.io>) 是基于Upstream Kubernetes构建的，现在是托管在云原生基金会(CNCF) 下的 [沙箱项目](https://www.cncf.io/sandbox-projects/).

<div align="left">
  <img src="docs/img/overview.png" width=80% title="OpenYurt Overview ">
</div>

OpenYurt是为满足典型边缘基础设施的各种DevOps需求而设计的。
通过OpenYurt来管理边缘应用程序，用户可以获得与中心式云计算应用管理一致的用户体验。
它解决了Kubernetes在云边一体化场景下的诸多挑战，如不可靠或断开的云边缘网络、边缘节点自治、边缘设备管理、跨地域业务部署等。
OpenYurt保持了完整的Kubernetes API兼容性，无厂商绑定，更重要的是，它使用简单。

## 架构

OpenYurt 遵循经典的云边一体化架构。
集群的Kubernetes管控面部署在云端(或者中心机房中)，而由集群管理的边缘节点位于靠近数据源的边缘站点中。
每个边缘节点都具有适量的计算资源，从而可以运行边缘应用以及OpenYurt系统组件。集群中的边缘节点可以分布在多个物理区域，这些物理区域在OpenYurt中称为Pools。
集群中的边缘节点可以分处于在多个物理区域中（region）。
<div align="left">
  <img src="docs/img/arch.png" width=70% title="OpenYurt architecture">
</div>

上图展示了OpenYurt的核心架构。OpenYurt 的主要组件包括：
- **YurtHub**：一个节点守护进程，充当Kubelet、Kube-Proxy、CNI插件等原生Kubernetes组件的出站流量代理。它将所有可能访问的API资源缓存到边缘节点的本地存储中。如果边缘节点离线，Yurthub可以帮助节点在重新启动后恢复状态。
- **Yurt Controller Manager**：基于原生节点控制器增强来支持边缘计算需求。例如，即使节点心跳丢失，处于自治模式的节点中的pod也不会从APIServer中驱逐。
- **Yurt App Manager**：它管理OpenYurt中引入的四个CRD资源:[NodePool](docs/enhancements/20201211-nodepool_uniteddeployment.md)，[YurtAppSet](docs/enhancements/20201211-nodepool_uniteddeployment.md)(以前的UnitedDeployment)，[YurtAppDaemon](docs/enhancements/20210729-yurtappdaemon.md)， [YurtIngress](docs/proposals/20210628-nodepool-ingress-support.md)。
`NodePool`为同一区域或站点内的节点资源提供了方便的管理。`YurtAppSet`定义了一个基于节点池维度的工作负载管理模型。`YurtAppDaemon`从节点池维度提供一种类似K8s DaemonSet的工作负载管理模型。`YurtIngress`负责将Ingress Controller部署到用户指定的节点池。
- **Yurt Tunnel (server/agent)**：`TunnelServer`通过反向代理与在每个边缘节点中运行的 TunnelAgent 守护进程建立连接并以此在云端的控制平面与处于企业内网(Intranet)环境的边缘节点之间建立安全的网络访问。

此外，OpenYurt还包括用于集成和定制的辅助控制器。

- **Node resource manager**: 统一管理OpenYurt集群的本地节点资源。 目前支持管理LVM、QuotaPath和Pmem内存。
  详情请参考[node-resource-manager](https://github.com/openyurtio/node-resource-manager)。
- **集成EdgeX Foundry平台，使用Kubernetes CRD管理边缘设备!**
<table>
<tr style="border:none">
<td style="width:80%;border:none">OpenYurt 引入了 <a href="https://github.com/openyurtio/yurt-edgex-manager">Yurt-edgex-manager</a> 来管理EdgeX Foundry软件套件的生命周期，并通过Kubernetes自定义资源引入 <a href="https://github.com/openyurtio/yurt-device-controller">Yurt-device-controller</a> 来管理EdgeX Foundry托管的边缘设备。详情请参阅简短的 <b>demo</b> 演示和有关组件的repo。
<td style="border:none"><a href="https://www.bilibili.com/video/BV1Mh411t7Q3"><img src="docs/img/demo.jpeg" width=150%></a>
</table>

## 开始之前

安装OpenYurt前，请检查[资源和系统要求](./docs/resource-and-system-requirements-cn.md)

## 开始使用
OpenYurt 支持最高版本为1.21的 Kubernetes 。使用更高版本的 Kubernetes 可能会导致兼容性问题。

为了用户更方便的使用OpenYurt，用户可以根据下表，选择合适的方法来安装OpenYurt。

| 体验方法                                 | 参考链接                                                     | 预计时间 |
| ---------------------------------------- | ---------------------- | -------- |
| 通过OpenYurt体验中心 | [OpenYurt体验中心](https://openyurt.io/docs/next/installation/openyurt-experience-center/overview) | < 1min   |
| 从零安装一个OpenYurt集群                 | [yurtctl](https://openyurt.io/docs/next/installation/yurtctl-init-join) | <5min    |
| 通过Operator把K8s集群转换为OpenYurt集群        | [yurtcluster-operator](https://openyurt.io/docs/next/installation/yurtcluster)       | <5min    |
| 手动转换K8s集群为OpenYurt集群            | [manual](https://openyurt.io/docs/next/installation/manually-setup)    | 10min   |

## 使用方法
OpenYurt官网提供详细的[教程](https://openyurt.io/docs/next/)来演示如何使用 OpenYurt。

## 发展规划
[2021年 发展规划](docs/roadmap.md)

## 社区
### 贡献
如果您愿意为 OpenYurt 项目做贡献，请参阅我们的 [CONTRIBUTING](CONTRIBUTING.md) 文档以获取详细信息。我们还准备了开发人员指南来帮助代码贡献者。

### 周会

| Item        | Value  |
|---------------------|---|
| 社区会议 | [从2020.9.2开始双周会议，周三上午11:00～12：00(北京时间)](https://calendar.google.com/calendar/u/0?cid=c3VudDRtODc2Y2c3Ymk3anN0ZDdkbHViZzRAZ3JvdXAuY2FsZW5kYXIuZ29vZ2xlLmNvbQ) |
| 会议链接 | https://us02web.zoom.us/j/82828315928?pwd=SVVxek01T2Z0SVYraktCcDV4RmZlUT09 |
| 会议纪要| [会议议程及纪要](https://shimo.im/docs/rGK3cXYWYkPrvWp8) |
| 会议视频| [B站 OpenYurt](https://space.bilibili.com/484245424/video) |

### 联络方式
如果您对本项目有任何疑问或想做出贡献，欢迎通过 github issue 或 pull request 来沟通相关问题，其他有效的沟通渠道如下所示：

- 邮件组: https://groups.google.com/g/openyurt/
- Slack: [channel](https://join.slack.com/t/openyurt/shared_invite/zt-10dxeg3v9-PRApjbfTWN6G3sIIHan2kA)
- Dingtalk Group (钉钉讨论群)

<div align="left">
  <img src="docs/img/ding.jpg" width=25% title="dingtalk">
</div>

## 许可证
OpenYurt 遵循 Apache 2.0许可证。有关详细信息请参见 [LICENSE](LICENSE) 文件。 OpenYurt 中的某些特定实现是基于 Kubernetes 的现有代码，这些实现都应归功于Kubernetes相关代码的原作者。
