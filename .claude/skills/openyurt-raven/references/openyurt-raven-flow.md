# OpenYurt Raven Configuration Flow

Follow these steps sequentially to configure Raven for cross-NodePool
networking.

---

## Phase 1: Prerequisites

### 1.1 Verify OpenYurt is deployed

```bash
kubectl get deployment yurt-manager -n kube-system
```

If not deployed, inform the user they must run `/openyurt-deploy` first.

### 1.2 Verify NodePools exist

```bash
kubectl get nodepools
```

At least two Edge NodePools are needed for cross-region communication.

### 1.3 Generate a VPN Pre-Shared Key (PSK)

```bash
PSK=$(openssl rand -hex 64)
echo "Generated PSK: $PSK"
```

### 1.4 Create a Secret with the PSK

```bash
kubectl create secret generic raven-ipsec-psk \
  --from-literal=psk=$PSK \
  -n kube-system
```

---

## Phase 2: Deploy Raven Agent

### 2.1 Install the raven-agent DaemonSet

**Ask the user for confirmation** before deploying, as raven-agent runs
privileged pods on all nodes.

```bash
helm repo add openyurt https://openyurtio.github.io/openyurt-helm
helm repo update
helm upgrade --install raven-agent openyurt/raven-agent -n kube-system
```

### 2.2 Wait for DaemonSet to be ready

```bash
kubectl -n kube-system rollout status daemonset/raven-agent --timeout=120s
```

### 2.3 Verify libreswan is available (for L3 tunnel)

If the user needs L3 tunneling, verify `libreswan` (or compatible IPsec
backend) is installed on the gateway-elected nodes:

```bash
ssh <user>@<node-ip> "ipsec --version"
```

---

## Phase 3: Configure Gateways

For each NodePool that requires cross-region communication:

### 3.1 Label nodes for gateway election

Label the nodes in the NodePool to join the gateway **before** creating the
Gateway CR:

```bash
kubectl label node -l apps.openyurt.io/nodepool=<NODEPOOL_NAME> \
  raven.openyurt.io/gateway=<NODEPOOL_NAME>-gw
```

> **Important:** The `v1beta1.SetDefaultsGateway` mutating webhook
> unconditionally sets `spec.nodeSelector` to
> `raven.openyurt.io/gateway: <gateway-name>`. Do NOT use
> `nodeSelectorTerms` with `apps.openyurt.io/nodepool` — the webhook will
> silently overwrite it.

### 3.2 Ask the user for configuration

- **Expose type:** `PublicIP` or `LoadBalancer`
- **Enable L3 tunnel:** yes/no
- **Enable L7 proxy:** yes/no

### 3.3 Create the Gateway CR

```yaml
apiVersion: raven.openyurt.io/v1beta1
kind: Gateway
metadata:
  name: <NODEPOOL_NAME>-gw
  annotations:
    # Fine-grained per-Gateway overrides (optional).
    # These override the cluster-wide raven-global-config ConfigMap.
    raven.openyurt.io/enable-tunnel: "true"
    raven.openyurt.io/enable-proxy: "true"
spec:
  nodeSelector:
    matchLabels:
      raven.openyurt.io/gateway: <NODEPOOL_NAME>-gw
  exposeType: <EXPOSE_TYPE>
  tunnelConfig:
    Replicas: 1
  proxyConfig:
    Replicas: 1
```

Apply via:

```bash
kubectl apply -f gateway.yaml
```

### 3.4 Verify Gateway election

```bash
kubectl get gateway <NODEPOOL_NAME>-gw -o jsonpath='{.status.activeEndpoints}'
```

If `activeEndpoints` is empty, check the Raven troubleshooting guide.

---

## Phase 4: Autonomous Connectivity Test

Deploy ephemeral test pods and verify cross-NodePool L3 connectivity
automatically. Do not ask the user to verify manually; run this and
parse the output:

```bash
#!/bin/bash
echo "=== Raven Cross-NodePool Connectivity Test ==="

# Deploy test DaemonSet
cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: raven-test
  namespace: default
spec:
  selector:
    matchLabels:
      app: raven-test
  template:
    metadata:
      labels:
        app: raven-test
    spec:
      tolerations:
      - operator: Exists
      containers:
      - name: test
        image: busybox
        command: ["sleep", "3600"]
EOF

echo "Waiting for test pods to be Ready..."
kubectl wait --for=condition=Ready pod -l app=raven-test --timeout=120s

# Get all test pod info
PODS=$(kubectl get pods -l app=raven-test -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.podIP}{"\t"}{.spec.nodeName}{"\n"}{end}')
echo ""
echo "Test pods deployed:"
echo "$PODS"
echo ""

# Get unique nodes
NODES=($(echo "$PODS" | awk '{print $3}' | sort -u))

if [ ${#NODES[@]} -lt 2 ]; then
    echo "⚠️  Only ${#NODES[@]} node(s) have test pods. Need at least 2 for cross-node test."
else
    # Pick first pod on first node, and first pod on a different node
    NODE1="${NODES[0]}"
    NODE2="${NODES[1]}"
    POD1=$(echo "$PODS" | awk -v n="$NODE1" '$3==n {print $1; exit}')
    POD2_IP=$(echo "$PODS" | awk -v n="$NODE2" '$3==n {print $2; exit}')
    
    echo "Testing: $POD1 (node: $NODE1) → $POD2_IP (node: $NODE2)"
    kubectl exec "$POD1" -- ping -c 3 -W 5 "$POD2_IP"
    
    if [ $? -eq 0 ]; then
        echo ""
        echo "✅ Raven L3 tunnel connectivity verified successfully!"
    else
        echo ""
        echo "❌ Raven L3 tunnel connectivity FAILED."
        echo "Consult openyurt-raven-troubleshooting.md."
    fi
fi

# Cleanup
echo ""
echo "Cleaning up test DaemonSet..."
kubectl delete daemonset raven-test --grace-period=0
echo "Done."
```
