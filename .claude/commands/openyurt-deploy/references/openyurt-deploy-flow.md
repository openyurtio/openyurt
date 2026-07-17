# OpenYurt Deployment Flow

Follow these steps sequentially to deploy OpenYurt.

## Phase 1: Preflight Validation
1. **Kubernetes Version:** Run `kubectl version --short`. Verify the server version is >= 1.24.
2. **Helm Version:** Run `helm version --short`. Verify it is v3 or higher.
3. **crictl Check (CRITICAL):** Ask the user which nodes they intend to convert. Instruct them to verify `crictl` is in the PATH on those nodes via SSH. If missing, they must install it, or Phase 3 will fail silently.

## Phase 2: Deploy yurt-manager
1. `helm repo add openyurt https://openyurtio.github.io/openyurt-helm`
2. `helm repo update`
3. `helm upgrade --install yurt-manager openyurt/yurt-manager -n kube-system`
4. Wait for pods to be Running: `kubectl get pods -n kube-system -l app.kubernetes.io/name=yurt-manager`

## Phase 3: Node Conversion (Label-Driven)
1. **Create NodePool:** Ask the user for a NodePool name. Create and apply the `NodePool` CR (type: Edge).
2. **Label Nodes:** 
   `kubectl label node <node-name> apps.openyurt.io/nodepool=<NODEPOOL_NAME>`
   `kubectl label node <node-name> apps.openyurt.io/desired-nodepool=<NODEPOOL_NAME>`
3. **Verify Completion:** Poll `kubectl get node <node-name> -o json`. Wait for:
   - `apps.openyurt.io/nodepool` label is present.
   - `openyurt.io/is-edge-worker=true` label is present.
   - `YurtConversionFailed` condition is absent or `False`.
4. **Enable Autonomy:** `kubectl label node <node-name> node.openyurt.io/autonomy-duration=5m`
