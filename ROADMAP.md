# OpenYurt Roadmap

This document defines a high level roadmap for OpenYurt development and upcoming releases. Community and contributor involvement is vital for successfully implementing all desired items for each release. We hope that the items listed below will inspire further engagement from the community to keep OpenYurt progressing and shipping exciting and valuable features.

## 2025 H1
- Support Kubernetes version 1.32.
- Support aggregate list/watch requests at the node pool level in order to reduce the overhead of control-plane and network traffic between cloud and edge.
- Merge Raven into the OpenYurt main repository.
- Develop a tool(or a controller in yurt-manager) to install yurthub component in a standard K8s cluster. 
- Reconstruct the framework of controllers and webhooks of yurt-manager.

## 2025 H2
- Support LoadBalancer Services across multiple node pools.
- Support aggregate list/watch requests for CRD resource at the node pool level.
- Provide network diagnostics capabilities.
- Support EdgeX version 4.0
- Support to install and maintain a scalable K8s cluster in local DC based on OpenYurt cluster.

## Pending
- Support Ingress Controller in multiple nodepools.
- Supporting a large number of edge nodes and providing lightweight runtime solutions is a high demand for edge computing.
- Integration of dashboard with IoT, provide Edgex Foundry management capabilities.
- Enrich the capabilities of the console.


## 2024 H1
- Support Kubernetes up to V1.30.
- Enhancement to edge autonomy capabilities.
- Upgrade YurtAppSet to v1beta1 version.
- Improve transparent management mechanism for control traffic from edge to cloud.
- Separate clients for yurt-manager component.

## 2024 H2
- Support multiplexer list/watch requests in node/nodepool level.
- Upgrade YurtIoTDock to support edgex v3 api.
- Establish the component mechanism of the iot system.
- Improve nodepool to support hostnetwork mode and node conversion between v1alpha1 and v1beta1 version.
