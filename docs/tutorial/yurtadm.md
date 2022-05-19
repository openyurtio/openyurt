# `Yurtadm` tutorial

This tutorial demonstrates how to use `yurtadm` to install/uninstall OpenYurt.
Please refer to the [Getting Started](https://github.com/openyurtio/openyurt#getting-started) section on the README page to prepare and build binary to `_output/bin/yurtadm`.
We assume a minikube cluster ([version 1.14 or less](https://github.com/kubernetes/minikube/releases/tag/v1.0.0))
is installed.

## Create OpenYurt cluster
`yurtadm init` will create an OpenYurt cluster, and the user doesn't need to do pre-work, such as install the runtime in advance or ensure that the swap partition of the node has been closed.

Using `yurtadm` to create an OpenYurt cluster can be done by doing the following:
```
$ _output/bin/yurtadm init --apiserver-advertise-address 1.2.3.4 --openyurt-version v0.5.0 --passwd 1234
```
The `--apiserver-advertise-address` is the IP address of master, `--passwd` is ssh password of master, `--openyurt-version` is the the OpenYurt cluster version.
In addition, and the OpenYurt cluster image registry can be set by the option `--image-registry`. If user want to get more help, please use `yurtadm init -h`.

## Join Edge-Node/Cloud-Node to OpenYurt

`yurtadm join` will automatically install the corresponding kubelet according to the cluster version, but the user needs to install the runtime in advance and ensure that the swap partition of the node has been closed.

Using `yurtadm` to join an Edge-Node to OpenYurt cluster can be by doing the following:
```
$ _output/bin/yurtadm join 1.2.3.4:6443 --token=zffaj3.a5vjzf09qn9ft3gt --node-type=edge-node --discovery-token-unsafe-skip-ca-verification --v=5
```

Using `yurtadm` to join a Cloud-Node to OpenYurt cluster can be by doing the following:
```
$ _output/bin/yurtadm join 1.2.3.4:6443 --token=zffaj3.a5vjzf09qn9ft3gt --node-type=cloud-node --discovery-token-unsafe-skip-ca-verification --v=5
```

## Reset nodes of OpenYurt

Using `yurtadm` to revert any changes made to this host by `yurtadm join` can be by doing the following:
```
$ _output/bin/yurtadm reset
```
