---
name: openyurt-service-topology
description: Configure and troubleshoot OpenYurt service topology, so that Service traffic stays within a node or a NodePool.
when_to_use: Use when the user wants to keep Service traffic inside a node or NodePool, is configuring the openyurt.io/topologyKeys annotation, or is debugging why edge traffic is still crossing NodePool boundaries.
disable-model-invocation: true
allowed-tools: >
  Bash(kubectl get *)
  Bash(kubectl describe *)
  Bash(kubectl logs *)
  Bash(kubectl annotate service *)
  Read
  Grep
---

# OpenYurt Service Topology

Service topology restricts which endpoints a client sees, so traffic from an edge
node is served by a backend on the same node or in the same NodePool instead of
crossing the network back to the cloud.

It is enforced **on the edge node, inside yurthub**, not by the API server. yurthub
intercepts `list`/`watch` responses for `endpointslices` and removes the endpoints
outside the requested topology (`pkg/yurthub/filter/servicetopology/filter.go`).

Two things follow, and most confusion traces to one of them:

- **Stored objects are never modified.** `kubectl get endpointslice` from your laptop
  always shows the **unfiltered** list. Only a request from a filtered component, on
  the node, sees the reduced set. Never conclude topology works from a workstation.
- **The feature has a cloud half too.** Changing the annotation mutates the *Service*,
  which by itself generates no EndpointSlice watch event. `yurt-manager` patches the
  EndpointSlice to force one, so the filter re-runs. If that controller is not
  running, annotation changes are silently inert.

## Configuration model

Topology is opt-in per Service, via an annotation on the **Service** (not the
EndpointSlice):

| Annotation value | Traffic is kept within |
| --- | --- |
| `kubernetes.io/hostname` | the same node |
| `openyurt.io/nodepool` | the same NodePool |
| `kubernetes.io/zone` | the same NodePool (alias of the above) |

```bash
kubectl annotate service <svc> openyurt.io/topologyKeys=openyurt.io/nodepool
```

Any other value, or no annotation, means the Service is left untouched.

## How to use this skill

1. Ask the user whether they want to **configure** topology or **diagnose** why it
   is not taking effect. If they are unsure, diagnose first — misconfiguration is
   far more common than a missing annotation.
2. For configuration, follow `references/service-topology-configure.md`.
3. For diagnosis, follow `references/service-topology-diagnose.md`. Work through the
   checks in order and stop at the first one that fails; each check narrows the
   cause and they are ordered cheapest-first.
4. Consult `references/service-topology-internals.md` when you need to explain a
   result, or when the behaviour does not match the decision tree.

## Rules

- **Never** conclude that topology works by reading EndpointSlices with your own
  credentials. That path is unfiltered by design. Verify from a filtered component
  on the node, as described in the diagnose reference.
- Treat an empty endpoint list as a legitimate outcome, not a bug. The filter
  deliberately returns an empty slice rather than falling back to all endpoints
  (`filter.go:242`, `filter.go:269`), so a NodePool with no local backend correctly
  serves nothing.
- Do not edit node labels to force a result. `openyurt.io/is-edge-worker` and the
  NodePool label are controller-owned; hand-editing them fights the controller.
- Prefer read-only commands. The only mutation this skill performs is annotating a
  Service, and you should confirm with the user before doing so.
