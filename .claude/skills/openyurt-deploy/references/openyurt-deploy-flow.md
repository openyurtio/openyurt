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

### 1.5 Autonomous Kubernetes-Native Preflight Check (CRITICAL)

For **each** worker node the user wants to convert, autonomously deploy an
ephemeral Job to verify `crictl` is available on the host. Edge nodes are
often behind NAT/firewalls, so SSH cannot be used.

Run this script to verify the CRI environment natively:

```bash
NODE_NAME="<node-name>"
cat <<EOF | kubectl apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: preflight-check-$NODE_NAME
  namespace: kube-system
spec:
  backoffLimit: 0
  template:
    spec:
      nodeName: $NODE_NAME
      hostNetwork: true
      tolerations:
      - operator: Exists
      containers:
      - name: check
        image: busybox
        command: ["/bin/sh", "-c"]
        args:
        - |
          if [ ! -x /host/usr/bin/crictl ] && [ ! -x /host/usr/local/bin/crictl ]; then
            echo "ERROR: crictl not found on host"
            exit 1
          fi
          if [ ! -S /host/run/containerd/containerd.sock ] && [ ! -S /host/run/k3s/containerd/containerd.sock ]; then
            echo "ERROR: CRI socket not found on host"
            exit 1
          fi
          echo "SUCCESS: crictl and CRI socket verified"
        volumeMounts:
        - name: host-root
          mountPath: /host
          readOnly: true
      volumes:
      - name: host-root
        hostPath:
          path: /
      restartPolicy: Never
EOF

echo "Waiting for preflight Job to complete..."
kubectl -n kube-system wait --for=condition=complete job/preflight-check-$NODE_NAME --timeout=60s || true

LOGS=$(kubectl -n kube-system logs job/preflight-check-$NODE_NAME 2>/dev/null)
kubectl -n kube-system delete job preflight-check-$NODE_NAME 2>/dev/null

if echo "$LOGS" | grep -q "ERROR"; then
  echo "Preflight failed for $NODE_NAME: $LOGS"
  echo "You must install cri-tools on the node manually before proceeding."
  exit 1
fi
echo "Preflight passed for $NODE_NAME."
```

If the preflight fails:
1. **Explain** to the user that `crictl` is a hard requirement.
2. Instruct them to install `cri-tools` on the edge node via their remote
   management system (since direct SSH is likely blocked by edge NAT).

> **Do not proceed with conversion until this preflight passes on every
> target node.**

---

## Step 2: Install yurt-manager via Helm

### 2.1 Add the Helm chart

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
`edge-factory-01`). **Ask the user for the name** before applying.

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
`YurtNodeConversionController` watches for changes to the
`apps.openyurt.io/nodepool` label on Node objects. When the label is added,
it automatically:

1. Cordons the node (sets `spec.unschedulable = true`)
2. Creates a `node-servant-conversion-<node-name>` Job in `kube-system`
3. The Job installs YurtHub and redirects kubelet traffic
4. On success, the controller sets `openyurt.io/is-edge-worker=true`,
   sets `topology.kubernetes.io/zone=<pool-name>`, and uncordons the node

### 4.1 Label a worker node

Convert one node at a time for easier monitoring. **Ask the user for
confirmation** before applying the label:

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

### 4.3 Automated Verification Loop

Write and execute a bash polling script to verify `isConvertedStable`. The
loop must verify all four conditions from the controller source
(`yurt_node_conversion_controller.go:583-591`):

```bash
NODE_NAME="<node-name>"
echo "Waiting for node $NODE_NAME to reach isConvertedStable..."
for i in $(seq 1 60); do
  STATUS=$(kubectl get node "$NODE_NAME" -o json 2>/dev/null)
  
  NODEPOOL=$(echo "$STATUS" | jq -r '.metadata.labels["apps.openyurt.io/nodepool"] // empty')
  EDGE_WORKER=$(echo "$STATUS" | jq -r '.metadata.labels["openyurt.io/is-edge-worker"] // empty')
  UNSCHEDULABLE=$(echo "$STATUS" | jq -r '.spec.unschedulable // false')
  COND_STATUS=$(echo "$STATUS" | jq -r '[.status.conditions[] | select(.type=="YurtNodeConversionFailed")] | .[0].status // empty')
  COND_REASON=$(echo "$STATUS" | jq -r '[.status.conditions[] | select(.type=="YurtNodeConversionFailed")] | .[0].reason // empty')
  
  if [ -n "$NODEPOOL" ] && [ "$EDGE_WORKER" = "true" ] && [ "$UNSCHEDULABLE" != "true" ] && [ "$COND_STATUS" = "False" ] && [ "$COND_REASON" = "Converted" ]; then
    echo "✅ Node $NODE_NAME is isConvertedStable. Conversion complete."
    break
  fi
  
  echo "  [$i/60] nodepool=$NODEPOOL edge-worker=$EDGE_WORKER unschedulable=$UNSCHEDULABLE condition=$COND_STATUS/$COND_REASON"
  sleep 5
done

if [ "$COND_REASON" != "Converted" ]; then
  echo "❌ Node $NODE_NAME did not reach Converted state within 5 minutes."
  echo "Consult openyurt-deploy-troubleshooting.md."
fi
```

Do not proceed to Step 5 until this polling loop succeeds for every node.

### 4.4 Repeat for additional nodes

Label each additional worker node one at a time. Wait for each node to
reach `Converted` before proceeding to the next.

---

## Step 5: Enable Edge Autonomy

```bash
kubectl annotate node <node-name> node.openyurt.io/autonomy-duration=5m
```

> **Note**: This is an annotation, not a label. Using `kubectl label` here
> will silently create a label instead of the annotation the controller
> expects.

---

## Step 6: Final Verification

### 6.1 Verify NodePool status

```bash
kubectl get nodepools -o wide
```

The `ReadyNodes` column should reflect the number of converted nodes.

### 6.2 Verify edge worker labels

```bash
kubectl get nodes -l openyurt.io/is-edge-worker=true
```

All converted nodes should appear in this list.

### 6.3 Verify topology zone labels

```bash
kubectl get nodes --show-labels | grep topology.kubernetes.io/zone
```

Each converted node should have `topology.kubernetes.io/zone=<pool-name>`.

### 6.4 Verify YurtHub is running on edge nodes

```bash
kubectl get pods -A -o wide | grep yurthub
```

### 6.5 Verify node conditions

```bash
kubectl get nodes -o custom-columns=NAME:.metadata.name,CONVERSION:.status.conditions[?\(@.type==\"YurtNodeConversionFailed\"\)].reason
```

All converted nodes should show `Converted`.

### 6.6 Final checklist

Present this final verification checklist to the user:

- [ ] **yurt-manager pods Ready**: `kubectl -n kube-system get deployment yurt-manager` shows available replicas.
- [ ] **NodePool exists**: `kubectl get nodepools` lists the created NodePool.
- [ ] **node-servant completed**: `kubectl -n kube-system get jobs -l openyurt.io/conversion-node` shows completions.
- [ ] **node has edge labels**: `kubectl get nodes -l openyurt.io/is-edge-worker=true` lists the converted nodes.
- [ ] **node is in zone**: Node has `topology.kubernetes.io/zone` set to the NodePool name.
- [ ] **node condition present**: `YurtNodeConversionFailed` shows `Reason: Converted`.
- [ ] **edge node Ready**: Converted nodes remain in a `Ready` state.
