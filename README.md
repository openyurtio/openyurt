# openyurtio/openyurt

<div align="center">

<img src="docs/img/OpenYurt.png" width="400" height="94"><br/>

[![Version](https://img.shields.io/badge/OpenYurt-v1.4.0-orange)](CHANGELOG.md)
[![License](https://img.shields.io/badge/license-Apache%202-4EB1BA.svg)](https://www.apache.org/licenses/LICENSE-2.0.html)
[![Go Report Card](https://goreportcard.com/badge/github.com/openyurtio/openyurt)](https://goreportcard.com/report/github.com/openyurtio/openyurt)
[![codecov](https://codecov.io/gh/openyurtio/openyurt/branch/master/graph/badge.svg)](https://codecov.io/gh/openyurtio/openyurt)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/openyurtio/openyurt/badge)](https://api.securityscorecards.dev/projects/github.com/openyurtio/openyurt)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/7117/badge)](https://bestpractices.coreinfrastructure.org/projects/7117)
[![](https://img.shields.io/badge/OpenYurt-Check%20Your%20Contribution-orange)](https://opensource.alibaba.com/contribution_leaderboard/details?projectValue=openyurt)

</div>

English | [简体中文](./README.zh.md)

| ![notification](docs/img/bell-outline-badge.svg) What is NEW!                                           |
|---------------------------------------------------------------------------------------------------------|
| Latest Release: Nov 8th, 2023. OpenYurt v1.4.0. Please check the [CHANGELOG](CHANGELOG.md) for details. |
| First Release: May 29th, 2020. OpenYurt v0.1.0-beta.1                                                   |

[OpenYurt](https://openyurt.io) is built based on upstream Kubernetes and now hosted by the Cloud Native Computing Foundation(CNCF) as a [Sandbox Level Project](https://www.cncf.io/sandbox-projects/).

<div align="left">
  <img src="docs/img/overview.png" width=80% title="OpenYurt Overview ">
</div>

OpenYurt has been designed to meet various DevOps requirements against typical edge infrastructures.
It provides consistent user experience for managing the edge applications as if they were running in the cloud infrastructure.
It addresses specific challenges for cloud-edge orchestration in Kubernetes such as unreliable or disconnected cloud-edge networking,
edge autonomy, edge device management, region-aware deployment, and so on. OpenYurt preserves intact Kubernetes API compatibility,
is vendor agnostic, and more importantly, is **SIMPLE** to use.

## Architecture

OpenYurt follows a classic cloud-edge architecture design.
It uses a centralized Kubernetes control plane residing in the cloud site to
manage multiple edge nodes residing in the edge sites. Each edge node has moderate compute resources available in order to
run edge applications plus the required OpenYurt components. The edge nodes in a cluster can span
multiple physical regions, which are referred to as `Pools` in OpenYurt.

<div align="left">
  <img src="docs/img/arch.png" width=70% title="OpenYurt architecture">
</div>

The above figure demonstrates the core OpenYurt architecture. The major components consist of:

- **[YurtHub](https://openyurt.io/docs/next/core-concepts/yurthub)**: YurtHub runs on worker nodes as static pod and serves as a node sidecar to handle requests that comes from components (like Kubelet, Kubeproxy, etc.) on worker nodes to kube-apiserver.
- **[Yurt-Manager](https://github.com/openyurtio/openyurt/tree/master/cmd/yurt-manager)**: include all controllers and webhooks for edge.
- **[Raven-Agent](https://openyurt.io/docs/next/core-concepts/raven)**: It is focused on edge-edge and edge-cloud communication in OpenYurt, and provides layer 3 network connectivity among pods in different physical regions, as there are in one vanilla Kubernetes cluster.
- **Yurt-Coordinator**: One instance of Yurt-Coordinator is deployed in every edge NodePool, and in conjunction with YurtHub to provide heartbeat delegation, cloud-edge traffic multiplexing abilities, etc.
- **[YurtIoTDock](https://openyurt.io/docs/next/core-concepts/yurt-iot-dock)**: One instance of YurtIoTDock is deployed in every edge NodePool, for bridging EdgeX Foundry platform and uses Kubernetes CRD to manage edge devices.

In addition, OpenYurt also includes auxiliary controllers for integration and customization purposes.

- **[Node resource manager](https://openyurt.io/docs/next/core-concepts/node-resource-manager)**: It manages additional edge node resources such as LVM, QuotaPath and Persistent Memory.
  Please refer to [node-resource-manager](https://github.com/openyurtio/node-resource-manager) repo for more details.

## Getting started

OpenYurt supports Kubernetes versions up to 1.23. Using higher Kubernetes versions may cause
compatibility issues. OpenYurt installation is divided into two parts:

- [Install OpenYurt Control Plane Components](https://openyurt.io/docs/installation/summary#part-1-install-control-plane-components)
- [Join Nodes](https://openyurt.io/docs/installation/summary#part-2-join-nodes)

## Roadmap

- [OpenYurt Roadmap](https://github.com/openyurtio/community/blob/main/roadmap.md)

## Community

### Contributing

If you are willing to be a contributor for the OpenYurt project, please refer to our [CONTRIBUTING](CONTRIBUTING.md) document for details.
We have also prepared a developer [guide](https://openyurt.io/docs/developer-manuals/how-to-contribute) to help the code contributors.

### Meeting

| Item                               | Value                                                                                                                                                                                         |
| ---------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| APAC Friendly Community meeting    | [Adjust to weekly APAC (Starting May 11, 2022), Wednesday 11:00AM GMT+8](https://calendar.google.com/calendar/u/0?cid=c3VudDRtODc2Y2c3Ymk3anN0ZDdkbHViZzRAZ3JvdXAuY2FsZW5kYXIuZ29vZ2xlLmNvbQ) |
| Meeting link APAC Friendly meeting | https://us02web.zoom.us/j/82828315928?pwd=SVVxek01T2Z0SVYraktCcDV4RmZlUT09                                                                                                                    |
| Meeting notes                      | [Notes and agenda](https://www.yuque.com/rambohech/intck9/yolxrybw2rofcab7)                                                                                                                                    |
| Meeting recordings                 | [OpenYurt bilibili Channel](https://space.bilibili.com/484245424/video)                                                                                                                       |

### Contact

If you have any questions or want to contribute, you are welcome to communicate most things via GitHub issues or pull requests.
Other active communication channels:

- Mailing List: https://groups.google.com/g/openyurt/
- Slack: [OpenYurt channel](https://join.slack.com/t/openyurt/shared_invite/zt-2ajsy47br-jl~zjumRsCAE~BlPRRsIvg) (_English_)
- DingTalk：Search GroupID `12640034121` (_Chinese_)

<div align="left">
  <img src="docs/img/ding.jpg" width=25% title="dingtalk">
</div>

## License

OpenYurt is under the Apache 2.0 license. See the [LICENSE](LICENSE) file for details.
Certain implementations in OpenYurt rely on the existing code from Kubernetes and the credits go to the original Kubernetes authors.
