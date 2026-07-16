# OpenYurt Deployment Flow

Step-by-step guide for deploying OpenYurt on an existing Kubernetes cluster.
Execute each step in order. If any step fails, consult
`openyurt-deploy-troubleshooting.md`.

---

## Step 1: Preflight Checks

### 1.1 Verify kubectl access

```bash
kubectl cluster-info
kubectl get nodes
```

Confirm the cluster is reachable and all nodes are in `Ready` state.

### 1.2 Verify Helm version

```bash
helm version --short
```

Helm **v3.x** is required. If Helm is not installed or is v2, stop and ask
the user to install Helm v3.

### 1.3 Verify Kubernetes version

```bash
kubectl version --short 2>/dev/null || kubectl version
```

Kubernetes **≥ 1.24** is required. OpenYurt v1.7.x does not support earlier
versions.

### 1.4 Identify node roles

```bash
kubectl get nodes -o wide --show-labels
```

Identify which nodes are:
- **Control-plane nodes**: Have the label `node-role.kubernetes.io/control-plane`
- **Worker nodes**: All other nodes (candidates for edge conversion)

Ask the user which worker nodes they want to convert to edge nodes.

### 1.5 Verify `crictl` on target edge nodes

For **each** worker node the user wants to convert, SSH in and verify:

```bash
ssh <user>@<node-ip> "which crictl && crictl --version"
```

`crictl` (from the `cri-tools` package) is a **hard dependency** of the
node-servant conversion process. It is used to list and restart containers
during conversion. If `crictl` is not present:

- **Explain** to the user that `crictl` is a hard requirement for the `node-servant` Job to convert the node.
- **Ask the user for explicit confirmation** before attempting to install it.
- **Install it only after confirmation**, using the appropriate command for their OS:

```bash
# Example: install cri-tools on Debian/Ubuntu
ssh <user>@<node-ip> "sudo apt-get update && sudo apt-get install -y cri-tools"

# Example: install cri-tools on CentOS/RHEL
ssh <user>@<node-ip> "sudo yum install -y cri-tools"
```

> **Do not proceed with conversion until `crictl` is confirmed on every
> target node.** The conversion Job will fail mid-run if `crictl` is missing.

---

## Step 2: Install yurt-manager via Helm

### 2.1 Add the Helm chart

The chart is included in the OpenYurt repository under `charts/yurt-manager/`.

**Ask the user for confirmation** before running the Helm install command.

If working from a local clone:

```bash
helm install yurt-manager ./charts/yurt-manager \
  --namespace kube-system \
  --create-namespace
```

If using a remote chart repository:

```bash
helm repo add openyurt https://openyurtio.github.io/openyurt-helm
helm repo update
helm install yurt-manager openyurt/yurt-manager \
  --namespace kube-system \
  --create-namespace
```

### 2.2 Key default values

These are the defaults from `charts/yurt-manager/values.yaml`:

| Parameter | Default | Description |
|-----------|---------|-------------|
| `image.registry` | `openyurt` | Container registry |
| `image.repository` | `yurt-manager` | Image name |
| `image.tag` | `""` (uses appVersion `v1.7.0`) | Image tag |
| `nodeServant.image.registry` | `openyurt` | node-servant registry |
| `nodeServant.image.repository` | `node-servant` | node-servant image |
| `nodeServant.image.tag` | `latest` | node-servant tag |
| `controllers` | `-nodelifecycle,*` | Enabled controllers |

### 2.3 Wait for yurt-manager to be ready

```bash
kubectl -n kube-system rollout status deployment/yurt-manager --timeout=120s
```

Verify the pod is running:

```bash
kubectl -n kube-system get pods -l app.kubernetes.io/name=yurt-manager
```

### 2.4 Verify CRDs are installed

```bash
kubectl get crd nodepools.apps.openyurt.io
```

The Helm chart installs CRDs automatically from `charts/yurt-manager/crds/`,
including `nodepools.apps.openyurt.io`, `yurtappsets.apps.openyurt.io`, etc.

---

## Step 3: Create Edge NodePool CR

### 3.1 Create an Edge NodePool

```bash
cat <<EOF | kubectl apply -f -
apiVersion: apps.openyurt.io/v1beta2
kind: NodePool
metadata:
  name: <edge-pool-name>
spec:
  type: Edge
EOF
```

Replace `<edge-pool-name>` with the desired name (e.g., `edge-beijing`,
`edge-factory-01`). **Ask the user for the name and wait for their confirmation** before applying the CR.

### 3.2 (Optional) Create a Cloud NodePool

For control-plane / cloud nodes:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: apps.openyurt.io/v1beta2
kind: NodePool
metadata:
  name: <cloud-pool-name>
spec:
  type: Cloud
EOF
```

### 3.3 Verify NodePool creation

```bash
kubectl get nodepools
```

Expected output shows the NodePool(s) with `Type` column set to `Edge`
or `Cloud`.

---

## Step 4: Convert Worker Nodes to Edge Nodes

This step uses the **label-driven conversion** mechanism. The
`YurtNodeConversionController` in yurt-manager watches for changes to the
`apps.openyurt.io/nodepool` label on Node objects. When the label is added,
it automatically:

1. Cordons the node (sets `spec.unschedulable = true`)
2. Creates a `node-servant-conversion-<node-name>` Job in `kube-system`
3. The Job installs YurtHub and redirects kubelet traffic
4. On success, the controller sets `openyurt.io/is-edge-worker=true`,
   sets `topology.kubernetes.io/zone=<pool-name>`, and uncordons the node

### 4.1 Label a worker node

Convert one node at a time for easier monitoring. **Ask the user for confirmation** before applying the label:

```bash
kubectl label node <node-name> apps.openyurt.io/nodepool=<edge-pool-name>
```

### 4.2 Monitor the conversion Job

```bash
# Watch the Job
kubectl -n kube-system get jobs -l openyurt.io/conversion-node=<node-name> -w

# Watch the Job Pod logs
kubectl -n kube-system logs -f job/node-servant-conversion-<node-name>
```

### 4.3 Check the node conversion condition

```bash
kubectl get node <node-name> -o jsonpath='{range .status.conditions[?(@.type=="YurtNodeConversionFailed")]}{.reason}{" "}{.message}{"\n"}{end}'
```

Expected progression:
- `Converting` → `conversion Job node-servant-conversion-<node> is running`
- `Converted` → `YurtHub installed and node converted successfully`

If you see `ConvertFailed`, consult the troubleshooting guide.

### 4.4 Repeat for additional nodes

Label each additional worker node one at a time:

```bash
kubectl label node <next-node> apps.openyurt.io/nodepool=<edge-pool-name>
```

Wait for each node to reach `Converted` before proceeding to the next.

---

## Step 5: Verification

### 5.1 Verify NodePool status

```bash
kubectl get nodepools -o wide
```

The `ReadyNodes` column should reflect the number of converted nodes.
The `Nodes` field in the status should list all member nodes.

### 5.2 Verify edge worker labels

```bash
kubectl get nodes -l openyurt.io/is-edge-worker=true
```

All converted nodes should appear in this list.

### 5.3 Verify topology zone labels

```bash
kubectl get nodes --show-labels | grep topology.kubernetes.io/zone
```

Each converted node should have `topology.kubernetes.io/zone=<pool-name>`.

### 5.4 Verify YurtHub is running on edge nodes

```bash
# Check for YurtHub static pod on each edge node
kubectl get pods -A -o wide | grep yurthub
```

### 5.5 Verify node conditions

```bash
kubectl get nodes -o custom-columns=NAME:.metadata.name,CONVERSION:.status.conditions[?\(@.type==\"YurtNodeConversionFailed\"\)].reason
```

All converted nodes should show `Converted`.

### 5.6 Final checklist

Present this final verification checklist to the user to confirm success:

- [x] **yurt-manager pods Ready**: `kubectl -n kube-system get deployment yurt-manager` shows 1/1 ready.
- [x] **NodePool exists**: `kubectl get nodepools` lists the newly created NodePool.
- [x] **node-servant completed**: `kubectl -n kube-system get jobs -l openyurt.io/conversion-node` shows completions.
- [x] **node has edge labels**: `kubectl get nodes -l openyurt.io/is-edge-worker=true` lists the converted nodes.
- [x] **node is in zone**: Node has `topology.kubernetes.io/zone` set to the NodePool name.
- [x] **node condition present**: `YurtNodeConversionFailed` shows `Reason: Converted`.
- [x] **edge node Ready**: Converted nodes remain in a `Ready` state.

```bash
echo "=== OpenYurt Deployment Complete ==="
```
