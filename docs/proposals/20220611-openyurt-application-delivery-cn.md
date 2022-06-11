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

# OpenYurt应用部署思考

## 部署场景

1）yurt-app-manager需要部署ingress-controller实例到每个nodepool

2）yurt-edgex-manager需要部署edgex实例到每个nodepool

## 当前方案

定义edgex、yurtingress crd，分别实现各自的controller，并协调资源创建。

## 当前问题

1）edge controller、ingress controller存在共性，需要部署实例到各个nodepool；

2）扩展性不够，无法支持更多资源类型，比如：将来要部署边缘网关、支持上层业务时，需要针对性开发新的controller；

## 问题思考

1）如果部署服务只是一个镜像，直接使用`yurtdaemonset`即可；

2）如果部署服务是多个资源，可以将资源封装为chart包，chart本身具备模板配置属性，同时方便部署。Chart部署使用fluxcd，或者argocd等cd系统解决。（备注：fluxcd的HelmRelease本质是执行helm install，它不能解决节点池问题）

通过`spec.chart`指定chart包，`spec.values`设置实例的values。

```yaml
#https://fluxcd.io/docs/components/helm/helmreleases/
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: backend
  namespace: default
spec:
  interval: 5m
  chart:
    spec:
      chart: podinfo
      version: ">=4.0.0 <5.0.0"
      sourceRef:
        kind: HelmRepository
        name: podinfo
        namespace: default
      interval: 1m
  upgrade:
    remediation:
      remediateLastFailure: true
  test:
    enable: true
  values:
    service:
      grpcService: backend
    resources:
      requests:
        cpu: 100m
        memory: 64Mi
```

3）进一步多个资源可以看作为应用整体，当前社区已经有OAM的方案，并有kubevela的实现。

- kubevela可以把chart作为应用组件，底层使用fluxcd部署；
- kubevela的topology可以支持多集群部署

https://kubevela.io/docs/tutorials/helm-multi-cluster

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
    - name: topology-local
      type: topology
      properties:
        clusters: ["local"]
    - name: topology-foo
      type: topology
      properties:
        clusters: ["foo"]
    - name: override-local
      type: override
      properties:
        components:
          - name: hello
            properties:
              values:
                configs:
                  MESSAGE: Welcome to Control Plane Cluster!
    - name: override-foo
      type: override
      properties:
        components:
          - name: hello
            properties:
              values:
                configs:
                  MESSAGE: Welcome to Your New Foo Cluster!
  workflow:
    steps:
      - name: deploy2local
        type: deploy
        properties:
          policies: ["topology-local", "override-local"]
      - name: manual-approval
        type: suspend
      - name: deploy2foo
        type: deploy
        properties:
          policies: ["topology-foo", "override-foo"]
```

## 结论

基于上面的实现，应用部署到多个nodepool可以类比应用部署到多个集群，即可以参考kubevela的多集群模型，抽象openyurt的nodepool应用部署模型。

## 落地步骤

1）将edgex，ingresscontroller资源封装成chart包，同时helm install后实例间资源不能有重复的；

2）可以基于kubevela开发nodepool特性的Application；或者openyurt对标实现自己的Controller；

## 缺点

1）由于缺少对部署后资源的watch，当某个资源被更新或者删除，controller很难感知；
