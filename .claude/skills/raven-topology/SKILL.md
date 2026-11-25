---
name: raven-topology
description: Diagnose OpenYurt Raven L3 VPN tunnels, L7 proxy endpoints, and Gateway CR election across cloud and edge NodePools.
when_to_use: Use when diagnosing cross-region pod connectivity, Gateway election failures, or Raven L7 proxy routing issues.
disable-model-invocation: true
allowed-tools: Read Grep Bash(kubectl --context * get *) Bash(kubectl --context * describe *) Bash(kubectl --context * logs *)
---

# Raven Topology Troubleshooting Skill

Guides autonomous read-only inspection of OpenYurt Raven control-plane and
data-plane state across NodePools. Ask the user to confirm the target kube
context `<CTX>` before running commands, and pass `--context <CTX>` to every
`kubectl` call.

## Troubleshooting Steps

1. **Check Global Raven Configuration (`kube-system/raven-cfg`)**
   ```bash
   kubectl --context <CTX> -n kube-system get configmap raven-cfg -o yaml
   ```
   Confirm whether `enable-l3-tunnel` and `enable-l7-proxy` are `"true"` as
   intended (`pkg/yurtmanager/controller/raven/util/constants.go`).

2. **Inspect `Gateway` CRs and Elected Active Endpoints**
   ```bash
   kubectl --context <CTX> get gateways.raven.openyurt.io -o yaml
   kubectl --context <CTX> get nodes --show-labels | grep raven.openyurt.io/gateway
   ```
   Every participating NodePool should have a `Gateway` (`raven.openyurt.io/v1beta1`)
   with populated `status.activeEndpoints` (`tunnel` / `proxy`). If `activeEndpoints`
   is empty, inspect `yurt-manager` controller logs in `kube-system` (`gatewaypickup`
   controller runs inside `yurt-manager`, not a separate controller Deployment):
   ```bash
   kubectl --context <CTX> -n kube-system logs -l app.kubernetes.io/name=yurt-manager --tail=100
   ```

3. **Verify `raven-agent-ds` Pods and Tunnel Logs**
   ```bash
   kubectl --context <CTX> -n kube-system get daemonset raven-agent-ds
   kubectl --context <CTX> -n kube-system get pods -l app=raven-agent -o wide
   kubectl --context <CTX> -n kube-system logs -l app=raven-agent --tail=50
   ```
   Verify pods are `Running`/`Ready` on all cloud and edge gateway nodes and check
   logs for IPsec (`libreswan`) or WireGuard establishment errors or PSK issues
   (`Secret/raven-agent-secret`).

4. **Verify L7 Proxy Services and DNS Mapping**
   When `enable-l7-proxy: "true"` is enabled, check the internal proxy service,
   exposed proxy/tunnel services, and DNS ConfigMap managed by `yurt-manager`:
   ```bash
   kubectl --context <CTX> -n kube-system get svc,endpoints x-raven-proxy-internal-svc
   kubectl --context <CTX> -n kube-system get svc -A | grep x-raven-
   kubectl --context <CTX> -n kube-system get configmap edge-tunnel-nodes -o yaml
   ```

## Common Pitfalls to Avoid

- Do NOT look for `raven-controller-manager`; Gateway reconciliation controllers
  (`gatewaypickup`, `gatewayinternalservice`, `gatewaypublicservice`,
  `gatewaydns`) are built into `yurt-manager`.
- Do NOT suggest checking `yurt-tunnel`; Raven replaces `yurt-tunnel` in modern
  OpenYurt deployments.
- Do NOT query `raven-agent` pods with `-l app=raven-agent-ds`; the DaemonSet is
  named `raven-agent-ds`, while pod labels use `app=raven-agent`.
- Do NOT rely on `kubectl port-forward` to test cross-pool Raven L3/L7 data paths.
