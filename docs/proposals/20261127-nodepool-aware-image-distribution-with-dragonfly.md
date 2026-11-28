# NodePool-aware Image Distribution with Dragonfly

|                  title                  | authors           | reviewers                              | creation-date | last-updated | status |
|:---------------------------------------:|-------------------|----------------------------------------|---------------|--------------|--------|
| NodePool-aware Image Distribution with Dragonfly | @harshitnub077 | @rambohe-ch @zyjhtangtang @luc99hen | 2026-11-27    |              | Draft  |

<!-- TOC -->
* [NodePool-aware Image Distribution with Dragonfly](#nodepool-aware-image-distribution-with-dragonfly)
  * [Glossary](#glossary)
  * [Summary](#summary)
  * [Motivation](#motivation)
    * [Goals](#goals)
    * [Non-Goals/Future Work](#non-goalsfuture-work)
  * [Proposal](#proposal)
    * [User Stories](#user-stories)
    * [Requirements](#requirements)
    * [Implementation Details/Notes/Constraints](#implementation-detailsnotesconstraints)
    * [Risks and Mitigations](#risks-and-mitigations)
  * [Alternatives](#alternatives)
  * [Upgrade Strategy](#upgrade-strategy)
  * [Additional Details](#additional-details)
    * [Test Plan](#test-plan)
  * [Implementation History](#implementation-history)
<!-- TOC -->

## Glossary

- **Dragonfly**: CNCF graduated P2P file and image distribution system. Its components are the manager (API, job state), the scheduler (assigns parents to peers), the seed peer (fetches from the origin) and dfdaemon (the peer on every node, also an HTTP proxy that containerd can use as a registry mirror).
- **Preheat job**: a Dragonfly manager job that downloads an image into the caches of selected peers ahead of time (`POST /oapi/v1/jobs`, `type: preheat`).
- **Image preheat (OTA)**: the v1.7 feature that pulls a DaemonSet's new image on each node before the OTA cutover ([proposal](20251101-image-preheating-for-OTA.md)).

## Summary

OTA image preheat (v1.7) takes image pulls off the upgrade's critical path, but every edge node still pulls from the registry by itself. A NodePool of N nodes behind one cloud-edge link carries each layer over that link N times.

This proposal makes image distribution **NodePool-aware** by integrating Dragonfly, in two steps:

1. **Phase 1:** a pluggable image preheat backend in yurt-manager, `--image-preheat-backend=job|dragonfly`. With `dragonfly`, the per-pod preheat Jobs become one Dragonfly preheat job per image per NodePool, aimed at that pool's nodes. A working implementation is in [#2903](https://github.com/openyurtio/openyurt/pull/2903).
2. **Phase 2:** a NodePool-scoped `ImagePreheat` API that any workload can use, plus the pieces real edge sites need: per-pool seed peers, private registries, metrics and e2e coverage.

It implements the 2026 H2 roadmap item *"Strengthen ecosystem integration with projects such as Dragonfly and HAMi"*.

## Motivation

- **WAN traffic grows with pool size.** `image-preheat-controller` creates one Job per pod (`pkg/yurtmanager/controller/daemonsetupgradestrategy/imagepreheat`), and every Job pulls every layer from the registry. On a local kind cluster with 3 edge nodes, preheating a 150 MB layer with today's controller made the registry serve the layer 3 times (472 MB). A 2 GB AI inference image on a 50-node pool would cross the link about 50 times.
- **The cloud-edge link is the scarcest resource OpenYurt manages.** OpenYurt already shares control-plane traffic per pool (pool-scope metadata, list/watch multiplexing, hub leader election). Image traffic is the largest per-node cost that still has no pool-level sharing.
- **Images keep growing.** AI inference, video analytics and other edge middleware ship images measured in GBs (see the v1.7 OTA preheat proposal).
- **Natural ecosystem fit.** Dragonfly is a CNCF graduated project with a stable preheat Open API and existing containerd mirror integration. The two projects have been presented together before ("OpenYurt & Dragonfly: Enhancing Efficient Distribution of LLMs in Cloud-Edge Collaborative Scenarios", KubeCon China 2024).

### Goals

1. Distribute an image to all nodes of a NodePool with roughly one copy crossing the cloud-edge link.
2. Keep the OTA contract unchanged: YurtHub `POST .../pods/{pod}/imagepull`, and the `PodNeedUpgrade` / `PodImageReady` conditions.
3. Make Dragonfly optional and off by default. With the default backend OpenYurt behaves exactly as today.
4. No Go dependency on the Dragonfly module. Use the manager's Open API.

### Non-Goals/Future Work

- Automatic preheat without a trigger (also a non-goal of the v1.7 proposal).
- Installing or operating Dragonfly. This proposal documents a reference setup only.
- Replacing kubelet or containerd image management.
- GPU sharing (HAMi), a sibling roadmap item.

## Proposal

### User Stories

#### Story 1

As an operator of 2,000 retail stores (one NodePool per store, 3 to 5 nodes, 20 Mbps uplink), I want a weekly 1.5 GB model-serving image to cross each store's uplink once, not once per node.

#### Story 2

As an operator of a factory whose NodePool loses the cloud link for hours, I want an image that was already preheated in the pool to reach a replaced node from inside the pool.

#### Story 3

As a platform engineer, I want to keep OTA upgrades exactly as they work today when Dragonfly is not installed.

### Requirements

#### Functional Requirements

- **FR1.** A backend selection flag. With `job` (the default), the current Job-based preheat is unchanged.
- **FR2.** With `dragonfly`, pods of one DaemonSet that target the same revision, have requested preheat and run in the same NodePool are preheated with one Dragonfly job per image. Each job is aimed at the InternalIPs of those nodes.
- **FR3.** Job progress maps onto `PodImageReady`: `True` when all jobs succeed, `False` with the Dragonfly error on failure or timeout. A later request for the same revision retries.
- **FR4 (Phase 2).** An `ImagePreheat` resource preheats images on selected NodePools for any workload.

#### Non-Functional Requirements

- **NFR1.** No new Go module dependency on Dragonfly.
- **NFR2.** Dragonfly outages must not mark pods failed. They requeue with backoff.
- **NFR3.** Least-privilege RBAC for the controller, with the token read uncached and no Secret informer.

### Implementation Details/Notes/Constraints

**Phase 1 ([#2903](https://github.com/openyurtio/openyurt/pull/2903))**

- Flags on yurt-manager:
  - `--image-preheat-backend=job|dragonfly`
  - `--dragonfly-manager-address`
  - `--dragonfly-token-secret=<ns>/<name>` (key `token`, a Dragonfly personal access token)
  - `--dragonfly-insecure-registries`
- **Batching:** on reconcile of a pod that requested preheat, the controller collects the matching pods of the same DaemonSet: same target revision, preheat requested, same `apps.openyurt.io/nodepool` value. A node without a pool label is batched alone.
- **Jobs:** for each unique image in the pod template the controller calls `POST /oapi/v1/jobs` with `{type: preheat, args: {type: image, url: <manifest url>, scope: all_peers, ips: [...]}}`. IPs are chunked at 100, the API's limit.
- **Tracking:** job ids go into the pod annotation `apps.openyurt.io/dragonfly-preheat-jobs: "<revision>;<created-unix>;<id>,<id>"` on every batch member. Later reconciles poll `GET /oapi/v1/jobs/{id}` every 10s instead of creating new jobs. All `SUCCESS` sets `PodImageReady=True`. Any `FAILURE`, or 60 minutes without finishing, sets `False` and drops the annotation so a new request starts over.
- **Edge-side configuration** (documentation, not code):
  - dfdaemon runs on each node with `host.idc` set to the node's NodePool, so the Dragonfly scheduler prefers parents in the same pool.
  - containerd uses dfdaemon (`127.0.0.1:4001`) as the registry mirror, so the cutover pull is served from the node-local cache.
- **RBAC** added to `image-preheat-controller`: `nodes get/list`, `pods list`, `secrets get`.

**Phase 2**

1. **`ImagePreheat` API** (`apps.openyurt.io/v1alpha1`):

   ```yaml
   apiVersion: apps.openyurt.io/v1alpha1
   kind: ImagePreheat
   metadata:
     name: infer-v2
   spec:
     images:
       - registry.example.com/ai/infer:v2
     nodePoolSelector:
       matchLabels: {region: north}
     backend: dragonfly          # or job
     imagePullSecrets:
       - name: regcred
   status:
     pools:
       - name: pool-a
         phase: Succeeded
         nodesReady: 50
   ```

   OTA and YurtAppSet can create these instead of calling a backend directly.
2. **Per-pool seed peer (optional).** It can be deployed with YurtAppSet so a disconnected pool keeps serving cached layers. Document the mapping from NodePool to Dragonfly scheduler cluster scopes (`idc`).
3. **Private registries:** pass credentials from `imagePullSecrets` as the job's `username`/`password`.
4. **Metrics:** jobs by state, preheat duration, bytes per pool.

### Risks and Mitigations

| Risk | Mitigation |
|---|---|
| The Dragonfly scheduler *prefers* same-`idc` parents but does not enforce it, so cross-pool transfers can happen | Phase 2 per-pool seed peers or scheduler clusters scoped by `idc`. The trade-off is documented |
| With this backend `PodImageReady=True` means "in the node's dfdaemon cache", not "in containerd" | The cutover pull is node-local through the mirror. Documented. The `job` backend keeps the old meaning |
| A Dragonfly manager outage blocks preheat | Errors requeue and pods are not marked failed. Operators can switch back to `job` |
| Cluster-wide `secrets get` | Phase 2: restrict to the token namespace with a Role |
| Jobs created before a failure part-way through a batch are recreated on retry | Store ids incrementally in Phase 2 |

## Alternatives

1. **Dragonfly as a containerd mirror only, with no OpenYurt change.** Existing preheat Jobs would pull through dfdaemon. This helps, but there is still one Job per pod, nothing is batched per pool, and OpenYurt has no visibility into distribution.
2. **A P2P layer inside YurtHub.** Duplicates a mature CNCF project and puts a large new surface into a node-critical component.
3. **A per-pool registry cache (pull-through registry).** Needs a stateful cache in every pool and doesn't use the existing preheat flow. Phase 2's per-pool seed peer gives the same result using Dragonfly.

## Upgrade Strategy

Phase 1 adds flags with a default of `job`, so existing clusters see no change on upgrade. Switching to `dragonfly` needs a Dragonfly installation reachable from yurt-manager, a token Secret, and the three flags. Switching back to `job` at any time restores the current behavior. Pods with an in-flight Dragonfly annotation are re-preheated by the job backend on their next request.

## Additional Details

### Test Plan

- **Unit tests (Phase 1, in #2903):** options validation, manifest url conversion, HTTP client (httpmock), and per-pool batching. They also cover chunking over 100 nodes, image dedup, polling, timeout, retry after failure, transient errors that requeue, nodes without InternalIP or pool, and the controller wiring.
- **e2e (Phase 2):** kind with two edge NodePools and Dragonfly installed. Assert that preheating an OTA DaemonSet sets `PodImageReady=True` on all pods and that registry blob downloads stay below the number of nodes.
- **Benchmark doc:** registry bytes and time-to-ready for the `job` and `dragonfly` backends at 3, 10 and 50 nodes.

## Implementation History

- [x] 11/27/2026: Phase 1 implementation opened as [#2903](https://github.com/openyurtio/openyurt/pull/2903)
- [x] 11/27/2026: Proposed idea in [#2906](https://github.com/openyurtio/openyurt/issues/2906)
- [ ] Present proposal at a community meeting
- [ ] Phase 2 implementation
