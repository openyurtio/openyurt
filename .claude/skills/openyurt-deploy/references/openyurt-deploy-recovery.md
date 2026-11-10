# Recovery: leaving pools, failed rounds, host cleanup

`<CTX>` is the locked context and `$S` is `${CLAUDE_SKILL_DIR}/scripts`. Every change
needs a change card and an explicit "yes". Work on one node at a time.

Start from the node state:

```bash
"$S/collect-diagnostics.sh" --context <CTX> <node>      # STATE=..., NOTE=..., logs
```

## How the v1.7.0 controller decides (why the flows differ)

`desiredActionFromNode` (pkg/yurtmanager/controller/yurtnodeconversion):

| nodepool label | is-edge-worker | desired action |
|---|---|---|
| set | not `true` | convert |
| absent | `true` | revert |
| otherwise | | none |

Behaviour that matters for recovery:

- **One Job per node.** It is named `node-servant-conversion-<node>` and owns the
  round while it exists. A failed Job keeps the node cordoned. The controller does
  not start a new round while that Job exists.
- **Automatic retry.** Finished Jobs have `ttlSecondsAfterFinished: 7200`. When a
  failed Job expires and the labels still request the same action, the controller
  starts a new round by itself. **Stopping the skill does not stop conversion.**
  Only removing the trigger stops it.
- `is-edge-worker` is set only when a convert round **succeeds**. After `ConvertFailed`
  it is still unset, so removing the pool label leaves the action at "none": **no revert
  runs**, and the host may still hold a half-done conversion.
- The defaulting webhook copies `apps.openyurt.io/desired-nodepool` into
  `apps.openyurt.io/nodepool` whenever the latter is missing. Removing only `nodepool`
  while `desired-nodepool` is set is undone immediately.

Report one of these two lines for every node you leave:

- `conversion stopped`: no label requests an action, no Job is running.
- `controller will retry automatically around <Job completion time + 2h>`: labels still request the action and the failed Job will expire.

## A - Leave a pool (node is CONVERTED)

1. Probe the webhooks on the node itself. It is in pool `X`, so the probe uses `X`:
   `"$S/check-install.sh" --context <CTX> --probe-node <node>`. Require `RESULT=OK`.
2. `"$S/wait-node-conversion.sh" baseline --context <CTX> <node>` and keep `BASELINE`.
3. Remove **both** labels in one request:

   ```bash
   kubectl --context <CTX> label node <node> apps.openyurt.io/nodepool- apps.openyurt.io/desired-nodepool-
   ```

   `kubectl label` warns when a label is absent. That warning is harmless.
4. Re-read the node after about 10 s:

   ```bash
   kubectl --context <CTX> get node <node> -L apps.openyurt.io/nodepool,apps.openyurt.io/desired-nodepool
   ```

   If a pool label is back, something outside OpenYurt is writing it (GitOps, a node
   provisioner, a label sync). Stop and report this; do not fight it.
5. `"$S/wait-node-conversion.sh" wait --context <CTX> <node> --action revert --baseline <BASELINE>`.
   `STABLE` means `Reverted`. `JOB_FAILED` means go to flow R.

## C - Change pools

Run flow A completely, until the node is `REVERTED`. Then convert it into the new
pool with flow.md Step 5. Never remove the label and add a new one back to back:
when the revert round has not run yet, the node keeps the old host configuration.

## B - ConvertFailed (state CONVERT_FAILED)

The node is cordoned, `is-edge-worker` is not set, and the failed Job is still present.

1. **Diagnose.** Read the Job logs in the diagnostics output and fix the cause with
   [openyurt-deploy-troubleshooting.md](openyurt-deploy-troubleshooting.md).
   Re-run `preflight-node.sh` when the cause is on the host.
2. **Retry** (supported path; convert is idempotent):
   1. Confirm the Job is finished (`CONVERSION_JOB=yes:failed:convert:...`) and has no
      active pods:
      `kubectl --context <CTX> -n kube-system get pods -l job-name=node-servant-conversion-<node>`.
   2. `"$S/wait-node-conversion.sh" baseline --context <CTX> <node>` and keep `BASELINE`
      (it is the failed Job's UID).
   3. Delete the failed Job:
      `kubectl --context <CTX> -n kube-system delete job node-servant-conversion-<node>`.
      The labels still request convert, so the controller creates a new Job.
   4. `"$S/wait-node-conversion.sh" wait --context <CTX> <node> --action convert --baseline <BASELINE>`.
      The old `ConvertFailed` condition does not end this wait. Only the new Job's result does.
3. **Retry limit.** Stop retrying after **2** retries with the same cause, or at once
   when the cause cannot be fixed from the cluster (unsupported host layout).
   Then ask the user to choose:
   - **(i) Abandon the conversion.** Continue with step 4.
   - **(ii) Leave the node as it is.** Tell the user plainly: the node stays cordoned,
     and *the controller will retry automatically around <failed Job completion + 2h>*,
     re-running host changes and container restarts. Do not describe the node as stopped.
4. **Abandon.** Each sub-step needs its own confirmation.
   1. Stop the conversion by removing both labels in one request:
      `kubectl --context <CTX> label node <node> apps.openyurt.io/nodepool- apps.openyurt.io/desired-nodepool-`.
      Re-read the node and confirm both labels are gone. From this point *conversion is stopped*.
   2. Check host leftovers: `"$S/preflight-node.sh" --context <CTX> <node>` (see the `ARTIFACT` lines).
      If every artifact is `absent`, skip to 4.5.
   3. Clean the host: `"$S/cleanup-node.sh" --context <CTX> <node>`.
      - The script refuses to run unless both labels are gone, the node is cordoned,
        `is-edge-worker` is not `true`, and the conversion Job is finished with no active pods.
      - It runs `node-servant revert` once, in a Job named `node-servant-conversion-<node>-rv-<4 hex>`.
        The prefix is required: revert restarts all non-pause containers on the node
        **except** pods whose name starts with `node-servant-conversion-<node>`, and the
        prefix is what keeps the cleanup container from stopping itself.
      - The Job carries no `openyurt.io/conversion-node` label, so the controller ignores it.
   4. On `RESULT=CLEANED`, re-run `preflight-node.sh` and check that the `ARTIFACT` lines are `absent`.
   5. Delete the failed conversion Job and the cleanup Job, then uncordon:

      ```bash
      kubectl --context <CTX> -n kube-system delete job node-servant-conversion-<node> node-servant-conversion-<node>-rv-<hex> --ignore-not-found
      kubectl --context <CTX> uncordon <node>
      ```

      The `YurtNodeConversionFailed=True/ConvertFailed` condition stays on the node. It is
      informational only, and the next conversion round overwrites it.
   6. On `RESULT=CLEANUP_FAILED` or `CLEANUP_TIMEOUT`:
      - **Do not uncordon. Do not delete either Job.** They are evidence, and they expire after 2h.
      - Give the user the `DIAGNOSTICS=` file and the `POSSIBLE_HOST_LEFTOVERS`.
      - The node is now `CLEANUP_FAILED`. It blocks uninstall, and manual host work is required.
        Summary for an operator: undo the `--kubeconfig=/var/lib/openyurt/kubelet.conf --bootstrap-kubeconfig=`
        suffix in `/var/lib/kubelet/kubeadm-flags.env`, restart kubelet, stop and remove `yurthub.service`
        and its drop-in, remove `/usr/local/bin/yurthub` and `/var/lib/openyurt/kubelet.conf`.
      - Do not try other automated host changes.

Name limits: the cleanup Job name must be at most 63 characters, and
`node-servant-conversion-<node>` at most 58 characters, because generated Pod names keep
only 58 characters of the prefix. The script refuses when either limit is exceeded;
the host then has to be cleaned manually. The controller's own Job has the same
58-character limit, so very long node names are an upstream limitation.

## R - RevertFailed (state REVERT_FAILED)

The node is cordoned, `is-edge-worker` is still `true`, there is no pool label, and the
failed revert Job is present. Flow B does not apply. Removing labels again changes
nothing, because they are already gone.

1. Diagnose and fix the cause, as in B.1.
2. Confirm the failed Job is finished and has no active pods.
3. `"$S/wait-node-conversion.sh" baseline --context <CTX> <node>` and keep `BASELINE`.
4. `kubectl --context <CTX> -n kube-system delete job node-servant-conversion-<node>`.
   With no label and `is-edge-worker=true`, the controller starts a new revert round.
5. `"$S/wait-node-conversion.sh" wait --context <CTX> <node> --action revert --baseline <BASELINE>`.
6. Stop after **2** retries with the same cause. Tell the user that the node stays
   cordoned, that the controller retries the revert by itself after the failed Job
   expires (about 2h), and that uninstall stays blocked until the node is `REVERTED`.

## Other states from `--summary`

| State | Meaning | What to do |
|---|---|---|
| `CONVERTING` / `REVERTING` / `CLEANING` | a Job is running | wait; do not delete it (D) |
| `CONVERT_NOT_STARTED` | pool label set, no round | troubleshooting.md "no conversion Job" |
| `REVERT_NOT_STARTED` | revert requested, no round | check yurt-manager is running and the controller is enabled |
| `DESIRED_ONLY` | `desired-nodepool` set without `nodepool` (webhook was down) | fix the webhook, then decide with the user: convert (any node update applies the default) or remove the label |
| `CLEANUP_PENDING` | conversion stopped, host not cleaned, node cordoned | continue B.4 |
| `CLEANUP_UNVERIFIED` | conversion stopped, node schedulable, `ConvertFailed` still recorded | run `preflight-node.sh`; if all `ARTIFACT` lines are `absent`, treat the node as clean |
| `EDGE_WITHOUT_RECORD` | edge node with no conversion record (e.g. joined by yurtadm) | outside this skill; removing its labels would start a revert Job |

## D - v1.7.0 known issue

Do not delete a **running or succeeded** `node-servant-conversion-<node>` Job. In
v1.7.0, deleting a Job before the controller has finalized it can make conversion
loop (fixed on master by #2645/#2706). Deleting a **failed** Job, as in B and R, is safe.
