# OpenYurt Deployment Troubleshooting

Use this decision tree when a deployment step fails.

## Scenario 1: yurt-manager pods are CrashLoopBackOff
- **Check:** `kubectl logs -n kube-system -l app.kubernetes.io/name=yurt-manager`
- **Resolution:** Verify if the cluster has RBAC enabled and if the Helm chart deployed the ServiceAccount properly. Ensure the API server is reachable.

## Scenario 2: Node Conversion Failed (YurtNodeConversionFailed=True)
If a node fails conversion, the `node-servant` Job likely failed.
- **Diagnostic Script:** Execute this to automatically find the failure reason.
  ```bash
  NODE_NAME="<failed-node-name>"
  POD=$(kubectl get pod -n kube-system -l job-name=node-servant-convert-$NODE_NAME -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
  
  if [ -z "$POD" ]; then
      echo "Diagnostic B: Node-servant pod not found. Check if the node has NoSchedule taints:"
      kubectl get node $NODE_NAME -o jsonpath='{.spec.taints}'
  else
      LOGS=$(kubectl logs $POD -n kube-system)
      if echo "$LOGS" | grep -q 'exec: "crictl": executable file not found'; then
          echo "Diagnostic A: Missing crictl. The preflight check was skipped."
          echo "Resolution: SSH into the node, install cri-tools, and delete the failed node-servant Job."
      else
          echo "Diagnostic C: Unknown error. Logs:"
          echo "$LOGS" | tail -n 20
      fi
  fi
  ```

## Scenario 3: Edge Autonomy not taking effect
- **Check:** Verify `node.openyurt.io/autonomy-duration` annotation is applied correctly.
- **Check:** Ensure the node is disconnected from the API server to trigger the autonomy behavior in the Yurthub cache.
