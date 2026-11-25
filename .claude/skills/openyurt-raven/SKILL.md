---
name: openyurt-raven
description: Configure Raven cross-NodePool networking in OpenYurt by deploying raven-agent, creating Gateway (raven.openyurt.io/v1beta1) CRs with L3 tunnel and L7 proxy support, and verifying cross-pool connectivity.
when_to_use: Use when the user wants to configure Raven VPN tunnels, cross-NodePool networking, L3/L7 proxy gateways, or troubleshoot cross-region connectivity between edge NodePools.
argument-hint: "[gateway ...]"
disable-model-invocation: true
allowed-tools: Read Grep Bash(helm version) Bash(kubectl --context * get *) Bash(kubectl --context * logs *)
---

# OpenYurt Raven Skill

Configures Raven for cross-NodePool L3 VPN tunneling and L7 proxying in
OpenYurt. Gateway control-plane reconciliation runs inside `yurt-manager`
(`pkg/yurtmanager/controller/raven/`), and the node data-plane DaemonSet
(`raven-agent-ds`) is installed from `openyurtio/raven`.

## Prerequisites

- OpenYurt (`yurt-manager` in `kube-system`) must already be deployed; otherwise run
  `/openyurt-deploy` first.
- Requirements on the operator workstation: `kubectl`, `helm` (v3 or v4), `openssl`.
- Everything is executed through the Kubernetes API and Helm; no SSH.

## Rules

**Target cluster.** Before any cluster command, show `kubectl config get-contexts`
and ask the user to confirm the target context `<CTX>`. Pass `--context <CTX>` to
every `kubectl` command and `--kube-context <CTX> --namespace kube-system` to
every `helm` command.

**Confirmation.** Before every change to the cluster (`helm upgrade --install`,
`kubectl apply/create/delete/label`, ephemeral check Jobs or test DaemonSets),
show a change card and wait for an explicit "yes":

```text
Context:  <CTX>
Target:   <release / node / resource>
Action:   <exact command>
Impact:   <summary of effect>
Rollback: <how to undo>
```

**PSK hygiene.** Always generate the VPN PSK with `openssl rand -hex 64` and
never echo or print the raw PSK value to terminal output or logs.

## Key Facts (OpenYurt source & `openyurtio/raven`)

- **Gateway CR (`raven.openyurt.io/v1beta1`, shortName `gw`):** Defined in
  `pkg/apis/raven/v1beta1/gateway_types.go`.
  - `spec.nodeSelector` uses `metav1.LabelSelector` (`matchLabels` / `matchExpressions`).
    The `SetDefaultsGateway` mutating webhook (`pkg/apis/raven/v1beta1/default.go`)
    merges `raven.openyurt.io/gateway: <gateway-name>` into
    `spec.nodeSelector.matchLabels` without overwriting other `matchLabels`, and
    defaults unset endpoint ports to `10262` (`proxy`) or `4500` (`tunnel`).
  - `spec.exposeType` must be `PublicIP` or `LoadBalancer` when set, and exposed
    gateways require `endpoints[*].underNAT: false`
    (`pkg/yurtmanager/webhook/gateway/v1beta1/gateway_validation.go`).
  - `spec.tunnelConfig.Replicas` cannot exceed `1`, and `spec.proxyConfig.Replicas`
    cannot exceed the count of `type: proxy` endpoints (typically `1`). Note the
    capital `R` in JSON/YAML (`Replicas`).
- **Global enablement (`kube-system/raven-cfg`):** The Raven controllers read the
  `raven-cfg` ConfigMap in `kube-system` for `enable-l3-tunnel` and
  `enable-l7-proxy` (`pkg/yurtmanager/controller/raven/util/constants.go` and
  `util.go`). These keys are rendered by the `raven-agent` Helm chart from
  `controller.enableTunnel` and `controller.enableProxy`.
- **Raven Agent chart resources (`openyurtio/raven/charts/raven-agent`):**
  Deploys `DaemonSet/raven-agent-ds` (pods labeled `app=raven-agent`),
  `ConfigMap/raven-cfg`, `ConfigMap/raven-agent-config`, and
  `Secret/raven-agent-secret` (key `vpn-connection-psk`, populated via
  `vpn.psk`) in `kube-system`. Notice the DaemonSet runs privileged hostNetwork
  pods across Linux nodes; always confirm with the user before installing.

## Reference Documents

| Document | Purpose |
|---|---|
| `references/openyurt-raven-flow.md` | Step-by-step Raven deployment, Gateway creation, and L3 verification |
| `references/openyurt-raven-troubleshooting.md` | Decision tree for `raven-agent-ds`, `Gateway` election, PSK, and L3/L7 routing |
