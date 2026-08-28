# Configuring service topology

Goal: make a Service's traffic stay on the same node, or inside the same NodePool.

Confirm the prerequisites first. Enabling the annotation on a cluster that cannot
enforce it produces no error and no warning — it simply does nothing — so verifying
before annotating saves the user a confusing debugging session.

---

## Step 1 — Decide the scope

Ask which behaviour is wanted:

| Intent | Annotation value |
| --- | --- |
| Serve only from a backend on the same node | `kubernetes.io/hostname` |
| Serve only from backends in the same NodePool | `openyurt.io/nodepool` |

`kubernetes.io/zone` is accepted and behaves identically to `openyurt.io/nodepool`
(`pkg/yurthub/filter/servicetopology/filter.go:150`). Prefer the explicit
`openyurt.io/nodepool` spelling; `kubernetes.io/zone` reads as if it maps to real
cloud zones, which it does not.

Node-local scope is strict. If no backend runs on the node, the client gets an empty
endpoint list and the call fails. Recommend it only for a Service backed by a
DaemonSet, or where a local backend is otherwise guaranteed.

---

## Step 2 — Verify prerequisites

**For node-local scope**, no cluster-level gate is involved; skip to step 3.

**For NodePool scope**, confirm both of the following.

1. Pool topology is enabled in yurthub. It is gated, and when the gate is off the
   filter silently returns traffic unfiltered (`filter.go:150-155`).

   ```bash
   kubectl -n kube-system get ds yurt-hub -o yaml | grep -E 'enable-node-pool|enable-pool-service-topology'
   ```

   Enabled if `--enable-pool-service-topology=true`, or (legacy path)
   `--enable-node-pool=true`. Defaults are `false` and `true` respectively
   (`cmd/yurthub/app/options/options.go:121,125`), so a default install is enabled via
   the legacy flag.

   Prefer `--enable-pool-service-topology=true` for new configuration:
   `--enable-node-pool` is deprecated and slated for removal (`options.go:233`). Note
   this is not a pure rename — the two flags read node membership from different
   resources, so confirm the corresponding CRs exist before switching:

   ```bash
   kubectl get nodebuckets -A     # required by --enable-pool-service-topology
   kubectl get nodepools          # required by --enable-node-pool
   ```

2. Every node that should participate is labelled into the NodePool.

   ```bash
   kubectl get nodes -L apps.openyurt.io/nodepool
   ```

   A node with no NodePool label silently degrades to node-local scope
   (`filter.go:199-202`). Do not hand-edit this label to fix it — node membership is
   controller-owned; add the node to the pool through the normal conversion flow.

---

## Step 3 — Confirm the consumer is a filtered component

Topology is enforced only for components yurthub is configured to filter. The default
set is (`cmd/yurthub/app/options/filters.go:53-59`): `kube-proxy`, `coredns`, and
`nginx-ingress-controller` — and only on `list`/`watch` of `endpoints` and
`endpointslices`.

If the traffic is routed by something else — a mesh sidecar, a custom controller, a
differently-named ingress — annotating the Service alone will not constrain it. Say
so before making the change rather than after.

That default is extendable, though. The `yurt-hub-cfg` ConfigMap adds components to a
filter, unioned with the defaults (`pkg/yurthub/configuration/manager.go:215-239`),
keyed by filter name with a comma-separated value (`manager.go:40`):

```yaml
data:
  servicetopology: "kube-proxy,coredns,nginx-ingress-controller,my-component"
```

It is watched by an informer, so it applies without a yurthub restart
(`manager.go:89-93`). Editing it changes behaviour for every node reading that
ConfigMap, so confirm with the user first.

---

## Step 4 — Apply the annotation

Confirm with the user before mutating, then:

```bash
kubectl annotate service <svc> -n <ns> openyurt.io/topologyKeys=openyurt.io/nodepool
```

Use `--overwrite` when changing an existing value. The value must match one of the
recognised strings exactly; an unrecognised value is ignored silently
(`filter.go:156-158`), so a typo behaves the same as no annotation at all.

To remove topology and restore cluster-wide routing:

```bash
kubectl annotate service <svc> -n <ns> openyurt.io/topologyKeys-
```

---

## Step 5 — Verify from the edge node

Do **not** verify with `kubectl get endpointslice` from a workstation. yurthub filters
the response in flight and never rewrites stored objects, so that view is unfiltered
by design and will always look "wrong".

Verify by observing what a filtered component on the node actually receives: from a
pod on the edge node, call the Service repeatedly and confirm that only in-scope
backends answer.

Interpretation:

- Only local/in-pool backends answer → working as intended.
- No backend answers → in scope, but nothing is running there. Expected: the filter
  returns an empty endpoint list rather than falling back to all endpoints
  (`filter.go:242,269`). Schedule a backend into the scope, or widen the scope.
- Out-of-scope backends still answer → configuration is not taking effect; switch to
  `service-topology-diagnose.md` and work the checks in order.

---

## Worked example

A Deployment spread across two NodePools (`beijing`, `shanghai`), where each pool
should serve itself. The backend echoes its own pod name so the answering replica is
identifiable.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: echo
spec:
  replicas: 4
  selector:
    matchLabels: { app: echo }
  template:
    metadata:
      labels: { app: echo }
    spec:
      containers:
        - name: echo
          image: hashicorp/http-echo
          args: ["-text=$(POD_NAME)", "-listen=:5678"]
          env:
            - name: POD_NAME
              valueFrom:
                fieldRef: { fieldPath: metadata.name }
          ports:
            - containerPort: 5678
---
apiVersion: v1
kind: Service
metadata:
  name: echo
  annotations:
    openyurt.io/topologyKeys: openyurt.io/nodepool
spec:
  selector: { app: echo }
  ports:
    - port: 80
      targetPort: 5678
```

Confirm replicas actually exist in both pools, otherwise the test proves nothing:

```bash
kubectl get pods -l app=echo -o custom-columns=POD:.metadata.name,NODE:.spec.nodeName
kubectl get nodes -L apps.openyurt.io/nodepool
```

From a pod on a node in `beijing`, call the Service repeatedly:

```bash
for i in $(seq 20); do wget -qO- http://echo.default.svc.cluster.local; echo; done | sort | uniq -c
```

Expected: only replicas scheduled on `beijing` nodes appear. Seeing `shanghai`
replicas means the annotation is not being enforced on this node — go to
`service-topology-diagnose.md`.

Then flip the scope and confirm the change actually propagates, which also exercises
the yurt-manager trigger path:

```bash
kubectl annotate service echo openyurt.io/topologyKeys=kubernetes.io/hostname --overwrite
```

Re-run the loop. Only replicas on the *local node* should answer, and if no replica
runs there the call now fails — which is correct node-scope behaviour, not a
regression. If the output does not change at all, the annotation is not reaching the
edge: see diagnose check 4.
