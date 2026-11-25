# Serving EndpointSlices with live pod IPs while disconnected

|                          title                          | authors  | reviewers | creation-date | last-updated |    status     |
|:-------------------------------------------------------:|----------| --------- |---------------| ------------ | ------------- |
| serve EndpointSlices with live pod IPs while disconnected | @Rad710  |           | 2026-09-17    |              | implementable |

<!-- TOC -->
* [Summary](#summary)
* [Motivation](#motivation)
    * [Goals](#goals)
    * [Non-Goals/Future Work](#non-goalsfuture-work)
* [Proposal](#proposal)
    * [Live pod IP source](#live-pod-ip-source)
    * [The livepodip filter](#the-livepodip-filter)
    * [Watch invalidation](#watch-invalidation)
* [API Changes](#api-changes)
* [Implementation History](#implementation-history)
<!-- TOC -->

## Summary
If an edge node reboots while the cloud is unreachable, every pod gets a new IP, but the EndpointSlices yurthub serves still carry the old ones. The endpoint controller lives in the cloud and cannot update them. kube-proxy and coredns program dead IPs and every ClusterIP service on the node stops working until the cloud is back.

This proposal makes yurthub read the live pod IPs from the container runtime and rewrite EndpointSlice addresses on the way out while disconnected. It also lets yurthub end open watches with 410 so kube-proxy and coredns list again.

## Motivation
We hit this on real hardware. After a disconnected reboot kube-proxy had DNAT rules to IPs no pod held. coredns runs as a daemonset and got a new IP too, so kube-dns pointed at a dead pod and cluster DNS was down. One application pod was unusable for 34 minutes. The only fix was editing yurthub's cache files by hand and restarting components.

Issues #880 and #1400 have a similar symptom but a different cause.

### Goals
1. While disconnected, serve EndpointSlice addresses that match the pods running on this node.
2. Consumers that already listed pick the change up without a restart.
3. Do it at serve time only. Nothing is written to the cache, so on reconnect the endpoint controller is authoritative again.

### Non-Goals/Future Work
1. Pods on other nodes. The source is this node's runtime.
2. Marking endpoints not ready. CRI cannot tell a pod without a sandbox from a hostNetwork pod.
3. The `Endpoints` resource, and the cloud working mode.

## Proposal

### Live pod IP source
`pkg/yurthub/kubernetes/cri` talks to the runtime over CRI and answers "what is the IP of pod ns/name" from `ListPodSandbox`. Results are cached for 2 seconds and concurrent lookups share one call. It is only built in edge mode. If the socket is missing it logs and stays nil, and the filter does nothing.

### The livepodip filter
`pkg/yurthub/filter/livepodip` rewrites `endpoints[].addresses` in EndpointSlices served to kube-proxy and coredns. It matches on `targetRef` (namespace and name), never on the IP, because host-local IPAM reuses freed IPs. For example, `app-a` was `10.42.1.39` before the outage and is `10.42.1.43` after the reboot, so the served slice says `10.42.1.43`. Anything it cannot resolve is left as cached. It only acts while the health checker says the cloud is unreachable.

### Watch invalidation
kube-proxy and coredns list once and then watch. While offline the multiplexer emits no events, so a corrected slice would never reach them. The filter implements a new optional interface, `filter.WatchInvalidator`. It polls the runtime every 5 seconds while disconnected and a watch is open. When an IP changed, `filterWatch` in the multiplexer ends the watch with 410 `StatusReasonExpired`. That makes client-go list again. A clean close would only make it re-watch.

## API Changes
New yurthub flag `--container-runtime-endpoint`, default `unix:///run/containerd/containerd.sock`, same name as kubelet and crictl.

New chart value `yurthub.livePodIPAgents` to add consumers beyond kube-proxy and coredns, rendered into `yurt-hub-cfg`.

New optional interface in `pkg/yurthub/filter/interfaces.go`:

```go
type WatchInvalidator interface {
	// Invalidated returns a channel that is closed when this filter's output
	// for already-served objects may have changed. One signal per call.
	Invalidated(stop <-chan struct{}) <-chan struct{}
}
```

New dependencies: `k8s.io/cri-api` and `k8s.io/cri-client` at v0.34.0, and `golang.org/x/sync` becomes direct for `singleflight`.

## Implementation History
- 2026-09-17: proposal opened, issue #2810
