# Service topology internals

Background for explaining results and for handling cases the decision tree does not
cover. All references are to the current implementation.

---

## The two halves

Service topology is not one component. It is an edge-side filter plus a cloud-side
controller, and they fail in different ways:

| | Where | Job |
| --- | --- | --- |
| **yurthub filter** | every edge node | Removes out-of-scope endpoints from `list`/`watch` responses |
| **yurt-manager controllers** | cloud | Force a watch event when the topology annotation changes, so the filter re-runs |

Missing the second half is the usual reason a correct-looking configuration behaves as
if it were never applied. See "Propagation" below.

---

## Where enforcement happens

Enforcement is entirely on the edge node, inside yurthub, as a response filter on the
proxy path. yurthub intercepts `list`/`watch` responses for `endpoints` and
`endpointslices` and rewrites the endpoint set before handing it to the local
component (`pkg/yurthub/filter/servicetopology/filter.go`).

Two consequences follow, and most confusion traces back to one of them:

1. **Stored objects are never modified.** Any read that does not pass through
   yurthub's filtered path — notably `kubectl` from a workstation — sees the full
   endpoint set. That is by design.
2. **Enforcement is per-node.** Two nodes in different NodePools legitimately see
   different endpoint sets for the same Service at the same moment. Divergence
   between nodes is the feature working, not drift.

---

## Decision flow

`Filter` handles only EndpointSlice objects; everything else passes through
unchanged (`filter.go:131-138`). For an EndpointSlice:

1. Resolve the owning Service from the EndpointSlice's
   `kubernetes.io/service-name` label and read its `openyurt.io/topologyKeys`
   annotation (`filter.go:161-184`).
2. Empty or unreadable annotation → return unchanged (`filter.go:142-144`).
3. Dispatch on the value (`filter.go:146-158`):
   - `kubernetes.io/hostname` → node handler
   - `openyurt.io/nodepool` or `kubernetes.io/zone` → pool handler, **if** the
     `enablePoolTopology` gate is on; otherwise return unchanged
   - anything else → return unchanged

Note that steps 2 and 3 collapse three very different situations — no annotation, an
unreadable Service, and a typo'd value — into the same observable outcome: traffic is
not constrained, with no error surfaced to the user. Distinguishing them requires
yurthub's logs.

---

## Propagation: why an annotation change is visible at all

The filter runs against `list`/`watch` responses, so it only re-evaluates when the
EndpointSlice is sent again. Changing `openyurt.io/topologyKeys` mutates the
**Service**, not the EndpointSlice — on its own that produces no EndpointSlice watch
event, and every edge node would keep serving the previously filtered view
indefinitely.

`yurt-manager` bridges that gap:

1. Watch Services; compare old and new `openyurt.io/topologyKeys`
   (`pkg/yurtmanager/controller/servicetopology/util/util.go:25-29`).
2. On change, enqueue that Service's EndpointSlices
   (`.../endpointslice/endpointslice_enqueue_handlers.go:44-62`).
3. Patch `openyurt.io/update-trigger` with the current Unix timestamp
   (`.../adapter/adapter.go:53-54`, `.../adapter/endpointslicev1_adapter.go:56-69`).

The patch is a no-op semantically; its only purpose is to generate a watch event that
makes yurthub re-filter. The controllers are `servicetopologyendpointslices` and
`servicetopologyendpoints` (`cmd/yurt-manager/names/controller_names.go:51-52`).

Consequence worth internalising: **topology configuration is applied by the edge, but
delivered by the cloud.** A cluster running yurt-manager with a restricted controller
set that excludes these will accept annotation changes that never take effect, with no
error anywhere. Pod churn on the Service will eventually deliver the change by
accident, which makes the failure look intermittent rather than systematic.

---

## The pool gate

`enablePoolTopology` is resolved once at filter-initialisation time
(`pkg/yurthub/filter/initializer/nodes_initializer.go:52-80`):

```
if  --enable-pool-service-topology  → enabled, nodes from NodeBucket CRs
elif --enable-node-pool             → enabled, nodes from NodePool CRs
else                                → disabled; nodesGetter returns an empty list
```

Defaults are `--enable-node-pool=true` and
`--enable-pool-service-topology=false` (`cmd/yurthub/app/options/options.go:121,125`).

Because the gate is read at initialisation, changing these flags requires restarting
yurthub; editing them without a restart produces no behaviour change and looks like
the flag was ignored.

`--enable-node-pool` is deprecated in favour of `--enable-pool-service-topology`
(`options.go:233`), but the two branches read node membership from **different
resources**. Migration is therefore not a rename: a cluster switching flags must also
have the corresponding CRs present and synced, or pool resolution fails and traffic
is returned unfiltered (`filter.go:204-208`).

---

## Node vs pool matching

- **Node scope** keeps endpoints whose hosting node equals the local node name.
- **Pool scope** resolves the local node's NodePool, expands it to a node list via
  `nodesGetter`, and keeps endpoints hosted on any node in that list
  (`filter.go:197-218`).

If the local node has no NodePool label, the pool handler logs at info level and
delegates to the node handler (`filter.go:199-202`). The request is honoured, but at a
narrower scope than asked for.

---

## EndpointSlice v1 vs v1beta1

The hosting node is carried differently by version, so the two are reassembled by
separate functions:

| Version | Node identity | Reassembler |
| --- | --- | --- |
| `discovery.k8s.io/v1beta1` | `Endpoint.Topology["kubernetes.io/hostname"]` | `reassembleV1beta1EndpointSlice` (`filter.go:221`) |
| `discovery.k8s.io/v1` | `Endpoint.NodeName` (`*string`, optional) | `reassembleEndpointSlice` (`filter.go:248`) |

Endpoints with no resolvable hosting node cannot be attributed to a node or NodePool
and are dropped. Under v1beta1 this happens naturally: a missing hostname matches
nothing. Under v1 the optional pointer must be checked before dereferencing —
[PR #2593](https://github.com/openyurtio/openyurt/pull/2593) adds that guard. Without
it, a v1 EndpointSlice carrying an endpoint with `nil` `NodeName` panics yurthub, so
the failure presents as a crash-looping hub rather than as missing endpoints.

Both reassemblers deliberately assign the filtered slice even when it is empty
(`filter.go:242,269`), rather than leaving the original endpoints in place. Empty is a
meaningful answer: "nothing in scope serves this Service." Falling back to the full
set would silently defeat the topology guarantee.

---

## Known rough edges

Useful to name explicitly when a user's mental model does not match observed
behaviour. These are properties of the current implementation, not bugs to fix
inside this skill.

1. **Silent no-ops dominate.** Unrecognised annotation value, disabled pool gate,
   unreadable Service, and failed pool resolution all return traffic unfiltered with
   nothing surfaced to the user beyond a yurthub log line
   (`filter.go:156-158,152-155,174-178,204-208`). There is no Service event and no
   status field to inspect, so logs are the only signal.
2. **Fail-open on Service lookup.** A failed Service read disables topology for that
   object rather than failing closed (`filter.go:174-178`). Worth stating plainly when
   the guarantee is being relied on for isolation: a transient read problem relaxes
   the constraint rather than enforcing it.
3. **Silent scope narrowing.** A node missing its NodePool label degrades pool scope
   to node scope (`filter.go:199-202`) — the opposite direction from the above, and
   easy to misread as the filter being too aggressive.
4. **`kubernetes.io/zone` is an alias, not zone semantics.** It maps to NodePool
   scope (`filter.go:150`), which can surprise anyone importing a mental model from
   upstream topology-aware routing.
5. **Component-scoped by default.** Only `kube-proxy`, `coredns`, and
   `nginx-ingress-controller` are filtered out of the box
   (`cmd/yurthub/app/options/filters.go:53-59`); other clients are unaffected no
   matter how the Service is annotated. This is extendable through the `yurt-hub-cfg`
   ConfigMap, whose entries are unioned with the defaults rather than replacing them
   (`pkg/yurthub/configuration/manager.go:215-239`), but it is opt-in and easy to
   overlook.
6. **Two different reload semantics.** The component list is watched by an informer
   and applies immediately (`manager.go:89-93`), while the pool gate is resolved once
   at initialisation and requires a yurthub restart (`nodes_initializer.go:52-80`).
   Configuration that "should have applied by now" is often simply on the wrong side
   of this split.
7. **`endpoints` is registered but never filtered.** The filter is registered for both
   `endpoints` and `endpointslices` (`cmd/yurthub/app/options/filters.go:56-57`), but
   `Filter()` only type-switches on the two EndpointSlice types and returns everything
   else unchanged (`filter.go:131-138`). No filter in the yurthub tree handles
   `v1.Endpoints`. A consumer reading the legacy `endpoints` resource therefore
   bypasses service topology entirely, while appearing fully configured. The
   registration and the implementation disagree; treat this as a real gap rather than
   a documentation nuance.
8. **Propagation depends on a separate controller.** Annotation changes reach edge
   nodes only because yurt-manager patches the EndpointSlice (see "Propagation"). If
   those controllers are disabled, configuration silently does not take effect, and
   the eventual delivery via unrelated pod churn makes it look intermittent.
