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

# OpenYurt应用部署实现思考

## Subset定义

对于多区域管理，业界已经有定义模型，如：openyurt的appset，openkruise的UnitedDeployment。

### openkruise模型

```yaml
apiVersion: apps.kruise.io/v1alpha1
kind: UnitedDeployment
metadata:
  name: sample-ud
spec:
  replicas: 6
  revisionHistoryLimit: 10
  selector:
    matchLabels:
      app: sample
  template:
    # statefulSetTemplate or advancedStatefulSetTemplate or cloneSetTemplate or deploymentTemplate
    statefulSetTemplate:
      metadata:
        labels:
          app: sample
      spec:
        selector:
          matchLabels:
            app: sample
        template:
          metadata:
            labels:
              app: sample
          spec:
            containers:
            - image: nginx:alpine
              name: nginx
  topology:
    subsets:
    - name: subset-a
      nodeSelectorTerm:
        matchExpressions:
        - key: node
          operator: In
          values:
          - zone-a
      replicas: 1
    - name: subset-b
      nodeSelectorTerm:
        matchExpressions:
        - key: node
          operator: In
          values:
          - zone-b
      replicas: 50%
    - name: subset-c
      nodeSelectorTerm:
        matchExpressions:
        - key: node
          operator: In
          values:
          - zone-c
```

https://openkruise.io/zh/docs/user-manuals/uniteddeployment/

## OAM 多区域

subset本质是带有特定label的节点。kubevela目前已经支持多集群，在多集群的基础上扩展多subset特性，即：集群关联的subset都需要部署实例。部署实例时生成的workload增加节点亲和设置。

### 用户体验

用户使用层面，在Application里增加subset类型的policy。

下面例子整体含义是：将Redis部署到local集群的beijing、hangzhou两个subset中。

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: first-vela-app2
spec:
  components:
    - name: redis
      type: helm
      properties:
        repoType: "helm"
        url: "https://charts.bitnami.com/bitnami"
        chart: "redis"
        version: "16.8.5"
  policies:
    - name: target-default
      type: topology
      properties:
        clusters: ["local"]
        namespace: "default"
    - name: default-subsets
      type: subset
      properties:
        nodeSelectorLabel: "apps.openyurt.io/nodepool"
        nodeSelectorValues: ["beijing","hangzhou"]
  workflow:
    steps:
      - name: deploy2default
        type: deploy
        properties:
          policies: ["target-default","default-subsets"]
```

### 渲染结果

KubeVela应用部署时，应获取当前部署任务的subset信息，并将subset传递给Component，让Component关联的Workload能配置上affinity。

```yaml
spec:
  selector:
    matchLabels:
      app: abc
  template:
    metadata:
      labels:
        app: abc
    spec:
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: apps.openyurt.io/nodepool
                operator: In
                values:
                - hangzhou
```

## 落地方案

### 方案1

#### helm改造

对helm类型的Component，为满足亲和设置，**需要增加char包规范**：chart的values.yaml中必须有亲和的key和value，即：chart包values.yaml必须包含属性nodeSelectorLabel，nodeSelectorValues。注：优化方案，稍后讨论。

#### fluxcd改造

fluxcd的addon需要将values渲染增加亲和的配置，亲和值依赖kubevela注入context中。

```tex

        if context.subsetEnable == "true" {
        			values: parameter.values & {
        			                             nodeSelectorLabel: context.nodeSelectorLabel
                                           nodeSelectorValues: context.nodeSelectorValues}
        }
        if context.subsetEnable == "false" && parameter.values != _|_ {
        			values: parameter.values
        }

```

#### kubevela改造

kubevela之前会跟进Component，Placement生成任务列表。

```go
#pkg/workflow/providers/multicluster/deploy.go:164
func applyComponents(apply oamProvider.ComponentApply, healthCheck oamProvider.ComponentHealthCheck, components []common.ApplicationComponent, placements []v1alpha1.PlacementDecision, parallelism int) (bool, string, error) {
	var tasks []*applyTask
	for _, comp := range components {
		for _, pl := range placements {
			tasks = append(tasks, &applyTask{component: comp, placement: pl})
		}
	}
	//....
}
```

改造后，需要基于Component、Placement、Subset考虑相关任务生成。

```go
#pkg/workflow/providers/multicluster/deploy.go:164
func applyComponents(apply oamProvider.ComponentApply, healthCheck oamProvider.ComponentHealthCheck, components []common.ApplicationComponent, placements []v1alpha1.PlacementDecision, parallelism int) (bool, string, error) {
	var tasks []*applyTask
	for _, comp := range components {
		for _, pl := range placements {
      for _, ss := range subsets {
        tasks = append(tasks, &applyTask{component: comp, placement: pl,subset: ss})
		}
	}
	//....
}
```

#### 内部逻辑

本质执行helm install，同时设置亲和值。

1 helm install redis-beijing redis:16.8.5 --set nodeSelectorLabel="apps.openyurt.io/nodepool" --set nodeSelectorValues="beijing"

2 helm install redis-hangzhou redis:16.8.5  --set nodeSelectorLabel="apps.openyurt.io/nodepool" --set nodeSelectorValues="hangzhou"

### 方案2

方案1存在的问题是，chart包设置固定的亲和值，**让kubevela支持属性渲染**，可以去除这个限制。

在helm的values的值可以从context里取：

```yaml
        values:
          customNodeSelectorLabel: context.nodeSelectorLabel
          customodeSelectorValues: context.nodeSelectorValues
```

整体配置如下：

```yaml
apiVersion: core.oam.dev/v1beta1
kind: Application
metadata:
  name: first-vela-app2
spec:
  components:
    - name: redis
      type: helm
      properties:
        repoType: "helm"
        url: "https://charts.bitnami.com/bitnami"
        chart: "redis"
        version: "16.8.5"
        values:
          customNodeSelectorLabel: context.nodeSelectorLabel
          customodeSelectorValues: context.nodeSelectorValues
  policies:
    - name: target-default
      type: topology
      properties:
        clusters: ["local"]
        namespace: "default"
    - name: default-subsets
      type: subset
      properties:
        nodeSelectorLabel: "apps.openyurt.io/nodepool"
        nodeSelectorValues: ["beijing","hangzhou"]
  workflow:
    steps:
      - name: deploy2default
        type: deploy
        properties:
          policies: ["target-default","default-subsets"]
```

## CUE测试

```yaml
  parameter: {
      	values?: #nestedmap
        }

        #nestedmap: {
        	...
        }
        innerVealues: {
          nodeSelectorLabel: "apps.openyurt.io/nodepool"
          nodeSelectorValues: ["beijing","hangzhou"]
        }
        parameter: {
        values: {"a": "b"} & innerVealues
        }
```
