# Troubleshooting Node Conversion

Start here whenever a conversion doesn't reach `Converted`, or you need to explain a failure to the user.
Diagnosis is driven by the `YurtNodeConversionFailed` Node condition plus the conversion Job's status —
check both before touching anything.

```bash
kubectl get node <node-name> -o jsonpath='{.status.conditions[?(@.type=="YurtNodeConversionFailed")]}{"\n"}'
kubectl get job -n kube-system node-servant-conversion-<node-name> -o wide
kubectl get pods -n kube-system -l openyurt.io/conversion-node=<node-name>
kubectl logs -n kube-system job/node-servant-conversion-<node-name> --all-containers
```

## Decision tree

**Condition reason is `Converting`, Job shows no pods yet**
The Job hasn't been scheduled. Check for a taint or resource pressure keeping it pending:

```bash
kubectl describe job -n kube-system node-servant-conversion-<node-name>
```

If it's stuck in `Pending`, the node likely doesn't have room to schedule a privileged, `hostPID`/
`hostNetwork` pod, or an admission webhook is rejecting it. Check `kubectl get events -n kube-system
--field-selector involvedObject.name=node-servant-conversion-<node-name>`.

**Condition reason is `Converting`, Job pod is `CrashLoopBackOff` or `Error`**
Read the Job logs first — the two most common causes:

1. `"crictl is required for container runtime"` (or similar from `NewContainerRuntimeForImage`) — the host
   node doesn't have `crictl`/`docker` on its `PATH`. This is the preflight check in
   `openyurt-deploy-flow.md` step 1; install `cri-tools` on the node and re-trigger by removing and
   re-adding the NodePool label (see below).
2. Binary download failure for the pinned YurtHub version — the node can't reach the default OSS mirror.
   Re-run the `yurt-manager` install with `--yurthub-binary-url` pointed at a reachable mirror
   (`openyurt-deploy-flow.md` step 2), then re-trigger conversion.

**Condition reason is `ConvertFailed`**
The Job exceeded its `backoffLimit` (default 3) without succeeding — the controller has already uncordoned
the node, so there's no scheduling impact, but the node is not converted. Fix the underlying cause found
in the Job logs, then re-trigger:

```bash
kubectl label node <node-name> apps.openyurt.io/nodepool-
kubectl label node <node-name> apps.openyurt.io/nodepool=<pool-name>
```

Removing and re-adding is the supported retry path — the label value must not be edited in place, since
the controller only reacts to add/remove transitions.

**Condition never appears at all**
The controller isn't watching this node, which almost always means one of:

- `yurt-manager`'s `--controllers` flag doesn't include `yurtnodeconversion` — check
  `kubectl logs -n kube-system deploy/yurt-manager | grep controllers`.
- The `NodePool` referenced by the label doesn't exist, or its `spec.type` is `Cloud` instead of `Edge` —
  the controller silently skips non-Edge pools. Verify with `kubectl get nodepool <pool-name> -o
  jsonpath='{.spec.type}'`.

**Job succeeded but workloads on the node are still misbehaving**
Confirm containers actually restarted — the controller only restarts non-pause containers on success, so a
container started before conversion may still hold the old `KUBERNETES_SERVICE_HOST`:

```bash
kubectl get pods -A --field-selector spec.nodeName=<node-name> \
  -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.startTime}{"\n"}{end}'
```

Any pod with a start time before the Job's completion time didn't get the restart and should be deleted
manually to pick up the new proxy config.

## When to stop and ask the user

- If the node is running bare pods (no Deployment/StatefulSet/DaemonSet owner) — they were deleted
  permanently by the restart step and can't be recovered by this skill.
- If retrying conversion twice with the same failure — stop and surface the Job logs verbatim rather than
  retrying a third time blind.
