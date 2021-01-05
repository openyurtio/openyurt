
# Use yurt-app-manager to manage your edge node and workload

In this tutorial, we will show how the yurt-app-manager helps users manage 
there edge nodes and workload.
Suppose you have a Kubernetes cluster in an Openyurt environment, or a native Kubernetes cluster with at least two nodes.

## Install yurt-app-manager

### 1. create and push yurt-app-manager image

Go to OpenYurt root directory:
```bash
$ cd $GOPATH/src/github.com/alibaba/openyurt
```

You shoud first set global linux environment variables:
  - **IMAGE_REPO**: which stand for your own image registry for yurt-app-manager
  - **IMAGE_TAG**: which stand for yurt-app-manager image tag
  
  For Example:
```bash
$ export IMAGE_REPO=registry.cn-hangzhou.aliyuncs.com/edge-kubernetes
$ export IMAGE_TAG="v0.3.0-"$(git rev-parse --short HEAD)
```

```bash
$ make clean
$ make release WHAT=cmd/yurt-app-manager ARCH=amd64 REGION=cn REPO=${IMAGE_REPO} GIT_VERSION=${IMAGE_TAG} 
```

If everything goes right, we will get `${IMAGE_REPO}/yurt-app-manager:${GIT_VERSION}` image:

```bash
$ docker images ${IMAGE_REPO}/yurt-app-manager:${GIT_VERSION} 
```

push yurt-app-manager image to your own registry
```bash
$ docker push ${IMAGE_REPO}/yurt-app-manager:${GIT_VERSION}  
```
### 2. Create yurt-app-manager yaml files

```bash
$ make gen-yaml WHAT=cmd/yurt-app-manager REPO=${IMAGE_REPO} GIT_VERSION=${IMAGE_TAG}
```

If everything goes right, we will have a yurt-app-manager.yaml files:
```bash
$ ls _output/yamls

yurt-app-manager.yaml
```

### 4. install yurt-app-manager operator 

```bash
$ kubectl apply -f _output/yamls/yurt-app-manager.yaml
```
The content of the `yurt-app-manager.yaml` file mainly includes three points:
- **NodePool CRD**
- **UnitedDeployment CRD**
- **yurtapp-controller-manager Deployment** which installed in `kube-system` namespaces 

``` bash
kubectl get pod -n kube-system |grep yurt-app-manager
```

## How to Use

The Examples of NodePool and UnitedDeployment are in `config/yurt-app-manager/samples/` directory

### NodePool 

- 1 create an nodepool 
```bash
cat <<EOF | kubectl apply -f -
apiVersion: apps.openyurt.io/v1alpha1
kind: NodePool
metadata:
  name: Pool-A 
spec:
  type: Edge
  annotations:
    apps.openyurt.io/example: test-Pool-A
  labels:
    apps.openyurt.io/example: test-Pool-A
  taints:
  - key: apps.openyurt.io/example
    value: test-Pool-A
    effect: NoSchedule
EOF
```

- 2 Get NodePool
```bash
$ kubectl get np Pool-A 

NAME       TYPE   READYNODES   NOTREADYNODES   AGE
Pool-A     Edge                                28s
```

- 3 Add Node To NodePool

Add an node into `Pool-A` NodePool, Set the `apps.openyurt.io/desired-nodepool` label on the host, and value is the name of the Pool-A NodePool
```bash
$ kubectl label node {Your_Node_Name} apps.openyurt.io/desired-nodepool=Pool-A

{Your_Node_Name} labeled
```

```bash
$ kubectl get np Pool-A 
NAME       TYPE   READYNODES   NOTREADYNODES   AGE
Pool-A     Edge   1            0               5m19s
```

Once a Node adds a NodePool, it inherits the annotations, labels, and taints defined in the nodepool Spec,at the same time, the Node will add a new tag: `apps.openyurt.io/nodepool`. For Example:
```bash
$ kubectl get node {Your_Node_Name} -o yaml 

apiVersion: v1
kind: Node
metadata:
  annotations:
    apps.openyurt.io/example: test-Pool-A
  labels:
    apps.openyurt.io/desired-nodepool: Pool-A 
    apps.openyurt.io/example: test-Pool-A
    apps.openyurt.io/nodepool: Pool-A 
spec:
  taints:
  - effect: NoSchedule
    key: apps.openyurt.io/example
    value: test-Pool-A
status:
***
```

### UnitedDeployment

- 1 create an uniteddeployment which use deployment template

```bash
cat <<EOF | kubectl apply -f -
apiVersion: apps.openyurt.io/v1alpha1
kind: UnitedDeployment
metadata:
  labels:
    controller-tools.k8s.io: "1.0"
  name: ud-test
spec:
  selector:
    matchLabels:
      app: ud-test
  workloadTemplate:
    deploymentTemplate:
      metadata:
        labels:
          app: ud-test
      spec:
        template:
          metadata:
            labels:
              app: ud-test
          spec:
            containers:
              - name: nginx
                image: nginx:1.19.1
  topology:
    pools:
    - name: edge
      nodeSelectorTerm:
        matchExpressions:
        - key: apps.openyurt.io/nodepool
          operator: In
          values:
          - Pool-A 
      replicas: 3 
      tolerations:
      - effect: NoSchedule
        key: apps.openyurt.io/example
        operator: Exists
  revisionHistoryLimit: 5
EOF
```

- 2 Get UnitedDeployment
```bash
$ kubectl get ud
NAME      READY   WORKLOADTEMPLATE   AGE
ud-test   1       Deployment         23s
```

check the sub deployment created by yurt-app-manager controller
```bash
$ kubectl get deploy
NAME                 READY   UP-TO-DATE   AVAILABLE   AGE
ud-test-edge-ttthd   1/1     1            1           100s
```

check the pod created by UnitedeDeployment , and you will find that these pods will be created on all the hosts under the `Pool-A` NodePool,  and all the pods created by UnitedDeployment use the same image: `nginx:1.19.1`

```
$ kubectl get pod -l app=ud-test

NAME                                  READY   STATUS    RESTARTS   AGE
ud-test-edge-ttthd-5db8f454dd-8jd4l   1/1     Running   0          7m
ud-test-edge-ttthd-5db8f454dd-ggmfb   1/1     Running   0          34s
ud-test-edge-ttthd-5db8f454dd-r6ptr   1/1     Running   0          34s  
```
- 3 Change UnitedDeployment workloadTemplate's image 
change image from `nginx:1.19.1` to `nginx:1.19.3`, and you will find that all pods created by UnitedDeployment use the same image:`nginx:1.19.3` 

```bash
$ kubectl patch ud ud-test --type='json' -p '[{"op": "replace", "path": "/spec/workloadTemplate/deploymentTemplate/spec/template/spec/containers/0/image", "value": "nginx:1.19.3"}]'
$ kubectl get pod -l app=ud-test
```

- 4 Scale the number of pods under a node pool
```bash
$ kubectl patch ud ud-test --type='json' -p '[{"op": "replace", "path": "/spec/topology/pools/0/replicas", "value": 6}]'
```
You will find that the number of pods will be expanded to six

```bash
$ kubectl get pod -l app=ud-test
NAME                                  READY   STATUS    RESTARTS   AGE
ud-test-edge-ttthd-5bbb4b8664-2f5ss   1/1     Running   0          10m
ud-test-edge-ttthd-5bbb4b8664-8sjs7   1/1     Running   0          3m20s
ud-test-edge-ttthd-5bbb4b8664-99dml   1/1     Running   0          10m
ud-test-edge-ttthd-5bbb4b8664-dvb8s   1/1     Running   0          10m
ud-test-edge-ttthd-5bbb4b8664-lxhgr   1/1     Running   0          3m20s
ud-test-edge-ttthd-5bbb4b8664-zj8ls   1/1     Running   0          3m20s
```