---
name: yurthub-autonomy
description: Diagnose OpenYurt edge node autonomy annotations, PodBindingController tolerations, and YurtHub offline cache state during cloud-edge disconnections.
when_to_use: Use when diagnosing unexpected pod evictions on disconnected edge nodes, autonomy-duration settings, or YurtHub local cache behavior.
disable-model-invocation: true
allowed-tools: Read Grep Bash(kubectl --context * get *) Bash(kubectl --context * describe *) Bash(kubectl --context * logs *)
---

# YurtHub Edge Autonomy Troubleshooting Skill

Guides autonomous read-only inspection of OpenYurt edge node autonomy settings
and pod eviction behavior during cloud-edge network partitions. Ask the user to
confirm the target kube context `<CTX>` before running commands, and pass
`--context <CTX>` to every `kubectl` call.

## Source Contracts

- `pkg/yurtmanager/controller/yurtcoordinator/podbinding/pod_binding_controller.go`:
  `resolveNodeAutonomySetting` reads:
  - `node.openyurt.io/autonomy-duration` (recommended, Go duration e.g. `"10m"`,
    or `"0"` / `"0s"` for infinite autonomy; sets `tolerationSeconds` on pods for
    `node.kubernetes.io/not-ready` and `node.kubernetes.io/unreachable`, preserving
    original values in `apps.openyurt.io/original-*-toleration-duration`).
  - `node.beta.openyurt.io/autonomy: "true"` and `apps.openyurt.io/binding: "true"`
    (deprecated legacy annotations; still honored at higher precedence when `"true"`).
- `pkg/yurthub/proxy/autonomy/autonomy.go`:
  The `Autonomy` node condition only reflects the deprecated
  `node.beta.openyurt.io/autonomy` annotation; do NOT expect `Autonomy=True`
  when only `node.openyurt.io/autonomy-duration` is set.
- `pkg/yurthub/storage/disk/storage.go` & `pkg/yurtadm/constants/constants.go`:
  YurtHub stores cached objects on disk under `/etc/kubernetes/cache/` and its
  working/error-key state under `/var/lib/yurthub`.

## Troubleshooting Steps

1. **Verify Edge Status and Autonomy Annotations**
   ```bash
   kubectl --context <CTX> get node <node-name> -o jsonpath='{.metadata.labels.openyurt\.io/is-edge-worker}{"\n"}{.metadata.annotations}{"\n"}'
   ```
   Note: `PodBindingController` skips nodes that are not edge nodes. Verify
   `node.openyurt.io/autonomy-duration` is a valid Go duration (e.g. `10m` or `0s`).

2. **Inspect Pod Eviction Tolerations on the Edge Node**
   ```bash
   kubectl --context <CTX> get pods -A --field-selector spec.nodeName=<node-name> \
     -o jsonpath='{range .items[*]}{.metadata.namespace}/{.metadata.name}{" not-ready="}{.spec.tolerations[?(@.key=="node.kubernetes.io/not-ready")].tolerationSeconds}{" unreachable="}{.spec.tolerations[?(@.key=="node.kubernetes.io/unreachable")].tolerationSeconds}{"\n"}{end}'
   ```
   If `tolerationSeconds` was not updated or pods are still being evicted, inspect
   `yurt-manager` logs for `PodBindingController` duration parsing or webhook
   errors (requires PodBinding feature gate / controller enabled in `yurt-manager`):
   ```bash
   kubectl --context <CTX> -n kube-system logs -l app.kubernetes.io/name=yurt-manager --tail=100 | grep -i -E "autonomy|podbinding"
   ```

3. **Check Edge-Node YurtHub Health and Events**
   Check `NodeLease` updates, `NodeReady` status transitions, and YurtHub
   configuration in `kube-system`:
   ```bash
   kubectl --context <CTX> get lease -n kube-node-lease <node-name> -o yaml
   kubectl --context <CTX> -n kube-system get configmap yurt-hub-cfg -o yaml
   ```

## Common Pitfalls to Avoid

- Do NOT look for `yurt-controller-manager`; `PodBindingController` runs inside
  `yurt-manager`.
- Do NOT use the legacy `node.beta.alibabacloud.com/autonomy` prefix or advise
  tweaking `kube-controller-manager`'s `--pod-eviction-timeout` as the fix.
- Do NOT treat `Autonomy=False` node condition as a failure when using
  `node.openyurt.io/autonomy-duration`; verify pod `tolerationSeconds` instead.
- Do NOT interpret `NotReady` alone during a cloud-edge partition as workload
  failure; workloads keep running autonomously on the edge node while YurtHub
  serves cached API state.
