# OpenYurt Deployment Flow

Follow these steps sequentially to deploy OpenYurt.

## Phase 1: Preflight Validation
1. **Kubernetes Version:** Run `kubectl version --short`. Verify the server version is >= 1.24.
2. **Helm Version:** Run `helm version --short`. Verify it is v3 or higher.
3. **crictl Preflight Check (CRITICAL):** Ask the user which nodes they intend to convert and for SSH access credentials to those nodes. Use the `ssh` command to autonomously execute `command -v crictl` on the target nodes to verify it exists in the PATH *before* proceeding. If missing, halt and instruct the user to install `cri-tools`, as the `node-servant` Job in Phase 3 will silently fail otherwise.

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
3. **Automated Verification Loop:** Write and execute a bash polling script using `kubectl get node <node-name> -o json` to verify `isConvertedStable`. The loop must verify:
   - `apps.openyurt.io/nodepool` label is present.
   - `openyurt.io/is-edge-worker` label is exactly `"true"`.
   - Node is uncordoned (`.spec.unschedulable` is false or missing).
   - The condition `YurtNodeConversionFailed` exists, its `.status` is `"False"`, and its `.reason` is `"Converted"`.
   Do not proceed to Step 4 until this polling loop succeeds.
4. **Enable Autonomy:** `kubectl annotate node <node-name> node.openyurt.io/autonomy-duration=5m`
