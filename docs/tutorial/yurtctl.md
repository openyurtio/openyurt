# `Yurtctl` tutorial

This tutorial demonstrates how to use `yurtctl` to install/uninstall OpenYurt.
Please refer to the [Getting Started](https://github.com/openyurtio/openyurt#getting-started) section on the README page to prepare and build binary to `_output/bin/yurtctl`.
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
$ kubectl describe node | grep Labels -A 5
Labels:      alibabacloud.com/is-edge-worker=true
$ kubectl describe node | grep Annotations -A 5
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
$ curl -s <http://127.0.0.1:10261>
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
Labels:             openyurt.io/is-edge-worker=false
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

## Setup Yurttunnel

Since version v0.2.0, users can setup the yurttunnel using `yurtctl convert`.

Assume that the origin cluster is a two-nodes minikube cluster:

```bash
$ kubectl get node -o wide
NAME           STATUS   ROLES    AGE   VERSION   INTERNAL-IP   EXTERNAL-IP   OS-IMAGE           KERNEL-VERSION     CONTAINER-RUNTIME
minikube       Ready    master   72m   v1.18.3   172.17.0.3    <none>        Ubuntu 20.04 LTS   4.19.76-linuxkit   docker://19.3.8
minikube-m02   Ready    <none>   71m   v1.18.3   172.17.0.4    <none>        Ubuntu 20.04 LTS   4.19.76-linuxkit   docker://19.3.8
```

Then, by simply running the `yurtctl convert` command with the enabling of the option `--deploy-yurttunnel`,
yurttunnel servers will be deployed on cloud nodes, and an yurttunnel agent will be deployed on every edge node.

```bash
$ yurtctl convert --deploy-yurttunnel --cloud-nodes minikube --provider minikube
I0831 12:35:51.719391   77322 convert.go:214] mark minikube as the cloud-node
I0831 12:35:51.728246   77322 convert.go:222] mark minikube-m02 as the edge-node
I0831 12:35:51.753830   77322 convert.go:251] the yurt-controller-manager is deployed
I0831 12:35:51.910440   77322 convert.go:270] yurt-tunnel-server is deployed
I0831 12:35:51.999384   77322 convert.go:278] yurt-tunnel-agent is deployed
I0831 12:35:51.999409   77322 convert.go:282] deploying the yurt-hub and resetting the kubelet service...
I0831 12:36:22.109338   77322 util.go:173] servant job(yurtctl-servant-convert-minikube-m02) has succeeded
I0831 12:36:22.109368   77322 convert.go:292] the yurt-hub is deployed
```

To verify that the yurttunnel works as expected, please refer to
the [yurttunnel tutorial](https://github.com/openyurtio/openyurt/blob/master/docs/tutorial/yurt-tunnel.md)

## Set the path of configuration
Sometimes the configuration of the node may be different. Users can set the path of the kubelet service configuration
by the option `--kubeadm-conf-path`, which is used by kubelet component to join the cluster on the edge node.
```
$ _output/bin/yurtctl convert --kubeadm-conf-path /etc/systemd/system/kubelet.service.d/10-kubeadm.conf
```
The path of the directory on edge node containing static pod files can be set by the
option `--pod-manifest-path`.
```
$ _output/bin/yurtctl convert --pod-manifest-path /etc/kubernetes/manifests
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

In addition, the path of the kubelet service configuration can be set by the option `--kubeadm-conf-path`,
and the path of the directory on edge node containing static pod files can be set by the option `--pod-manifest-path`.

## Subcommand
### Convert a Kubernetes node to Yurt edge node

You can use the subcommand `yurtctl convert edgenode` to convert a Kubernetes node to Yurt edge node separately.

This command can convert the node locally or remotely. One or more nodes that need to be converted can be specified
by the option `--edge-nodes`. If this option is not used, the local node will be converted.

Convert the Kubernetes node n80 to an Yurt edge node:
```
$ _output/bin/yurtctl convert edgenode --edge-nodes n80
I0209 11:03:30.101308   13729 edgenode.go:251] mark n80 as the edge-node
I0209 11:03:30.136313   13729 edgenode.go:279] setting up yurthub on node
I0209 11:03:30.137013   13729 edgenode.go:294] setting up yurthub apiserver addr
I0209 11:03:30.137500   13729 edgenode.go:311] create the /etc/kubernetes/manifests/yurt-hub.yaml
I0209 11:03:40.140073   13729 edgenode.go:403] yurt-hub healthz is OK
I0209 11:03:40.140555   13729 edgenode.go:330] revised kubeconfig /var/lib/openyurt/kubelet.conf is generated
I0209 11:03:40.141592   13729 edgenode.go:351] kubelet.service drop-in file is revised
I0209 11:03:40.141634   13729 edgenode.go:354] systemctl daemon-reload
I0209 11:03:40.403453   13729 edgenode.go:359] systemctl restart kubelet
I0209 11:03:40.469911   13729 edgenode.go:364] kubelet has been restarted
```
You can verify this by inspecting its labels and getting the status of pod yurt-hub.

### Revert an Yurt edge node to Kubernetes node

The subcommand `yurtctl revert edgenode` can revert a Yurt edge node. The option `--edge-nodes` of command can be used
to specify one or more nodes to be reverted which is the same as the `yurtctl convert edgenode`.

Assume that the cluster is an OpenYurt cluster and has an Yurt edge node n80:
```
$ kubectl describe node n80 | grep Labels
Labels:             openyurt.io/is-edge-worker=true
$ kubectl get pod -A | grep yurt-hub
kube-system   yurt-hub-n80               1/1     Running   0          7m32s
```
Using `yurtctl revert edgenode` to revert:
```
$ _output/bin/yurtctl revert edgenode --edge-nodes n80
I0209 11:01:46.837812   12217 edgenode.go:225] label alibabacloud.com/is-edge-worker is removed
I0209 11:01:46.838454   12217 edgenode.go:252] found backup file /etc/systemd/system/kubelet.service.d/10-kubeadm.conf.bk, will use it to revert the node
I0209 11:01:46.838689   12217 edgenode.go:259] systemctl daemon-reload
I0209 11:01:47.085554   12217 edgenode.go:265] systemctl restart kubelet
I0209 11:01:47.153388   12217 edgenode.go:270] kubelet has been reset back to default
I0209 11:01:47.153680   12217 edgenode.go:282] yurt-hub has been removed
```
You can verify this by inspecting its labels and getting the status of pod yurt-hub.

## Troubleshooting

### 1. Failure due to pulling image timeout

The default timeout value of cluster conversion is 2 minutes. Sometimes pulling the related images
might take more than 2 minutes. To avoid the conversion failure due to pulling images timeout, you can:
  - use mirrored image from aliyun container registry(ACR)
```bash
$ _output/bin/yurtctl convert --provider minikube \
  --yurt-controller-manager-image registry.cn-hangzhou.aliyuncs.com/openyurt/yurt-controller-manager:latest \
  --yurt-tunnel-agent-image registry.cn-hangzhou.aliyuncs.com/openyurt/yurt-tunnel-agent:latest \
  --yurt-tunnel-server-image registry.cn-hangzhou.aliyuncs.com/openyurt/yurt-tunnel-server:latest \
  --yurtctl-servant-image registry.cn-hangzhou.aliyuncs.com/openyurt/yurtctl-servant:latest \
  --yurthub-image registry.cn-hangzhou.aliyuncs.com/openyurt/yurthub:latest
```
  - or pull all images on the node manually
  or use automation tools such as `broadcastjob`(from [Kruise](https://github.com/openkruise/kruise/blob/master/docs/tutorial/broadcastjob.md)) in advance.

### 2. Adhoc failure recovery

In case any adhoc failure makes the Kubelet fail to communicate with APIServer, one can recover the original Kubelet setup by
running the following command in edge node directly:
```
$ sudo sed -i "s|--kubeconfig=.*kubelet.conf|--kubeconfig=/etc/kubernetes/kubelet.conf|g;" /etc/systemd/system/kubelet.service.d/10-kubeadm.conf && sudo systemctl daemon-reload && sudo systemctl restart kubelet.service
```
