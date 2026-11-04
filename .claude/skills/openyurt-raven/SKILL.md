---
name: openyurt-raven
description: >
  Configure Raven for cross-region edge networking in OpenYurt. Installs
  raven-agent, creates Gateway CRs with L3 tunnel and L7 proxy support,
  and verifies cross-NodePool connectivity end-to-end.
when_to_use: >
  Use when the user wants to configure VPNs, cross-NodePool networking,
  or L3/L7 tunnels via Raven. Also use when diagnosing cross-region
  connectivity failures between edge NodePools.
allowed-tools: >
  Bash(kubectl *)
  Bash(helm *)
  Bash(openssl *)
  Read
  Grep
disable-model-invocation: true
---

# OpenYurt Raven Skill

This skill guides you through deploying Raven for cross-NodePool networking
in OpenYurt.

## Prerequisites

OpenYurt must be deployed first. If `yurt-manager` is not installed, inform
the user they must run `/openyurt-deploy` first.

## Key Concepts

- **Gateway CR**: A cluster-scoped resource (`raven.openyurt.io/v1beta1`)
  that defines a network endpoint for a group of nodes. Each NodePool
  typically has one Gateway.

- **L3 Tunnel**: IPsec-based VPN tunnel for pod-to-pod traffic across
  NodePools. Requires `libreswan` or a compatible VPN backend.

- **L7 Proxy**: HTTP/gRPC proxy for cross-NodePool service access without
  full tunnel overhead.

- **Fine-grained configuration**: Gateway-level annotations
  (`raven.openyurt.io/enable-proxy`, `raven.openyurt.io/enable-tunnel`)
  can override the cluster-wide `raven-global-config` ConfigMap, allowing
  different NodePools to have different tunnel/proxy configurations.

- **`SetDefaultsGateway` mutating webhook**: The `v1beta1.SetDefaultsGateway`
  webhook unconditionally sets `spec.nodeSelector` to
  `raven.openyurt.io/gateway: <gateway-name>`. Using `nodeSelectorTerms`
  for `apps.openyurt.io/nodepool` will be silently overwritten by the
  webhook. Always use the `LabelSelector` shape with
  `raven.openyurt.io/gateway` to match the defaulting behavior.

## Reference Documents

| Document | Purpose |
|----------|---------|
| `references/openyurt-raven-flow.md` | Step-by-step Raven configuration |
| `references/openyurt-raven-troubleshooting.md` | Troubleshooting Raven connectivity |

## Rules

- Always generate the VPN PSK using `openssl rand -hex 64`. Do not use
  weaker entropy sources.
- Label nodes with `raven.openyurt.io/gateway=<gw-name>` **before** creating
  the Gateway CR, so the nodeSelector can match immediately.
- Do not rely on `nodeSelectorTerms` with `apps.openyurt.io/nodepool` — the
  mutating webhook will overwrite it (see Key Concepts above).
- Confirm with the user before deploying the raven-agent DaemonSet, as it
  runs privileged pods on all nodes.
