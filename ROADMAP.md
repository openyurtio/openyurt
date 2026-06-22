# OpenYurt Roadmap

This document defines a high level roadmap for OpenYurt development and upcoming releases. Community and contributor involvement is vital for successfully implementing all desired items for each release. We hope that the items listed below will inspire further engagement from the community to keep OpenYurt progressing and shipping exciting and valuable features.

## 2026 H1
- Provide OpenYurt Skills for deploying OpenYurt on existing Kubernetes clusters and enabling edge autonomy.
- Optimize resource consumption of node-side components (including YurtHub, Raven, etc.).
- Establish baseline performance benchmarks for edge components (YurtHub, Kubelet, Raven) at 50/100/200 node scales, and maintain results in community documentation.
- Optimize the certificate management controller.
- Improve stability by increasing unit test coverage across core modules.
- Support Kubernetes 1.34.

  ## 2026 H2
- Provide OpenYurt Skills for configuring Raven and other advanced features.
- Support fine-grained network configuration for Raven L3 and L7, allowing per-NodePool enablement or disablement.
- Support NodePool-level configuration of Raven forward-node-ip.
- Strengthen ecosystem integration with projects such as Dragonfly and HAMi.
- Improve stability by adding end-to-end test cases for critical edge scenarios.
- Support Kubernetes 1.36.

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
