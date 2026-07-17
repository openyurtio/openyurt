# OpenYurt Deployment Troubleshooting

Use this decision tree when a deployment step fails.

## Scenario 1: yurt-manager pods are CrashLoopBackOff
- **Check:** `kubectl logs -n kube-system -l app.kubernetes.io/name=yurt-manager`
- **Resolution:** Verify if the cluster has RBAC enabled and if the Helm chart deployed the ServiceAccount properly. Ensure the API server is reachable.

## Scenario 2: Node Conversion Failed (YurtConversionFailed=True)
If a node fails conversion, the `node-servant` Job likely failed.
- **Check:** Find the `node-servant` pod for that node in the `kube-system` namespace.
- **Check:** `kubectl logs <node-servant-pod> -n kube-system`
- **Diagnostic A (Missing crictl):** If the logs say `exec: "crictl": executable file not found in $PATH`, the preflight check was skipped. Instruct the user to SSH into the node, install `cri-tools`, and manually delete the failed `node-servant` Job to re-trigger the controller.
- **Diagnostic B (Taints):** Check if the node has `NoSchedule` taints preventing the `node-servant` DaemonSet/Job from running.

## Scenario 3: Edge Autonomy not taking effect
- **Check:** Verify `node.openyurt.io/autonomy-duration` label is applied correctly.
- **Check:** Ensure the node is disconnected from the API server to trigger the autonomy behavior in the Yurthub cache.
