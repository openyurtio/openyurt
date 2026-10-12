# OpenYurt Deployment Troubleshooting

Decision tree for diagnosing and resolving common deployment failures. Follow
each branch step-by-step and stop at the first one that matches.

---

## 1. Conversion Job Failures

Did the conversion fail (Node stuck in `Converting` or condition shows
`ConvertFailed`)?

**↓ Yes**

Is the `node-servant-conversion-<node>` Job created?

```bash
kubectl -n kube-system get jobs -l openyurt.io/conversion-node=<node-name>
```

- **Yes, Job exists:**
  - Check the Pod status:
    ```bash
    kubectl -n kube-system get pods -l openyurt.io/conversion-node=<node-name>
    ```
  - Is the Pod `Pending`?
    - **Yes:** Check for scheduling issues:
      ```bash
      kubectl -n kube-system describe pod -l openyurt.io/conversion-node=<node-name>
      ```
      Common causes: node is cordoned by another controller, image pull
      failure, or resource constraints.
    - **No (Running/Error/CrashLoopBackOff):** Check the logs:
      ```bash
      kubectl -n kube-system logs job/node-servant-conversion-<node-name>
      ```
      - Does the log say `crictl: command not found` or
        `exec: "crictl": executable file not found`?
        - **Yes:** `crictl` is missing. 
          Instruct the user to install `cri-tools` via their remote
          management system natively. E.g.:
          - Debian/Ubuntu: `sudo apt-get update && sudo apt-get install -y cri-tools`
          - CentOS/RHEL: `sudo yum install -y cri-tools`
          
          Then delete the stale Job to trigger a retry:
          ```bash
          kubectl -n kube-system delete job node-servant-conversion-<node-name>
          ```
          The controller will recreate it automatically.
      - Does the log say `failed to get apiServerAddress`?
        - **Yes:** Verify `/etc/kubernetes/kubelet.conf` exists on the node
          and contains a valid `server` address using an ephemeral Job:
          ```bash
          NODE_NAME="<node-name>"
          cat <<EOF | kubectl apply -f -
          apiVersion: batch/v1
          kind: Job
          metadata:
            name: read-kubelet-conf-$NODE_NAME
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
                  image: busybox
                  command: ["/bin/sh", "-c"]
                  args: ["cat /host/etc/kubernetes/kubelet.conf | grep server || echo 'Missing server config'"]
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
          kubectl -n kube-system wait --for=condition=complete job/read-kubelet-conf-$NODE_NAME --timeout=30s || true
          kubectl -n kube-system logs job/read-kubelet-conf-$NODE_NAME 2>/dev/null
          kubectl -n kube-system delete job read-kubelet-conf-$NODE_NAME 2>/dev/null
          ```
      - Does the log show CRI socket errors (e.g., `containerd.sock: no such file`)?
        - **Yes:** The CRI socket path differs between kubeadm and k3s:
          - kubeadm: `/run/containerd/containerd.sock`
          - k3s: `/run/k3s/containerd/containerd.sock`
          
          Check which exists using an ephemeral Job:
          ```bash
          NODE_NAME="<node-name>"
          cat <<EOF | kubectl apply -f -
          apiVersion: batch/v1
          kind: Job
          metadata:
            name: check-cri-socket-$NODE_NAME
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
                  image: busybox
                  command: ["/bin/sh", "-c"]
                  args:
                  - |
                    ls -la /host/run/containerd/containerd.sock 2>/dev/null || echo "No kubeadm socket"
                    ls -la /host/run/k3s/containerd/containerd.sock 2>/dev/null || echo "No k3s socket"
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
          kubectl -n kube-system wait --for=condition=complete job/check-cri-socket-$NODE_NAME --timeout=30s || true
          kubectl -n kube-system logs job/check-cri-socket-$NODE_NAME 2>/dev/null
          kubectl -n kube-system delete job check-cri-socket-$NODE_NAME 2>/dev/null
          ```
      - None of the above? Run the automated diagnostic script:
        ```bash
        NODE_NAME="<node-name>"
        POD=$(kubectl get pod -n kube-system -l job-name=node-servant-conversion-$NODE_NAME -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
        if [ -z "$POD" ]; then
            echo "No servant pod found. Checking node taints:"
            kubectl get node $NODE_NAME -o jsonpath='{.spec.taints}'
        else
            echo "Last 30 lines of servant logs:"
            kubectl logs $POD -n kube-system --tail=30
        fi
        ```

- **No, Job is missing:**
  - Is `yurt-manager` running?
    ```bash
    kubectl -n kube-system get deployment yurt-manager
    ```
    - **No:** Go to Section 2 (yurt-manager Issues).
    - **Yes:** Verify the NodePool type:
      ```bash
      kubectl get nodepool <pool-name> -o jsonpath='{.spec.type}'
      ```
      - Is the NodePool type `Cloud`?
        - **Yes:** The controller intentionally skips `Cloud` NodePools.
          Only `Edge` NodePools trigger conversion.
      - **No:** Check if the node already has the edge worker label:
        ```bash
        kubectl get node <node-name> -o jsonpath='{.metadata.labels.openyurt\.io/is-edge-worker}'
        ```
        - If `true`, the node is already converted.

---

## 2. yurt-manager Issues

Did `yurt-manager` fail to start or are CRDs missing?

**↓ Yes**

Is the `yurt-manager` Pod stuck in `Pending`?

```bash
kubectl -n kube-system describe pod -l app.kubernetes.io/name=yurt-manager
```

- **Yes (Pending):** Check if the master node has the
  `node-role.kubernetes.io/control-plane` label. yurt-manager has a
  nodeAffinity for control-plane nodes.

Are there errors in the `yurt-manager` logs?

```bash
kubectl -n kube-system logs deployment/yurt-manager --tail=50
```

- **Certificate/webhook errors:** The manager creates self-signed certs at
  startup. Ensure it has RBAC permissions for secrets in `kube-system`.
- **RBAC errors:** Verify the ClusterRoleBinding exists:
  ```bash
  kubectl get clusterrolebinding yurt-manager-controller-role-binding
  ```

Are CRDs missing?

```bash
kubectl get crd | grep openyurt
```

- **Yes:** Reinstall with Helm:
  ```bash
  helm upgrade --install yurt-manager openyurt/yurt-manager -n kube-system
  ```

---

## 3. NodePool Label Webhook Rejection

Did `kubectl label node` return:
`apps.openyurt.io/nodepool can only be added or removed, not modified in-place`?

**↓ Yes**

- **Explanation:** The OpenYurt admission webhook strictly forbids changing
  the value of this label in-place to prevent race conditions.
- **Resolution:**
  1. Remove the label (this triggers a revert):
     ```bash
     kubectl label node <node-name> apps.openyurt.io/nodepool-
     ```
  2. Wait for the node condition to show `Reverted`:
     ```bash
     kubectl get node <node-name> -o jsonpath='{range .status.conditions[?(@.type=="YurtNodeConversionFailed")]}{.reason}{end}'
     ```
  3. Re-add the label with the new NodePool name:
     ```bash
     kubectl label node <node-name> apps.openyurt.io/nodepool=<new-pool-name>
     ```

---

## 4. Edge Autonomy Not Taking Effect

- **Check 1:** Verify `node.openyurt.io/autonomy-duration` is applied as an
  **annotation** (not a label):
  ```bash
  kubectl get node <node-name> -o jsonpath='{.metadata.annotations.node\.openyurt\.io/autonomy-duration}'
  ```
- **Check 2:** Autonomy only manifests when the node is disconnected from
  the API server. The YurtHub cache serves stale data during disconnection.
  This is by design and cannot be verified while the node has connectivity.
- **Check 3:** Verify YurtHub is running as a static pod on the node:
  ```bash
  kubectl get pods -A --field-selector spec.nodeName=<node-name> | grep yurthub
  ```
