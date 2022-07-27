# Yurtadm

## 1. Background

In order to allow users to quickly obtain an OpenYurt test cluster, OpenYurt provides the command `yurtadm init` to initialize the cluster. Users only need to select the version of the OpenYurt cluster mirror to create the corresponding version of OpenYurt. Then yurt-app-manager, yurt-controller-manager, yurt-tunnel-server, yurt-tunnel-agent components will be automatically deployed.
To expand the cluster later, users can use the `yurtadm join` command to add edge nodes or cloud nodes to the cluster.

## 2. Ability

Using yurtadm, you can do:
- Create a simple openyurt cluster with just one command.
- Create a High Availability OpenYurt cluster.

## 3. Supported Versions

Currently, yurtadm supports:
- openyurt verion: v0.7.0
- k8s version: v1.19.8, v1.20.10, v1.21.14(default)

## 4. Process
### 4.1 Compile Yurtadm

When initializing the cluster, you need to obtain the Yurtadm executable first. To quickly build and install yurtadm, you can execute the following command to complete the installation if the build system has golang 1.13+ and bash installed:

```bash
git clone https://github.com/openyurtio/openyurt.git
cd openyurt
make build WHAT="yurtadm" ARCH="amd64" REGION=cn
```

The executable will be stored in the _output/ directory.

### 4.2 Initialize the OpenYurt Cluster

Execute the following command as root account, no need to install container runtimes such as docker in advance. Docker will be automatically installed during the execution of `yurtadm init`.

```bash
# Initialize an OpenYurt cluster.
yurtadm init --apiserver-advertise-address 1.2.3.4 --openyurt-version v0.7.0 --passwd xxx

# Initialize an OpenYurt high availability cluster.
yurtadm init --apiserver-advertise-address 1.2.3.4,1.2.3.5,1.2.3.6 --openyurt-version v0.7.0 --passwd xxx
```

`yurtadm init` will use sealer to create a K8s cluster. And kubeadm, kubectl, docker, etc. will all be installed during this process.

Note: The following components are installed during `yurtadm init` :
- kubeadm
- kubectl
- kubelet
- kube-proxy
- docker

### 4.3 Joining Node to Cluster

Currently, you can use kubeadm token create to get bootstrap token.
Get bootstrap token from the master:

```bash
kubeadm token create
W0720 20:46:19.782354   31205 configset.go:348] WARNING: kubeadm cannot validate component configs for API groups [kubelet.config.k8s.io kubeproxy.config.k8s.io]
abcdef.0123456789abcdef
```

Before `yurtadm join` you need to:
- Install a runtime (like docker) on the worker node
- Configure `/etc/docker/daemon.json`
- Copy the yurtadm command to the node to be joined

`vi /etc/docker/daemon.json`, change the docker driver to systemd and configure the sea.hub registry (When sealer creates the master, images are automatically saved in sea.hub):

```bash
{
  "exec-opts": ["native.cgroupdriver=systemd"],
  "insecure-registries": [
    "sea.hub:5000"
  ]
}
```

restart docker:

```bash
systemctl daemon-reload
systemctl restart docker
```

Then execute the `yurtadm join` command in the worker node:

```bash
# Join the edge node to cluster.
yurtadm join 1.2.3.4:6443 --token=abcdef.0123456789abcdef --node-type=edge --discovery-token-unsafe-skip-ca-verification --v=5

# Join the edge node to a high availability cluster.
yurtadm join 1.2.3.4:6443,1.2.3.5:6443,1.2.3.6:6443 --token=abcdef.0123456789abcdef --node-type=edge --discovery-token-unsafe-skip-ca-verification --v=5

# Join the cloud node to cluster.
yurtadm join 1.2.3.4:6443 --token=abcdef.0123456789abcdef --node-type=cloud --discovery-token-unsafe-skip-ca-verification --v=5

# Join the cloud node to a high availability cluster.
yurtadm join 1.2.3.4:6443,1.2.3.5:6443,1.2.3.6:6443 --token=abcdef.0123456789abcdef --node-type=cloud --discovery-token-unsafe-skip-ca-verification --v=5
```

Note: The following components are installed during `yurtadm init` :
- kubeadm
- kubectl
- kubelet
- kube-proxy

### 4.4 Delete Joined Node

When you need to delete a node joined using `yurtadm join`, the steps are as follows:

In master:

```bash
kubectl drain {NodeName} --delete-local-data --force --ignore-daemonsets
kubectl delete node {NodeName}
```

In joined node:

1. Execute the reset process:

```bash
yurtadm reset
```

2. Delete `/etc/cni/net.d` dir:

```bash
rm -rf /etc/cni/net.d
```

3. Clean /etc/hosts, Delete the record `sea.hub:5000`.

## 5. Reference

### 5.1 yurtadm init flags

| **Flag**                                       | **Description**                                              |
| ------------------------------------------ | ------------------------------------------------------------ |
| --apiserver-advertise-address <br />string | The IP address the API Server will advertise it's listening on. |
| --cluster-cidr <br />string                | Choose a CIDR range of the pods in the cluster (default "10.244.0.0/16") |
| --image-repository <br />string            | Choose a registry to pull cluster images from (default "registry.cn-hangzhou.aliyuncs.com/openyurt") |
| --k8s-version <br />string                 | Choose a specific Kubernetes version for the control plane. (default "1.21.14") |
| --kube-proxy-bind-address <br />string     | Choose an IP address for the proxy server to serve on (default "0.0.0.0") |
| --openyurt-version <br />string            | Choose a specific OpenYurt version for the control plane. (default "v0.7.0") |
| -p, --passwd <br />string                  | Set master server ssh password                               |
| --pod-subnet <br />string                  | PodSubnet is the subnet used by Pods. (default "10.244.0.0/16") |
| --service-subnet <br />string              | ServiceSubnet is the subnet used by kubernetes Services. (default "10.96.0.0/12") |
| --yurt-tunnel-server-address<br />string   | The yurt-tunnel-server address.                              |

### 5.2 yurtadm join flags

| **Flag**                                          | **Description**                                              |
| --------------------------------------------- | ------------------------------------------------------------ |
| --cri-socket <br />string                     | Path to the CRI socket to connect (default "/var/run/dockershim.sock") |
| --discovery-token-ca-cert-hash <br />strings  | For token-based discovery, validate that the root CA public key matches this hash (format: "<type>:<value>"). |
| --discovery-token-unsafe-skip-ca-verification | For token-based discovery, allow joining without --discovery-token-ca-cert-hash pinning. |
| --ignore-preflight-errors<br />strings        | A list of checks whose errors will be shown as warnings. Example: 'IsPrivilegedUser,Swap'. Value 'all' ignores errors from all checks. |
| --kubernetes-resource-server<br />string      | Sets the address for downloading k8s node resources (default "dl.k8s.io") |
| --node-labels<br />string                     | Sets the labels for joining node                             |
| --node-name<br />string                       | Specify the node name. if not specified, hostname will be used. |
| --node-type<br />string                       | Sets the node is edge or cloud (default "edge")              |
| --organizations <br />string                  | Organizations that will be added into hub's client certificate |
| --pause-image<br />string                     | Sets the image version of pause container (default "registry.cn-hangzhou.aliyuncs.com/google_containers/pause:3.2") |
| --skip-phases<br />strings                    | List of phases to be skipped                                 |
| --token<br />string                           | Use this token for both discovery-token and tls-bootstrap-token when those values are not provided. |
| --yurthub-image<br />string                   | Sets the image version of yurthub component (default "registry.cn-hangzhou.aliyuncs.com/openyurt/yurthub:v0.7.0") |