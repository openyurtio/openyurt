---
description: Deploy OpenYurt onto an existing Kubernetes cluster and convert worker nodes into edge nodes.
when_to_use: Use when the user wants to install OpenYurt on a running cluster, create a NodePool, convert existing worker nodes into edge nodes, or diagnose/undo a failed node conversion.
disable-model-invocation: true
allowed-tools: >
  Bash(kubectl *)
  Bash(helm *)
  Read
  Grep
---

# OpenYurt Deploy

Guides the end-to-end setup of OpenYurt on a cluster that already has a working control plane: installing
`yurt-manager`, creating a `NodePool`, and converting existing worker nodes into edge nodes via the
label-driven `YurtNodeConversionController`. No SSH or node-side shell access is used — every step is a
`kubectl` or `helm` command against the API server. The actual privileged work (installing YurtHub,
redirecting kubelet traffic) happens inside a Job that the controller schedules on the target node, not
from this session.

`disable-model-invocation` is set because this skill cordons nodes, restarts non-pause containers, and
changes NodePool membership — it should only run when a user explicitly asks for it, never picked up from
ambient conversation.

## Flow

1. Read `references/openyurt-deploy-flow.md` and follow it step by step: preflight, `yurt-manager`
   install/verification, `NodePool` creation, then labeling nodes to trigger conversion.
2. If a node's `YurtNodeConversionFailed` condition reports `ConvertFailed`, or the conversion Job doesn't
   reach `Converted` within a few minutes, switch to `references/openyurt-deploy-troubleshooting.md`.
3. To undo a conversion (single node or full teardown), use `references/openyurt-deploy-uninstall.md`.

## Guardrails

- Never label a node with `apps.openyurt.io/nodepool` without first telling the user the node will be
  cordoned and its non-pause containers restarted (see the flow doc's warning). Bare pods without an
  owning controller are deleted permanently, not recreated.
- Never write to the `openyurt.io/is-edge-worker` label directly — it is controller-managed and reflects
  a completed conversion, not a request for one.
- If `kubectl` isn't pointed at the cluster the user expects, stop and confirm the context
  (`kubectl config current-context`) before making any change.
