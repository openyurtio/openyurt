# Roadmap

This document outlines the development roadmap for the OpenYurt project.

## v0.4.0 Roadmap(Release Plan: 2021.5)

- Add Cloud Native IOT Device Management API definition.
- Support IOT Device Management that comply with cloud native IOT API
- Support autonomy feature in node pool level.
- Support manage configmap in node pool with unique setting.
- Upgrade openyurt components to support Kubernetes 1.18.
- Add basic Pod network recovery mechanism to handle edge node restarts.
- Improve `YurtCtl` user experience.
- Add minimal hardware requirement and system requirement info of OpenYurt.

## v0.5.0 Roadmap

- Support IOT Device Management integrated with EdgeX Foundry that comply with cloud native IOT API
- Yurt-tunnel support more flexible settings for forwarding requests from cloud to edge
- Add local storage statics collection and report
- Support Pods that use `InClusterConfig` access kube-apiserver run on edge nodes without modification.
- Improve OpenYurt user experience(yurtctl init/join/reset)
- Support service to bound east-west traffic within a nodePool

## v0.6.0 Roadmap

- Launch OpenYurt Experience Center to support end users to learn openyurt easily.
- Support Ingress controller at NodePool level.
- Local storage supports multi-devicepath
- Add YurtAppDaemon for managing workloads like DaemonSet at NodePool level.
- Add YurtCluster Operator(A declarative way for kubernetes and openyurt conversion)
- Update Docs and homepage website