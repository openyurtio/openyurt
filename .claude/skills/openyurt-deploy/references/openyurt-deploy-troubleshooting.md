# Troubleshooting (OpenYurt v1.7.0)

Always start with the diagnostics for the affected node:

```bash
"$S/collect-diagnostics.sh" --context <CTX> <node>
```

`$S` is `${CLAUDE_SKILL_DIR}/scripts`. Find the symptom below. After fixing the
cause, continue in [openyurt-deploy-recovery.md](openyurt-deploy-recovery.md);
do not just re-label the node.

## preflight-node.sh

| CHECK | Cause | Fix |
|---|---|---|
| `systemd=FAIL` | host not managed by systemd | not supported |
| `tool_crictl=FAIL` | node-servant runs `crictl` after `chroot` into the host and looks it up in `PATH` | install cri-tools on the node through the node's own management channel; the binary must be in `/usr/local/bin`, `/usr/bin`, ... |
| `tool_systemctl` / `tool_bash=FAIL` | node-servant runs `bash -c "systemctl daemon-reload"` and `systemctl restart kubelet` | install bash or systemd tools; otherwise not supported |
| `kubeadm_dropin=FAIL`, `kubelet_kubeconfig=FAIL` | not a kubeadm kubelet layout; node-servant reads the API server address from `10-kubeadm.conf` -> `kubelet.conf` | not supported (e.g. k3s, binary installs) |
| `kubeadm_flags=FAIL` | `/var/lib/kubelet/kubeadm-flags.env` is missing or has no `KUBELET_KUBEADM_ARGS="..."` | kubelet redirection appends to this file; not supported without it |
| `cri_socket=FAIL` docker | runtime is docker / dockershim | not supported |
| `cri_socket=FAIL` multiple | several CRI sockets, and kubelet has no valid explicit endpoint | set `--container-runtime-endpoint` (or `containerRuntimeEndpoint` in the kubelet config) to the runtime that is in use |
| `crictl_connect=WARN` | the unprivileged probe could not query the runtime | usually harmless; the conversion Job is privileged |
| `RESULT=TIMEOUT` | Pod never ran: image pull (air-gapped -> `--image <mirror>/busybox:1.36.1`), admission policy forbidding hostPath in kube-system, node NotReady | see the printed pods/events |

## check-install.sh

| CHECK / RESULT | Cause | Fix |
|---|---|---|
| `helm_chart=FAIL`, `yurt_manager_image=FAIL` | another version is installed | stop; upgrading is the user's decision |
| `node_servant_image=FAIL ... latest` | chart default `nodeServant.image.tag=latest` | needs `helm upgrade ... --set nodeServant.image.tag=v1.7.0` (change card; it restarts yurt-manager) |
| `controller_yurt-node-conversion-controller=FAIL` | `--controllers` does not enable it. Matching follows `IsControllerEnabled`: the first explicit `name`/`-name` wins, and `*` enables everything not named | fix `controllers` in the Helm values, e.g. `-nodelifecycle,*` (the chart default) |
| `node_webhook_flag=FAIL` | `--disable-independent-webhooks` contains `node` or `*` | remove it; without the Node webhook, label protections and defaulting are off |
| `webhook_*=FAIL`, `webhook_endpoints=FAIL` | webhook configuration not registered yet, or yurt-manager not ready | wait for `rollout status`; check the yurt-manager logs |
| `helm_namespace=FAIL` | release installed in a namespace other than kube-system | not supported by this skill (Jobs and webhooks follow the release namespace) |
| `MISSING_SHARED` | `yurt-hub-cfg` / `yurt-hub` RBAC not applied | flow.md Step 3b |
| `PROBE_UNAVAILABLE` | the NodePool used for the probe does not exist | create the pool first; for a node that references a deleted pool, see "node references a missing NodePool" |
| `WEBHOOK_UNHEALTHY` mutating FAIL | the dry-run result did not contain the defaulted `nodepool` | the mutating Node webhook is not being called: check `yurt-manager-mutating-webhook-configuration`, the service endpoints, and the yurt-manager logs |
| `WEBHOOK_UNHEALTHY` validating FAIL "accepted" | an `is-edge-worker` change was not rejected | the validating Node webhook is not effective (failurePolicy Ignore hides errors). Do not change labels until it is fixed |
| validating FAIL "another reason" | the request failed before validation, e.g. the defaulting webhook could not get the NodePool named by `desired-nodepool` | read the message; fix the labels or the pool |

## wait-node-conversion.sh

### NOT_STARTED - no conversion Job

- The NodePool is not `type: Edge`. The controller logs `skips conversion: nodepool "<p>" is not Edge type`.
- The label is not set: run `kubectl --context <CTX> get node <node> --show-labels`. The webhook may have been down when `desired-nodepool` was set.
- The node already has `is-edge-worker=true`, so the desired action is "none".
- A finished Job from an earlier round still exists. The controller keeps it while the
  action matches its own, so the new round does not start until that Job is gone.
  See recovery.md before you delete anything.
- yurt-manager is not running, or the conversion controller is disabled: run `check-install.sh`.

### JOB_FAILED - read the Job logs

| Log fragment | Cause | Fix |
|---|---|---|
| `crictl is required for container runtime` / `executable file not found` | crictl missing on the host | install cri-tools, then retry (recovery.md B) |
| `Found multiple CRI sockets` | ambiguous runtime | set an explicit kubelet runtime endpoint |
| `get apiserverAddr err: no file exists in list` | no `10-kubeadm.conf` | not supported |
| `kubeadm-flags.env error format` | unexpected `kubeadm-flags.env` content | not supported without a `KUBELET_KUBEADM_ARGS="..."` line |
| YurtHub health / readyz check timeout | YurtHub cannot reach the API server or is denied | check `journalctl -u yurthub` on the node (operator), then the 403 row below |
| YurtHub `403` / `forbidden` | missing shared RBAC: v1.7.0 keeps it in the yurthub chart, so installing only yurt-manager is not enough | apply Step 3b, then retry |
| `ImagePullBackOff` on the Job pod | node-servant image not reachable from the node | mirror the image, then `helm upgrade --set nodeServant.image.registry=...` |

### TIMEOUT

The round is still running. Keep the Job; follow `kubectl --context <CTX> -n kube-system logs -f job/node-servant-conversion-<node>`.
Pods stuck in `Pending` usually mean the image cannot be pulled, or that admission
rejected the privileged hostPath Pod in kube-system.

## Other symptoms

- **Label change rejected with `can only be added or removed, not modified in-place`:**
  this is expected. Use recovery.md C (leave, wait for `Reverted`, join).
- **Label change rejected for `openyurt.io/is-edge-worker`:** this is expected. That label belongs to the controller.
- **Node references a missing NodePool:** the defaulting webhook fails every update
  of that node. Recreate the pool with the same name and `type: Edge`, or have the user
  remove the labels. Removing them may start a revert when `is-edge-worker=true`.
- **ClusterRole `yurt-hub-multiplexer` has no rules:** this is not a fault. The
  hubleaderrbac controller fills it for pools that share pool-scope metadata.
- **`Autonomy` condition is `False` / "autonomy disabled" after setting `autonomy-duration`:**
  this is expected. Verify through pod tolerations instead (flow.md Step 6).
- **No `topology.kubernetes.io/zone` label after conversion:** this is expected on v1.7.0; that label was added after v1.7.0.
- **Very long node names (prefix `node-servant-conversion-<node>` over 58 characters):** generated Pod
  names are truncated, so node-servant's self-exclusion no longer matches. This is an upstream limitation,
  and conversion may stop its own Job pod.
