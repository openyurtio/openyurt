# CHANGELOG

## v1.2.0

### What's New

**Two modes of edge autonomy have been provided**

The original edge autonomy feature can make the pods on nodes un-evicted even if node crashed by adding annotation to node.
After improving edge autonomy capability, two modes of edge autonomy are provided by adding annotation to workloads(like Deployment) as following:
- node edge autonomy: pods with node edge autonomy annotation will not be un-evicted even if node crashed.
- nodePool edge autonomy: when the reason of node NotReady is cloud-edge network off, pods will not be un-evicted, but pods will be evicted and recreated on other ready node in the nodePool if node crashed.

By the way, The original edge autonomy by annotating node will be kept in the next several versions, but node edge autonomy
will be recommended to replace the original way.

**Reduce the control-plane traffic between cloud and edge**

Based on the Pool-Coordinator in the nodePool, A leader Yurthub will be elected in the nodePool. Leader Yurthub will
list/watch pool-scope data(like endpoints/endpointslices) from cloud and write into pool-coordinator. then all components(like kube-proxy/coredns)
in the nodePool will get pool-scope data from pool-coordinator instead of cloud kube-apiserver, so large volume control-plane traffic
will be reduced.

**Use raven component to replace yurt-tunnel component**

Raven has released version v0.3, and provide cross-regional network communication ability based on PodIP or NodeIP, but yurt-tunnel
can only provide cloud-edge requests forwarding for kubectl logs/exec commands. because raven provides much more than the capabilities
provided by yurt-tunnel, and raven has been proven by a lot of work. so raven component is officially recommended to replace yurt-tunnel.

### Other Notable changes

- proposal of yurtadm join refactoring by @YTGhost in https://github.com/openyurtio/openyurt/pull/1048
- [Proposal] edgex auto-collector proposal by @LavenderQAQ in https://github.com/openyurtio/openyurt/pull/1051
- add timeout config in yurthub to handle those watch requests by @AndyEWang in https://github.com/openyurtio/openyurt/pull/1056
- refactor yurtadm join by @YTGhost in https://github.com/openyurtio/openyurt/pull/1049
- expose helm values for yurthub cacheagents by @huiwq1990 in https://github.com/openyurtio/openyurt/pull/1062
- refactor yurthub cache to adapt different storages by @Congrool in https://github.com/openyurtio/openyurt/pull/882
- add proposal of static pod upgrade model by @xavier-hou in https://github.com/openyurtio/openyurt/pull/1065
- refactor yurtadm reset by @YTGhost in https://github.com/openyurtio/openyurt/pull/1075
- bugfix: update the dependency yurt-app-manager-api from v0.18.8 to v0.6.0 by @YTGhost in https://github.com/openyurtio/openyurt/pull/1115
- Feature: yurtadm reset/join modification. Do not remove k8s binaries, add flag for using local cni binaries. by @Windrow14 in https://github.com/openyurtio/openyurt/pull/1124
- Improve certificate manager by @rambohe-ch in https://github.com/openyurtio/openyurt/pull/1133
- fix: update package dependencies by @fengshunli in https://github.com/openyurtio/openyurt/pull/1149
- fix: add common builder by @fengshunli in https://github.com/openyurtio/openyurt/pull/1152
- generate yurtadm docs by @huiwq1990 in https://github.com/openyurtio/openyurt/pull/1159
- add inclusterconfig filter for commenting kube-proxy configmap by @rambohe-ch in https://github.com/openyurtio/openyurt/pull/1158
- delete yurt tunnel helm charts by @River-sh in https://github.com/openyurtio/openyurt/pull/1161

### Fixes

- bugfix: StreamResponseFilter of data filter framework can't work if size of one object is over 32KB by @rambohe-ch in https://github.com/openyurtio/openyurt/pull/1066
- bugfix: add ignore preflight errors to adapt kubeadm before version 1.23.0 by @YTGhost in https://github.com/openyurtio/openyurt/pull/1092
- bugfix: dynamically switch apiVersion of JoinConfiguration to adapt to different versions of k8s by @YTGhost in https://github.com/openyurtio/openyurt/pull/1112
- bugfix: yurthub can not exit when SIGINT/SIGTERM happened by @rambohe-ch in https://github.com/openyurtio/openyurt/pull/1143

### Contributors

**Thank you to everyone who contributed to this release!** ❤

- [@YTGhost](https://github.com/YTGhost)
- [@Congrool](https://github.com/Congrool)
- [@LavenderQAQ](https://github.com/LavenderQAQ)
- [@AndyEWang](https://github.com/AndyEWang)
- [@huiwq1990](https://github.com/huiwq1990)
- [@rudolf-chy](https://github.com/rudolf-chy)
- [@xavier-hou](https://github.com/xavier-hou)
- [@gbtyy](https://github.com/gbtyy)
- [@huweihuang](https://github.com/huweihuang)
- [@zzguang](https://github.com/zzguang)
- [@Windrow14](https://github.com/Windrow14)
- [@fengshunli](https://github.com/fengshunli)
- [@gnunu](https://github.com/gnunu)
- [@luc99hen](https://github.com/luc99hen)
- [@donychen1134](https://github.com/donychen1134)
- [@LindaYu17](https://github.com/LindaYu17)
- [@fujitatomoya](https://github.com/fujitatomoya)
- [@River-sh](https://github.com/River-sh)
- [@rambohe-ch](https://github.com/rambohe-ch)

And thank you very much to everyone else not listed here who contributed in other ways like filing issues,
giving feedback, helping users in community group, etc.

## v1.1.0

### What's New

**Support OTA/Auto upgrade model for DaemonSet workload**

Extend native DaemonSet `OnDelete` upgrade model by providing OTA and Auto two upgrade models.
- OTA: workload owner can control the upgrade of workload through the exposed REST API on edge nodes.
- Auto: Solve the DaemonSet upgrade process blocking problem which caused by node NotReady when the cloud-edge is disconnected.

**Support autonomy feature validation in e2e tests**

In order to test autonomy feature, network interface of control-plane is disconnected for simulating cloud-edge network
disconnected, and then stop components(like kube-proxy, flannel, coredns, etc.) and check the recovery of these components.

**Improve the Yurthub configuration for enabling the data filter function**

Compares to the previous three configuration items, which include the component name, resource, and
request verb. after improvement, only component name is need to configure for enabling data filter function. the original
configuration format is also supported in order to keep consistency.

### Other Notable changes

- cache agent change optimize by @huiwq1990 in https://github.com/openyurtio/openyurt/pull/1008
- Check if error via ListKeys of Storage Interface. by @fujitatomoya in https://github.com/openyurtio/openyurt/pull/1015
- Add released openyurt versions to projectInfo when building binaries by @Congrool in https://github.com/openyurtio/openyurt/pull/1016
- add auto pod upgrade controller for daemoset by @xavier-hou in https://github.com/openyurtio/openyurt/pull/970
- add ota update RESTful API by @xavier-hou in https://github.com/openyurtio/openyurt/pull/1004
- make servicetopology filter in yurthub work properly when service or nodepool change by @LinFCai in https://github.com/openyurtio/openyurt/pull/1019
- improve data filter framework by @rambohe-ch in https://github.com/openyurtio/openyurt/pull/1025
- add proposal to unify cloud edge comms solution by @zzguang in https://github.com/openyurtio/openyurt/pull/1027
- improve health checker for adapting coordinator by @rambohe-ch in https://github.com/openyurtio/openyurt/pull/1032
- Edge-autonomy-e2e-test implementation by @lorrielau in https://github.com/openyurtio/openyurt/pull/1022
- improve e2e tests for supporting mac env and coredns autonomy by @rambohe-ch in https://github.com/openyurtio/openyurt/pull/1045
- proposal of yurthub cache refactoring by @Congrool in https://github.com/openyurtio/openyurt/pull/897

### Fixes

- even no endpoints left after filter, an empty object should be returned to clients by @rambohe-ch in https://github.com/openyurtio/openyurt/pull/1028
- non resource handle miss for coredns by @rambohe-ch in https://github.com/openyurtio/openyurt/pull/1044

### Contributors

**Thank you to everyone who contributed to this release!** ❤

- [@windydayc](https://github.com/windydayc)
- [@luc99hen](https://github.com/luc99hen)
- [@Congrool](https://github.com/Congrool)
- [@huiwq1990](https://github.com/huiwq1990)
- [@fujitatomoya](https://github.com/fujitatomoya)
- [@LinFCai](https://github.com/LinFCai)
- [@xavier-hou](https://github.com/xavier-hou)
- [@lorrielau](https://github.com/lorrielau)
- [@YTGhost](https://github.com/YTGhost)
- [@zzguang](https://github.com/zzguang)
- [@Lan-ce-lot](https://github.com/Lan-ce-lot)

And thank you very much to everyone else not listed here who contributed in other ways like filing issues,
giving feedback, helping users in community group, etc.

## v1.0

We're excited to announce the release of OpenYurt 1.0.0!🎉🎉🎉

Thanks to all the new and existing contributors who helped make this release happen!

If you're new to OpenYurt, feel free to browse [OpenYurt website](https://openyurt.io), then start with [OpenYurt Installation](https://openyurt.io/docs/installation/summary/) and learn about [its core concepts](https://openyurt.io/docs/core-concepts/architecture).

### Acknowledgements ❤️

Nearly 20 people have contributed to this release and 8 of them are new contributors, Thanks to everyone!

@huiwq1990 @Congrool @zhangzhenyuyu @rambohe-ch @gnunu @LinFCai @guoguodan @ankyit @luckymrwang @zzguang @hxcGit @Sodawyx
@luc99hen @River-sh @slm940208 @windydayc @lorrielau @fujitatomoya @donychen1134

### What's New

#### API version

The version of `NodePool` API has been upgraded to `v1beta1`, more details in the https://github.com/openyurtio/yurt-app-manager/pull/104

Meanwhile, all APIs management in OpenYurt will be migrated to [openyurtio/api](https://github.com/openyurtio/api) repo, and we recommend you
to import this package to use APIs of OpenYurt.

#### Code Quality

We track unit test coverage with [CodeCov](about.codecov.io)
Code coverage for some repos as following:
- openyurtio/openyurt: 47%
- openyurtio/yurt-app-manager: 37%
- openyurtio/raven: 53%

and more details of unit tests coverage can be found in https://codecov.io/gh/openyurtio

In addition to unit tests, other levels of testing are also added.
- upgrade e2e test for openyurt by @lorrielau in https://github.com/openyurtio/openyurt/pull/945
- add fuzz test for openyurtio/yurt-app-manager by @huiwq1990 in https://github.com/openyurtio/yurt-app-manager/pull/67
- e2e test for openyurtio/yurt-app-manager by @huiwq1990 in https://github.com/openyurtio/yurt-app-manager/pull/107

#### Performance Test

OpenYurt makes Kubernetes work in cloud-edge collaborative environment with a non-intrusive design. so performance of
some OpenYurt components have been considered carefully. several test reports have been submitted so that end users can clearly
see the working status of OpenYurt components.
- yurthub performance test report by @luc99hen in https://openyurt.io/docs/test-report/yurthub-performance-test
- pods recovery efficiency test report by @Sodawyx in https://openyurt.io/docs/test-report/pod-recover-efficiency-test

#### Installation Upgrade

early installation way(convert K8s to OpenYurt) is removed. OpenYurt Cluster installation is divided into two parts:
- [Install OpenYurt Control Plane Components](https://openyurt.io/docs/installation/summary#part-1-install-control-plane-components)
- [Join Nodes](https://openyurt.io/docs/installation/yurtadm-join)

and all Control Plane Components of OpenYurt are managed by helm charts in repo: https://github.com/openyurtio/openyurt-helm

### Other Notable changes

- upgrade kubeadm to 1.22 by @huiwq1990 in https://github.com/openyurtio/openyurt/pull/864
- [Proposal] Proposal to install openyurt components using helm by @zhangzhenyuyu in https://github.com/openyurtio/openyurt/pull/849
- support yurtadm token subcommand by @huiwq1990 in https://github.com/openyurtio/openyurt/pull/875
- bugfix: only set signer name when not nil in order to prevent panic. by @rambohe-ch in https://github.com/openyurtio/openyurt/pull/877
- [proposal] add proposal of multiplexing cloud-edge traffic by @rambohe-ch in https://github.com/openyurtio/openyurt/pull/804
- yurthub return fake token when edge node disconnected with K8s APIServer by @LinFCai in https://github.com/openyurtio/openyurt/pull/868
- deprecate cert-mgr-mode option of yurthub by @Congrool in https://github.com/openyurtio/openyurt/pull/901
- [Proposal] add proposal of daemosnet update model by @hxcGit in https://github.com/openyurtio/openyurt/pull/921
- fix: cache the server version info of kubernetes by @Sodawyx in https://github.com/openyurtio/openyurt/pull/936
- add yurt-tunnel-dns yaml by @rambohe-ch in https://github.com/openyurtio/openyurt/pull/956
- Separate YurtHubHost  & YurtHubProxyHost by @luc99hen in https://github.com/openyurtio/openyurt/pull/959
- merge endpoints filter into service topology filter by @rambohe-ch in https://github.com/openyurtio/openyurt/pull/963
- support yurtadm join to join multiple master nodes by @windydayc in https://github.com/openyurtio/openyurt/pull/964
- feature: add lantency metrics for yurthub by @luc99hen in https://github.com/openyurtio/openyurt/pull/965
- bump ginkgo to v2 by @lorrielau in https://github.com/openyurtio/openyurt/pull/945
- beta.kubernetes.io is deprecated, use kubernetes.io instead by @fujitatomoya in https://github.com/openyurtio/openyurt/pull/969

**Full Changelog**: https://github.com/openyurtio/openyurt/compare/v0.7.0...v1.0.0-rc1

Thanks again to all the contributors!

---
## v0.7.0

### What's New

**Raven: enable edge-edge and edge-cloud communication in a non-intrusive way**

Raven is component of the OpenYurt to enhance cluster networking capabilities. This enhancement is focused on edge-edge and edge-cloud communication in OpenYurt. It will provide layer 3 network connectivity among pods in different physical regions, as there are in one vanilla Kubernetes cluster.
More information can be found at: (([#637](https://github.com/openyurtio/openyurt/pull/637), [Raven](https://openyurt.io/docs/next/core-concepts/raven/), [@DrmagicE](https://github.com/DrmagicE), [@BSWANG](https://github.com/BSWANG), [@njucjc](https://github.com/njucjc))

**Support Kubernetes V1.22**

Enable OpenYurt can work on the Kubernetes v1.22, includes adapting API change(such as v1beta1.CSR deprecation), adapt StreamingProxyRedirects feature and handle v1.EndpointSlice in service topology and so on. More information can be
found at: ([#809](https://github.com/openyurtio/openyurt/pull/809), [#834](https://github.com/openyurtio/openyurt/pull/834), [@rambohe-ch](https://github.com/rambohe-ch), [@JameKeal](https://github.com/JameKeal), [@huiwq1990](https://github.com/huiwq1990))

**Support EdgeX Foundry V2.1**

Support EdgeX Foundry Jakarta version, and EdgeX Jakarta is the first LTS version and be widely considered as a production ready version. More information can be
found at: ([#4](https://github.com/openyurtio/yurt-edgex-manager/pull/4), [#30](https://github.com/openyurtio/yurt-device-controller/pull/30), [@lwmqwer](https://github.com/lwmqwer), [@wawlian](https://github.com/wawlian), [@qclc](https://github.com/qclc))

**Support IPv6 network in OpenYurt**

Support OpenYurt can run on the IPv6 network environment. More information can be found at: ([#842](https://github.com/openyurtio/openyurt/pull/842), [@tydra-wang](https://github.com/tydra-wang))

### Other Notable Changes

- add nodepool governance capability proposal ([#772](https://github.com/openyurtio/openyurt/pull/772), [@Peeknut](https://github.com/Peeknut))
- add proposal of multiplexing cloud-edge traffic ([#804](https://github.com/openyurtio/openyurt/pull/804), [@rambohe-ch](https://github.com/rambohe-ch))
- provide flannel image and cni binary for edge network ([#80](https://github.com/openyurtio/openyurt.io/pull/80), [@yingjianjian](https://github.com/yingjianjian))
- Remove convert and revert command from yurtctl ([#826](https://github.com/openyurtio/openyurt/pull/826), [@lonelyCZ](https://github.com/lonelyCZ))
- add tenant isolation for components such as kube-proxy&flannel which run in ns kube-system ([#787](https://github.com/openyurtio/openyurt/pull/787), [@YRXING](https://github.com/YRXING))
- Rename yurtctl init/join/reset to yurtadm init/join/reset ([#819](https://github.com/openyurtio/openyurt/pull/819), [@lonelyCZ](https://github.com/lonelyCZ))
- Use configmap to configure the data source of filter framework ([#749](https://github.com/openyurtio/openyurt/pull/749), [#790](https://github.com/openyurtio/openyurt/pull/790), [@yingjianjian](https://github.com/yingjianjian), [@rambohe-ch](https://github.com/rambohe-ch))
- add yurtctl test init cmd to setup OpenYurt cluster with kind ([#783](https://github.com/openyurtio/openyurt/pull/783), [@Congrool](https://github.com/Congrool))
- support local up openyurt on mac machine ([#836](https://github.com/openyurtio/openyurt/pull/836), [@rambohe-ch](https://github.com/rambohe-ch), [@Congrool](https://github.com/Congrool))
- cleanup: io/ioutil([#813](https://github.com/openyurtio/openyurt/pull/813), [@cndoit18](https://github.com/cndoit18))
- use verb %w with fmt.Errorf() when generate new wrapped error ([#832](https://github.com/openyurtio/openyurt/pull/832), [@zhaodiaoer](https://github.com/zhaodiaoer))
- decouple yurtctl with yurtadm ([#848](https://github.com/openyurtio/openyurt/pull/848), [@Congrool](https://github.com/Congrool))
- add enable-node-pool parameter for yurthub in order to disable nodepools list/watch in filters when testing ([#822](https://github.com/openyurtio/openyurt/pull/822), [@rambohe-ch](https://github.com/rambohe-ch))
- ingress: update edge ingress proposal to add enhancements ([#816](https://github.com/openyurtio/openyurt/pull/816), [@zzguang](https://github.com/zzguang))
- add configmap delete handler for approver ([#793](https://github.com/openyurtio/openyurt/pull/793), [@huiwq1990](https://github.com/huiwq1990))
- fix: a typo in yurtctl util.go which uses 'lable' as 'label' ([#784](https://github.com/openyurtio/openyurt/pull/784), [@donychen1134](https://github.com/donychen1134))

### Bug Fixes

- ungzip response by yurthub when response header contains content-encoding=gzip ([#794](https://github.com/openyurtio/openyurt/pull/794), [@rambohe-ch](https://github.com/rambohe-ch))
- fix mistaken selflink in yurthub ([#785](https://github.com/openyurtio/openyurt/pull/785), [@Congrool](https://github.com/Congrool))

---
## v0.6.0

### What's New

**Support YurtAppDaemon to deploy workload to different NodePools**

A YurtAppDaemon ensures that all (or some) NodePools run a copy of a Deployment or StatefulSet. As nodepools are added to the cluster,
Deployment or StatefulSet are added to them. As nodepools are removed from the cluster, those Deployments or StatefulSet are garbage collected.
The behavior of YurtAppDaemon is similar to that of DaemonSet, except that YurtAppDaemon creates workloads from a node pool.
More information can be found at: ([#422](https://github.com/openyurtio/openyurt/pull/422), [yurt-app-manager](https://github.com/openyurtio/yurt-app-manager), [@kadisi](https://github.com/kadisi))

**Using YurtIngress to unify service across NodePools**

YurtIngress acts as a unified interface for services access request from outside the NodePool, it abstracts and simplifies
service access logic to users, it also reduces the complexity of NodePool services management. More information can be
found at: ([#373](https://github.com/openyurtio/openyurt/pull/373), [#645](https://github.com/openyurtio/openyurt/pull/645), [yurt-app-manager](https://github.com/openyurtio/yurt-app-manager), [@zzguang](https://github.com/zzguang), [@gnunu](https://github.com/gnunu), [@LindaYu17](https://github.com/LindaYu17))

**Improve the user experience of OpenYurt**

- OpenYurt Experience Center

New users who want to try out OpenYurt's capabilities do not need to install an OpenYurt cluster from scratch.
They can apply for a test account on the OpenYurt Experience Center and immediately have an OpenYurt cluster available.
More information can be found at: ([OpenYurt Experience Center Introduction](https://openyurt.io/zh/docs/next/installation/openyurt-experience-center/overview/), [@luc99hen](https://github.com/luc99hen), [@qclc](https://github.com/qclc), [@Peeknut](https://github.com/Peeknut))

- YurtCluster

This YurtCluster Operator is to translate a vanilla Kubernetes cluster into an OpenYurt cluster, through a simple API (YurtCluster CRD).
And we recommend that you do the cluster conversion based on the declarative API of YurtCluster Operator. More information can be
found at: ([#389](https://github.com/openyurtio/openyurt/pull/389), [#518](https://github.com/openyurtio/openyurt/pull/518), [yurtcluster-operator](https://github.com/openyurtio/yurtcluster-operator), [@SataQiu](https://github.com/SataQiu), [@gnunu](https://github.com/gnunu))

- Yurtctl init/join

In order to improve efficiency of creating OpenYurt cluster, a new tool named [sealer](https://github.com/alibaba/sealer) has
been integrated into `yurtctl init` command. and OpenYurt cluster image(based on Kubernetes v1.19.7 version) has been prepared.
Users can use `yurtctl init` command to create OpenYurt cluster control-plane, and use `yurtctl join` to add worker nodes(including
cloud nodes and edge nodes). More information can be found at: ([#704](https://github.com/openyurtio/openyurt/pull/704), [#697](https://github.com/openyurtio/openyurt/pull/697), [@Peeknut](https://github.com/Peeknut), [@rambohe-ch](https://github.com/rambohe-ch), [@adamzhoul](https://github.com/adamzhoul))

**Update docs of OpenYurt**

The docs of OpenYurt installation, core concepts, user manuals, developer manuals etc. have been updated, and all of them are located at [OpenYurt Docs](https://openyurt.io/zh/docs/next/).
Thanks to all contributors for maintaining docs for OpenYurt. ([@huangyuqi](https://github.com/huangyuqi), [@kadisi](https://github.com/kadisi), [@luc99hen](https://github.com/orgs/openyurtio/people/luc99hen), [@SataQiu](https://github.com/SataQiu), [@mowangdk](https://github.com/orgs/openyurtio/people/mowangdk), [@rambohe-ch](https://github.com/rambohe-ch), [@zyjhtangtang](https://github.com/zyjhtangtang), [@qclc](https://github.com/qclc), [@Peeknut](https://github.com/Peeknut), [@Congrool](https://github.com/Congrool), [@zzguang](https://github.com/zzguang), [@adamzhoul](https://github.com/adamzhoul), [@windydayc](https://github.com/windydayc), [@villanel](https://github.com/villanel))

### Other Notable Changes

- Proposal: enhance cluster networking capabilities ([#637](https://github.com/openyurtio/openyurt/pull/637), [@DrmagicE](https://github.com/DrmagicE), [@BSWANG](https://github.com/BSWANG))
- add node-servant ([#516](https://github.com/openyurtio/openyurt/pull/516), [adamzhoul](https://github.com/adamzhoul))
- improve yurt-tunnel-server to automatically update server certificates when service address changed ([#525](https://github.com/openyurtio/openyurt/pull/525), [@YRXING](https://github.com/YRXING))
- automatically clean dummy interface and iptables rule when yurthub is stopped by k8s ([#530](https://github.com/openyurtio/openyurt/pull/530), [@Congrool](https://github.com/Congrool))
- enhancement: add openyurt.io/skip-discard annotation verify for discardcloudservice filter ([#524](https://github.com/openyurtio/openyurt/pull/542), [@rambohe-ch](https://github.com/rambohe-ch))
- inject working_mode ([#552](https://github.com/openyurtio/openyurt/pull/552), [@ngau66](https://github.com/ngau66))
- Yurtctl revert adds the function of deleting yurt app manager ([#555](https://github.com/openyurtio/openyurt/pull/555), [@yanyhui](https://github.com/yanyhui))
- Add edge device demo in the README.md ([#553](https://github.com/openyurtio/openyurt/pull/553), [#554](https://github.com/openyurtio/openyurt/pull/554), [@Fei-Guo](https://github.com/Fei-Guo), [@qclc](https://github.com/qclc), [@lwmqwer](https://github.com/lwmqwer))
- Refactor: separate the creation of informers from tunnel server component ([#585](https://github.com/openyurtio/openyurt/pull/585), [@YRXING](https://github.com/YRXING))
- add make push for pushing images generated during make release ([#601](https://github.com/openyurtio/openyurt/pull/601), [@gnunu](https://github.com/gnunu))
- add trafficforward that contains two diversion modes: DNAT and DNS ([#606](https://github.com/openyurtio/openyurt/pull/606), [@JcJinChen](https://github.com/JcJinChen))
- yurthub verify bootstrap ca on start ([#631](https://github.com/openyurtio/openyurt/pull/631), [@gnunu](https://github.com/gnunu))
- deprecate kubelet certificate management mode ([#639](https://github.com/openyurtio/openyurt/pull/639), [@qclc](https://github.com/qclc))
- remove k8s.io/kubernetes dependency from OpenYurt ([#650](https://github.com/openyurtio/openyurt/pull/650), [#664](https://github.com/openyurtio/openyurt/pull/664), [#681](https://github.com/openyurtio/openyurt/pull/681), [#697](https://github.com/openyurtio/openyurt/pull/697), [#704](https://github.com/openyurtio/openyurt/pull/704), [@rambohe-ch](https://github.com/rambohe-ch), [@qclc](https://github.com/qclc), [@Peeknut](https://github.com/Peeknut), [@Rachel-Shao](https://github.com/Rachel-Shao))
- add unit tests for yurthub data filtering framework ([#670](https://github.com/openyurtio/openyurt/pull/670), [@windydayc](https://github.com/windydayc))
- enable yurthub to handle upgrade request ([#673](https://github.com/openyurtio/openyurt/pull/673), [@Congrool](https://github.com/Congrool))
- Yurtctl: add precheck for reducing convert failure ([#675](https://github.com/openyurtio/openyurt/pull/675), [@Peeknut](https://github.com/Peeknut))
- ingress: add nodepool endpoints filtering for nginx ingress controller ([#696](https://github.com/openyurtio/openyurt/pull/696), [@zzguang](https://github.com/zzguang))

### Bug Fixes

- fix some bugs when local up openyurt  ([#517](https://github.com/openyurtio/openyurt/pull/517), [@Congrool](https://github.com/Congrool))
- reject delete pod request by yurthub when cloud-edge network disconnected ([#593](https://github.com/openyurtio/openyurt/pull/593), [@rambohe-ch](https://github.com/rambohe-ch))
- service topology filter can not work when hub agent work on cloud mode ([#607](https://github.com/openyurtio/openyurt/pull/607), [@rambohe-ch](https://github.com/rambohe-ch))
- fix transport race conditions in yurthub ([#683](https://github.com/openyurtio/openyurt/pull/683), [@rambohe-ch](https://github.com/rambohe-ch), [@DrmagicE](https://github.com/DrmagicE))

---

## v0.5.0

### What's New

**Manage EdgeX Foundry system in OpenYurt in a cloud-native, non-intrusive way**

- yurt-edgex-manager

  Yurt-edgex-manager enable OpenYurt to be able to manage the EdgeX lifecycle. Each EdgeX CR (Custom Resource) stands for an EdgeX instance.
Users can deploy/update/delete EdgeX in OpenYurt cluster by operate the EdgeX CR directly. ([yurt-edgex-manager](https://github.com/openyurtio/yurt-edgex-manager), [@yixingjia](https://github.com/yixingjia), [@lwmqwer](https://github.com/lwmqwer))

- yurt-device-controller

  Yurt-device-controller aims to provider device management functionalities to OpenYurt cluster by integrating with edge computing/IOT platforms, like EdgeX in a cloud native way.
It will automatically synchronize the device status to device CR (custom resource) in the cloud and any update to the device will pass through to the edge side seamlessly. More information can be found at: ([yurt-device-controller](https://github.com/openyurtio/yurt-device-controller), [@charleszheng44](https://github.com/charleszheng44), [@qclc](https://github.com/qclc), [@Peeknut](https://github.com/Peeknut), [@rambohe-ch](https://github.com/rambohe-ch), [@yixingjia](https://github.com/yixingjia))

**Yurt-tunnel support more flexible settings for forwarding requests from cloud to edge**

- Support forward https request from cloud to edge for components(like prometheus) on the cloud nodes can access the https service(like node-exporter) on the edge node.
Please refer to the details: ([#442](https://github.com/openyurtio/openyurt/pull/442), [@rambohe-ch](https://github.com/rambohe-ch), [@Fei-Guo](https://github.com/Fei-Guo), [@DrmagicE](https://github.com/DrmagicE), [@SataQiu](https://github.com/SataQiu))

- support forward cloud requests to edge node's localhost endpoint for components(like prometheus) on cloud nodes collect edge components(like yurthub) metrics(http://127.0.0.1:10267).
  Please refer to the details: ([#443](https://github.com/openyurtio/openyurt/pull/443), [@rambohe-ch](https://github.com/rambohe-ch), [@Fei-Guo](https://github.com/Fei-Guo))

### Other Notable Changes
- Proposal: YurtAppDaemon ([#422](https://github.com/openyurtio/openyurt/pull/422), [@kadisi](https://github.com/kadisi), [@zzguang](https://github.com/zzguang), [@gnunu](https://github.com/gnunu), [@Fei-Guo](https://github.com/Fei-Guo), [@rambohe-ch](https://github.com/rambohe-ch))
- update yurtcluster operator proposal ([#429](https://github.com/openyurtio/openyurt/pull/429), [@SataQiu](https://github.com/SataQiu))
- yurthub use original bearer token to forward requests for inclusterconfig pods ([#437](https://github.com/openyurtio/openyurt/pull/437), [@rambohe-ch](https://github.com/rambohe-ch))
- yurthub support working on cloud nodes ([#483](https://github.com/openyurtio/openyurt/pull/483) and [#495](https://github.com/openyurtio/openyurt/pull/495), [@DrmagicE](https://github.com/DrmagicE))
- support discard cloud service(like LoadBalancer service) on edge side ([#440](https://github.com/openyurtio/openyurt/pull/440), [@rambohe-ch](https://github.com/rambohe-ch), [@Fei-Guo](https://github.com/Fei-Guo))
- improve list/watch node pool resource in serviceTopology filter ([#454](https://github.com/openyurtio/openyurt/pull/454), [@neo502721](https://github.com/neo502721))
- add configmap to configure user agents for specify response cache. ([#466](https://github.com/openyurtio/openyurt/pull/466),  [@rambohe-ch](https://github.com/rambohe-ch))
- improve the usage of certificates ([#475](https://github.com/openyurtio/openyurt/pull/475), [@ke-jobs](https://github.com/ke-jobs))
- improve unit tests for yurt-tunnel. ([#470](https://github.com/openyurtio/openyurt/pull/470), [@YRXING](https://github.com/YRXING))
- stop renew node lease when kubelet's heartbeat is stopped ([#482](https://github.com/openyurtio/openyurt/pull/482), [@SataQiu](https://github.com/SataQiu))
- add check-license-header script to github action ([#487](https://github.com/openyurtio/openyurt/pull/487), [@lonelyCZ](https://github.com/lonelyCZ))
- Add tunnel server address for converting ([#494](https://github.com/openyurtio/openyurt/pull/494), [adamzhoul](https://github.com/adamzhoul))

### Bug Fixes

- fix incomplete data copy of resource filter  ([#452](https://github.com/openyurtio/openyurt/pull/452), [@SataQiu](https://github.com/SataQiu))
- remove excess chan that will block the program ([#446](https://github.com/openyurtio/openyurt/pull/446), [@zc2638](https://github.com/zc2638))
- use buffered channel for signal notifications ([#471](https://github.com/openyurtio/openyurt/pull/471), [@SataQiu](https://github.com/SataQiu))
- add create to yurt-tunnel-server ClusterRole ([#500](https://github.com/openyurtio/openyurt/pull/500), [adamzhoul](https://github.com/adamzhoul))

---

## v0.4.1

### What's New

**Join or Reset node in one step**

In order to enable users to use OpenYurt clusters quickly and reduce the cost of users learning OpenYurt, yurtctl has provided the subcommands `convert` and `revert`, to implement the conversion between OpenYurt cluster and Kubernetes cluster. However, It still has some shortcomings:
- No support for new nodes to join the cluster directly;
- Users are required to pre-built a kubernetes cluster, and then do conversion, which has a relatively high learning cost for beginners.

So, we need to add subcommands `init`, `join`, `reset` for yurtctl. and `join` and `reset` subcommands can be used in v0.4.1, and `init` subcommand will comes in next version.
Please refer to the [proposal doc](https://github.com/openyurtio/openyurt/blob/master/docs/proposals/20210607-adding-subcommands-server-join-reset-for-yurtctl.md) and [usage doc](https://github.com/openyurtio/openyurt/blob/master/docs/tutorial/yurtctl.md#join-edge-nodecloud-node-to-openyurt) for details. ([#387](https://github.com/openyurtio/openyurt/pull/387), [#402](https://github.com/openyurtio/openyurt/pull/402), [@zyjhtangtang](https://github.com/zyjhtangtang))

**Support Pods use `InClusterConfig` access kube-apiserver through yurthub**

Many users in OpenYurt community have requested that support InClusterConfig for pods to access kube-apiserver through yurthub on edge nodes. so pods on cloud can move to edge cluster smoothly. so we add the following features.
-  yurthub supports https serve.
- env `KUBERNETES_SERVICE_HOST` and `KUBERNETES_SERVICE_PORT` should be the address that yurthub listening, like `169.254.2.1:10261`

Please refer to the issue [#372](https://github.com/openyurtio/openyurt/issues/372) for details. ([#386](https://github.com/openyurtio/openyurt/pull/386), [#394](https://github.com/openyurtio/openyurt/pull/394), [@luckymrwang](https://github.com/luckymrwang), [@rambohe-ch](https://github.com/rambohe-ch))

**Support filter cloud response data on edge side**

In the cloud-edge scenario, In response to requests from edge components (such as kube-proxy) or user pods to the cloud, it is hoped that some customized processing can be performed on the data returned by the cloud, we consider providing a generic data filtering framework in the yurthub component, which can customize the data returned from the cloud without being aware of the edge components and user pods, so as to meet business needs simply and conveniently. And provides two specific filtering handlers.

1. support endpointslice filter for keeping service traffic in-bound of nodePool
2. support master service mutation for pod use InClusterConfig access kube-apiserver

Please refer to the [Proposal doc](https://github.com/openyurtio/openyurt/blob/master/docs/proposals/20210720-data-filtering-framework.md) for details. ([#388](https://github.com/openyurtio/openyurt/pull/388), [#394](https://github.com/openyurtio/openyurt/pull/394), [@rambohe-ch](https://github.com/rambohe-ch), [@Fei-Guo](https://github.com/Fei-Guo))

### Other Notable Changes
- Yurtctl modify kube-controllersetting to close the nodelifecycle-controller ([#399](https://github.com/openyurtio/openyurt/pull/399), [@Peeknut](https://github.com/Peeknut))
- Proposal: EdgeX integration with OpenYurt ([#357](https://github.com/openyurtio/openyurt/pull/357), [@yixingjia](https://github.com/yixingjia), [@lwmqwer](https://github.com/lwmqwer))
- Proposal: add ingress feature support to nodepool ([#373](https://github.com/openyurtio/openyurt/pull/373), [@zzguang](https://github.com/zzguang), [@wenjun93](https://github.com/wenjun93))
- Proposal: OpenYurt Convertor Operator for converting K8S to OpenYurt ([#389](https://github.com/openyurtio/openyurt/pull/389), [@gnunu](https://github.com/gnunu))
- add traffic(from cloud to edge) collector metrics for yurthub ([#398](https://github.com/openyurtio/openyurt/pull/398),  [@rambohe-ch](https://github.com/rambohe-ch))
- Add sync.Pool to cache *bufio.Reader in tunnel server ([#381](https://github.com/openyurtio/openyurt/pull/381), [@DrmagicE](https://github.com/DrmagicE))
- improve tunnel availability ([#375](https://github.com/openyurtio/openyurt/pull/375), [@aholic](https://github.com/aholic))
- yurtctl adds parameter enable app manager to control automatic deployment of yurtappmanager. ([#352](https://github.com/openyurtio/openyurt/pull/352), [@yanhui](https://github.com/yanyhui))

### Bug Fixes

- fix tunnel-agent/tunnel-server crashes when the local certificate can not be loaded correctly ([#378](https://github.com/openyurtio/openyurt/pull/378), [@SataQiu](https://github.com/SataQiu))
- fix the error when cert-mgr-mode set to kubelet ([#359](https://github.com/openyurtio/openyurt/pull/359), [@qclc](https://github.com/qclc))
- fix the same prefix key lock error ([#396](https://github.com/openyurtio/openyurt/pull/396), [@rambohe-ch](https://github.com/rambohe-ch))
---

## v0.4.0

### What's New

**Node Resource Manager Released**

Node resource manager is released in this version, which provides local node resources management of OpenYurt cluster in a unified manner.
It currently supports LVM, QuotaPath and Pmem, and create or update the compute, storage resources based on local devices.
It works as daemonset spread on each edge node, and manages local resources with a predefined spec stored in configmap.
Please refer to the [usage doc](https://github.com/openyurtio/node-resource-manager/blob/main/docs/configmap.md) for details.
([#1](https://github.com/openyurtio/node-resource-manager/pull/1), [@mowangdk](https://github.com/mowangdk), [@wenjun93](https://github.com/wenjun93))

**Add Cloud Native IOT Device Management API definition**

Inspiring by the Unix philosophy, "Do one thing and do it well", we believe that Kubernetes should focus on managing computing resources
while edge devices management can be done by adopting existing edge computing platforms. Therefore, we define several generic
custom resource definitions(CRD) that act as the mediator between OpenYurt and the edge platform.
Any existing edge platforms can be integrated into the OpenYurt by implementing custom controllers for these CRDs.
In addition, these CRDs allow users to manage edge devices in a declarative way, which provides users with a Kubernetes-native experience.([#233](https://github.com/openyurtio/openyurt/pull/233), [#236](https://github.com/openyurtio/openyurt/pull/236), [@Fei-Guo](https://github.com/Fei-Guo), [@yixingjia](https://github.com/yixingjia), [@charleszheng44](https://github.com/charleszheng44))

**Kubernetes V1.18 is supported**

OpenYurt officially supports version v1.18 of Kubernetes.
Now, OpenYurt users are able to convert v1.18 Kubernetes cluster to OpenYurt cluster or
deploy components of OpenYurt on v1.18 Kubernetes cluster manually. the main work for supporting v1.18 Kubernetes as following:
  1. refactor serializer of cache manager to adapt ClientNegotiator in k8s.io/apimachinery v0.18
  2. add context parameter in api for calling client-go v0.18

and based on Kubernetes compatibility, v1.16 Kubernetes is still supported. ([#288](https://github.com/openyurtio/openyurt/pull/288), [@rambohe-ch](https://github.com/rambohe-ch))

**UnitedDeployment support patch for pool**

UnitedDeployment controller provides a new way to manage pods in multi-pool by using multiple workloads.
Each workload managed by UnitedDeployment is called a pool and user can only configure the workload replicas in the pool.
Based on the patch feature, besides the workload replicas configuration, user can easily configure other fields(like images and
resoures) of workloads in the pool.([#242](https://github.com/openyurtio/openyurt/issues/242), [#12](https://github.com/openyurtio/yurt-app-manager/pull/12), [@kadisi](https://github.com/kadisi))

**Support caching CRD resources by yurthub**

Because resources in the resourceToKindMap can be cached by yurt-hub component, when network between cloud and edge disconnected,
if any pod(eg: calico) on the edge node that used some resources(like crd) not in the above map want to run continuously,
that is to say, the pod can not restarted successfully because resources(like crd) are not cached by yurt-hub.
This PR can solve this limitation. Now yurt-hub is able to cache all kubernetes resources, including crd resource that defined by user.([#162](https://github.com/openyurtio/openyurt/issues/162), [#231](https://github.com/openyurtio/openyurt/pull/231), [#225](https://github.com/openyurtio/openyurt/pull/225), [#265](https://github.com/openyurtio/openyurt/pull/265), [@qclc](https://github.com/qclc), [@rambohe-ch](https://github.com/rambohe-ch))

**Prometheus and Yurt-Tunnel-Server cross-node deployment is supported via DNS**

In the edge computing scenario, the IP addresses of the edge nodes are likely to be the same. So we can not rely on the node IP
to forward the request but should use the node hostname(unique in one cluster).
This PR provides the ability for the yurt-tunnel-server to handle requests in the form of scheme://[hostname]:[port]/[req_path].([#270](https://github.com/openyurtio/openyurt/pull/270), [#284](https://github.com/openyurtio/openyurt/pull/284), [@SataQiu](https://github.com/SataQiu), [@rambohe-ch](https://github.com/rambohe-ch))

**Support kind cluster and node level conversion by yurtctl**

OpenYurt supports the conversion between the OpenYurt cluster and the Kubernetes cluster created by minikube, kubeadm, and ack.
Now OpenYurt supports the conversion between kind cluster and OpenYurt cluster.
([#230](https://github.com/openyurtio/openyurt/issues/230), [#206](https://github.com/openyurtio/openyurt/pull/206), [#220](https://github.com/openyurtio/openyurt/pull/220), [#234](https://github.com/openyurtio/openyurt/pull/234), [@Peeknut](https://github.com/Peeknut))

### Other Notable Changes
- add edge-pod-network doc ([#302](https://github.com/openyurtio/openyurt/pull/302), [@wenjun93](https://github.com/wenjun93))
- add resource and system requirements ([#315](https://github.com/openyurtio/openyurt/pull/315), [@wawlian](https://github.com/wawlian))
- feature: add dummy network interface for yurthub ([#289](https://github.com/openyurtio/openyurt/pull/289), [@rambohe-ch](https://github.com/rambohe-ch))
- refactor: divide the yurthubServer into hubServer and proxyServert ([#237](https://github.com/openyurtio/openyurt/pull/237), [@rambohe-ch](https://github.com/rambohe-ch))
- using lease for cluster remote server healthz checker. ([#249](https://github.com/openyurtio/openyurt/pull/249), [@zyjhtangtang](https://github.com/zyjhtangtang))
- yurtctl cluster-info subcommand that list edge/cloud nodes ([#208](https://github.com/openyurtio/openyurt/pull/208), [@neo502721](https://github.com/neo502721))
- Feature: Support specified kubeadm conf for join cluster ([#210](https://github.com/openyurtio/openyurt/pull/210), [@liangyuanpeng](https://github.com/liangyuanpeng))
- Add --feature-gate flag to yurt-controller-manager ([#222](https://github.com/openyurtio/openyurt/pull/222), [@DrmagicE](https://github.com/DrmagicE))
- feature: add promtheus metrics for yurthub ([#238](https://github.com/openyurtio/openyurt/pull/238), [@rambohe-ch](https://github.com/rambohe-ch))
- Update the Manual document ([#228](https://github.com/openyurtio/openyurt/pull/228), [@yixingjia](https://github.com/yixingjia))
- feature: add meta server for handling prometheus metrics and pprof by yurttunnel ([#253](https://github.com/openyurtio/openyurt/pull/253), [@rambohe-ch](https://github.com/rambohe-ch))
- feature: add 'yurthub-healthcheck-timeout' flag for 'yurtctl convert' command ([#290](https://github.com/openyurtio/openyurt/pull/290), [@SataQiu](https://github.com/SataQiu))

### Bug Fixes

- fix list runtimeclass and csidriver from cache error when cloud-edge network disconnected ([#258](https://github.com/openyurtio/openyurt/pull/258), [@rambohe-ch](https://github.com/rambohe-ch))
- fix the error of cluster status duration statistics ([#295](https://github.com/openyurtio/openyurt/pull/295), [@zyjhtangtang](https://github.com/zyjhtangtang))
- fix bug when ServeHTTP panic ([#198](https://github.com/openyurtio/openyurt/pull/198), [@aholic](https://github.com/aholic))
- solve ips repeated question from addr.go ([#209](https://github.com/openyurtio/openyurt/pull/209), [@luhaopei](https://github.com/luhaopei))
- fix t.Fatalf from a non-test goroutine ([#269](https://github.com/openyurtio/openyurt/pull/269), [@contrun](https://github.com/contrun))
- Uniform label for installation of yurt-tunnel-agent openyurt.io/is-edge-worker=true ([#275](https://github.com/openyurtio/openyurt/pull/275), [@yanhui](https://github.com/yanyhui))
- fix the bug that dns controller updates dns records incorrectly ([#283](https://github.com/openyurtio/openyurt/pull/283), [@SataQiu](https://github.com/SataQiu))
- It solves the problem that the cloud node configuration taint ([#299](https://github.com/openyurtio/openyurt/pull/299), [@yanhui](https://github.com/yanyhui))
- Fixed incorrect representation in code comments. ([#296](https://github.com/openyurtio/openyurt/pull/296), [@felix0080](https://github.com/felix0080))
- fix systemctl restart in manually-setup tutorial ([#205](https://github.com/openyurtio/openyurt/pull/205), [@DrmagicE](https://github.com/DrmagicE))
---

## v0.3.0

### Project

- Add new component Yurt App Manager that runs on cloud nodes
- Add new provider=kubeadm for yurtctl
- Add hubself certificate mode for yurthub
- Support log flush for yurt-tunnel
- New tutorials for Yurt App Manager

### yurt-app-manager

- Implement NodePool CRD that provides a convenient management experience for a pool of nodes within the same region or site
- Implement UnitedDeployment CRD by defining a new edge application management methodology of using per node pool workload
- Add tutorials to use Yurt App Manager

### yurthub

- Add hubself certificate mode for generating and rotating certificate that used to connect with kube-apiserver as default mode
- Add timeout mechanism for proxying watch request
- Optimize the response when cache data is not found

### yurt-tunnel

- Add integration test
- Support log flush request from kube-apiserver
- Optimize tunnel interceptor for separating context dailer and proxy request
- Optimize the usage of sharedIndexInformer

### yurtctl

- Add new provider=kubeadm that kubernetes cluster installed by kubeadm can be converted to openyurt cluster
- Adapt new certificate mode of yurthub when convert edge node
- Fix image pull policy from `Always` to `IfNotPresent` for all components deployment setting

---

## v0.2.0

### Project

- Support Kubernetes 1.16 dependency for all components
- Support multi-arch binaries and images (arm/arm64/amd64)
- Add e2e test framework and tests for node autonomy
- New tutorials (e2e test and yurt-tunnel)

### yurt-tunnel

#### Features

- Implement yurt-tunnel-server and yurt-tunnel-agent based on Kubernetes apiserver network proxy framework
- Implement cert-manager to manage yurt-tunnel certificates
- Add timeout mechanism for yurt-tunnel

### yurtctl

#### Features

- Add global lock to prevent multiple yurtctl invocations concurrently
- Add timeout for acquiring global lock
- Allow user to set the label prefix used to identify edge nodes
- Deploy yurt-tunnel using convert option

#### Bugs

- Remove kubelet config bootstrap args during manual setup

---

## v0.1.0-beta.1

### yurt-controller-manager

#### Features

- Avoid evicting Pods from nodes that have been marked as `autonomy` nodes

### yurthub

#### Features

- Use Kubelet certificate to communicate with APIServer
- Implement a http proxy for all Kubelet to APIServer requests
- Cache the responses of Kubelet to APIServer requests in local storage
- Monitor network connectivity and switch to offline mode if health check fails
- In offline mode, response Kubelet to APIServer requests based on the cached states
- Resync and clean up the states once node is online again
- Support to proxy for other node daemons

### yurtctl

#### Features

- Support install/uninstall all OpenYurt components in a native Kubernetes cluster
- Pre-installation validation check
