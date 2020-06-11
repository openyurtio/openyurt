# `Yurtctl` tutorial

This tutorial demonstrates how to use `yurtctl` to install/uninstall OpenYurt.
Please refer to the [Getting Started](https://github.com/alibaba/openyurt#getting-started) section on the README page to prepare and build binary to `_output/bin/yurtctl` .
We assume a minikube cluster ([version 1.14 or less](https://github.com/kubernetes/minikube/releases/tag/v1.0.0)) 
is installed. 

## Convert a minikube cluster

Let us use `yurtctl` to convert a standard Kubernetes cluster to an OpenYurt cluster.

1. Run the following command
```bash
$ _output/bin/yurtctl convert --provider minikube
```

2. `yurtctl` will install all required components and reset the kubelet in the edge node. The output looks like:
```bash
convert.go:148] mark minikube as the edge-node
convert.go:178] deploy the yurt controller manager
convert.go:190] deploying the yurt-hub and resetting the kubelet service...
util.go:137] servant job(yurtctl-servant-convert-minikube) has succeeded
```

3. yurt controller manager and yurthub Pods will be up and running in one minute. Let us verify them:
```bash
$ kubectl get deploy yurt-controller-manager -n kube-system
NAME            READY   UP-TO-DATE   AVAILABLE   AGE
yurt-ctrl-mgr   1/1     1            1           23h
$ kubectl get po yurt-hub-minikube -n kube-system
NAME                READY   STATUS    RESTARTS   AGE
yurt-hub-minikube   1/1     Running   0          23h
```

4. Next, we mark desired edge nodes as autonomous (only pods running on the autonomous edge nodes will be prevented from being evicted during disconnection):
```bash
$ _output/bin/yurtctl markautonomous
I0602 14:11:08.610222   89160 markautonomous.go:149] mark minikube-m02 as autonomous
```

5. As the minikube cluster only contains one node, the node will be marked as an autonomous edge node. Let us verify this by inspecting the node's labels and annotations:
```
$ kubectl describe node | grep Labels -A 3
Labels:      alibabacloud.com/is-edge-worker=true
$ kubectl describe node | grep Annotations -A 3
Annotations: node.beta.alibabacloud.com/autonomy: true
```

By now, the OpenYurt cluster is ready. Users will not notice any differences compared to native Kubernetes when operating the cluster.
If you login to the node, you will find the local cache has been populated:

```
$ minikube ssh
$ ls /etc/kubernetes/cache/kubelet/
configmaps  events  leases  nodes  pods  secrets  services
```


### Test node autonomy

To test if edge node autonomy works as expected, we will simulate a node "offline" scenario. 
1. Let's first create a sample pod:
```bash
kubectl apply -f-<<EOF
apiVersion: v1
kind: Pod
metadata:
  name: bbox
spec:
  containers:
  - image: busybox
    command:
    - top
    name: bbox
EOF
```

2. Make the edge node "offline" by changing the `yurthub`'s server-addr to an unreachable address:
```bash
$ minikube ssh
$ sudo sed -i 's|--server-addr=.*|--server-addr=https://1.1.1.1:1111|' /etc/kubernetes/manifests/yurt-hub.yaml 
```

3. Now `yurthub` is disconnected from the apiserver and works in offline mode. To verify this, we can do the following:
```bash
$ minikube ssh
$ curl -s http://127.0.0.1:10261
{
  "kind": "Status",
  "metadata": {

  },
  "status": "Failure",
  "message": "request( get : /) is not supported when cluster is unhealthy",
  "reason": "BadRequest",
  "code": 400
}
```

4. After 40 seconds, the edge node status becomes `NotReady`, but the pod/bbox won't be evicted and keeps running on the node:
```bash
$ kubectl get node && kubectl get po
NAME       STATUS     ROLES    AGE   VERSION
minikube   NotReady   master   58m   v1.18.2
NAME   READY   STATUS    RESTARTS   AGE
bbox   1/1     Running   0          19m
```

## Convert a multi-nodes Kubernetes cluster

An OpenYurt cluster may consist of some edge nodes and some nodes in the cloud site. 
`yurtctl` allows users to specify a list of cloud nodes that won't be converted.


1. Start with a [two-nodes ack cluster](https://cn.aliyun.com/product/kubernetes), 
```bash
$ kubectl get node 
NAME                     STATUS   ROLES    AGE   VERSION
us-west-1.192.168.0.87   Ready    <none>   19h   v1.14.8-aliyun.1
us-west-1.192.168.0.88   Ready    <none>   19h   v1.14.8-aliyun.1
```

2. You can convert only one node to edge node(i.e., minikube-m02) by using this command:
```bash
$ _output/bin/yurtctl convert --provider ack --cloud-nodes us-west-1.192.168.0.87
I0529 11:21:05.835781    9231 convert.go:145] mark us-west-1.192.168.0.87 as the cloud-node
I0529 11:21:05.861064    9231 convert.go:153] mark us-west-1.192.168.0.88 as the edge-node
I0529 11:21:05.951483    9231 convert.go:183] deploy the yurt controller manager
I0529 11:21:05.974443    9231 convert.go:195] deploying the yurt-hub and resetting the kubelet service...
I0529 11:21:26.075075    9231 util.go:147] servant job(yurtctl-servant-convert-us-west-1.192.168.0.88) has succeeded
```

3. Node `us-west-1.192.168.0.87` will be marked as a non-edge node. You can verify this by inspecting its labels:
```bash
$ kubectl describe node us-west-1.192.168.0.87 | grep Labels
Labels:             alibabacloud.com/is-edge-worker=false
```

4. Same as before, we make desired edge nodes autonomous:
```bash
$ _output/bin/yurtctl markautonomous
I0602 11:22:05.610222   89160 markautonomous.go:149] mark us-west-1.192.168.0.88 as autonomous
```

5. When the OpenYurt cluster contains cloud nodes, yurt controller manager will be deployed on the cloud node (in this case, the node `us-west-1.192.168.0.87`):
```bash
$ kubectl get pods -A -o=custom-columns='NAME:.metadata.name,NODE:.spec.nodeName'
NAME                                               NODE
yurt-controller-manager-6947f6f748-lxfdx           us-west-1.192.168.0.87
```

## Revert/Uninstall OpenYurt

Using `yurtctl` to revert an OpenYurt cluster can be done by doing the following:
```
$ _output/bin/yurtctl revert
revert.go:100] label alibabacloud.com/is-edge-worker is removed
revert.go:110] yurt controller manager is removed
revert.go:124] ServiceAccount node-controller is created
util.go:137] servant job(yurtctl-servant-revert-minikube-m02) has succeeded
revert.go:133] yurt-hub is removed, kubelet service is reset
```
Note that before performing the uninstall, please make sure all edge nodes are reachable from the apiserver. 

## Troubleshooting

### 1. Failure due to pulling image timeout

The default timeout value of cluster conversion is 2 minutes. Sometimes pulling the related images 
might take more than 2 minutes. To avoid the conversion failure due to pulling images timeout, you can pull all images on the node manually
or use automation tools such as `broadcastjob`(from [Kruise](https://github.com/openkruise/kruise/blob/master/docs/concepts/broadcastJob/README.md)) in advance.

### 2. Adhoc failure recovery

In case any adhoc failure makes the Kubelet fail to communicate with APIServer, one can recover the original Kubelet setup by
running the following command in edge node directly:
```
$ sudo sed -i "s|--kubeconfig=.*kubelet.conf|--kubeconfig=/etc/kubernetes/kubelet.conf|g;" /etc/systemd/system/kubelet.service.d/10-kubeadm.conf && sudo systemctl daemon-reload && sudo systemctl restart kubelet.service
```
