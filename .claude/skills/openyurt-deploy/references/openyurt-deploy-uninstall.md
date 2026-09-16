# OpenYurt Uninstall & Rollback

If the user requests to roll back the OpenYurt deployment and revert edge
nodes to standard worker nodes, execute these steps in order.

---

## Phase 1: Revert Node Conversion

For **each** converted edge node, execute the following. Do one node at a
time and wait for the revert to complete before moving to the next.

### 1.1 Remove the autonomy annotation (if set)

```bash
kubectl annotate node <node-name> node.openyurt.io/autonomy-duration-
```

### 1.2 Remove nodepool labels to trigger revert

```bash
kubectl label node <node-name> apps.openyurt.io/desired-nodepool-
kubectl label node <node-name> apps.openyurt.io/nodepool-
```

Removing the `apps.openyurt.io/nodepool` label triggers the
`YurtNodeConversionController` to run a revert Job
(`node-servant-revert-<node-name>`).

### 1.3 Wait for revert to complete

Poll the node condition until it shows `Reverted`:

```bash
NODE_NAME="<node-name>"
echo "Waiting for node $NODE_NAME to revert..."
for i in $(seq 1 60); do
  REASON=$(kubectl get node "$NODE_NAME" -o jsonpath='{range .status.conditions[?(@.type=="YurtNodeConversionFailed")]}{.reason}{end}' 2>/dev/null)
  EDGE_LABEL=$(kubectl get node "$NODE_NAME" -o jsonpath='{.metadata.labels.openyurt\.io/is-edge-worker}' 2>/dev/null)
  
  if [ "$REASON" = "Reverted" ] && [ -z "$EDGE_LABEL" ]; then
    echo "✅ Node $NODE_NAME reverted successfully."
    break
  fi
  
  echo "  [$i/60] condition=$REASON edge-worker=$EDGE_LABEL"
  sleep 5
done
```

### 1.4 Verify the revert

```bash
kubectl get node <node-name> -o jsonpath='{.metadata.labels}' | jq 'with_entries(select(.key | startswith("openyurt")))'
```

The `openyurt.io/is-edge-worker` label should be absent.

---

## Phase 2: Delete NodePool CRs

```bash
kubectl delete nodepool <edge-pool-name>
kubectl delete nodepool <cloud-pool-name>  # if created
```

Verify no NodePools remain:

```bash
kubectl get nodepools
```

---

## Phase 3: Uninstall yurt-manager

```bash
helm uninstall yurt-manager -n kube-system
```

### 3.1 Verify pods are terminated

```bash
kubectl -n kube-system get pods -l app.kubernetes.io/name=yurt-manager
```

### 3.2 Remove CRDs (optional — ask the user first)

**Warning:** This permanently removes all NodePool, YurtAppSet, and other
OpenYurt custom resource data from the cluster.

```bash
kubectl delete crd \
  nodepools.apps.openyurt.io \
  yurtappsets.apps.openyurt.io \
  yurtappdaemons.apps.openyurt.io \
  yurtappoverriders.apps.openyurt.io \
  platforms.iot.openyurt.io \
  deviceservices.iot.openyurt.io \
  devices.iot.openyurt.io \
  deviceprofiles.iot.openyurt.io 2>/dev/null

echo "CRDs removed."
```

---

## Phase 4: Clean up stale resources

Check for any leftover conversion Jobs:

```bash
kubectl -n kube-system get jobs | grep node-servant
```

If any remain, delete them:

```bash
kubectl -n kube-system delete jobs -l app=node-servant
```

Check for any leftover YurtHub static pod manifests on the nodes. Deploy a privileged
cleanup DaemonSet to remove them natively:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: yurthub-manifest-cleanup
  namespace: kube-system
spec:
  selector:
    matchLabels:
      app: cleanup
  template:
    metadata:
      labels:
        app: cleanup
    spec:
      hostNetwork: true
      tolerations:
      - operator: Exists
      containers:
      - name: cleanup
        image: busybox
        command: ["/bin/sh", "-c"]
        args: ["rm -f /host/etc/kubernetes/manifests/yurthub* && echo 'Cleanup complete' && sleep 3600"]
        volumeMounts:
        - name: host-root
          mountPath: /host
      volumes:
      - name: host-root
        hostPath:
          path: /
EOF

echo "Waiting for cleanup DaemonSet..."
sleep 10
kubectl -n kube-system delete daemonset yurthub-manifest-cleanup
echo "Static pod manifests cleaned up natively."
```

Kubelet will automatically stop the static pod once the manifest is removed.
