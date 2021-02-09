# Manually Setup

This tutorial shows how to setup OpenYurt cluster manually. The cluster used in this tutorial is a
two-nodes ACK(version 1.14.8) cluster, and all the yaml files used in this tutorial can be found
at `config/setup/`.

## Label cloud nodes and edge nodes

When disconnected from the apiserver, only the pod running on the autonomous edge node will
be prevented from being evicted from nodes. Therefore, we first need to divide nodes into two categories, the cloud node
and the edge node, by using label `alibabacloud.com/is-edge-worker`. Assume that the given Kubernetes cluster
has two nodes,
```bash
$ kubectl get nodes
NAME                     STATUS   ROLES    AGE     VERSION
us-west-1.192.168.0.87   Ready    <none>   3d23h   v1.14.8-aliyun.1
us-west-1.192.168.0.88   Ready    <none>   3d23h   v1.14.8-aliyun.1
```
and we will use node `us-west-1.192.168.0.87` as the cloud node.

We label the cloud node with value `false`,
```bash
$ kubectl label node us-west-1.192.168.0.87 alibabacloud.com/is-edge-worker=false
node/us-west-1.192.168.0.87 labeled
```

and the edge node with value `true`.
```bash
$ kubectl label node us-west-1.192.168.0.88 alibabacloud.com/is-edge-worker=true
node/us-west-1.192.168.0.88 labeled
```

To active the autonomous mode, we annotate the edge node by typing following command
```bash
$ kubectl annotate node us-west-1.192.168.0.88 node.beta.alibabacloud.com/autonomy=true
node/us-west-1.192.168.0.88 annotated
```

## Setup Yurt-controller-manager

Next, we need to deploy the Yurt controller manager, which prevents apiserver from evicting pods running on the
autonomous edge nodes during disconnection.
```bash
$ kc ap -f config/setup/yurt-controller-manager.yaml
deployment.apps/yurt-controller-manager created
```
## Disable the default nodelifecycle controller

To allow the yurt-controller-mamanger to work properly, we need to turn off the default nodelifecycle controller.
The nodelifecycle controller can be disabled by restarting the kube-controller-manager with a proper `--controllers`
option. Assume that the original option looks like `--controllers=*,bootstrapsigner,tokencleaner`, to disable
the nodelifecycle controller, we change the option to `--controllers=*,bootstrapsigner,tokencleaner,-nodelifecycle`.

If the kube-controller-manager is deployed as a static pod on the master node, and you have the permission to log in
to the master node, then above operations can be done by revising the file
`/etc/kubernetes/manifests/kube-controller-manager.yaml`. After revision, the kube-controller-manager will be
restarted automatically.

## Setup Yurthub

After the Yurt controller manager is up and running, we will setup Yurthub as the static pod. Before proceeding,
please get the apiserver's address (i.e., ip:port), which will be used to replace the place holder in the template
file `config/setup/yurthub.yaml`. In the following command, we assume that the address of the apiserver is 1.2.3.4:5678
```bash
$ cat config/setup/yurthub.yaml |
sed 's|__pki_path__|/etc/kubernetes/pki|;
s|__kubernetes_service_host__|1.2.3.4|;
s|__kubernetes_service_port_https__|5678|' > /tmp/yurthub-ack.yaml &&
scp -i <yourt-ssh-identity-file> /tmp/yurthub-ack.yaml root@us-west-1.192.168.0.88:/etc/kubernetes/manifests
```
and the Yurthub will be ready in minutes.

## Setup Yurt-tunnel (Optional)

Please refer to this [document](.//yurt-tunnel.md#5-setup-the-yurt-tunnel-manually) to setup Yurttunnel manually.

## Reset the Kubelet

By now, we have setup all required components for the OpenYurt cluster, next, we only need to reset the
kubelet service to let it access the apiserver through the yurthub (The following steps assume that we are logged
in to the edge node as the root user).

To do so, we create a new kubeconfig file for the kubelet service based on the original one (i.e., `/etc/kubernetes/kubelet.confg`).
```bash
$ mkdir -p /var/lib/openyurt && cp /etc/kubernetes/kubelet.conf /var/lib/openyurt
```

As kubelet will connect to the Yurthub through http, we need to remove unnecessary field from the newly created kubeconfig file
```bash
sed -i '/certificate-authority-data/d;
    /client-key/d;
    /client-certificate/d;
    /user:/d;
    s/ https.*/ http:\/\/127.0.0.1:10261/g' /var/lib/openyurt/kubelet.conf
```

In order to let kubelet to use the revised kubeconfig, we edit the drop-in file of the kubelet
service (i.e., `/etc/systemd/system/kubelet.service.d/10-kubeadm.conf`)
```bash
sed -i "s|KUBELET_KUBECONFIG_ARGS=--bootstrap-kubeconfig=\/etc\/kubernetes\/bootstrap-kubelet.conf\ --kubeconfig=\/etc\/kubernetes\/kubelet.conf|KUBELET_KUBECONFIG_ARGS=--kubeconfig=\/var\/lib\/openyurt\/kubelet.conf|g" \
    /etc/systemd/system/kubelet.service.d/10-kubeadm.conf
```

Finally, we restart the kubelet service
```bash
# assume we are logged in to the edge node already
$ systemctl daemon-reload && systemctl restart kubelet
```
