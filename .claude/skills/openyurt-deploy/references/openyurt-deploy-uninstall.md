# OpenYurt Uninstall & Rollback

Procedures for reverting node conversions and removing OpenYurt from the
cluster. Follow the steps in order.

---

## Step 1: Revert Edge Nodes

Removing the `apps.openyurt.io/nodepool` label from a converted node
triggers the `YurtNodeConversionController` to create a **revert** Job.
The revert Job uninstalls YurtHub and restores direct kubelet-to-apiserver
communication.

### 1.1 Remove the NodePool label from each edge node

**Ask the user for explicit confirmation** before removing the labels, as this will trigger the revert process.

```bash
# List all edge nodes
kubectl get nodes -l openyurt.io/is-edge-worker=true

# Remove the NodePool label (triggers revert)
kubectl label node <node-name> apps.openyurt.io/nodepool-
```

### 1.2 Monitor the revert Job

```bash
kubectl -n kube-system get jobs -l openyurt.io/conversion-node=<node-name> -w
kubectl -n kube-system logs -f job/node-servant-conversion-<node-name>
```

### 1.3 Verify the revert completed

```bash
kubectl get node <node-name> -o jsonpath='{range .status.conditions[?(@.type=="YurtNodeConversionFailed")]}{.reason}{" "}{.message}{"\n"}{end}'
```

Expected output: `Reverted YurtHub uninstalled and node reverted successfully`

The controller also:
- Removes the `openyurt.io/is-edge-worker` label
- Removes the `topology.kubernetes.io/zone` label (only if its value
  matches a NodePool name)
- Uncordons the node

### 1.4 Repeat for all edge nodes

```bash
# Bulk remove labels (use with caution)
for node in $(kubectl get nodes -l openyurt.io/is-edge-worker=true -o name); do
  kubectl label "$node" apps.openyurt.io/nodepool-
  echo "Triggered revert for $node"
done
```

Wait for all nodes to reach `Reverted` state before proceeding.

---

## Step 2: Delete NodePool CRs

```bash
kubectl get nodepools
kubectl delete nodepool <edge-pool-name>
kubectl delete nodepool <cloud-pool-name>  # if created
```

Verify no NodePools remain:

```bash
kubectl get nodepools
```

---

## Step 3: Uninstall yurt-manager

```bash
helm uninstall yurt-manager -n kube-system
```

Verify the Deployment and associated resources are removed:

```bash
kubectl -n kube-system get deployment yurt-manager
kubectl -n kube-system get service yurt-manager-webhook-service
kubectl -n kube-system get secret yurt-manager-webhook-certs
```

All should return `not found`.

---

## Step 4: (Optional) Remove CRDs

Helm does **not** remove CRDs on uninstall by design. If you want a full
cleanup:

```bash
kubectl delete crd \
  nodepools.apps.openyurt.io \
  nodebuckets.apps.openyurt.io \
  yurtappdaemons.apps.openyurt.io \
  yurtappsets.apps.openyurt.io \
  yurtstaticsets.apps.openyurt.io \
  platformadmins.iot.openyurt.io \
  poolservices.network.openyurt.io \
  gateways.raven.openyurt.io
```

> **Warning**: Deleting CRDs will delete **all** custom resources of those
> types across the cluster. Only do this if you are fully removing OpenYurt.

---

## Step 5: (Optional) Clean up RBAC

```bash
kubectl delete clusterrolebinding yurt-manager-controller-role-binding
kubectl delete clusterrolebinding yurt-manager-webhook-role-binding
kubectl delete clusterrolebinding yurt-hub-multiplexer-binding
kubectl delete clusterrole yurt-hub-multiplexer
```

---

## Step 6: Verify Full Cleanup

```bash
echo "=== OpenYurt Uninstall Verification ==="
echo ""
echo "--- CRDs ---"
kubectl get crd | grep openyurt || echo "No OpenYurt CRDs found"
echo ""
echo "--- NodePools ---"
kubectl get nodepools 2>/dev/null || echo "NodePool CRD not found (good)"
echo ""
echo "--- Edge Worker Labels ---"
kubectl get nodes -l openyurt.io/is-edge-worker=true 2>/dev/null || echo "No edge worker nodes found (good)"
echo ""
echo "--- yurt-manager ---"
kubectl -n kube-system get deployment yurt-manager 2>/dev/null || echo "yurt-manager not found (good)"
echo ""
echo "--- Conversion Jobs ---"
kubectl -n kube-system get jobs -l openyurt.io/conversion-node 2>/dev/null || echo "No conversion jobs found (good)"
echo ""
echo "=== Uninstall Complete ==="
```

---

## Troubleshooting Revert Failures

If a node revert fails (shows `RevertFailed`):

```bash
# Check the revert Job logs
kubectl -n kube-system logs job/node-servant-conversion-<node-name>

# SSH into the node and check YurtHub status
ssh <user>@<node-ip> "ls /etc/kubernetes/manifests/ | grep yurthub"
ssh <user>@<node-ip> "systemctl status kubelet"
```

If the revert Job cannot clean up properly, manual intervention may be
required on the node:

```bash
# Remove YurtHub static pod manifest
ssh <user>@<node-ip> "sudo rm -f /etc/kubernetes/manifests/yurthub.yaml"

# Restore kubelet configuration to point directly to API server
ssh <user>@<node-ip> "sudo systemctl restart kubelet"
```

After manual cleanup, remove any remaining labels:

```bash
kubectl label node <node-name> openyurt.io/is-edge-worker-
kubectl label node <node-name> topology.kubernetes.io/zone-
```
