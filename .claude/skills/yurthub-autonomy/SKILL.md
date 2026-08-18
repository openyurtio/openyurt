---
name: openyurt-yurthub-autonomy
description: "Troubleshoots OpenYurt edge node autonomy and pod eviction issues during cloud disconnections."
---

# YurtHub Edge Autonomy Troubleshooting Skill

This skill allows the AI to autonomously debug issues where edge pods are incorrectly evicted or crash during cloud disconnection events.

## Context
In standard Kubernetes, if a node loses connectivity to the control plane, the master eventually evicts the pods on that node. OpenYurt introduces `YurtHub` and node autonomy annotations to ensure edge nodes can operate autonomously during network partitions (cloud-edge disconnections).

## Troubleshooting Steps

1. **Verify Node Autonomy Annotation**
   Check if the edge node is properly marked for autonomy:
   `kubectl get node <node-name> -o jsonpath='{.metadata.annotations.node\.beta\.alibabacloud\.com/autonomy}'`
   This must be set to `"true"`. If it's missing, the cloud controller will evict pods during a network partition.

2. **Verify YurtHub Local Cache**
   When disconnected from the cloud, `YurtHub` falls back to its local disk cache (usually located at `/etc/kubernetes/cache/yurthub`).
   To check if a pod's metadata was successfully cached before the disconnection, query the `yurthub` metrics endpoint on the node or inspect the local cache directory via `yurt-node-servant` if accessible.

3. **Check Kubelet Heartbeat Forgery**
   If the cloud controller shows the node as `NotReady` but pods are not evicted, verify that the `yurt-controller-manager` is correctly intercepting the node status.
   If pods *are* being evicted despite the autonomy annotation, check the logs of `yurt-controller-manager` for `NodeLease` or heartbeat lease renewal errors.

## Common Hallucinations to Avoid
- Do NOT advise the user to tweak standard Kubernetes `pod-eviction-timeout` in the `kube-controller-manager`. OpenYurt handles this via `yurt-controller-manager` and autonomy annotations.
- Do NOT assume a `NotReady` node state in OpenYurt means the node is down. It often just means the edge-cloud connection is severed, but the edge node and its pods are running autonomously.
