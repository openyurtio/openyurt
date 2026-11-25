# Raven Troubleshooting

Decision tree for diagnosing Raven L3 tunnel and L7 proxy failures. Pass
`--context <CTX>` on every `kubectl` command.

---

## 1. `raven-agent-ds` Not Running

```bash
kubectl --context <CTX> -n kube-system get daemonset raven-agent-ds
kubectl --context <CTX> -n kube-system get pods -l app=raven-agent -o wide
```

- **DaemonSet missing:** Install the `raven-agent` Helm chart in `kube-system`.
- **Pods not Ready:** Inspect logs:
  ```bash
  kubectl --context <CTX> -n kube-system logs -l app=raven-agent --tail=50
  ```
  - **IPsec / `libreswan` errors:** Ensure `libreswan` is installed on nodes when
    `vpn.driver=libreswan` (run the read-only `check-ipsec` Job from
    `openyurt-raven-flow.md`), or switch `vpn.driver` to `wireguard` where kernel
    support is available.

---

## 2. `Gateway` CR Not Electing Active Endpoints

```bash
kubectl --context <CTX> get gateway <gw-name> -o yaml
```

Check `status.activeEndpoints` and `status.nodes`. The `gatewaypickup`
controller inside `yurt-manager` manages election
(`pkg/yurtmanager/controller/raven/gatewaypickup/gateway_pickup_controller.go`):

- **Global enablement disabled in `kube-system/raven-cfg`:**
  `util.CheckServer` skips election for `proxy` / `tunnel` when
  `enable-l7-proxy` / `enable-l3-tunnel` are not `"true"`:
  ```bash
  kubectl --context <CTX> -n kube-system get configmap raven-cfg -o yaml
  ```
- **Nodes not matched:** Confirm nodes carry `raven.openyurt.io/gateway=<gw-name>`
  and match `spec.nodeSelector`, and are `Ready` (`NodeReady=True`):
  ```bash
  kubectl --context <CTX> get nodes -l raven.openyurt.io/gateway=<gw-name>
  ```
- **Missing or invalid `spec.endpoints`:**
  `electEndpoints` only selects from `Ready` nodes listed in `spec.endpoints`
  whose `type` (`tunnel` or `proxy`) matches an enabled feature.
  Check `yurt-manager` logs for webhook or reconciliation errors:
  ```bash
  kubectl --context <CTX> -n kube-system logs -l app.kubernetes.io/name=yurt-manager --tail=100
  ```
- **Webhook validation rejections (`gateway_validation.go`):**
  - `exposeType` is set (`PublicIP` or `LoadBalancer`) while an endpoint has
    `underNAT: true` (must be `false`).
  - `spec.tunnelConfig.Replicas > 1`.
  - `spec.proxyConfig.Replicas` exceeds the count of `type: proxy` endpoints.

---

## 3. VPN PSK Mismatch

`raven-agent-ds` reads `VPN_CONNECTION_PSK` from `Secret/raven-agent-secret` key
`vpn-connection-psk` in `kube-system`. Do not print raw secret contents; compare
checksums if needed:

```bash
kubectl --context <CTX> -n kube-system get secret raven-agent-secret \
  -o jsonpath='{.data.vpn-connection-psk}' | base64 -d | sha256sum
```

If rotated or reconfigured via Helm `--set vpn.psk=...`, restart
`daemonset/raven-agent-ds` (requires user confirmation):

```bash
kubectl --context <CTX> -n kube-system rollout restart daemonset/raven-agent-ds
```

---

## 4. Cross-NodePool L3 Traffic Drops

- **Ports & protocols:** IPsec requires UDP `4500` (and `500` / ESP protocol `50`
  depending on encapsulation) open on gateway public IPs; WireGuard uses UDP `4500`.
- **Calico CNI conflict:** On Calico clusters, verify `FelixConfiguration` sets
  `iptablesMangleAllowAction: Return`.
- **Additional CIDRs:** If non-pod node or extra CIDR routing is needed, check
  `spec` / `raven.openyurt.io/extra-allowed-source-cidrs` on the `Gateway` CR.

---

## 5. L7 Proxy (`enable-l7-proxy`) Not Routing Control-Plane / Service Calls

The `yurt-manager` Raven controllers manage `kube-system/x-raven-proxy-internal-svc`
and the DNS ConfigMap `kube-system/edge-tunnel-nodes` (`tunnel-nodes` key, used
by `raven-proxy-dns` CoreDNS):

```bash
kubectl --context <CTX> -n kube-system get svc -l app.kubernetes.io/managed-by=raven-gateway-internal-service-controller || \
kubectl --context <CTX> -n kube-system get svc x-raven-proxy-internal-svc
kubectl --context <CTX> -n kube-system get endpoints x-raven-proxy-internal-svc
kubectl --context <CTX> -n kube-system get configmap edge-tunnel-nodes -o yaml
```

If `x-raven-proxy-internal-svc` has no endpoints, verify `enable-l7-proxy: "true"`
in `kube-system/raven-cfg` and confirm at least one active endpoint has
`type: proxy`.
