# Label-Driven YurtHub Installation and Uninstallation

|          title          | authors     | reviewers | creation-date | last-updated | status |
|:-----------------------:|-------------| --------- |---------------| ------------ | ------ |
| Label-Driven YurtHub Installation and Uninstallation | @Vacantlot-07734 |           | 2026-03-03    |              | Draft  |

<!-- TOC -->
* [Label-Driven YurtHub Installation and Uninstallation](#label-driven-yurthub-installation-and-uninstallation)
    * [Summary](#summary)
    * [Motivation](#motivation)
        * [Goals](#goals)
        * [Non-Goals/Future Work](#non-goalsfuture-work)
    * [Proposal](#proposal)
      * [Overall Architecture Design](#overall-architecture-design)
      * [YurtHubInstallController Core Logic](#yurthubinstallcontroller-core-logic)
        * [Controller Wiring and RBAC](#controller-wiring-and-rbac)
        * [State Machine Workflow (Executable Rules)](#state-machine-workflow-executable-rules)
      * [Node-Servant Installation and Uninstallation Extensions](#node-servant-installation-and-uninstallation-extensions)
    * [Implementation History](#implementation-history)
<!-- TOC -->

## Summary
OpenYurt's core edge node component, **YurtHub**, is primarily installed and deployed as a systemd service using the community-provided `yurtadm` tool when a node joins the cluster. Although migrating an existing standard Kubernetes node to an OpenYurt node can be achieved by manually configuring and pulling up YurtHub (e.g., manually adding `/etc/kubernetes/manifests/yurt-hub.yaml` to start it in StaticPod mode), this highly intrusive and tedious operation poses a significant hurdle to achieving smooth migration and automated lifecycle management of existing nodes in large-scale clusters.

This proposal aims to introduce a **Label-Driven** declarative automated deployment mechanism. Users simply need to apply a specific label (e.g., `openyurt.io/is-edge-worker=true` along with a valid NodePool label) on the corresponding Kubernetes Node. The OpenYurt control plane (via a newly added Controller) will automatically listen for these changes and dispatch a privileged Job to schedule `node-servant`, automatically installing YurtHub as a Systemd Service on the corresponding Node. Conversely, when the label is removed, it automatically performs resource cleanup and uninstalls YurtHub.

## Motivation

Currently, YurtHub operates as a transparent proxy between all edge node system components (such as kubelet, CNI, CoreDNS, kube-proxy) and the Kubernetes API Server. However, in practical applications, users often expect to smoothly and seamlessly integrate their existing standard Kubernetes nodes into the OpenYurt control plane. Relying on manual intervention to configure the node environment (like writing StaticPod configurations or configuring systemd) not only significantly increases O&M costs but also easily leads to service disruptions due to misconfigurations.

To improve user experience, reduce integration costs, and enhance the framework's flexibility, the community wishes to support on-demand automated installation and uninstallation driven by Labels:
1. **Automated Installation**: When any Node in the cluster is assigned an edge attribute label (e.g., `openyurt.io/is-edge-worker=true`), the YurtHub system components should be automatically pulled up and started on that node.
2. **Graceful Uninstallation**: When the label is removed, YurtHub should be safely and orderly stopped, disabled, and its related dependency configurations cleaned up to avoid environmental pollution.

This feature will greatly simplify the difficulty of migrating Kubernetes nodes in edge environments into the OpenYurt ecosystem.

### Goals
1. **Design Controller**: Design and implement an Operator/Controller (namely `YurtHubInstallController`) that watches Node Labels, triggering and managing the YurtHub lifecycle on target edge nodes.
2. **Implement Privileged Installation and Uninstallation Operations**:
   - Relying on the existing `node-servant` component, add and complete installation and uninstallation capabilities for the Systemd-based YurtHub binary.
   - Implement the takeover and rollback of Kubelet traffic proxy configurations before and after deployment.
   - Ensure the installation process possesses Idempotency, as well as automatic retry and graceful exit capabilities in case of errors.

### Non-Goals/Future Work
1. This proposal currently focuses solely on **YurtHub component deployment and lifecycle management**. It does not currently involve transforming the dispatch and deployment logic of other OpenYurt core system components (like raven-agent, yurt-manager, etc.).
2. The current scheme temporarily does not support dynamically generating and dispatching the Bootstrap Token required for node management by the Controller. The deployment phase will rely on externally configured or manually specified valid fixed Tokens. In future planning, deep integration with the Kubernetes API to automatically dispatch Tokens can be considered to further enhance security and usability.

## Proposal

### Overall Architecture Design
This proposal adopts a **Controller + Job** approach to implement a task dispatch mechanism triggered by Labels.

- **Control Plane (YurtManager)**: A new `YurtHubInstallController` is added to listen to all Node events with specific Labels and their corresponding Annotation updates. When a state change is required, it creates and dispatches a `node-servant-install/uninstall` Job in the `kube-system` Namespace.
- **Target Node (Node)**: The deployed Job carries a NodeSelector/NodeName pointing to the target Node, and starts a privileged `node-servant` container on the node with HostNetwork/HostPID. This container mounts the host filesystem to write directly to `/usr/local/bin`, systemd service units, and related configurations, pulling it up as a host service.

### YurtHubInstallController Core Logic

#### Controller Wiring and RBAC
The controller is wired into the `yurt-manager` control plane through the existing registration chain:
1. Add a controller name/alias in `cmd/yurt-manager/names/controller_names.go`.
2. Add controller configuration and options in `pkg/yurtmanager/controller/apis/config/types.go` and `cmd/yurt-manager/app/options/...`.
3. Register the controller initializer in `pkg/yurtmanager/controller/base/controller.go`.

The controller requires the following core control permissions:
- **Node (corev1)**: `get`, `list`, `watch`, `update`, `patch`
- **Job (batchv1)**: `get`, `list`, `watch`, `create`, `update`, `patch`, `delete`

**Event Watch Strategy**:
1. **Watch Node**:
   - Listen for Node change events where the specific identity label `openyurt.io/is-edge-worker=true` is added or removed.
   - Listen for changes to deployment status annotations, including:
     - `openyurt.io/yurthub-install-status`
     - `openyurt.io/yurthub-install-retry-count`
     - `openyurt.io/yurthub-install-active-job`
     - `openyurt.io/yurthub-install-locked`
2. **Watch Job**:
   - Listen for install/uninstall Jobs containing the label `openyurt.io/yurthub-install-node=<NodeName>`.
   - Job results are mapped back to Node Reconcile to drive state transitions.

#### State Machine Workflow (Executable Rules)
To ensure fault tolerance and traceability, the controller records state and execution metadata on Node annotations.

**State and metadata annotations**:
- `openyurt.io/yurthub-install-status`: `installing`, `installed`, `uninstalling`, `uninstalled`, `failed`
- `openyurt.io/yurthub-install-last-action`: `install` or `uninstall`
- `openyurt.io/yurthub-install-active-job`: current active Job name
- `openyurt.io/yurthub-install-retry-count`: integer retry counter
- `openyurt.io/yurthub-install-locked`: `true` or `false`
- `openyurt.io/yurthub-install-lock-reason`: e.g. `max-retries-exceeded`

**Workflow rules (state machine)**:
1. **Precheck and lock gate**:
   - If `openyurt.io/yurthub-install-locked=true`, do not create new Jobs.
   - Unlock is manual: operators clear `locked/lock-reason` and reset `retry-count` after remediation.
   - If `wantInstall=true` but `apps.openyurt.io/nodepool` is missing, mark `status=failed`, `last-action=install`, and set a failure reason. No install Job is created until the NodePool label appears.
2. **Active Job processing**:
   - If `active-job` exists and the Job is still running, requeue and wait.
   - If Job succeeds:
     - `last-action=install` -> set `status=installed`
     - `last-action=uninstall` -> set `status=uninstalled`
     - clear `active-job` and reset `retry-count`
   - If Job fails:
     - set `status=failed`
     - clear `active-job`
     - increment `retry-count`
     - if `retry-count >= maxRetry` (default 3), set `locked=true` and `lock-reason=max-retries-exceeded`
3. **Desired-state convergence (only when no active Job and not locked)**:
   - If `wantInstall=true` and `status!=installed`, create an Install Job and set `status=installing`, `last-action=install`, `active-job=<jobName>`.
   - If `wantInstall=false` and `status` is not `""`/`uninstalled`, create an Uninstall Job and set `status=uninstalling`, `last-action=uninstall`, `active-job=<jobName>`.
4. **Backoff and retry**:
   - Retry only when `status=failed`, `locked=false`, and `retry-count < maxRetry`.
   - Use bounded backoff (example: 30s -> 60s -> 120s).
5. **Label flapping**:
   - Running Jobs are not interrupted.
   - If the edge-worker label flips during `installing`/`uninstalling`, the controller waits for current Job completion, then runs the next reconcile based on desired state.
   - Per-node execution remains serial by design: at most one active Job is tracked in `openyurt.io/yurthub-install-active-job`.

### Node-Servant Installation and Uninstallation Extensions
Using `node-servant` as the edge operative executor, providing `install` and `uninstall` CLI support.
**Installation Workflow**:
Inside the container, mount the host root directory (`/`) to `/openyurt`, and write various resources to their host paths:
1. Download a specified version of the `yurthub` binary.
2. Construct the `bootstrap-hub.conf` file to inject authentication parameters.
3. Render configurations and generate the `yurthub.service` and its `10-yurthub.conf` parameter-injected Systemd Unit, writing them to the host.
4. Call `nsenter --mount=/proc/1/ns/mnt -- systemctl daemon-reload`, then `enable yurthub.service`, then `start yurthub.service` under the privileged container to manage the host service layer.
5. Transfer the Server API redirect in the node's Kubelet configuration to `127.0.0.1:10261`, meaning all requests sent to the APIServer are forcibly routed to YurtHub.

**Uninstallation Workflow**:
1. Top priority is to rollback and restore the ApiServer proxy address configuration in the Kubelet.
2. Likewise, use `nsenter` to directly stop and disable the corresponding Service on the host. If the system prompts that the service itself is `not loaded` or `not found`, this phase's error is skipped over directly.
3. Clear various resource paths previously created and persisted via `node-servant`.
4. Determine whether to simultaneously remove historical residual resources and caches generated by the YurtHub working state according to the trusted `--clean-data` directive.

## Implementation History
- [x] 03/03/2026: Proposed idea based on implementation document.
