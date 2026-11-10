---
name: openyurt-deploy
description: Deploy OpenYurt v1.7.0 on an existing kubeadm-based Kubernetes cluster and convert worker nodes into edge nodes with the label-driven YurtNodeConversionController; also reverts nodes, recovers failed conversions and uninstalls.
when_to_use: Use when the user asks to install yurt-manager, create an Edge NodePool, convert or revert a worker node, enable edge node autonomy, recover a node stuck in ConvertFailed/RevertFailed, or uninstall OpenYurt v1.7.0. Example requests - "deploy OpenYurt on this cluster", "turn worker-1 into an edge node", "why is my node stuck converting", "remove OpenYurt".
argument-hint: "[node-name ...]"
disable-model-invocation: true
allowed-tools: Read Grep Bash(helm version) Bash(${CLAUDE_SKILL_DIR}/scripts/check-install.sh *) Bash(${CLAUDE_SKILL_DIR}/scripts/wait-node-conversion.sh *) Bash(${CLAUDE_SKILL_DIR}/scripts/collect-diagnostics.sh *)
---

# OpenYurt deploy (v1.7.0)

Deploys OpenYurt **v1.7.0** on a running cluster and converts existing worker
nodes into edge nodes. Conversion is label-driven: adding
`apps.openyurt.io/nodepool=<edge-pool>` to a node makes yurt-manager run a
privileged `node-servant-conversion-<node>` Job on that node, which installs
YurtHub as a systemd service, redirects kubelet through it and restarts the
node's containers. Everything here is done through the API server; no SSH.

Node candidates passed by the user: `$ARGUMENTS`

Current kube context (for display only, never used implicitly):
!`kubectl config current-context 2>/dev/null || echo "<none>"`

## Supported scope

Stop and tell the user if any of these does not hold; the scripts check them.

- OpenYurt **v1.7.0** only, Helm release `yurt-manager` in namespace **`kube-system`**.
  Charts must come from the `v1.7.0` tag (the published openyurt-helm repo has no 1.7.0 chart).
- Linux nodes managed by **systemd**, **kubeadm** kubelet layout
  (`10-kubeadm.conf` with `--kubeconfig=...kubelet.conf`, `/var/lib/kubelet/kubeadm-flags.env`).
- CRI **containerd or CRI-O**, `crictl` on the host `PATH`. Not k3s, not dockershim.
- Kubernetes >= 1.24, Helm v3 or v4.

## How to run

1. Follow [references/openyurt-deploy-flow.md](references/openyurt-deploy-flow.md) step by step.
2. When a script reports a failure, go to
   [references/openyurt-deploy-troubleshooting.md](references/openyurt-deploy-troubleshooting.md).
3. For failed rounds, leaving/changing pools and host cleanup, use
   [references/openyurt-deploy-recovery.md](references/openyurt-deploy-recovery.md).
4. To remove OpenYurt, use [references/openyurt-deploy-uninstall.md](references/openyurt-deploy-uninstall.md).

Scripts live in `${CLAUDE_SKILL_DIR}/scripts/`. Each prints `KEY=VALUE` lines and
ends with `RESULT=<value>`; act on `RESULT` and the exit code, not on prose.

| Script | Changes cluster? | Purpose |
|---|---|---|
| `prepare-charts.sh --dest <dir> [--repo <clone>]` | no (local files) | extract v1.7.0 charts, render `yurthub-cfg.yaml` for kube-system |
| `preflight-node.sh --context <ctx> <node>` | creates/deletes a read-only Job | host requirements + leftover OpenYurt artifacts |
| `check-install.sh --context <ctx> [--probe-node <n> [--probe-pool <p>]] [--require-autonomy]` | no (dry-run only) | reuse decision for an existing install; Node webhook probe |
| `wait-node-conversion.sh baseline\|wait ...` | no | identify and wait for the current convert/revert round |
| `cleanup-node.sh --context <ctx> <node>` | **yes, host-level** | out-of-band host revert for an abandoned conversion |
| `collect-diagnostics.sh --context <ctx> <node>\|--summary` | no | diagnostics; `--summary` classifies every node |

## Rules

**Target cluster.** Before any other step, show the current context and ask the
user to confirm the target. Record it as `<CTX>`. From then on, every `kubectl`
call carries `--context <CTX>`, every `helm` call carries
`--kube-context <CTX> --namespace kube-system`, and every script gets
`--context <CTX>`. Never run a cluster command without the explicit context,
even if the default context still looks right; it may change mid-session.

**Confirmation.** Before every change to the cluster (helm install/upgrade/uninstall,
`kubectl apply/create/delete/label/annotate/cordon/uncordon`, `preflight-node.sh`,
`cleanup-node.sh`) show a change card and wait for an explicit "yes":

```
Context:  <CTX>
Target:   <node / release / object>
Action:   <exact command>
Impact:   <e.g. node is cordoned; every non-pause container on it is stopped and
          restarted per its restartPolicy (restartPolicy: Never containers do not come back)>
Rollback: <how to undo, or "see recovery.md">
```

This is a process requirement. `allowed-tools` only pre-approves the read-only
scripts above; depending on the user's permission settings other commands may or
may not prompt, so never rely on a permission prompt as the confirmation.

**Node operations.**

- Run `preflight-node.sh` on each node before converting it; do not convert on `RESULT=FAIL`.
- Before every node label change, run `check-install.sh --probe-node <that node>`
  and require `RESULT=OK`. The Node webhooks use `failurePolicy: Ignore`; when they
  are not working, pool-label protection and `desired-nodepool` defaulting silently stop.
- Convert, revert or recover one node at a time.
- Run `wait-node-conversion.sh baseline` before the label change and pass its
  `BASELINE` to `wait`.
- Never set or remove `openyurt.io/is-edge-worker` yourself; the webhook rejects it
  and it is owned by the conversion controller.
- Leaving a pool: remove `apps.openyurt.io/nodepool` **and** `apps.openyurt.io/desired-nodepool`
  in the same `kubectl label` command, then re-read the node.
- Changing pools: leave, wait for `Reverted`, then join the new pool. Never remove
  and re-add the label back to back.
- Never delete a running or succeeded `node-servant-conversion-<node>` Job
  (v1.7.0 can loop, see recovery.md D).

**Reporting.** When you stop, state for each touched node its state from
`collect-diagnostics.sh --summary`, and whether conversion is **stopped** or the
controller will **retry automatically** (failed Jobs are deleted after 2h and the
round restarts while the labels still ask for it).

**Uninstall** is blocked while any node is not `NONE` or `REVERTED`.

## Key facts (v1.7.0 source)

- Completion: `isConvertedStable` = nodepool label set, `openyurt.io/is-edge-worker=true`,
  node schedulable, condition `YurtNodeConversionFailed` = `False/Converted`.
  Revert mirrors it with `Reverted`. Failure reasons are `ConvertFailed` / `RevertFailed` (status `True`).
- The `Autonomy` node condition only reflects the deprecated `node.beta.openyurt.io/autonomy`
  annotation; do not use it to verify `node.openyurt.io/autonomy-duration`.
- A NodePool that is not `type: Edge` makes the controller skip conversion silently.
- v1.7.0 does not set `topology.kubernetes.io/zone` and has no conversion Job finalizer;
  both arrived later on master. Do not expect them.
