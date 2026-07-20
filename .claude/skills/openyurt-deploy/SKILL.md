---
name: openyurt-deploy
description: >
  Deploy OpenYurt onto an existing Kubernetes cluster. Installs yurt-manager
  via Helm, creates Edge NodePool CRs, converts existing worker nodes to edge
  nodes using the label-driven YurtNodeConversionController, and verifies the
  deployment end-to-end.
when_to_use: >
  Use when the user wants to deploy OpenYurt on an existing Kubernetes cluster,
  install yurt-manager, create NodePools, convert existing worker nodes to edge
  nodes, or verify an OpenYurt deployment. Also use when the user asks about
  uninstalling or rolling back an OpenYurt deployment.
allowed-tools: >
  Bash(kubectl *)
  Bash(helm *)
  Bash(ssh *)
  Bash(systemctl *)
  Bash(journalctl *)
  Read
  Grep
disable-model-invocation: true
---

# OpenYurt Deployment Skill

This skill guides you through deploying OpenYurt on an existing Kubernetes
cluster. It covers the full lifecycle: preflight checks, installation,
node conversion, verification, troubleshooting, and uninstallation.

> **Supported OpenYurt versions**:
> - v1.7.x (Kubernetes ≥ 1.24)

## How to Use This Skill

Follow the deployment flow document for the step-by-step happy path.
If you encounter errors at any step, consult the troubleshooting guide.
For rollback or cleanup, see the uninstall document.

### Reference Documents

| Document | Purpose |
|----------|---------|
| `references/openyurt-deploy-flow.md` | Step-by-step deployment (happy path) |
| `references/openyurt-deploy-troubleshooting.md` | Decision tree for common failures |
| `references/openyurt-deploy-uninstall.md` | Rollback and uninstall procedures |

## Key Concepts

- **yurt-manager**: The central control-plane component deployed as a
  Deployment on control-plane nodes. Manages NodePools, node conversion,
  and other OpenYurt controllers.

- **NodePool**: A cluster-scoped CR (`apps.openyurt.io/v1beta2`) that groups
  nodes. Type can be `Edge` or `Cloud`.

- **Label-driven node conversion**: Applying the label
  `apps.openyurt.io/nodepool=<pool-name>` to a worker node triggers the
  `YurtNodeConversionController`, which automatically creates a
  `node-servant-conversion-<node>` Job to install YurtHub and redirect
  kubelet traffic.

- **node-servant**: A privileged Job that runs on the target node, installs
  YurtHub as a static pod, and redirects kubelet API server traffic through
  YurtHub. Requires `crictl` on the node.

## Important Safety Rules

1. **`crictl` is a hard dependency**: The node-servant conversion process
   uses `crictl` to list and restart containers. It is NOT installed by
   default on standard kubeadm setups. Always verify its presence on each
   target edge node before labeling. The preflight check in the deploy flow
   handles this autonomously via SSH.

2. **Do not modify the `apps.openyurt.io/nodepool` label in-place**: The
   admission webhook forbids in-place value changes. To reassign a node to
   a different NodePool, first remove the label, then re-add it with the
   new value.

3. **Conversion is automatic and asynchronous**: Once the label is applied,
   the controller handles everything. Monitor progress via the
   `YurtNodeConversionFailed` node condition and the conversion Job.

4. **Convert one node at a time**: Although the controller supports
   concurrent conversions, converting nodes one at a time makes it easier
   to diagnose failures.

5. **Do not hand-edit controller-owned labels**: `openyurt.io/is-edge-worker`
   is set by `ensureEdgeWorkerLabel()` in the conversion controller. Setting
   it manually fights the controller and can cause reconciliation loops.

6. **CRI socket paths vary**: `containerd` socket path differs between
   kubeadm (`/run/containerd/containerd.sock`) and k3s
   (`/run/k3s/containerd/containerd.sock`). The troubleshooting reference
   covers both paths.
