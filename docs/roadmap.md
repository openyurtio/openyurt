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

## v1.0 Roadmap

**ControlPlane SIG**

- [API upgrade] upgrade version from v1alpha1 to v1beta1 for NodePool kind ([#77](https://github.com/openyurtio/yurt-app-manager/issues/77))
- [performanace test] add performance metrics and test data for yurthub component ([#915](https://github.com/openyurtio/openyurt/issues/915))
- Improve unit test coverage of openyurtio/openyurt repo
  - [unit test] improve unit test coverage for yurtctl ([#920](https://github.com/openyurtio/openyurt/issues/920))
  - [unit test] improve unit test coverage for yurtadm ([#919](https://github.com/openyurtio/openyurt/issues/919))
  - [unit test] improve unit test coverage for yurt-controller-manager ([#918](https://github.com/openyurtio/openyurt/issues/918))
  - [unit test] improve unit test coverage for yurthub ([#917](https://github.com/openyurtio/openyurt/issues/917))
  - [unit test] improve unit test coverage for yurt-tunnel ([#916](https://github.com/openyurtio/openyurt/issues/916))
- Improve unit test coverage of openyurtio/yurt-app-manager repo
  - [unit test] improve unit test coverage for yurtingress ([#76](https://github.com/openyurtio/yurt-app-manager/issues/76))
  - [unit test] improve unit test coverage for nodepool ([#73](https://github.com/openyurtio/yurt-app-manager/issues/73))
  - [unit test] improve unit test coverage for yurtappdaemon ([#75](https://github.com/openyurtio/yurt-app-manager/issues/75))
  - [unit test] improve unit test coverage for yurtappset ([#74](https://github.com/openyurtio/yurt-app-manager/issues/74))
- Rename UnitedDeployment to YurtAppSet ([#735](https://github.com/openyurtio/openyurt/issues/735))
- Update english version docs for homepage docs.

detail info: https://github.com/orgs/openyurtio/projects/6/views/1

**DataPlane SIG**

- support WireGuard backend ([#13](https://github.com/openyurtio/raven/issues/13))
- support kube-proxy ipvs mode ([#16](https://github.com/openyurtio/raven/issues/16))
- [feature request]support raven gateway to work in a high availability mode ([#39](https://github.com/openyurtio/raven/issues/39))
- make raven code unittest coverage over 50% ([#54](https://github.com/openyurtio/raven/issues/54))
- [feature request] Integrate codecov to evaluate test coverage ([#8](https://github.com/openyurtio/node-resource-manager/issues/8))

**IoT SIG**

- [feature request] add e2e test ([#39](https://github.com/openyurtio/yurt-device-controller/issues/39))
- [unit test] improve unit test coverage for yurt-device-controller ([#41](https://github.com/openyurtio/yurt-device-controller/issues/41))
- [feature request]add ci workflow for helm chart ([#42](https://github.com/openyurtio/yurt-device-controller/issues/42))
- Add ci workflow for helm chart ([#35](https://github.com/openyurtio/yurt-edgex-manager/issues/35))
- [unit test] improve unit test coverage for yurt-edgex-manager ([#39](https://github.com/openyurtio/yurt-edgex-manager/issues/39))

## v1.1 Roadmap

**ControlPlane SIG**

- Improve components crd naming convention and style ([#852](https://github.com/openyurtio/openyurt/issues/852))
- Improve service topology function when nodepool or service change ([#871](https://github.com/openyurtio/openyurt/issues/871))
- Support OTA/Auto update model for DaemonSet workload ([#914](https://github.com/openyurtio/openyurt/issues/914))

detail info: https://github.com/orgs/openyurtio/projects/7

**DataPlane SIG**

- Add unit tests ([#14](https://github.com/openyurtio/node-resource-manager/pull/14))

detail info: https://github.com/orgs/openyurtio/projects/8/views/1

**IoT SIG**

- [Feature] Add webhook for edgex ([#22](https://github.com/openyurtio/yurt-edgex-manager/issues/22))
- Update yurt-edgex-manager to support kubernetes 1.22+ ([#21](https://github.com/openyurtio/yurt-edgex-manager/issues/21))
- [unit test] improve unit test coverage for yurt-device-controller ([#41](https://github.com/openyurtio/yurt-device-controller/issues/41))
- Add webhook test case in e2e ([#34](https://github.com/openyurtio/yurt-edgex-manager/issues/34))

detail info: https://github.com/orgs/openyurtio/projects/4

## v1.2 Roadmap

**ControlPlane SIG**

- Provide NodePool Governance Capability
  - add yurt-coordinator-certificate controller ([#774](https://github.com/openyurtio/openyurt/issues/774))
  - add admission webhook ([#775](https://github.com/openyurtio/openyurt/issues/775))
  - remove nodelifecycle controller and add yurt-coordinator controller in yurt-controller-manager component ([#776](https://github.com/openyurtio/openyurt/issues/776))
  - add yurt-coordinator component ([#777](https://github.com/openyurtio/openyurt/issues/777))
  - yurthub are delegated to report heartbeats for nodes that disconnected with cloud ([#779](https://github.com/openyurtio/openyurt/issues/779))
- yurt-coordinator supports share pool scope data in the nodepool ([#778](https://github.com/openyurtio/openyurt/issues/778))
- Improve Yurtadm Join command ([#889](https://github.com/openyurtio/openyurt/issues/889))
- Improve Yurtadm Reset command ([#1058](https://github.com/openyurtio/openyurt/issues/1058))

detail info: https://github.com/orgs/openyurtio/projects/10

**DataPlane SIG**

- support SLB as public network exporter for gateway ([#22](https://github.com/openyurtio/raven/issues/22))
- add reconciliation loop to check route entries and vpn connections periodically. ([#10](https://github.com/openyurtio/raven/issues/10))
- [Raven-L7] Endpoints manager implementation ([#69](https://github.com/openyurtio/raven/issues/69))
- [Raven-L7] Raven l7 proxy implementation ([#70](https://github.com/openyurtio/raven/issues/70))
- [Raven-L7] DNS manager implementation ([#66](https://github.com/openyurtio/raven/issues/66))
- [Raven-L7] Cert manager implementation ([#67](https://github.com/openyurtio/raven/issues/67))
- support to use helm to deploy raven ([#73](https://github.com/openyurtio/raven/issues/73))

detail info: https://github.com/orgs/openyurtio/projects/9

**IoT SIG**

- [Design] Auto collect edgex deployment information and feed yurt-edgex-manager ([#24](https://github.com/openyurtio/yurt-edgex-manager/issues/24))
- [EdgeX Auto-Collector] Perform special processing on some of the edgex environment variables ([#63](https://github.com/openyurtio/yurt-edgex-manager/issues/63))
- [EdgeX Auto-Collector] Rewrite the controller part which read the edgex configuration section ([#62](https://github.com/openyurtio/yurt-edgex-manager/issues/62))
- [EdgeX Auto-Collector] Synchronize the images of edgex to openyurt ([#60](https://github.com/openyurtio/yurt-edgex-manager/issues/60))
- [EdgeX Auto-Collector] Collect volumes information about each component ([#61](https://github.com/openyurtio/yurt-edgex-manager/issues/61))
- [EdgeX Auto-Collector] Set the Auto-Collector to be triggered periodically ([#65](https://github.com/openyurtio/yurt-edgex-manager/issues/65))
- [EdgeX Auto-Collector] Modify CRD to give users the option to deploy a secure or insecure version of edgex ([#67](https://github.com/openyurtio/yurt-edgex-manager/issues/67))
- [EdgeX Auto-Collector] Upgrade apiVersion and deprecate additionalServices and additionalDeployments in new version ([#68](https://github.com/openyurtio/yurt-edgex-manager/issues/68))

detail info: https://github.com/orgs/openyurtio/projects/2

## v1.3 Roadmap (Release Plan: 2023.4)

**ControlPlane SIG**

- move scattered controllers into yurt-controller-manager ([#1067](https://github.com/openyurtio/openyurt/issues/1067))
- combine yurtctl tool into yurtadm tool ([#1059](https://github.com/openyurtio/openyurt/issues/1059))
- support OTA and Auto upgrade model for static pod ([#1079](https://github.com/openyurtio/openyurt/issues/1079))
- install yurthub component on edge nodes depending on StaticPod cr resource ([#1080](https://github.com/openyurtio/openyurt/issues/1080))
- support filter chain to mutate response data in YurtHub [#1188](https://github.com/openyurtio/openyurt/issues/1188)
- support NodePort service isolated for specified nodePool [#1183](https://github.com/openyurtio/openyurt/issues/1183)

detail info: https://github.com/orgs/openyurtio/projects/11

## Roadmap from 2023

Because of three SIGs(controlplane, dataplane, IoT) have been launched in openyurt community, so roadmap from 2023 for all SIGs will be managed in [openyurtio/community repo](https://github.com/openyurtio/community), and the link of roadmap is: https://github.com/openyurtio/community/blob/main/roadmap.md, and this file is only used for managing the stale roadmap.
