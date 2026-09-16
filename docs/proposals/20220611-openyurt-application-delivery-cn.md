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

3）crd抽象的参数永远不够用，比如：yurtingress不支持imagePullSecret、不支持禁止创建webhook等；

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

## 落地方案调研

1）fluxcd的helm-controller（https://github.com/fluxcd/helm-controller），会依赖HelmRepository、HelmChart、HelmRelease等CRD，引入这套机制会增加openyurt部署和维护的复杂性。另外上层devops产品也可能会使用fluxcd，有可能会引起冲突；

2）kubevela部署chart应用依赖fluxcd，而且fluxcd需要在openyurt集群中部署。因此，这个方案不会比直接使用fluxcd简单。

3）opneyurt自己实现基于nodepool的chart部署模型；

注：helm的调谐逻辑，可以从helm-controller移植过来；

## 落地步骤

1）将nginx-ingress封装到docker镜像里，避免私有化或者内网场景时公网不通、拉取包需要密码等问题；

2）扩展yurtingress的参数values，类型为`*apiextensionsv1.JSON`，用它存储chart的自定义参数；

3）开发基于nodepool的调谐逻辑；
  - 根据nodepool、自定义资源的存在关系，进行增、删、改自定义资源；
  - 对资源增加finalizer；
  - 生成chart的默认值，并与自定义值镜像merge；
  - 执行helm更新操作；

  例子实现：https://github.com/openyurtio/yurt-app-manager/pull/124

## 缺点

1）由于缺少对部署后资源的watch，当某个资源被更新或者删除，controller很难感知；
