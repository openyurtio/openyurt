# Uninstall OpenYurt v1.7.0

`<CTX>` is the locked context and `$S` is `${CLAUDE_SKILL_DIR}/scripts`. Every change
needs a change card and an explicit "yes". The order matters: the conversion
controller performs the host-level revert, so it must stay installed until
every node is reverted.

## 1. Classify every node

```bash
"$S/collect-diagnostics.sh" --context <CTX> --summary
```

| State | Action before uninstall |
|---|---|
| `NONE`, `REVERTED` | nothing |
| `CONVERTED` | recovery.md flow A (leave the pool), one node at a time |
| `CONVERTING`, `REVERTING`, `CLEANING` | wait for the running Job |
| `CONVERT_FAILED`, `CLEANUP_PENDING` | recovery.md flow B (retry, then leave the pool, or abandon with cleanup) |
| `REVERT_FAILED` | recovery.md flow R |
| `CLEANUP_FAILED` | manual host work (recovery.md B.4.6); **blocks uninstall** |
| `CLEANUP_UNVERIFIED` | `preflight-node.sh`; continue only if every `ARTIFACT` is `absent` and the user confirms |
| `DESIRED_ONLY`, `CONVERT_NOT_STARTED`, `REVERT_NOT_STARTED` | resolve per recovery.md "Other states" |
| `EDGE_WITHOUT_RECORD` | not managed by this skill; ask the user how the node was joined and stop |

Re-run `--summary` until `BLOCKS_UNINSTALL=no`. The only exception is
`CLEANUP_UNVERIFIED` nodes that preflight showed to be clean and that the user
explicitly accepted. **Do not continue while any node is `REVERT_FAILED`,
`CONVERT_FAILED`, `CLEANUP_PENDING` or `CLEANUP_FAILED`.**

## 2. Remove OpenYurt objects

1. Delete the NodePools. They must have no nodes left:

   ```bash
   kubectl --context <CTX> get nodepools.apps.openyurt.io
   kubectl --context <CTX> delete nodepools.apps.openyurt.io <pool> [...]
   ```

2. Delete the shared yurthub resources with the **same** v1.7.0 manifest, rendered for
   kube-system. If you no longer have it, re-create it with
   `"$S/prepare-charts.sh" --dest <empty-dir>`:

   ```bash
   kubectl --context <CTX> delete -f <YURTHUB_CFG_MANIFEST> --ignore-not-found
   ```

3. Uninstall the release:

   ```bash
   helm --kube-context <CTX> --namespace kube-system uninstall yurt-manager
   kubectl --context <CTX> -n kube-system get jobs | grep node-servant-conversion || true
   ```

   Leftover finished conversion Jobs expire on their own after 2h. You may delete them now.

4. CRDs are optional and need a separate confirmation. Helm does not delete CRDs
   installed from `crds/`. Deleting them removes **all** OpenYurt custom resources
   in the cluster:

   ```bash
   kubectl --context <CTX> get crd -o name | grep -E '\.openyurt\.io$'
   kubectl --context <CTX> delete crd <name> [...]
   ```

5. Webhook configurations: yurt-manager registers
   `yurt-manager-mutating-webhook-configuration` and
   `yurt-manager-validating-webhook-configuration` at runtime. If they are still present
   after the uninstall, delete them (separate confirmation). Their failure policies differ:
   - Node, Pod and Endpoints webhooks use `failurePolicy: Ignore`.
   - The NodePool, YurtAppSet, YurtStaticSet, Gateway and PlatformAdmin webhooks use
     `failurePolicy: Fail`. While the configuration points at the missing service, every
     write to those resources fails, including deletion of leftover NodePools. This is
     why NodePools are deleted in step 2.1, **before** the release is uninstalled.

   ```bash
   kubectl --context <CTX> get mutatingwebhookconfiguration,validatingwebhookconfiguration | grep yurt-manager || true
   ```

## 3. Wrong order: yurt-manager already removed while nodes are still converted

Without the controller, removing labels does nothing on the host, and converted
nodes keep sending kubelet traffic through YurtHub.

1. Reinstall the same version: flow.md Step 3 (3a + 3b), with `--set nodeServant.image.tag=v1.7.0`.
   Recreate each pool the converted nodes still reference, with the same name and `type: Edge`.
   Otherwise the defaulting webhook rejects every update of those nodes.
2. Run `check-install.sh --probe-node <node>` and require `RESULT=OK`. Then revert each
   node with recovery.md flow A.
3. Restart this uninstall document from step 1.
