# Configure Raven for cross-region networking

You are helping an operator configure [Raven](https://github.com/openyurtio/raven) on an existing OpenYurt cluster. Raven provides L3 VPN tunnels and L7 reverse-proxy connectivity between NodePools in different network regions. Follow these steps carefully, verifying each one before proceeding.

## Step 1 — Verify OpenYurt is already deployed

```bash
# Check yurt-manager is running
kubectl get deployment -A -l app.kubernetes.io/name=yurt-manager

# Check at least one NodePool exists
kubectl get nodepool
```

If `yurt-manager` is not found or no NodePools exist, stop and ask the user to run `/openyurt-deploy` first.

## Step 2 — Add the Raven Helm repository

```bash
helm repo add raven https://openyurtio.github.io/raven-helm-charts
helm repo update
helm repo list | grep raven
```

## Step 3 — Generate a VPN pre-shared key

Raven uses a PSK for IPSec/libreswan tunnels. Generate a strong one:

```bash
PSK=$(openssl rand -hex 64)
echo "Generated PSK (save this securely): ${PSK}"
```

Store the PSK in a Kubernetes Secret so Raven agents on all nodes can access it:

```bash
kubectl create namespace raven-system --dry-run=client -o yaml | kubectl apply -f -

kubectl create secret generic raven-agent-psk \
  --namespace raven-system \
  --from-literal=psk="${PSK}" \
  --dry-run=client -o yaml | kubectl apply -f -
```

> **Important**: Record the PSK value. All Gateway nodes must use the same PSK. A mismatch will prevent tunnel establishment.

## Step 4 — Install raven-agent via Helm

Ask the user which NodePools should act as **Gateway** nodes (i.e., host the VPN endpoint). Gateways need either a public IP or a LoadBalancer service.

```bash
helm upgrade --install raven-agent raven/raven-agent \
  --namespace raven-system \
  --create-namespace \
  --set agent.pskSecretName=raven-agent-psk \
  --wait \
  --timeout 5m
```

Verify the DaemonSet is running on all nodes:

```bash
kubectl -n raven-system rollout status daemonset/raven-agent --timeout=3m
kubectl -n raven-system get pods -o wide
```

If any pod is not Running, inspect its logs:

```bash
kubectl -n raven-system logs <pod-name> --tail=50
```

**Common issues:**
- `libreswan` not installed on the node — Raven requires `libreswan` for IPSec tunnels. Install it on each Gateway node:
  ```bash
  # Debian/Ubuntu
  sudo apt-get install -y libreswan
  # RHEL/CentOS
  sudo yum install -y libreswan
  ```
- Firewall blocking UDP 500/4500 — open these ports on Gateway nodes.

## Step 5 — Create Gateway CRs

For each NodePool that needs cross-region connectivity, create a `Gateway` CR. A Gateway CR designates one or more nodes as the VPN endpoint for that NodePool.

First, list available NodePools and their member nodes:

```bash
kubectl get nodepool -o wide
kubectl get nodes -L apps.openyurt.io/nodepool
```

### Option A — PublicIP expose type

Use this when Gateway nodes have a static public IP:

```bash
POOL_NAME=<nodepool-name>
GATEWAY_NODE=<gateway-node-name>
PUBLIC_IP=<gateway-public-ip>

kubectl apply -f - <<EOF
apiVersion: raven.openyurt.io/v1beta1
kind: Gateway
metadata:
  name: gw-${POOL_NAME}
spec:
  nodeSelector:
    matchLabels:
      apps.openyurt.io/nodepool: ${POOL_NAME}
  endpoints:
    - nodeName: ${GATEWAY_NODE}
      underNAT: false
      type: Public
      publicIP: ${PUBLIC_IP}
      port: 4500
  exposeType: PublicIP
EOF
```

### Option B — LoadBalancer expose type

Use this when the cluster supports cloud LoadBalancers (e.g., AWS ELB, Azure LB):

```bash
POOL_NAME=<nodepool-name>
GATEWAY_NODE=<gateway-node-name>

kubectl apply -f - <<EOF
apiVersion: raven.openyurt.io/v1beta1
kind: Gateway
metadata:
  name: gw-${POOL_NAME}
spec:
  nodeSelector:
    matchLabels:
      apps.openyurt.io/nodepool: ${POOL_NAME}
  endpoints:
    - nodeName: ${GATEWAY_NODE}
      underNAT: true
      type: Public
      port: 4500
  exposeType: LoadBalancer
EOF
```

Repeat for every NodePool that needs inter-region connectivity.

## Step 6 — Configure L3 tunnel and L7 proxy

Raven supports two independent data-plane modes. Ask the user which they need:

### Enable/Disable L3 VPN tunnel

L3 tunnels provide transparent pod-to-pod IP connectivity across NodePools.

```bash
# Enable L3 tunnel (default)
kubectl patch gateway gw-<pool> --type='merge' \
  -p '{"spec":{"tunnelConfig":{"enabled":true}}}'

# Disable L3 tunnel
kubectl patch gateway gw-<pool> --type='merge' \
  -p '{"spec":{"tunnelConfig":{"enabled":false}}}'
```

### Enable/Disable L7 reverse proxy

L7 proxy allows cloud-side services to reach edge pods using HTTP/HTTPS via a reverse proxy, without requiring L3 connectivity.

```bash
# Enable L7 proxy
kubectl patch gateway gw-<pool> --type='merge' \
  -p '{"spec":{"proxyConfig":{"enabled":true}}}'

# Disable L7 proxy
kubectl patch gateway gw-<pool> --type='merge' \
  -p '{"spec":{"proxyConfig":{"enabled":false}}}'
```

## Step 7 — Verify cross-NodePool connectivity

Check that Gateway CRs are active and have endpoints assigned:

```bash
kubectl get gateway -o wide
kubectl describe gateway gw-<pool>
```

Look for `Status.ActiveEndpoints` — it should list the active Gateway node(s) for each pool.

Test pod-to-pod connectivity across NodePools. Launch test pods in two different NodePools:

```bash
POOL_A=<first-nodepool>
POOL_B=<second-nodepool>

# Deploy a test pod in pool A
kubectl run test-a --image=busybox --restart=Never \
  --overrides="{\"spec\":{\"nodeSelector\":{\"apps.openyurt.io/nodepool\":\"${POOL_A}\"}}}" \
  -- sleep 3600

# Deploy a test pod in pool B
kubectl run test-b --image=busybox --restart=Never \
  --overrides="{\"spec\":{\"nodeSelector\":{\"apps.openyurt.io/nodepool\":\"${POOL_B}\"}}}" \
  -- sleep 3600

# Wait for pods to be Running
kubectl wait pod/test-a pod/test-b --for=condition=Ready --timeout=2m

# Get IPs
POD_A_IP=$(kubectl get pod test-a -o jsonpath='{.status.podIP}')
POD_B_IP=$(kubectl get pod test-b -o jsonpath='{.status.podIP}')

echo "Pod A IP: ${POD_A_IP}"
echo "Pod B IP: ${POD_B_IP}"

# Test connectivity from A to B
kubectl exec test-a -- ping -c 3 "${POD_B_IP}"

# Clean up test pods
kubectl delete pod test-a test-b
```

**If ping fails:**
- Verify both Gateways show `ActiveEndpoints` in `kubectl describe gateway`.
- Check raven-agent logs on the Gateway nodes for IPSec negotiation errors.
- Confirm `libreswan` is installed and running on Gateway nodes.
- Verify firewall rules allow UDP 500 and 4500 between Gateway node IPs.
- Confirm both Gateways share the same PSK Secret (`raven-agent-psk`).

## Completion

Summarise what was configured:

- Raven PSK Secret created in `raven-system` ✅
- `raven-agent` DaemonSet installed and running ✅
- Gateway CRs created for NodePools: list each pool ✅
- L3 tunnel enabled/disabled (per user choice) ✅
- L7 proxy enabled/disabled (per user choice) ✅
- Cross-NodePool connectivity verified ✅

Next steps:
- Review the [Raven documentation](https://github.com/openyurtio/raven) for advanced topology configuration.
- Configure `YurtAppSet` to distribute workloads across pools.
- Use `node.openyurt.io/autonomy-duration` to tune edge autonomy per node.
