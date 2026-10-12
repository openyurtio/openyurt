# Deployment Flow

Target: an existing Kubernetes cluster with a healthy control plane. This flow installs `yurt-manager`,
creates a `NodePool`, and converts existing worker nodes into edge nodes. It does not cover joining brand
new nodes (`yurtadm join`) — that's a separate path for nodes that aren't in the cluster yet.

## 1. Preflight

```bash
kubectl config current-context
kubectl get nodes -o wide
```

Confirm the context matches the cluster the user intends to modify, and identify which nodes are
candidates for conversion.

The conversion Job later chroots into each target node's root filesystem and needs `crictl` (or `docker`)
resolvable on the **host's** `PATH` — this is how `node-servant` detects the CRI socket
(`pkg/node-servant/components/runtime.go`). `kubeadm` does not install `crictl` by default on
containerd-based nodes, so check each candidate node before labeling it:

```bash
kubectl debug node/<node-name> -it --image=busybox -- chroot /host which crictl
```

If it's missing, the conversion Job will fail fast during the container-runtime detection step rather than
leaving the node half-converted — but it's cheaper to catch this before cordoning the node at all.

## 2. Install yurt-manager

`yurt-manager` runs on a control-plane node and owns the `YurtNodeConversionController` that does the
actual conversion work. Install or upgrade it via the chart in `charts/yurt-manager`, making sure the
`yurtnodeconversion` controller is enabled:

```bash
helm upgrade --install yurt-manager ./charts/yurt-manager \
  --namespace kube-system \
  --set controllers="*,yurtnodeconversion"
```

`controllers` follows a `foo,-bar,*` allow/deny list — the default chart value is `-nodelifecycle,*`, which
already enables everything except `nodelifecycle`, but be explicit about `yurtnodeconversion` since it's
what this flow depends on. Optional flags if the cluster can't reach the default YurtHub binary mirror:

```bash
--set extraArgs="{--yurthub-version=v1.7.0,--yurthub-binary-url=http://<internal-mirror>/yurthub}"
```

Verify the controller is up:

```bash
kubectl get deploy -n kube-system yurt-manager
kubectl logs -n kube-system deploy/yurt-manager | grep -i yurtnodeconversion
```

## 3. Create a NodePool

Every node being converted must belong to a `NodePool` with `spec.type: Edge` — the controller checks this
before it will touch the node.

```yaml
apiVersion: apps.openyurt.io/v1beta2
kind: NodePool
metadata:
  name: <pool-name>
spec:
  type: Edge
```

```bash
kubectl apply -f nodepool.yaml
kubectl get nodepool <pool-name>
```

`spec.labels`, `spec.annotations`, and `spec.taints` on the NodePool are applied to every member node once
converted — set them here if the workload placement needs them, rather than patching nodes individually
afterward.

## 4. Convert nodes

> **Warning:** labeling a node cordons it (`spec.unschedulable=true`) for the duration of the conversion,
> and on success the controller restarts every non-pause container on that node so their
> `KUBERNETES_SERVICE_HOST` env picks up the new YurtHub proxy. Bare pods with no owning controller are
> deleted and **not** recreated. Confirm with the user before labeling a node that's running production
> traffic, especially single-replica StatefulSets without a PodDisruptionBudget.

```bash
kubectl label node <node-name> apps.openyurt.io/nodepool=<pool-name>
```

This is the only trigger — there's no separate `yurtadm convert` command for existing nodes. The
controller reconciles on the label, cordons the node, sets a `YurtNodeConversionFailed` condition with
`reason=Converting`, and creates a Job named `node-servant-conversion-<node-name>` in `kube-system`.

Watch progress:

```bash
kubectl get node <node-name> -o jsonpath='{.status.conditions[?(@.type=="YurtNodeConversionFailed")]}'
kubectl get job -n kube-system node-servant-conversion-<node-name>
kubectl logs -n kube-system job/node-servant-conversion-<node-name>
```

On success the condition reason moves to `Converted`, the node gets `openyurt.io/is-edge-worker=true`, and
it's uncordoned automatically. Do not set `openyurt.io/is-edge-worker` by hand — it's the controller's
source of truth for conversion state, and writing it directly doesn't run any of the underlying setup.

If the Job doesn't reach `Converted` within a few minutes, or the condition reason becomes `ConvertFailed`,
go to `openyurt-deploy-troubleshooting.md`.
