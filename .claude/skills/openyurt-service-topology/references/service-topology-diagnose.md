# Diagnosing service topology

Symptom this tree solves: *"I set `openyurt.io/topologyKeys`, but traffic still
reaches backends outside the node/NodePool."*

Work through the checks in order and **stop at the first failure**. They are ordered
cheapest-first, and each one rules out a distinct cause. Every check names the source
that defines the behaviour, so the conclusion can be justified rather than guessed.

## Symptom index

Use this to jump straight to the likely cause, then still confirm with check 0.

| Symptom | Start at |
| --- | --- |
| Never worked, on any node | 1 — annotation value |
| Works for CoreDNS but not my app | 2 — component not filtered |
| Worked, then I changed the annotation and nothing happened | 4 — propagation |
| Works on some nodes, not others | 6 — NodePool membership |
| Traffic is narrower than asked (pool requested, node behaviour) | 6 — NodePool membership |
| Stopped working after a yurthub restart | 7 — Service read fail-open |
| Only my legacy `endpoints` consumer is unfiltered | 3 — `endpoints` is not filtered |
| yurthub is crash-looping | 9 — nil `NodeName` |

Almost every report is checks 1–6. Checks 7–9 are for cases that survive the obvious
ones.

---

## 0. Establish where you are observing from

Before anything else, make sure the observation is meaningful.

Service topology is applied by yurthub on the edge node while proxying `list`/`watch`
responses. The objects stored in the cluster are never rewritten
(`pkg/yurthub/filter/servicetopology/filter.go`). Therefore:

- `kubectl get endpointslice <name> -o yaml` run from a workstation shows the
  **unfiltered** endpoint set. This is expected and proves nothing.
- A correct verification must observe what a *filtered component on the node*
  receives.

Practical verification: from a pod on the edge node, resolve and call the Service
repeatedly and confirm which backends answer, or inspect the endpoints as seen by
kube-proxy on that node. If the user has been checking from their laptop, this alone
often explains the report.

---

## 1. Is the annotation present, on the Service, and a recognised value?

```bash
kubectl get svc <svc> -n <ns> -o jsonpath='{.metadata.annotations.openyurt\.io/topologyKeys}{"\n"}'
```

The annotation must be on the **Service**, not the EndpointSlice or the Deployment.
yurthub resolves the Service that owns the EndpointSlice and reads the annotation
from it (`filter.go:174-183`).

Recognised values (`filter.go:44-46`):

- `kubernetes.io/hostname` — node-local
- `openyurt.io/nodepool` — NodePool-local
- `kubernetes.io/zone` — NodePool-local (alias)

An unrecognised value is **not an error**. The filter's `switch` falls through to
`default` and returns the object untouched (`filter.go:156-158`), so a typo such as
`openyurt.io/nodePool` (capital P) or `kubernetes.io/nodepool` silently disables
topology. This is the single most common cause. Compare the value byte-for-byte
against the list above.

---

## 2. Is the consuming component actually filtered?

yurthub applies a filter only to requests from components it has been told to filter.
By default that is (`cmd/yurthub/app/options/filters.go:53-59`):

```
kube-proxy, coredns, nginx-ingress-controller
```

The component is taken from the request's client component, which yurthub truncates
from the `User-Agent` header, and the filter is looked up by the tuple
`(component, verb, resource)` (`pkg/yurthub/configuration/manager.go:253-259`,
`filter/approver/approver.go:46`). If the traffic is driven by some other client — a
custom controller, a service mesh sidecar, an application calling the API directly, or
an ingress running under a different binary name — that client's EndpointSlice reads
are **not** filtered, and topology appears to do nothing even though the Service is
annotated correctly.

Confirm which component the client reports:

```bash
kubectl -n kube-system logs <yurt-hub-pod> | grep -i "filter settings are as follows"
```

**This is fixable without changing the application.** The default list is a baseline,
not a hard limit: the `yurt-hub-cfg` ConfigMap can add components to any filter, and
the entries are unioned with the defaults rather than replacing them
(`pkg/yurthub/configuration/manager.go:215-239`). The key is the filter name, the
value is a comma-separated component list (`manager.go:40`):

```bash
kubectl -n kube-system get configmap yurt-hub-cfg -o yaml
```

```yaml
data:
  servicetopology: "kube-proxy,coredns,nginx-ingress-controller,my-component"
```

Because the ConfigMap is watched by an informer (`manager.go:89-93`), this takes
effect without restarting yurthub — unlike the pool gate in check 5, which is read
once at startup. Note that adding a component here changes which clients get filtered
endpoint sets, so confirm with the user before editing a shared ConfigMap.

---

## 3. Is the request a `list`/`watch`, and is it EndpointSlice rather than Endpoints?

Two separate constraints live here.

**Verb.** The filter is registered only for `list` and `watch` (`filters.go:55-58`). A
plain `get` of a single object is **not** filtered. This matters mainly when
reproducing by hand: reproduce with a `list`/`watch`, or you will measure the wrong
path.

**Resource.** This one is a genuine trap. The filter is registered for *both*
resources (`filters.go:56-57`):

```
endpoints      → list, watch
endpointslices → list, watch
```

but `Filter()` only type-switches on `*discovery.v1.EndpointSlice` and
`*discovery.v1beta1.EndpointSlice`; anything else falls through to `default` and is
returned unchanged (`filter.go:131-138`). A `v1.Endpoints` object therefore passes
through **completely unfiltered**, despite the registration implying otherwise. No
filter in the yurthub tree handles `v1.Endpoints`.

Practical consequence: a component that consumes the legacy `endpoints` resource
instead of `endpointslices` silently bypasses service topology entirely. The
configuration looks correct, the component is in the filtered list, and topology
still does not apply.

If a user reports that one specific consumer ignores topology while others honour it,
determine which resource that consumer reads before going further down this tree.
Worth reporting upstream if encountered in practice — the registration and the
implementation disagree.

---

## 4. Did the annotation change actually reach the edge nodes?

Check this whenever topology *used to* behave correctly and stopped, or when a freshly
changed annotation appears to do nothing.

yurthub filters `list`/`watch` **responses**. If the Service annotation changes but the
EndpointSlice object itself does not, no watch event is generated, and edge nodes keep
serving the previously filtered view until something unrelated modifies the
EndpointSlice.

OpenYurt closes that gap on the cloud side rather than the edge. `yurt-manager` watches
Services, detects a changed `openyurt.io/topologyKeys`
(`pkg/yurtmanager/controller/servicetopology/util/util.go:25-29`), enqueues that
Service's EndpointSlices
(`.../endpointslice/endpointslice_enqueue_handlers.go:44-62`), and patches an
`openyurt.io/update-trigger` annotation onto them with the current timestamp
(`.../adapter/adapter.go:53-54`, `.../adapter/endpointslicev1_adapter.go:56-69`). That
patch is what produces the watch event that makes yurthub re-filter.

So the annotation propagates **only because a controller pokes the EndpointSlice**. If
that controller is not running, the annotation change is effectively inert.

```bash
# Did the trigger actually fire? Compare against when the annotation was changed.
kubectl get endpointslice -n <ns> -l kubernetes.io/service-name=<svc> \
  -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.metadata.annotations.openyurt\.io/update-trigger}{"\n"}{end}'

# Is the controller enabled and healthy?
kubectl -n kube-system logs deploy/yurt-manager | grep -i servicetopology
```

The relevant controllers are `servicetopologyendpointslices` and
`servicetopologyendpoints` (`cmd/yurt-manager/names/controller_names.go:51-52`). If
yurt-manager is running with a restricted controller set that excludes them, this is
the cause.

Workaround to confirm the diagnosis: force an unrelated EndpointSlice change (for
example, restart one backend pod). If topology snaps to the new behaviour immediately
afterwards, propagation — not configuration — was the problem.

---

## 5. For NodePool topology only: is pool topology enabled in yurthub?

This check does not apply to `kubernetes.io/hostname`, which always works.

For `openyurt.io/nodepool` and `kubernetes.io/zone`, the filter consults an
`enablePoolTopology` gate, and **returns the object unfiltered when the gate is off**
(`filter.go:150-155`). There is no error and no event — topology simply does not
happen.

The gate is derived in `pkg/yurthub/filter/initializer/nodes_initializer.go:58-73`:

| yurthub flags | Result | Node membership read from |
| --- | --- | --- |
| `--enable-pool-service-topology=true` | enabled | **NodeBucket** CRs |
| else `--enable-node-pool=true` | enabled | **NodePool** CRs |
| neither | **disabled — silent no-op** | — |

Defaults (`cmd/yurthub/app/options/options.go:121,125`): `--enable-node-pool` is
`true`, `--enable-pool-service-topology` is `false`. So on a default install the gate
is on via the legacy path.

Inspect the flags actually in use:

```bash
kubectl -n kube-system get ds yurt-hub -o yaml | grep -E 'enable-node-pool|enable-pool-service-topology'
```

Two traps live here:

- **Explicitly disabling the legacy flag without enabling the new one** turns pool
  topology off silently. `--enable-node-pool=false` alone is enough to break it.
- **`--enable-node-pool` is deprecated** and slated for removal in favour of
  `--enable-pool-service-topology` (`options.go:233`). These are not a rename: they
  read node membership from *different* CRs (NodeBucket vs NodePool, above). A
  cluster that relies on the default today will lose pool topology when the
  deprecated flag is removed unless `--enable-pool-service-topology=true` is set and
  NodeBuckets are present. Flag this proactively when you see a cluster depending on
  the default.

---

## 6. Is this node actually a member of a NodePool?

```bash
kubectl get node <node> -o jsonpath='{.metadata.labels.apps\.openyurt\.io/nodepool}{"\n"}'
```

If the node carries no NodePool label, `nodePoolTopologyHandler` logs at info level
and **silently falls back to node-local topology** (`filter.go:199-202`).

The visible effect is confusing in a specific way: topology *is* applied, but the
scope is narrower than requested — the user asked for NodePool and got node. Traffic
to backends elsewhere in the intended pool disappears, which usually gets reported as
"topology is too aggressive" rather than as a labelling problem.

Confirm from yurthub's own logs on that node:

```bash
kubectl -n kube-system logs <yurt-hub-pod> | grep -i "not added into node pool"
```

---

## 7. Can yurthub read the Service?

To find the topology annotation, the filter looks the Service up through its lister.
If that lookup fails, it logs a warning and returns an empty topology type
(`filter.go:174-178`), which means **the object is returned unfiltered**.

This is a fail-open path, and worth calling out explicitly: a transient or permission
problem reading the Service does not surface as a topology error, it surfaces as
topology silently not being enforced. Look for:

```bash
kubectl -n kube-system logs <yurt-hub-pod> | grep "could not get service"
```

Corresponding symptom: topology works intermittently, or stops working after a
yurthub restart while its informer cache is still warming.

---

## 8. Is the node-membership source present and synced?

When pool topology is enabled, the filter resolves the node list for the pool through
`nodesGetter`, and on error returns the object unfiltered (`filter.go:204-208`).

Depending on which gate is active (check 5), the source differs:

- `--enable-pool-service-topology=true` → **NodeBucket** CRs
  (`nodes_initializer.go:59-61`, GVR `nodebuckets`)
- `--enable-node-pool=true` → **NodePool** CRs (`nodes_initializer.go:62-64`)

```bash
kubectl get nodebuckets -A     # when using --enable-pool-service-topology
kubectl get nodepools          # when using --enable-node-pool
```

A missing or unsynced CR here produces the same visible outcome as a disabled gate —
unfiltered traffic — which is why check 5 must be settled before this one.

---

## 9. EndpointSlice version and unlocatable endpoints

The two EndpointSlice versions carry the hosting node differently, and the filter
handles them separately:

- `discovery.k8s.io/v1` uses `Endpoint.NodeName`, an optional `*string`
  (`filter.go:248`).
- `discovery.k8s.io/v1beta1` uses `Endpoint.Topology[kubernetes.io/hostname]`
  (`filter.go:230,236`).

Endpoints whose hosting node is unknown carry no usable identity, cannot be placed in
a node or NodePool, and are therefore dropped. Under v1beta1 a missing hostname
simply never matches. Under v1 a `nil` `NodeName` must be skipped explicitly before
dereferencing; that guard is the subject of
[PR #2593](https://github.com/openyurtio/openyurt/pull/2593). On builds predating that
fix, a v1 EndpointSlice containing an endpoint with `nil` `NodeName` panics yurthub
rather than degrading, so the symptom is a crash-looping yurthub, not missing
endpoints.

Check for such endpoints:

```bash
kubectl get endpointslice -n <ns> -l kubernetes.io/service-name=<svc> \
  -o jsonpath='{range .items[*].endpoints[*]}{.nodeName}{"\n"}{end}'
```

Blank lines indicate endpoints with no `nodeName`. Expect these to be absent from the
filtered result — that is correct behaviour, not data loss.

---

## Reporting the conclusion

State which check failed, what the source says, and the concrete fix. Do not report
"topology is working" unless it was verified from a filtered component on the node
per check 0. If every check passes and behaviour is still wrong, say so plainly and
capture: the Service annotation, the yurthub flags, the node's NodePool label, the
EndpointSlice version, and the yurthub logs for that node. That set is what a
maintainer needs to triage further.
