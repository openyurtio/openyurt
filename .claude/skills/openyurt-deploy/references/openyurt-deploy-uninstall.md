# Uninstall / Rollback

## Revert a single node

Removing the NodePool label is the supported rollback path — it triggers the same controller, running the
Job with the `revert` subcommand instead of `convert`:

```bash
kubectl label node <node-name> apps.openyurt.io/nodepool-
```

This cordons the node, sets `YurtNodeConversionFailed` to `reason=Reverting`, and runs a Job that:

1. Restores kubelet's traffic to point directly at the real API server (done first, to minimize
   disruption).
2. Stops and disables the `yurthub.service` systemd unit (idempotent — a missing or already-stopped
   service is not an error).
3. Removes the systemd unit file and drop-in directory, then reloads systemd.
4. Removes the `yurthub` binary and its bootstrap config, plus its data/cache directory.
5. Restarts non-pause containers on the node so they pick up the restored API server address.

On success, the controller removes `openyurt.io/is-edge-worker`, uncordons the node, and sets the
condition reason to `Reverted`. Confirm:

```bash
kubectl get node <node-name> -o jsonpath='{.status.conditions[?(@.type=="YurtNodeConversionFailed")]}{"\n"}'
kubectl get node <node-name> -o jsonpath='{.metadata.labels.openyurt\.io/is-edge-worker}{"\n"}'
```

The second command should print nothing once revert succeeds. If revert fails
(`reason=RevertFailed`), the node is uncordoned but still partially converted — check the Job logs the same
way as in `openyurt-deploy-troubleshooting.md` before retrying with the same label-remove command (it's
idempotent, safe to repeat).

Same caveat as conversion applies in reverse: the container restart in step 5 will restart every non-pause
container on the node, so confirm with the user before reverting a node carrying production traffic.

## Tear down a NodePool

Only delete a `NodePool` after every member node has been reverted (no node should still carry that pool's
label) — deleting it out from under converted nodes doesn't revert them, it just orphans the reference.

```bash
kubectl get nodes -l apps.openyurt.io/nodepool=<pool-name>
# should return no nodes before proceeding
kubectl delete nodepool <pool-name>
```

## Remove yurt-manager

Only after all NodePools in the cluster are gone or you're decommissioning OpenYurt entirely:

```bash
helm uninstall yurt-manager -n kube-system
```

This does not touch already-reverted nodes — they're back to being plain Kubernetes nodes at that point,
independent of whether `yurt-manager` is still installed.
