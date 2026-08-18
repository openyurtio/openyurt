---
name: openyurt-raven-topology
description: "Troubleshoots OpenYurt Raven L3/L7 cross-region network connectivity and gateway routing."
---

# Raven Topology Troubleshooting Skill

This skill allows the AI to autonomously debug cross-region edge network proxying issues by querying the Raven components.

## Context
In OpenYurt, edge nodes are often distributed across different physical regions (NodePools) with distinct NATs. The `raven-agent` provides non-intrusive L3 VPN and L7 proxy capabilities. 
Each NodePool elects one `Gateway` node to handle cross-region traffic.

## Troubleshooting Steps

1. **Verify Gateway Election**
   Execute `kubectl get gateways.raven.openyurt.io` to ensure every active NodePool has exactly one elected Gateway.
   If a NodePool has no Gateway, check the `raven-controller-manager` logs.

2. **Verify VPN Tunnels (L3)**
   On the Gateway node, check the IPSec/WireGuard tunnel status:
   `kubectl get pods -n kube-system -l app=raven-agent-ds`
   Check the logs of the `raven-agent` running on the Gateway to verify VPN handshakes are established with other Gateways.

3. **Verify Proxy Endpoints (L7)**
   If standard service traffic (e.g., NodePort or ClusterIP) fails across edge nodes in different regions, check the Raven proxy backend endpoints:
   `kubectl get endpoints <service-name> -n <namespace>`
   Ensure that cross-region endpoints are correctly intercepted and rewritten by the Raven L7 proxy logic.

## Common Hallucinations to Avoid
- Do NOT assume standard `kube-proxy` rules apply uniformly across NodePools. Raven intercepts cross-node traffic dynamically.
- Do NOT tell the user to restart `yurt-tunnel`. `yurt-tunnel` is deprecated and replaced by Raven!
- Do NOT use `kubectl port-forward` to test cross-region connectivity. It bypasses the Raven routing tables.
