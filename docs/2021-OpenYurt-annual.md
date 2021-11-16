# OpenYurt Sandbox Annual Review

## Background

[OpenYurt](https://openyurt.io/en-us/) is an extension of the upstream Kubernetes for edge computing use cases.
It has been designed to meet various DevOps requirements against typical edge infrastructures. With OpenYurt,
users are able to manage their edge applications as if they were running in the cloud infrastructure.
It addresses specific challenges for cloud-edge orchestration in Kubernetes such as unreliable or disconnected
cloud-edge networking, edge node autonomy, edge device management, region-aware deployment and so on.
Currently, OpenYurt primarily provides the following capabilities:
- A node daemon which proxies the network traffics from Kubelet and/or other daemons so that node components still
  function even if the node is disconnected to the cloud.
- A tunnel service to allow the cloud site control plane to access the node components running in the edge nodes.
- A supplement to upstream controllers to deal with edge failures.
- A workload for managing edge applications across multiple edge regions.
- A networking solution to enable intra-region east-west service traffic.
- An integration with [EdgeX Foundry](https://github.com/edgexfoundry) to allow managing edge devices through Kubernetes.

### Alignment with CNCF

- OpenYurt was accepted as a CNCF Sandbox project on Sept 8, 2020.
- OpenYurt falls in the scope of CNCF Runtime SIG (the SIG presentation [recording](https://www.youtube.com/watch?v=L6X3Bx0e7B0&t=2183s)).

## Development metrics

The OpenYurt devstats page and dashboards can be found [here](https://openyurt.devstats.cncf.io/d/8/dashboards?orgId=1&refresh=15m&search=open).

- According to devstats, OpenYurt currently has [54](https://openyurt.devstats.cncf.io/d/22/prs-authors-table?orgId=1) contributors from [22](https://openyurt.devstats.cncf.io/d/5/companies-table?orgId=1)
companies. On average, there are [27 commits per month](https://openyurt.devstats.cncf.io/d/74/contributions-chart?orgId=1&var-period=m&var-metric=commits&var-repogroup_name=All&var-country_name=All&var-company_name=All&var-company=all&from=now-2y&to=now) contained within [20 merged PRs per month](https://openyurt.devstats.cncf.io/d/74/contributions-chart?orgId=1&var-period=m&var-metric=mergedprs&var-repogroup_name=All&var-country_name=All&var-company_name=All&var-company=all&from=now-2y&to=now).
- [New PRs in last year](https://openyurt.devstats.cncf.io/d/15/new-prs-in-repository-groups?orgId=1).
- The community has grown since the project entered the CNCF sandbox.
  - We held bi-weekly community meetings constantly (total 32 as of Nov 2021). The meeting records can be found in [here](https://search.bilibili.com/video?keyword=openyurt). The average number of  meeting attendees is ~30.
  - Number of contributors: 10+ -> **54**
  - Github stars: 200+ -> **1000+**
  - Github forks: 30+ -> **200+**
  - Contributing organizations: 2 -> **22**

## Maintainers

We have established [the community membership roadmap](https://github.com/openyurtio/community/blob/main/community-membership.md), and quite a few active and qualified contributors
have been promoted to maintainers. Compared to three maintainers (all from Alibaba) initially, we now have [eight maintainers](https://github.com/openyurtio/openyurt/blob/master/MAINTAINERS.md) from six organizations.
- Chao Zheng (ByteDance)
- Fei Guo (Alibaba)
- Lifang Zhang (China Telecom)
- Linbo He (Alibaba)
- Shida Qiu (Alibaba)
- Shaoqiang Chen (Intel)
- Tao Chen (ZheJiang University)
- Yixing Jia (VMware)

## Project adoption

OpenYurt has been adopted as the foundation of public cloud Kubernetes edge solutions. Many public services in Alibaba cloud,
CDN and IoT for example, have fully leveraged OpenYurt to manage their edge infrastructures spread across multiple regions.
Beyond the public cloud services, there are six other public adopters, out of which four use OpenYurt in production. They are:
- **China Merchants Group**: Using OpenYurt to manage edge nodes and applications installed in parking lots.
- **Sangfor Technologies**: Using OpenYurt for devOps leveraging its edge autonomy capability.
- **Onecloud**: Using OpenYurt to manage edge nodes across different regions and networks.
- **China Telecom**: Using OpenYurt to manage nodes across branch sites.

The other two organizations are actively evaluating OpenYurt for their edge cloud services.

## Project goals

Our primary goal is twofold: 1) make OpenYurt an easy-to-use and end-to-end framework to manage edge applications using
Kubernetes; 2) expand OpenYurt and integrate it with other successful edge open source projects to resolve the
pain points of using Kubernetes in edge computing. In the past year, we have achieved the following development outcomes.
- Release cadence: 5 minor releases, roughly once every three months. The latest release is v0.5.0.
- Key features added to the project (complete changelog can be found in [here](https://github.com/openyurtio/openyurt/blob/master/CHANGELOG.md)):
  - 0.2.0
    - Add YurtTunnel Support.
    - Complete YurtCtl CLI tool.
  - 0.3.0
    - Add YurtAppManager to support a new workload for edge applications.
    - Add NodePool support to manage multiple edge regions.
  - 0.4.0
    - Add edge node resource manager to manage edge local storages.
    - Support caching custom resources locally in edge nodes.
    - Support filtering cloud response data in edge nodes to accommodate special networking requirements.
  - 0.5.0
    - Integrate with EdgeX Foundry and provide an end-to-end solution to manage edge devices through Kubernetes custom resources.
    - Add Yurt-edgeX-manager and Yurt-device-manager to support the integration.

In the near future, we aim to achieve the following technical goals:
- Support ingress controller at NodePool level.
- Use YurtCluster CRD to manage openyurt components declaratively.
- Support edge workload autonomy.
- Improving user experiences.
- A complete project roadmap can be found in [here](https://github.com/openyurtio/openyurt/blob/master/docs/roadmap.md).

From the community collaboration perspective, we have built close connections with the EdgeX Foundry community for project integration. We have also made progress in
collaborating with the WasmEdge community and plan to support webassembly runtime in the future. We have organized a few offline meetups for OpenYurt community members and
sponsored a few academic programs (internship, programming contest etc) with Universities. We are looking for more user adoptions and community collaborations constantly.

## How the CNCF can help to achieve the upcoming goals

- More channels to advocate the project.
- More chances to collaborate with other projects in CNCF or even out of CNCF.
- Technical writing support for project documents.

## Incubation readiness
- Yes, we are preparing for the incubation proposal.
