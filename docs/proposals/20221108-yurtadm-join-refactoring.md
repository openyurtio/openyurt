---
title: Yurtadm Join Refactoring
authors:
  - "@YTGhost"
reviewers:
  - "@rambohe-ch"
creation-date: 2022-11-08
last-updated: 2022-11-08
status: provisional
---

# Yurtadm Join Refactoring

<!-- END Remove before PR -->

## Table of Contents

- [Title](#title)
  - [Table of Contents](#table-of-contents)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals/Future Work](#non-goalsfuture-work)
  - [Proposal](#proposal)
    - [User Stories](#user-stories)
    - [Implementation Details](#implementation-details)
      - [Yurtadm join analysis](#yurtadm-join-analysis)
        - [prepare phase](#prepare-phase)
        - [preflight phase](#preflight-phase)
        - [joinnode phase](#joinnode-phase)
        - [postcheck phase](#postcheck-phase)
      - [Kubeadm join analysis](#kubeadm-join-analysis)
        - [preflight phase](#preflight-phase)
        - [kubelet-start phase](#kubelet-start-phase)
      - [The difference between the two processes](#the-difference-between-the-two-processes)
      - [The refactored process](#the-refactored-process)
        - [init joindata phase](#init-joindata-phase)
        - [prepare phase](#prepare-phase)
        - [joinnode phase](#joinnode-phase)
        - [postcheck phase](#postcheck-phase)

## Summary

`yurtadm join` references `k8s.io/kubernetes` dependencies, especially `k8s.io/kubernetes/cmd/kubeadm`. It should be refactor to remove the dependencies. This proposal wants to import `kubeadm` binary file to replace the code that was imported.

## Motivation

Compare to import code, using `kubeadm` binary directly has the following advantages:

- It's very easy to follow K8s to update kubeadm version, do not need to maintain the code of `kubeadm join` in OpenYurt.
- reduce the complexity of `yurtadm join`

### Goals

- After refactoring, the original join functionality can be maintained by calling `kubeadm join` and some of custom operations required by openyurt.
- Make the code subsequent maintainability and expandability more guaranteed.

### Non-Goals/Future Work

- Abstract the interface, kubeadm join as one of the implementations.
- Refactor `yurtadm reset` and so on.

## Proposal

### User Stories

As a developer, I can understand the `yurtadm join` process more clearly due to the reduced complexity of the `yurtadm join` part of the code

### Implementation Details

#### Yurtadm join analysis

The process of yurtadm join is divided into five phases:

1. init joindata

2. prepare

3. preflight

4. joinnode

5. postcheck

##### init joindata phase

- Initializes the join data

- Call `RetrieveBootstrapConfig` and change the server address to the master IP address

##### prepare phase

This phase focuses on initializing the system's environment in `/pkg/yurtadm/cmd/join/phases/prepare.go`, with the following actions:

1. Clean up the `/etc/kubernetes/manifests` folder, i.e. clean up the static Pod manifest folder
2. Turn on packet forwarding (SetIpv4Forward)
3. Turn on `bridge-nf-call-iptables` (SetBridgeSetting)
4. Turn off SELinux (SetSELinux)
5. Check if Kubelet and cni-plugin are installed, if not, they will be installed (CheckAndInstallKubelet)
6. Configure the kubelet service, i.e. configure `/etc/systemd/system/kubelet.service` (SetKubeletService)
7. Configure the kubelet startup parameters, i.e., configure `/etc/systemd/system/kubelet.service.d/10-kubeadm.conf` (SetKubeletUnitConfig)
8. Change the connection address to the yurthub listener address (http://127.0.0.1:10261) i.e. configure `/etc/kubernetes/kubelet.conf` (SetKubeletConfigForNode)
9. Configure `/etc/kubernetes/pki/ca.crt` (SetKubeletCaCert)

##### preflight phase

This phase is mainly for preflight checks, which include checking some files, folders and environment configuration work. The relevant code for the checks is a direct call to the `RunJoinNodeChecks` function in the kubeadm source code.

##### joinnode phase

The main thing to do in this phase is to start the kubelet, as follows:

1. Get the kubelet configuration from the control plane node, parse it and write it to `/var/lib/kubelet/config.yaml`
2. configure `/var/lib/kubelet/kubeadm-flags.env`
3. Generate the yaml file `/etc/kubernetes/manifests/yurthub.yaml` for the YurtHub static Pod to start YurtHub when the kubelet starts
4. Try to start the kubelet

##### postcheck phase

The main thing to do in this phase is to check the health of the components launched on the node, mainly including:

1. the health status of the kubelet
2. the health status of the YurtHub static Pod
3. adding Annotation to the node: `kubeadm.alpha.kubernetes.io/cri-socket`

#### Kubeadm join analysis

The kubeadm join process is divided into the following phases:

1. preflight

2. kubelet-start

##### preflight phase

This phase mainly performs some preflash checks logic, mainly including.

1. basic checks, i.e. `RunJoinNodeChecks`
2. Get the `InitCfg`, which also includes a call to `RetrieveBootstrapConfig`, which will be used in the `kubelet-start` phase

##### kubelet-start phase

This phase will mainly start the kubelet, the main process is.

1. Write the configuration of `tlsBootstrapCfg` to `/etc/kubernetes/bootstrap-kubelet.conf`
2. Take the `CertificateAuthorityData` from `tlsBootstrapCfg` and write it to `/etc/kubernetes/pki/ca.crt`.
3. Check if there is a node in the cluster with the same name as the node you want to join and in Ready state (the note says that an error will occur if there is a control-plane node with the same name)
4. Try to stop the kubelet
5. Configure `/var/lib/kubelet/config.yaml`
6. Configure `/var/lib/kubelet/kubeadm-flags.env`
7. Try to start the kubelet
8. Convert `/etc/kubernetes/bootstrap-kubelet.conf` to `/etc/kubernetes/kubelet.conf`.
9. Add Annotation to the node: `kubeadm.alpha.kubernetes.io/cri-socket`
10. Delete `/etc/kubernetes/bootstrap-kubelet.conf`.

#### The difference between the two processes

1. Yurtadm will do some operations to initialize the system environment, such as turning on packet forwarding, etc.
2. Yurtadm will change the connection address in `/etc/kubernetes/kubelet.conf` to the yurthub listener address `http://127.0.0.1:10261`.
3. Preflight checks will additionally ignore `FileAvailable--etc-kubernetes-kubelet.conf` and `FileAvailable--etc-kubernetes-pki-ca.crt` errors
4. Configure `/var/lib/kubelet/kubeadm-flags.env` with some additional parameters, such as setting `--rotate-certificates` to `false`, etc.
5. Generate the yaml file `/etc/kubernetes/manifests/yurthub.yaml` for the YurtHub static Pod
6. Additionally check the health status of YurtHub
7. Modified the Server address when calling `RetrieveBootstrapConfig`.

#### The refactored process

According to the above analysis, it can be divided into four phases as follows.

1. init joindata: mainly to initialize the data needed for the later stages
2. prepare: mainly to initialize the system environment
3. joinnode: mainly to call kubeadm join, before the call you need to determine whether the current environment has kubeadm, if not, you need to download
4. postcheck: check the health status of the startup component

##### init joindata phase

Consistent with yurtadm join

##### prepare phase

1. Clean up the `/etc/kubernetes/manifests` folder, i.e. clean up the static Pod manifest folder
2. Turn on packet forwarding (SetIpv4Forward)
3. Turn on `bridge-nf-call-iptables` (SetBridgeSetting)
4. Turn off SELinux (SetSELinux)
5. Check if Kubelet and cni-plugin are installed, if not, they will be installed (CheckAndInstallKubelet)
6. Configure the kubelet service, i.e. configure `/etc/systemd/system/kubelet.service` (SetKubeletService)
7. Configure the kubelet startup parameters, i.e., configure `/etc/systemd/system/kubelet.service.d/10-kubeadm.conf` (SetKubeletUnitConfig)
8. Change the connection address to the yurthub listening address (http://127.0.0.1:10261) i.e. configure `/etc/kubernetes/kubelet.conf` (SetKubeletConfigForNode)
9. Configure `/var/lib/kubelet/discovery.conf` for configuring `discovery`
10. Configure `/var/lib/kubelet/kubeadm-join.conf` to configure additional required `KubeletExtraArgs`, the path to `discovery.conf` will also be added to it

##### joinnode phase

Use `kubeadm join` and read `/var/lib/kubeadm-join.conf` with `--config` to configure the additional required `KubeletExtraArgs` and `discovery`

`kubeadm-join.conf` example:

```yaml
apiVersion: kubeadm.k8s.io/v1beta3
kind: JoinConfiguration
discovery:
  file:
    kubeConfigPath: /var/lib/kubelet/discovery.conf
  tlsBootstrapToken: xfn7gd.0ztozki1f5cif7h1
nodeRegistration:
  criSocket: /var/run/dockershim.sock
  name: vm-16-6-centos
  ignorePreflightErrors:
    - FileAvailable--etc-kubernetes-kubelet.conf
  kubeletExtraArgs:
    rotate-certificates: "false"
    pod-infra-container-image: registry.cn-hangzhou.aliyuncs.com/google_containers/pause:3.2
    node-labels: openyurt.io/is-edge-worker=true
    network-plugin: cni
```

`discovery.conf` example:

```yaml
apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUM2VENDQWRHZ0F3SUJBZ0lCQURBTkJna3Foa2lHOXcwQkFRc0ZBREFWTVJNd0VRWURWUVFERXdwcmRXSmwKY201bGRHVnpNQ0FYRFRJeU1UQXlOekUxTURBeU1sb1lEekl4TWpJeE1EQXpNVFV3TURJeVdqQVZNUk13RVFZRApWUVFERXdwcmRXSmxjbTVsZEdWek1JSUJJakFOQmdrcWhraUc5dzBCQVFFRkFBT0NBUThBTUlJQkNnS0NBUUVBCnlFWlpSQUhmdWRIL05nZElKaXJnNW1IcllzT093UHdhWDR6dFJaRG4zdFB4ZTYxY0hBdkhGT0h3M0xqKzFRSmMKWWE4OGZ1NFF6cUh2TnBGeTlZeTE3YVo3c1JuQUhTemcvSkVvcjlxQlVJakN0WlUxMTN5dlRMSDZRZnZBTWl4eQpoTkdIMWRMWnV5SnY5OUUweklMUUNrWGN5SzF6a1ZFeWZKcmZaS2lTalYwQndteWl1QjUwcktmc0pZc21rS2ZICnp0SEtzWmFPaElwcmd3NG02SmttSUZKQjBFTlBhaktKNjRLWng3bXUzOVIySlJNc3A0WXpieWJTQnNacnNGcUIKbnZhTHRlVk9QcHVyK1hYUkptUUFvbkc3Zkl5QU9GY21PTmovekNHbzhpb04xd2NFVU1paWhFaVN6UzVUM2wwQgpwUUdOV0JGVUFyMmZ0NTRaV2syUnd3SURBUUFCbzBJd1FEQU9CZ05WSFE4QkFmOEVCQU1DQXFRd0R3WURWUjBUCkFRSC9CQVV3QXdFQi96QWRCZ05WSFE0RUZnUVU2Z0RhTUZEaURQaU90eGcxSktxSk1YZkRXV1l3RFFZSktvWkkKaHZjTkFRRUxCUUFEZ2dFQkFHNzAvU3hRTlFZTjJKUC9paUllN2kxdXAyT1VZUXlXYjlqMVF6dnlkNHdRTElMTQpvUzZHZVplblBYRlRnemVFZE5uaUsxcEV6b0JJMUNOa0N4S01QVUx3RWJzRlVDb1M3T2JVTWNoQkw2MG0xOC9GCnFGRG04QVFqRUdXTllOeHVuRXlFVUpUQXBibStndTdCYlIvZzR2Uk5UTWZ3NzFiSzlGci9iUmdBMHZEVEdhK0QKU29RekxKZms2UTRKQW41MHFGRDlCdVViV3ExSkY4eWxqc1BiMnlqWlVqSkhaWmd6cTRJL2FaYjZ5TjJ2MTAxTQpiSHRRR2crUDZFb05rWWZDR25zRGZNYnJhYnZpK3VyTUNNcmZzLzhCQnlUU3c3SjB6UEdkbTQ5b1l1SXE1YnVqCjdJVmR1eE9JempnU2hqSm91bjdpWWpFQXRnR3U3REgzYVd4Qm9tZz0KLS0tLS1FTkQgQ0VSVElGSUNBVEUtLS0tLQo=
    server: https://apiserver.cluster.local:6443
  name: ""
contexts: null
current-context: ""
kind: Config
preferences: {}
users: null
```

##### postcheck phase

1. Check the health status of kubelet
2. Check the health status of YurtHub static Pods
