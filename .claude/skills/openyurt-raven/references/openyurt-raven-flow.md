# OpenYurt Raven Configuration Flow

Execute these steps in order. `<CTX>` is the kube context confirmed in Step 0.
Every cluster change requires a change card and explicit "yes" (see `SKILL.md`).

---

## Step 0: Lock Target Cluster and Prerequisites

1. Show `kubectl config get-contexts` and ask the user to confirm the target
   context `<CTX>`.
2. Verify OpenYurt (`yurt-manager`) and `NodePool`s exist:

```bash
kubectl --context <CTX> get deployment yurt-manager -n kube-system
kubectl --context <CTX> get nodepools.apps.openyurt.io
helm version
```

If `yurt-manager` is missing, stop and ask the user to run `/openyurt-deploy`
first.

---

## Step 1: Collect Cluster Parameters

Ask the user for:

- **Target NodePools / Gateways:** At least two NodePools are normally used for
  cross-pool networking (e.g., cloud pool + edge pool, or two edge pools).
- **L3 tunnel enabled:** `true` or `false` (`controller.enableTunnel`).
- **L7 proxy enabled:** `true` or `false` (`controller.enableProxy`).
- **VPN driver (when L3 is enabled):** `libreswan` (default, IPsec UDP `4500` /
  ESP) or `wireguard` (requires WireGuard kernel module on gateway nodes).
- **Gateway `exposeType`:** `PublicIP` or `LoadBalancer`.

If the cluster uses **Calico**, note that `FelixConfiguration` must set
`iptablesMangleAllowAction: Return` so Raven's `PREROUTING` rules take effect.

---

## Step 2: Install `raven-agent` via Helm

Show a change card explaining that `raven-agent-ds` runs privileged
`hostNetwork: true` pods on Linux nodes, then install in `kube-system`:

```bash
helm repo add openyurt https://openyurtio.github.io/openyurt-helm
helm repo update
```

Generate a fresh PSK and pass it directly without printing it to stdout:

```bash
PSK=$(openssl rand -hex 64)
helm upgrade --install raven-agent openyurt/raven-agent \
  --kube-context <CTX> \
  --namespace kube-system \
  --set vpn.psk="$PSK" \
  --set vpn.driver="<DRIVER>" \
  --set controller.enableTunnel="<ENABLE_TUNNEL>" \
  --set controller.enableProxy="<ENABLE_PROXY>"
unset PSK
```

*(If installing from a source checkout of `github.com/openyurtio/raven` instead
of the Helm repo, point to `./charts/raven-agent`.)*

Verify rollout and ConfigMap state:

```bash
kubectl --context <CTX> -n kube-system rollout status daemonset/raven-agent-ds --timeout=120s
kubectl --context <CTX> -n kube-system get configmap raven-cfg -o yaml
```

### Optional Pre-Check for `libreswan` IPsec Binary

If `vpn.driver=libreswan` and L3 tunneling is enabled, you can verify `ipsec`
on gateway candidate nodes through a short-lived Job (with change card confirmation):

```bash
NODE_NAME="<node-name>"
cat <<EOF | kubectl --context <CTX> apply -f -
apiVersion: batch/v1
kind: Job
metadata:
  name: check-ipsec-$NODE_NAME
  namespace: kube-system
spec:
  template:
    spec:
      nodeName: $NODE_NAME
      hostNetwork: true
      tolerations:
      - operator: Exists
      containers:
      - name: check
        image: busybox:1.36.1
        command: ["/bin/sh", "-c"]
        args: ["if [ ! -x /host/usr/sbin/ipsec ] && [ ! -x /host/usr/libexec/ipsec/whack ] && [ ! -x /host/usr/local/sbin/ipsec ]; then echo 'IPsec binary not found'; else echo 'IPsec verified'; fi"]
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
kubectl --context <CTX> -n kube-system wait --for=condition=complete job/check-ipsec-$NODE_NAME --timeout=30s || true
kubectl --context <CTX> -n kube-system logs job/check-ipsec-$NODE_NAME 2>/dev/null
kubectl --context <CTX> -n kube-system delete job check-ipsec-$NODE_NAME 2>/dev/null
```

---

## Step 3: Label Nodes and Create `Gateway` CRs

For each NodePool participating in Raven:

### 3.1 Label nodes for Gateway selection

Label the nodes **before** creating the `Gateway` CR so the `gatewaypickup`
controller discovers them immediately:

```bash
kubectl --context <CTX> label node -l apps.openyurt.io/nodepool=<NODEPOOL_NAME> \
  raven.openyurt.io/gateway=<NODEPOOL_NAME>-gw
```

> **Note:** `SetDefaultsGateway` (`pkg/apis/raven/v1beta1/default.go`) merges
> `raven.openyurt.io/gateway: <gateway-name>` into
> `spec.nodeSelector.matchLabels`. Keep `matchLabels` aligned with the label
> applied to your nodes.

### 3.2 Create the `Gateway` CR (`raven.openyurt.io/v1beta1`)

Adjust `endpoints` to match the gateway-eligible nodes in that pool. Note that
when `exposeType` is set (`PublicIP` or `LoadBalancer`), `gateway_validation.go`
requires `underNAT: false` on exposed endpoints; edge gateways behind NAT omit
`exposeType` and set `underNAT: true`:

```yaml
apiVersion: raven.openyurt.io/v1beta1
kind: Gateway
metadata:
  name: <NODEPOOL_NAME>-gw
  # Optional extra CIDRs routed through this gateway:
  # annotations:
  #   raven.openyurt.io/extra-allowed-source-cidrs: "192.168.100.0/24"
spec:
  nodeSelector:
    matchLabels:
      raven.openyurt.io/gateway: <NODEPOOL_NAME>-gw
  exposeType: PublicIP # Omit when endpoints are underNAT: true
  endpoints:
    - nodeName: <GATEWAY_NODE_NAME>
      type: tunnel
      underNAT: false
    - nodeName: <GATEWAY_NODE_NAME>
      type: proxy
      underNAT: false
  tunnelConfig:
    Replicas: 1
  proxyConfig:
    Replicas: 1
```

Apply with a change card confirmation:

```bash
kubectl --context <CTX> apply -f gateway.yaml
```

### 3.3 Verify active endpoint election

```bash
kubectl --context <CTX> get gateways.raven.openyurt.io
kubectl --context <CTX> get gateway <NODEPOOL_NAME>-gw -o jsonpath='{.status.activeEndpoints}{"\n"}'
```

If `activeEndpoints` is empty, inspect `references/openyurt-raven-troubleshooting.md`.

---

## Step 4: Verify Cross-Node Connectivity

With user confirmation, deploy a short-lived test `DaemonSet`, verify pod-to-pod
reachability across distinct nodes/pools without writing temporary files to
disk, and clean up afterwards:

```bash
cat <<EOF | kubectl --context <CTX> apply -f -
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
        image: busybox:1.36.1
        command: ["sleep", "3600"]
EOF

kubectl --context <CTX> -n default wait --for=condition=Ready pod -l app=raven-test --timeout=120s

PODS=$(kubectl --context <CTX> -n default get pods -l app=raven-test -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.podIP}{"\t"}{.spec.nodeName}{"\n"}{end}')
printf '%s\n' "$PODS"

POD1=$(printf '%s\n' "$PODS" | awk 'NR==1 {print $1}')
NODE1=$(printf '%s\n' "$PODS" | awk 'NR==1 {print $3}')
POD2_LINE=$(printf '%s\n' "$PODS" | awk -v n="$NODE1" '$3!=n {print $1 "\t" $2 "\t" $3; exit}')

if [ -z "$POD2_LINE" ]; then
  echo "Need test pods on at least 2 nodes across pools to test Raven."
else
  POD2_IP=$(printf '%s\n' "$POD2_LINE" | awk '{print $2}')
  NODE2=$(printf '%s\n' "$POD2_LINE" | awk '{print $3}')
  echo "Testing $POD1 ($NODE1) -> $POD2_IP ($NODE2)"
  kubectl --context <CTX> -n default exec "$POD1" -- ping -c 3 -W 5 "$POD2_IP"
fi

kubectl --context <CTX> -n default delete daemonset raven-test --ignore-not-found
```
