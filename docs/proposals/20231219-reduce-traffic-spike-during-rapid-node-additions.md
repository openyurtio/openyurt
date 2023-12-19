|                            title                            | authors     | reviewers | creation-date | last-updated | status      |
|:-----------------------------------------------------------:|-------------| --------- |---------------| ------------ | ----------- |
| Reduce cloud-edge traffic spike during rapid node additions | @rambohe-ch |       | 2023-12-19    |    |  |

# Reduce cloud-edge traffic spike during rapid node additions

## Summary

Introducing the NodeBucket resource provides a scalable and efficient way to manage large NodePools in OpenYurt clusters,
significantly reducing cloud-edge traffic during rapid node additions and maintaining cluster stability.

## Motivation

In OpenYurt, NodePool resources manage a group of nodes, and NodePool.status.nodes hold information about all nodes in the pool.
YurtHubs rely on NodePool.status.nodes to determine endpoints within the same NodePool, providing service topology capabilities at the node pool level.
In scenarios where a large NodePool (e.g., with 1000 nodes) experiences rapid growth, such as the addition of 100 edge nodes within a short period of 1 minute,
the cloud-edge traffic can escalate dramatically. Notably, for each node addition, the NodePool data is updated, resulting in 100 updates for 100 new nodes.
These updates are then propagated to every YurtHub component. Assuming a NodePool resource with 1000 nodes information is 60KB in size, the average traffic can be calculated as follows:

```
100 (updates) * 1000 (nodes) * 60KB (size per update) / 60 seconds = 100 MB/s = 800 Mbps.
```

Such a traffic spike can strain the network and impact cluster stability.

### Goals

- Introduce a scalable solution for managing large-scale NodePools in OpenYurt clusters.
- Minimize cloud-edge network traffic caused by frequent NodePool updates during rapid node additions.
- Maintain cluster stability by preventing bandwidth saturation.

### Non-Goals/Future Work

- It does not seek to replace the existing NodePool resource but rather to complement it and optimize it for scalability.
- Controllers in yurt-manager component that are using NodePool will remain unchanged.

## Proposal

### Solution Introduction

- The proposed solution is the implementation of a NodeBucket resource that holds node information within the NodePool.
The NodePool to NodeBucket ratio would be 1:N. For example, if one NodeBucket can store information for up to 100 nodes,
then a NodePool with 1000 nodes could be split into 10 NodeBuckets, each being only 1/10th the size of the original (e.g., 6KB).

- Following the previously described NodeBucket CRD segment, a significant change in design is that YurtHubs will monitor NodeBucket instead of NodePool on all edge nodes.
This shift is critical for enabling the NodeBucket resource to reduce traffic and increase scalability effectively.

- During the addition of 100 nodes within 1 minute, only the NodeBuckets would be updated and propagated to all YurtHubs. The cloud-edge NodeBucket communication traffic would then be:

```
100 (updates) * 1000 (nodes) * 6KB (size per update) / 60 seconds = 10 MB/s = 80 Mbps.
```

This represents a tenfold reduction in cloud-edge traffic compared to the current NodePool approach.

### NodeBucket API

```
type Node struct {
    // Name is the name of node
    Name string `json:"name,omitempty"`
}

type NodeBucket struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    // NumNodes represents the number of nodes in the nodebucket
    NumNodes int32 `json:"numNodes"`
    // Nodes represents a subset of nodes in the nodepool
    Nodes []Node `json:"nodes"`
}

type NodeBucketList struct {
    metav1.TypeMeta `json:",inline"`
    metav1.ListMeta `json:"metadata,omitempty"`
    Items           []NodeBucket `json:"items"`
}
```

1. A label denoted as `openyurt.io/pool-name={nodepool-name}` will be appended to each NodeBucket associated with a NodePool, allowing for the aggregation and retrieval of all relevant NodeBuckets using a LabelSelector based on this specific label.

2. The `OwnerReference` field within NodeBucket will be configured to reference its corresponding NodePool, establishing a clear ownership relationship.

### User Stories

#### Story 1（General）

As a cluster administrator, I manage a large OpenYurt cluster with several node pools, each containing hundreds of edge nodes.

My challenge is to scale the cluster quickly by adding new nodes to the pools without overloading the network bandwidth or affecting the cluster's performance.

With the current NodePool setup, I've noticed cloud-edge traffic spikes and stability issues during rapid node pool expansions.

## Implementation History

- [ ]  12/19/2023: Draft proposal created;