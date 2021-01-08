# v0.3.0

## Project

- Add new component Yurt App Manager that runs on cloud nodes
- Add new provider=kubeadm for yurtctl
- Add hubself certificate mode for yurthub
- Support log flush for yurt-tunnel
- New tutorials for Yurt App Manager

## yurt-app-manager

- Implement NodePool CRD that provides a convenient management experience for a pool of nodes within the same region or site
- Implement UnitedDeployment CRD by defining a new edge application management methodology of using per node pool workload
- Add tutorials to use Yurt App Manager

## yurthub

- Add hubself certificate mode for generating and rotating certificate that used to connect with kube-apiserver as default mode
- Add timeout mechanism for proxying watch request
- Optimize the response when cache data is not found

## yurt-tunnel

- Add integration test
- Support log flush request from kube-apiserver
- Optimize tunnel interceptor for separating context dailer and proxy request
- Optimize the usage of sharedIndexInformer


## yurtctl

- Add new provider=kubeadm that kubernetes cluster installed by kubeadm can be converted to openyurt cluster
- Adapt new certificate mode of yurthub when convert edge node
- Fix image pull policy from `Always` to `IfNotPresent` for all components deployment setting

# v0.2.0

## Project

- Support Kubernetes 1.16 dependency for all components
- Support multi-arch binaries and images (arm/arm64/amd64)
- Add e2e test framework and tests for node autonomy
- New tutorials (e2e test and yurt-tunnel)

## yurt-tunnel

### Features

- Implement yurt-tunnel-server and yurt-tunnel-agent based on Kubernetes apiserver network proxy framework
- Implement cert-manager to manage yurt-tunnel certificates
- Add timeout mechanism for yurt-tunnel

## yurtctl

### Features

- Add global lock to prevent multiple yurtctl invocations concurrently
- Add timeout for acquiring global lock
- Allow user to set the label prefix used to identify edge nodes
- Deploy yurt-tunnel using convert option

### Bugs

- Remove kubelet config bootstrap args during manual setup 

---

# v0.1.0-beta.1

## yurt-controller-manager

### Features

- Avoid evicting Pods from nodes that have been marked as `autonomy` nodes

## yurthub

### Features

- Use Kubelet certificate to communicate with APIServer
- Implement a http proxy for all Kubelet to APIServer requests
- Cache the responses of Kubelet to APIServer requests in local storage
- Monitor network connectivity and switch to offline mode if health check fails
- In offline mode, response Kubelet to APIServer requests based on the cached states
- Resync and clean up the states once node is online again
- Support to proxy for other node daemons 

## yurtctl

### Features

- Support install/uninstall all OpenYurt components in a native Kubernetes cluster
- Pre-installation validation check
