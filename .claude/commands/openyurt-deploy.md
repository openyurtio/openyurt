# Deploy OpenYurt on an existing Kubernetes cluster

You are helping an operator deploy OpenYurt on an existing Kubernetes cluster. Follow these steps carefully, verifying each one before proceeding. If a step fails, diagnose the error and suggest a fix before moving on.

## Step 1 — Check prerequisites

Run the following checks and report the results:

```bash
# Check kubectl is available and the cluster is reachable
kubectl version --short 2>/dev/null || kubectl version

# Check Kubernetes server version (must be >= 1.24)
kubectl version -o json | python3 -c "
import json, sys
v = json.load(sys.stdin)['serverVersion']
major, minor = int(v['major']), int(v['minor'].rstrip('+'))
ok = (major > 1) or (major == 1 and minor >= 24)
print(f'Kubernetes {v[\"major\"]}.{v[\"minor\"]} — {\"OK\" if ok else \"UNSUPPORTED (need >= 1.24)\"}')"

# Check Helm is installed and version >= 3
helm version --short
```

If kubectl cannot reach the cluster, stop and ask the user to configure their kubeconfig. If Helm is missing or < v3, instruct the user to install it from https://helm.sh/docs/intro/install/ before continuing.

## Step 2 — Add the OpenYurt Helm repository

```bash
helm repo add openyurt https://openyurtio.github.io/openyurt-helm
helm repo update
```

Confirm that `openyurt` appears in the repo list:

```bash
helm repo list | grep openyurt
```

## Step 3 — Install yurt-manager via Helm

Ask the user for the target namespace (default: `kube-system`).

```bash
NAMESPACE=${NAMESPACE:-kube-system}

helm upgrade --install yurt-manager openyurt/yurt-manager \
  --namespace "${NAMESPACE}" \
  --create-namespace \
  --wait \
  --timeout 5m
```

After installation, verify the Deployment is running:

```bash
kubectl -n "${NAMESPACE}" rollout status deployment/yurt-manager --timeout=3m
kubectl -n "${NAMESPACE}" get pods -l app.kubernetes.io/name=yurt-manager
```

If the pod is not Ready within 3 minutes, fetch the logs and report:

```bash
kubectl -n "${NAMESPACE}" logs -l app.kubernetes.io/name=yurt-manager --tail=50
```

## Step 4 — Create an Edge NodePool CR

Ask the user for the NodePool name (default: `edge`).

```bash
POOL_NAME=${POOL_NAME:-edge}

kubectl apply -f - <<EOF
apiVersion: apps.openyurt.io/v1beta2
kind: NodePool
metadata:
  name: ${POOL_NAME}
spec:
  type: Edge
EOF
```

Confirm the NodePool was created:

```bash
kubectl get nodepool "${POOL_NAME}"
```

## Step 5 — Convert nodes to edge nodes

List the available nodes so the user can choose which ones to convert:

```bash
kubectl get nodes -o wide
```

Ask the user which nodes should join the `${POOL_NAME}` NodePool. For each selected node, apply the NodePool membership label. The `YurtNodeConversionController` in `yurt-manager` watches this label and automatically launches a `node-servant` Job to perform the conversion.

```bash
NODE_NAME=<node-name>   # repeat for each node
kubectl label node "${NODE_NAME}" "apps.openyurt.io/nodepool=${POOL_NAME}" --overwrite
```

Wait for the `node-servant` conversion Job to complete on each node:

```bash
# List conversion Jobs (they are created in the yurt-manager namespace)
kubectl get jobs -A -l "apps.openyurt.io/nodepool=${POOL_NAME}"

# Wait for all Jobs to complete (timeout 5 minutes)
kubectl wait jobs \
  -l "apps.openyurt.io/nodepool=${POOL_NAME}" \
  --for=condition=complete \
  --all-namespaces \
  --timeout=5m
```

If a Job fails, inspect its pod logs:

```bash
kubectl get pods -A -l "apps.openyurt.io/nodepool=${POOL_NAME}" --field-selector=status.phase=Failed
kubectl logs <failed-pod-name> -n <namespace>
```

Common conversion failure causes:
- The node cannot reach the API server — verify network connectivity.
- The `node-servant` image cannot be pulled — check image pull secrets.
- The node already has conflicting labels — remove them and re-label.

## Step 6 — Enable edge autonomy

For each converted edge node, annotate it with the desired autonomy duration. This instructs `yurthub` to cache API responses so the node can operate independently if the cloud connection is lost.

```bash
NODE_NAME=<node-name>
DURATION=3600s   # customise: e.g. 1800s, 7200s

kubectl annotate node "${NODE_NAME}" \
  "node.openyurt.io/autonomy-duration=${DURATION}" \
  --overwrite
```

> **Tip**: `node.beta.openyurt.io/autonomy: "true"` is the older (deprecated) annotation. Use `node.openyurt.io/autonomy-duration` instead.

## Step 7 — Verify the Autonomy condition

Check that each edge node reports a healthy `Autonomy` condition:

```bash
for NODE in $(kubectl get nodes -l "apps.openyurt.io/nodepool=${POOL_NAME}" -o name); do
  echo "--- ${NODE} ---"
  kubectl get "${NODE}" \
    -o jsonpath='{range .status.conditions[?(@.type=="Autonomy")]}{.type}{"\t"}{.status}{"\t"}{.message}{"\n"}{end}'
done
```

Expected output for each node:

```
Autonomy    True    The autonomy is enabled and it works fine
```

If the condition is missing or `False`, check the `yurthub` logs on that node:

```bash
kubectl get pods -A | grep yurthub   # find the yurthub pod on the node
kubectl logs <yurthub-pod> -n <namespace> --tail=50
```

## Completion

If all nodes show `Autonomy: True`, the OpenYurt deployment is complete. Summarise:

- Kubernetes version ✅
- `yurt-manager` installed and running ✅
- NodePool `${POOL_NAME}` created ✅
- Nodes converted: list each node name ✅
- Edge autonomy enabled with duration `${DURATION}` ✅

Next steps:
- Run `/openyurt-raven` to configure cross-region networking with Raven.
- Explore `YurtAppSet` and `YurtAppDaemon` for edge workload management.
- See the [OpenYurt documentation](https://openyurt.io/docs) for advanced configuration.
