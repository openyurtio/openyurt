# Roadmap

This document outlines the development roadmap for the OpenYurt project.

## v0.4.0 Roadmap(Release Plan: 2021.5)

- Add Cloud Native IOT Device Management API definition.
- Support IOT Device Management that comply with cloud native IOT API
- Support autonomy feature in node pool level.
- Support manage configmap in node pool with unique setting.
- Upgrade openyurt components to support Kubernetes 1.18.
- Add basic Pod network recovery mechanism to handle edge node restarts.
- Improve `YurtCtl` user experience.
- Add minimal hardware requirement and system requirement info of OpenYurt.

## v0.5.0 Roadmap

- Support IOT Device Management integrated with EdgeX Foundry that comply with cloud native IOT API
- Yurt-tunnel support more flexible settings for forwarding requests from cloud to edge
- Add local storage statics collection and report
- Support Pods that use `InClusterConfig` access kube-apiserver run on edge nodes without modification.
- Improve OpenYurt user experience(yurtctl init/join/reset)
- Support service to bound east-west traffic within a nodePool

## v0.6.0 Roadmap

- Launch OpenYurt Experience Center to support end users to learn openyurt easily.
- Support Ingress controller at NodePool level.
- Local storage supports multi-devicepath
- Add YurtAppDaemon for managing workloads like DaemonSet at NodePool level.
- Add YurtCluster Operator(A declarative way for kubernetes and openyurt conversion)
- Update Docs and homepage website

## v0.7.0 Roadmap

- Adapt kubernetes v1.22+ version
- Release edge network project [raven](https://github.com/openyurtio/raven)
  - inter-pods and service communication across public network
  - integrate yurt-tunnel component into raven
- Support more features for edge device
  - define `YurtDeviceInterface` for integrating IOT systems seamlessly
  - improve Yurt-Device-Controller version and stability
  - support EdgeX TLS version
- Improve OpenYurt Experience Center
  - support github id as user name to register

## v0.8.0 Roadmap

**ControlPlane SIG**

- Provide NodePool Governance Capability
  - Yurthub adds lease proxy mechanism ([#779](https://github.com/openyurtio/openyurt/issues/779))
  - Add pool-coordinator-controller component ([#774](https://github.com/openyurtio/openyurt/issues/774))
  - Yurthub supports writing metadata to pool-coordinator ([#778](https://github.com/openyurtio/openyurt/issues/778))
  - Add pool-coordinator component ([#777](https://github.com/openyurtio/openyurt/issues/777))
  - Add admission webhook ([#775](https://github.com/openyurtio/openyurt/issues/775))
  - Modify Yurt-Controller-Manager ([#776](https://github.com/openyurtio/openyurt/issues/776))
- Support to use Helm charts to intsll OpenYurt ([#824](https://github.com/openyurtio/openyurt/issues/824))
- Improve OpenYurt Experience Center
  - support deploy EdgeX Foundry on the edge site
- Rename UnitedDeployment to YurtAppSet ([#735](https://github.com/openyurtio/openyurt/issues/735))
- Update english version docs for homepage docs.

detail info: https://github.com/openyurtio/openyurt/projects/5

**DataPlane SIG**

- support WireGuard backend ([#13](https://github.com/openyurtio/raven/issues/13))
- support SLB as public network exporter for gateway ([#22](https://github.com/openyurtio/raven/issues/22))
- support kube-proxy ipvs mode ([#16](https://github.com/openyurtio/raven/issues/16))
- add reconciliation loop to check route entries and vpn connections periodically. ([#10](https://github.com/openyurtio/raven/issues/10))
- support distribute route path decision ([#14](https://github.com/openyurtio/raven/issues/14))
- merge YurtTunnel into raven

detail info: https://github.com/openyurtio/raven/projects/3

**IoT SIG**

- define `YurtDeviceInterface` for integrating IOT systems seamlessly
- support enable security features for EdgeX instance by yurt-edgex-manager
- Added the definition of equipment Command and data processing process Pipeline.
- Manage Benchmark based on OpenYurt+EdgeX cloud native device
