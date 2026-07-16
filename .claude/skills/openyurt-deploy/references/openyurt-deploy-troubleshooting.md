# OpenYurt Deployment Troubleshooting

Decision tree for diagnosing and resolving common deployment failures. Claude should follow this step-by-step logic when an error occurs.

---

## 1. Conversion Job Failures

Did the conversion fail (Node stuck in `Converting` or condition shows `ConvertFailed`)?

**↓ Yes**

Is the `node-servant-conversion-<node>` Job created?
Run `kubectl -n kube-system get jobs -l openyurt.io/conversion-node=<node-name>`

- **Yes, Job exists:**
  - **↓**
  - Check the Pod status: `kubectl -n kube-system get pods -l openyurt.io/conversion-node=<node-name>`
  - Is the Pod `Pending`?
    - **Yes:** Check for scheduling issues (`kubectl describe pod...`). E.g., is the node cordoned by another controller? Is there an image pull failure?
    - **No (Running/Error/CrashLoopBackOff):** Check the logs: `kubectl -n kube-system logs job/node-servant-conversion-<node-name>`
    - Does the log say `crictl: command not found`?
      - **Yes:** Ask the operator for confirmation to install `cri-tools` on the node, then delete the stale Job to retry.
      - **No:** Does the log say `failed to get apiServerAddress`?
        - **Yes:** Verify `/etc/kubernetes/kubelet.conf` exists and contains a valid `server` address.

- **No, Job is missing:**
  - **↓**
  - Is `yurt-manager` running successfully?
    - **No:** Go to Section 2 (yurt-manager Issues).
    - **Yes:** Verify the NodePool type. Is the NodePool type `Cloud`?
      - **Yes:** The controller intentionally skips `Cloud` NodePools. Only `Edge` NodePools trigger conversion.
      - **No:** Check if the node already has `openyurt.io/is-edge-worker=true`. If yes, it's already converted.

---

## 2. yurt-manager Issues

Did `yurt-manager` fail to start or are CRDs missing?

**↓ Yes**

Is the `yurt-manager` Pod stuck in `Pending`?
- **Yes:** Check `node-role.kubernetes.io/control-plane` label on the master node (yurt-manager has a nodeAffinity for control-plane nodes).

Are there errors in the `yurt-manager` logs (`kubectl -n kube-system logs deployment/yurt-manager`)?
- **Yes:**
  - Are they certificate/webhook errors? The manager creates self-signed certs at startup. Ensure it has permissions.
  - Are they RBAC errors? Verify `yurt-manager-controller-role-binding` exists.

Are CRDs missing (`kubectl get crd | grep openyurt`)?
- **Yes:** Reinstall/upgrade with Helm: `helm upgrade --install yurt-manager ./charts/yurt-manager -n kube-system`

---

## 3. NodePool Label Webhook Rejection

Did `kubectl label node` return: `apps.openyurt.io/nodepool can only be added or removed, not modified in-place`?

**↓ Yes**

- **Explanation:** The OpenYurt admission webhook strictly forbids changing the value of this label in-place to prevent race conditions.
- **Resolution:**
  1. Remove the label: `kubectl label node <node-name> apps.openyurt.io/nodepool-` (this triggers a revert).
  2. Wait for the node condition to show `Reverted`.
  3. Re-add the label with the new NodePool name: `kubectl label node <node-name> apps.openyurt.io/nodepool=<new-pool-name>`
