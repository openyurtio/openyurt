# CHANGELOG

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
