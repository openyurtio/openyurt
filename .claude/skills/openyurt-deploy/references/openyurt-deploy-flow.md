# OpenYurt v1.7.0 deployment flow

Execute the steps in order. `<CTX>` is the context confirmed in Step 0, and
`$S` is `${CLAUDE_SKILL_DIR}/scripts`. Every change needs a change card and an
explicit "yes" (see SKILL.md). When a step fails, go to
[openyurt-deploy-troubleshooting.md](openyurt-deploy-troubleshooting.md).

## Step 0 - Lock the target cluster

1. Show `kubectl config get-contexts` and the current context. Ask the user which
   context is the target and record it as `<CTX>`.
2. Check versions:

   ```bash
   kubectl --context <CTX> version
   helm version
   ```

   Server version must be >= 1.24 and Helm must be v3 or v4; otherwise stop.

## Step 1 - Pick the nodes

```bash
kubectl --context <CTX> get nodes -o wide -L apps.openyurt.io/nodepool,apps.openyurt.io/desired-nodepool,openyurt.io/is-edge-worker
"$S/collect-diagnostics.sh" --context <CTX> --summary
```

Candidates are worker nodes (no `node-role.kubernetes.io/control-plane`) in state
`NONE` or `REVERTED`. Nodes in any other state need
[openyurt-deploy-recovery.md](openyurt-deploy-recovery.md) first. Confirm the
list with the user. If they passed node names as arguments, start from those.

## Step 2 - Preflight every candidate

```bash
"$S/preflight-node.sh" --context <CTX> <node>
```

Creates a short-lived Job in kube-system, which needs a change card. If the cluster
pulls images from a private registry, pass `--image <registry>/busybox:1.36.1`.

| RESULT | Action |
|---|---|
| `PASS` | continue. Lines marked `WARN` are informational |
| `FAIL` | do not convert this node; show the FAIL lines and the fix from troubleshooting.md |
| `TIMEOUT` | the Pod could not run (image pull, admission, node down); see troubleshooting.md |

Also check the `ARTIFACT` lines. If any of them says `present` on a node that was never converted,
an earlier conversion left something on the host. Report this to the user before you continue.

## Step 3 - Install or reuse yurt-manager

```bash
"$S/check-install.sh" --context <CTX>
```

| RESULT (exit) | Branch |
|---|---|
| `OK` (0) | reuse the installation and go to Step 4 |
| `NOT_INSTALLED` (10) | fresh install (3a + 3b) |
| `MISSING_SHARED` (11) | yurt-manager is fine, only the shared yurthub resources are missing (3b only) |
| `INCOMPATIBLE` (12) | stop. Show every `CHECK ...=FAIL` line. Upgrading or reinstalling an existing release is the user's decision and outside this flow |

### 3a. Install yurt-manager (fresh install only)

Get the charts from the v1.7.0 tag. Use `--repo` with a local clone that has the
tag; without it the script downloads the release tarball:

```bash
"$S/prepare-charts.sh" --dest <empty-dir> [--repo <path-to-openyurt-clone>]
```

The script prints `YURT_MANAGER_CHART=` and `YURTHUB_CFG_MANIFEST=`. Keep both
paths. You also need the manifest for uninstall. Then:

```bash
helm --kube-context <CTX> --namespace kube-system install yurt-manager <YURT_MANAGER_CHART> \
  --set image.tag=v1.7.0 --set nodeServant.image.tag=v1.7.0
kubectl --context <CTX> -n kube-system rollout status deployment/yurt-manager --timeout=300s
```

Notes:
- `nodeServant.image.tag` defaults to `latest` in the chart. Always pin it.
- For a private registry, add `--set image.registry=<r> --set nodeServant.image.registry=<r>`.
- Do not install the `charts/yurthub` chart itself. In v1.7.0 the conversion Job
  installs YurtHub as a systemd service. Only its shared RBAC and ConfigMap are needed (3b).

### 3b. Apply the shared yurthub resources

These are the `yurt-hub` ClusterRole/ClusterRoleBinding for `system:nodes`, the
static-set bootstrap RBAC, and the `kube-system/yurt-hub-cfg` ConfigMap. Without
them, YurtHub on converted nodes gets 403 responses. Run `prepare-charts.sh` first if you skipped 3a:

```bash
kubectl --context <CTX> apply -f <YURTHUB_CFG_MANIFEST>
```

Re-run `check-install.sh --context <CTX>` and require `RESULT=OK`.

## Step 4 - Create the Edge NodePool

Ask the user for the pool name.

```bash
cat <<'EOF' | kubectl --context <CTX> apply -f -
apiVersion: apps.openyurt.io/v1beta2
kind: NodePool
metadata:
  name: <pool>
spec:
  type: Edge
EOF
kubectl --context <CTX> get nodepools.apps.openyurt.io <pool> -o jsonpath='{.spec.type}{"\n"}'
```

`type` must be `Edge`. The controller silently skips nodes in a `Cloud` pool.
`spec.hostNetwork` and `spec.enableLeaderElection` are optional, and this flow does not change them.

## Step 5 - Convert one node

Repeat for each node, one at a time.

1. Probe the Node webhooks for this node and pool. This uses only dry-run requests:

   ```bash
   "$S/check-install.sh" --context <CTX> --probe-node <node> --probe-pool <pool>
   ```

   Continue only on `RESULT=OK`. On `WEBHOOK_UNHEALTHY` or `PROBE_UNAVAILABLE`, see troubleshooting.md.
2. Record the baseline **before** you change anything:

   ```bash
   "$S/wait-node-conversion.sh" baseline --context <CTX> <node>
   ```

   Keep the `BASELINE=` value.
3. Show a change card. Impact: the node is cordoned; YurtHub is installed on the host;
   kubelet is redirected and restarted; every non-pause container on the node is
   stopped and restarted according to its `restartPolicy`. Then:

   ```bash
   kubectl --context <CTX> label node <node> apps.openyurt.io/nodepool=<pool>
   ```

4. Wait:

   ```bash
   "$S/wait-node-conversion.sh" wait --context <CTX> <node> --action convert --baseline <BASELINE>
   ```

| RESULT (exit) | Meaning / next |
|---|---|
| `STABLE` / `ALREADY_STABLE` (0) | converted; go to the next node |
| `JOB_FAILED` (1) | the Job of this round failed. Go to recovery.md, flow B |
| `TIMEOUT` (3) | round still running; run `collect-diagnostics.sh <node>` and decide with the user whether to keep waiting |
| `NOT_STARTED` (4) | no round started; see troubleshooting.md "no conversion Job" |

## Step 6 - Node autonomy (optional)

Only do this if the user asks for it. `node.openyurt.io/autonomy-duration` makes the
pod-binding controller set `tolerationSeconds` on the pods of the node for the
`node.kubernetes.io/not-ready` and `node.kubernetes.io/unreachable` `NoExecute`
tolerations. Pods are then not evicted while the node is offline for less than
the duration. `0` means never evict.

```bash
"$S/check-install.sh" --context <CTX> --require-autonomy     # requires RESULT=OK
kubectl --context <CTX> annotate node <node> node.openyurt.io/autonomy-duration=<duration, e.g. 10m> --overwrite
```

Verify on a pod running on the node. The original value is saved in a
`apps.openyurt.io/original-*-toleration-duration` annotation:

```bash
kubectl --context <CTX> get pods -A --field-selector spec.nodeName=<node> \
  -o jsonpath='{range .items[*]}{.metadata.namespace}/{.metadata.name}{" "}{.spec.tolerations[?(@.key=="node.kubernetes.io/unreachable")].tolerationSeconds}{"\n"}{end}'
```

Do **not** use the node's `Autonomy` condition for this check. YurtHub sets that
condition only from the deprecated `node.beta.openyurt.io/autonomy=true` annotation,
so it shows `False` / "autonomy disabled" when only `autonomy-duration` is set.

## Step 7 - Summary

```bash
"$S/collect-diagnostics.sh" --context <CTX> --summary
kubectl --context <CTX> get nodepools.apps.openyurt.io
```

Report to the user:
- yurt-manager release and version;
- the pool;
- each node's state (expected `CONVERTED`);
- autonomy settings, if any;
- where the `YURTHUB_CFG_MANIFEST` is kept (it is needed for uninstall).

In v1.7.0 do not expect a `topology.kubernetes.io/zone` label on converted nodes.

## Newer than v1.7.0 (master, not covered here)

- The conversion Job carries the finalizer `openyurt.io/node-conversion`.
- Converted nodes get `topology.kubernetes.io/zone=<pool>`.
- The yurt-manager chart ships the yurthub shared resources itself.

Do not apply these expectations to a v1.7.0 install.
