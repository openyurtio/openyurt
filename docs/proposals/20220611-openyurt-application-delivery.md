---
title: Proposal Template
authors:
  - "@huiwq1990"
reviewers:
  - "@rambohe-ch"
creation-date: 2022-06-11
last-updated: 2022-06-11
status: provisional
---

# OpenYurt Application Delivery

## Table of Contents

A table of contents is helpful for quickly jumping to sections of a proposal and for highlighting
any additional information provided beyond the standard proposal template.
[Tools for generating](https://github.com/ekalinin/github-markdown-toc) a table of contents from markdown are available.

- [Title](#title)
  - [Table of Contents](#table-of-contents)
  - [Glossary](#glossary)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals/Future Work](#non-goalsfuture-work)
  - [Proposal](#proposal)
    - [User Stories](#user-stories)
    - [Implementation Details](#implementation-detailsnotesconstraints)
      - [OpenYurt Self-Defined Method](#goals)
      - [KubeVela Method](#goals)

## Glossary

Refer to the [Open Application Model](https://oam.dev/).

## Summary

Applications are usually a combine of workload,ingress,service etc. OpenYurt provides some workload controllers, but it's not friendly for application developers.

In this proposal, we would like to introduce an application controller which could delivery applications and consider openyurt cluster's features.

## Motivation

Currently, project `yurt-app-manager` deploys `ingress-controller` instances to every nodepool, project `yurt-edgex-manager` deploys `edgex` instances to every nodepool. So we could find the common ground is delivery resources to nodepool and the feature is useful if we want to develop new moduel as edge gateway.

By the way, `uniteddeployment` has the nodepool featrue, but it could only deploy one deployment or statefulset workload, not include other resrouces.

How to deploy  resource collections, the most common way is use helm chart. `FluxCD` already implement the `HelmRelease` controller, but it's not support nodepool feature.

After investigation, we find out [OAM](https://oam.dev/) and [kubevela](https://kubevela.io/). Kubevela already defines application modules, and could delivery helm charts to multi-clusters. So if kubevela could deploy application to multi-nodepools, it will satisfy our requests.

### Goals

- Openyurt support application deploy
- Both ingress-controller and edgex could use the controller to deploy

### Non-Goals/Future Work

- Treat application as a whole, application's inner resource not support reconcile

## Proposal

### User Stories

- Package kubernetes resources as a helm chart, and create application crd instance

### Implementation Details

#### OpenYurt Self-Defined Method

Define openyurt's application module and develop the application controller.

```yaml
apiVersion: apps.openyurt.io/v1alpha1
kind: Application
metadata:
  name: helm-hello
spec:
spec:
  interval: 5m
  chart:
    spec:
      chart: chartmuseum
      version: "2.14.2"
      url: "https://jhidalgo3.github.io/helm-charts/"
  values: {}
  policies:
    nodepools: ["hangzhou","beijing"]
```

#### KubeVela Method

As kubevela already implements application deploy, and application policies. We could extend nodepool policy type to implement.

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: helm-hello
spec:
  components:
    - name: hello
      type: helm
      properties:
        repoType: "helm"
        url: "https://jhidalgo3.github.io/helm-charts/"
        chart: "hello-kubernetes-chart"
        version: "3.0.0"
  policies:
    - name: foo-cluster-only
      type: topology
      properties:
        clusters: ["foo"]
```

