# Yurtadm

## 1.Background
In order to allow users to quickly obtain an OpenYurt test cluster, OpenYurt provides the command `yurtadm init` to initialize the cluster. Users only need to select the version of the OpenYurt cluster mirror to create the corresponding version of OpenYurt. Then yurt-app-manager, yurt-controller-manager, yurt-tunnel-server, yurt-tunnel-agent components will be automatically deployed.
To expand the cluster later, users can use the `yurtadm join` command to add edge nodes or cloud nodes to the cluster.

## 2.Ability
Using yurtadm, you can do:
- Create a simple openyurt cluster with just one command.
- Create a High-Availability OpenYurt cluster.

## 3.Process
### 3.1 Compile Yurtadm
When initializing the cluster, you need to obtain the Yurtadm executable first. To quickly build and install yurtadm, you can execute the following command to complete the installation if the build system has golang 1.13+ and bash installed:

```bash
git clone https://github.com/openyurtio/openyurt.git
cd openyurt
make build WHAT="yurtadm" ARCH="amd64" REGION=cn
```

The executable will be stored in the _output/ directory.

### 3.2 Initialize the OpenYurt cluster
Execute the following command as root account, no need to install container runtimes such as docker in advance. Docker will be automatically installed during the execution of `yurtadm init`.

```bash
# Initialize an OpenYurt cluster.
yurtadm init --apiserver-advertise-address 192.168.152.131 --openyurt-version latest --passwd 1234

# Initialize an OpenYurt cluster with multiple masters.
yurtadm init --apiserver-advertise-address 192.168.152.131,192.168.152.132 --openyurt-version v0.7.0 --passwd 1234
```
`yurtadm init` will use sealer to create a K8s cluster. And kubeadm, kubectl, docker, etc. will all be installed during this process.

### 3.3 Joining node to cluster
Currently, you can use kubeadm token create to get bootstrap token.
Get bootstrap token from the master:

```bash
kubeadm token create
W0720 20:46:19.782354   31205 configset.go:348] WARNING: kubeadm cannot validate component configs for API groups [kubelet.config.k8s.io kubeproxy.config.k8s.io]
zffaj3.a5vjzf09qn9ft3gt
```

Before `yurtadm join` you need to install a runtime (like docker) on the worker node. Then execute the `yurtadm join` command in the worker node:

```bash
# Join the edge node to cluster.
yurtadm join 192.168.152.131:6443 --token=zffaj3.a5vjzf09qn9ft3gt --node-type=edge --discovery-token-unsafe-skip-ca-verification --v=5

# Join the edge node to cluster with multiple masters.
yurtadm join 192.168.152.131:6443,192.168.152.132:6443 --token=zffaj3.a5vjzf09qn9ft3gt --node-type=edge --discovery-token-unsafe-skip-ca-verification --v=5

# Join the cloud node to cluster.
yurtadm join 192.168.152.131:6443 --token=zffaj3.a5vjzf09qn9ft3gt --node-type=cloud --discovery-token-unsafe-skip-ca-verification --v=5

# Join the cloud node to cluster with multiple masters.
yurtadm join 192.168.152.131:6443,192.168.152.132:6443 --token=zffaj3.a5vjzf09qn9ft3gt --node-type=cloud --discovery-token-unsafe-skip-ca-verification --v=5
```

## Other Problems
Temporarily yurtadm only supports openyurt v0.7.0 and latest, k8s v1.19.8 version.