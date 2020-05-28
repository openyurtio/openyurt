# OpenYurt 

[![License](https://img.shields.io/badge/license-Apache%202-4EB1BA.svg)](https://www.apache.org/licenses/LICENSE-2.0.html)
[![Go Report Card](https://goreportcard.com/badge/github.com/alibaba/openyurt)](https://goreportcard.com/report/github.com/alibaba/openyurt)

|![notification](img/bell-outline-badge.svg) What is NEW!|
|------------------|
|May 29th, 2020. OpenYurt v0.1.0 is **RELEASED**! Please check the [CHANGELOG](CHANGELOG.md) for details.|

OpenYurt is built based on native Kubernetes and targets to extend it to support edge computing seamlessly.
In a nutshell, OpenYurt enables users to manage applications that run in the edge infrastructure as if they were running
in the cloud infrastructure. 

OpenYurt is suitable for common edge computing use cases whose requirements include:
- Minimizing the network traffic over long distances between the devices and the workloads.
- Overcoming the network bandwidth or reliability limitations.
- Processing data remotely to reduce latency.
- Providing a better security model to handle sensitive data.

OpenYurt has the following advantages in terms of compatibility and usability.
- **Kubernetes native**. It provides full Kubernetes API compatibility. All Kubernetes workloads, services, 
  operators, CNI plugins, and CSI plugins are supported.
- **Seamless conversion**. It provides a tool to easily convert a native Kubernative to be "edge" ready.
  The extra resource and maintenance costs of the OpenYurt components are very low.
- **Node autonomy**. It provides mechanisms to tolerate unstable or disconnected cloud-edge networking.
  The applications run in the edge nodes are not affected even if the nodes are offline. 
- **Cloud platform agnostic**. OpenYurt can be easily deployed in any public cloud Kubernetes services.

## Architecture

As shown in the figure, OpenYurt follows a classic edge application architecture design - 
a centralized Kubernetes master resides in the cloud site, which
manages multiple edge nodes reside in the edge site. Each edge node has moderate compute resources allowing
running a number of edge applications plus the Kubernetes node daemons. The edge nodes in a cluster can span
multiple physical regions. The terms `region` and `unit` are interchangeable in OpenYurt.
<div align="left">
  <img src="img/arch.png" width=70% title="OpenYurt architecture">
</div>

\
The major OpenYurt components consist of:
- **YurtHub**: A node daemon that serves as a proxy for the outbound traffic from the 
  Kubernetes node daemons (Kubelet, Kubeproxy, CNI plugins and so on). It caches the 
  states of all the resources that the Kubernetes node daemons
  might access in the edge node's local storage. In case the edge node is offline, those daemons can
  recover the states upon node restarts.
- **Yurt controller manager**: It manages a few controllers such as 
  the node controller and the unit controller (not released yet) for different edge computing use cases. For example,
  the Pods in the nodes that are in the `autonomy` mode will not be evicted from APIServer even if the 
  node heartbeats are missing.
- **Yurt tunnel server**: It connects with the `TunnelAgent` daemon running in each edge node via a
  reverse proxy to establish a secure network access between the cloud site control plane and the edge nodes 
  that are connected to the intranet.

## Getting started

OpenYurt supports Kubernetes versions from 1.12 to 1.14. Please make sure your Kubernetes cluster version is within the supported version 
range to avoid any compatibility issues. We will support higher Kubernetes versions shortly.

To get started with OpenYurt, clone the git repository and build it from source. You can build OpenYurt on any Unix/Linux system that 
has golang 1.13+ and bash installed, as follows:

```bash
$ git clone https://github.com/alibaba/openyurt.git
$ cd openyurt
$ make
hack/make-rules/build.sh
Building cmd/yurtctl
Building cmd/yurthub
Building cmd/yurt-controller-manage
```

The OpenYurt related binaries will be placed at `_output/bin`. You can convert an existing Kubernetes cluster to a Yurt cluster 
[manually](./docs/tutorial/setup-manually.md) or using command line tool [yurtctl](./docs/tutorial/yurtctl.md).

## Usage

We provider a detailed [**tutorial**](./docs/tutorial/README.md) to demonstrate how to use OpenYurt to manage edge applications.

## Developer Guide

There's a `Makefile` in the root folder which describes the options to build and install. Here are some common ones:

## Uninstall

## Community

If you have any questions or want to contribute, you are welcome to communicate most things via GitHub issues or pull requests.

Other active communication channels:

- Mailing List: TODO
- Dingtalk Group(钉钉讨论群)

<div align="left">
  <img src="img/ding.jpeg" width=25% title="dingtalk">
</div>

## Copyright

Certain implementations in OpenYurt rely on the existing code from Kubernetes and the credits go to the original Kubernetes authors.

