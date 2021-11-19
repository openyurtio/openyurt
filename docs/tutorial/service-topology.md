# Service Topology

This document introduces how to use *Service Topology* in OpenYurt cluster.

*Service Topology* enables a service to route traffic based on the Node topology of the cluster. For example, a service can specify that traffic be preferentially routed to endpoints that are on the same Node as the client, or in the same availability NodePool.

To use *service topology*, the `EndpointSliceProxying` [featur gate](https://kubernetes.io/docs/reference/command-line-tools-reference/feature-gates/) must be enabled, and kube-proxy needs to be configured to connect to Yurthub instead of the API server.

You can set the `topologyKeys` values of a service to direct traffic as follows. If `topologyKeys` is not specified or empty, no topology constraints will be applied.

|    **annotation Key**    |                  **annotation Value**                  |               **explain**               |
| :----------------------: | :----------------------------------------------------: | :-------------------------------------: |
| openyurt.io/topologyKeys |                 kubernetes.io/hostname                 |   Only to endpoints on the same node.   |
| openyurt.io/topologyKeys | kubernetes.io/zone<br /> or <br />openyurt.io/nodepool | Only to endpoints on the same nodepool. |

## Prerequisites

1. Kubernetes v1.18 or above, since Endpointslice resource need to be supported.
2. Yurt-app-manager is deployed in the cluster.

## How to use

Ensure that kubernetes version is v1.18+.

```bash
$ kubectl get node
NAME                STATUS   ROLES    AGE   VERSION
k8s-control-plane   Ready    master   79s   v1.18.19
k8s-worker          Ready    <none>   47s   v1.18.19
k8s-worker2         Ready    <none>   48s   v1.18.19
```

Ensure that yurt-app-manager is deployed in the cluster.

```BASH
$ kubectl get pod -n kube-system
NAME                                        READY   STATUS    RESTARTS   AGE
coredns-66bff467f8-6972s                    1/1     Running   0          9m30s
coredns-66bff467f8-b5c8d                    1/1     Running   0          9m30s
etcd-k8s-control-plane                      1/1     Running   0          9m39s
kindnet-645vg                               1/1     Running   0          9m12s
kindnet-jghgv                               1/1     Running   0          9m30s
kindnet-zkqzs                               1/1     Running   1          9m12s
kube-apiserver-k8s-control-plane            1/1     Running   0          9m39s
kube-controller-manager-k8s-control-plane   1/1     Running   0          6m33s
kube-proxy-cbsmj                            1/1     Running   0          9m12s
kube-proxy-cqwcs                            1/1     Running   0          9m30s
kube-proxy-m9dgk                            1/1     Running   0          9m12s
kube-scheduler-k8s-control-plane            1/1     Running   0          9m39s
yurt-app-manager-699ffdcb78-bc6j6           1/1     Running   0          2m12s
yurt-app-manager-699ffdcb78-npg49           1/1     Running   0          2m12s
yurt-controller-manager-6c95788bf-7zckb     1/1     Running   0          7m11s
yurt-hub-k8s-control-plane                  1/1     Running   0          4m30s
yurt-hub-k8s-worker                         1/1     Running   0          5m46s
yurt-hub-k8s-worker2                        1/1     Running   0          5m47s
```

### Configure kube-proxy

To use *service topology*, the `EndpointSliceProxying` [featur gate](https://kubernetes.io/docs/reference/command-line-tools-reference/feature-gates/) must be enabled, and kube-proxy needs to be configured to connect to Yurthub instead of the API server.

```bash
$ kubectl edit cm -n kube-system kube-proxy
apiVersion: v1
data:
  config.conf: |-
    apiVersion: kubeproxy.config.k8s.io/v1alpha1
    bindAddress: 0.0.0.0
    featureGates: # enable EndpointSliceProxying feature gate.
      EndpointSliceProxying: true
    clientConnection:
      acceptContentTypes: ""
      burst: 0
      contentType: ""
      #kubeconfig: /var/lib/kube-proxy/kubeconfig.conf # <- comment this line.
      qps: 0
    clusterCIDR: 10.244.0.0/16
    configSyncPeriod: 0s
```

Restart kube-proxy.

```bash
$ kubectl delete pod --selector k8s-app=kube-proxy -n kube-system
pod "kube-proxy-cbsmj" deleted
pod "kube-proxy-cqwcs" deleted
pod "kube-proxy-m9dgk" deleted
```

### Create NodePools

- Create test nodepools.

```bash
$ cat << EOF | kubectl apply -f -
apiVersion: apps.openyurt.io/v1alpha1
kind: NodePool
metadata:
  name: beijing
spec:
  type: Cloud

---

apiVersion: apps.openyurt.io/v1alpha1
kind: NodePool
metadata:
  name: hangzhou
spec:
  type: Edge
  annotations:
    apps.openyurt.io/example: test-hangzhou
  labels:
    apps.openyurt.io/example: test-hangzhou

---

apiVersion: apps.openyurt.io/v1alpha1
kind: NodePool
metadata:
  name: shanghai
spec:
  type: Edge
  annotations:
    apps.openyurt.io/example: test-shanghai
  labels:
    apps.openyurt.io/example: test-shanghai
EOF
```

- Add nodes to the nodepool.

```bash
$ kubectl label node k8s-control-plane apps.openyurt.io/desired-nodepool=beijing
node/k8s-control-plane labeled

$ kubectl label node k8s-worker apps.openyurt.io/desired-nodepool=hangzhou
node/k8s-worker labeled

$ kubectl label node k8s-worker2 apps.openyurt.io/desired-nodepool=shanghai
node/k8s-worker2 labeled
```

- Get NodePool.

```bash
$ kubectl get np
NAME       TYPE    READYNODES   NOTREADYNODES   AGE
beijing    Cloud   1            0               2m44s
hangzhou   Edge    1            0               2m44s
shanghai   Edge    1            0               2m44s
```

### Create UnitedDeployment

- Create test unitedDeployment. To facilitate testing, we use a `serve_hostname` image. Each time port 9376 is accessed, the hostname container returns its own hostname.

```bash
$ cat << EOF | kubectl apply -f -
apiVersion: apps.openyurt.io/v1alpha1
kind: UnitedDeployment
metadata:
  labels:
    controller-tools.k8s.io: "1.0"
  name: ud-hostname
spec:
  selector:
    matchLabels:
      app: ud-hostname
  workloadTemplate:
    deploymentTemplate:
      metadata:
        labels:
          app: ud-hostname
      spec:
        template:
          metadata:
            labels:
              app: ud-hostname
          spec:
            containers:
              - name: hostname
                image: mirrorgooglecontainers/serve_hostname
                ports:
                - containerPort: 9376
                  protocol: TCP
  topology:
    pools:
    - name: hangzhou
      nodeSelectorTerm:
        matchExpressions:
        - key: apps.openyurt.io/nodepool
          operator: In
          values:
          - hangzhou
      replicas: 2
    - name: shanghai
      nodeSelectorTerm:
        matchExpressions:
        - key: apps.openyurt.io/nodepool
          operator: In
          values:
          - shanghai
      replicas: 2
  revisionHistoryLimit: 5
EOF
```

- Get pods that created by the unitedDeployment.

```bash
$ kubectl get pod -l app=ud-hostname
NAME                                          READY   STATUS    RESTARTS   AGE
ud-hostname-hangzhou-65t9j-66fb4c9977-5d4sj   1/1     Running   0          52m
ud-hostname-hangzhou-65t9j-66fb4c9977-r7wj8   1/1     Running   0          52m
ud-hostname-shanghai-ppdwk-6cf7b64669-79m9s   1/1     Running   0          52m
ud-hostname-shanghai-ppdwk-6cf7b64669-c9d6b   1/1     Running   0          52m
```

### Create a test pod

```bash
$ kubectl run nginx-test --image=nginx:1.19.3
pod/nginx-test created

$ kubectl get pod nginx-test -owide
NAME         READY   STATUS    RESTARTS   AGE   IP           NODE          NOMINATED NODE   READINESS GATES
nginx-test   1/1     Running   0          45m   10.244.1.5   k8s-worker2   <none>           <none>
```

We can see nginx-test is be scheduled to the k8s-worker2 node, so it is in the shanghai nodepool.

### Create Service with TopologyKeys

```
$ cat << EOF | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  name: svc-hostname-topology
  annotations:
    openyurt.io/topologyKeys: openyurt.io/nodepool
spec:
  selector:
    app: ud-hostname
  type: ClusterIP
  ports:
  - port: 80
    targetPort: 9376
EOF
```

### Create Service without TopologyKeys

```
$ cat << EOF | kubectl apply -f -
apiVersion: v1
kind: Service
metadata:
  name: svc-hostname
spec:
  selector:
    app: ud-hostname
  type: ClusterIP
  ports:
  - port: 80
    targetPort: 9376
EOF
```

### Test Service Topology

We use nginx-test to test *service topology*. The pod nginx-test is located in Shanghai, so its traffic can only be routed to the nodes that in shanghai nodepool when it accesses a service with the `openyurt.io/topologyKeys: openyurt.io/nodepool` annotation.

For comparison, we first test the service that without serviceTopology annotation. As we can see, its traffic can be routed to any nodes.

```bash
$ kubectl exec -it nginx-test bash
root@nginx-test:/# curl svc-hostname:80
ud-hostname-hangzhou-65t9j-66fb4c9977-5d4sj
root@nginx-test:/# curl svc-hostname:80
ud-hostname-shanghai-ppdwk-6cf7b64669-c9d6b
root@nginx-test:/# curl svc-hostname:80
ud-hostname-hangzhou-65t9j-66fb4c9977-r7wj8
root@nginx-test:/# curl svc-hostname:80
ud-hostname-shanghai-ppdwk-6cf7b64669-c9d6b
root@nginx-test:/# curl svc-hostname:80
ud-hostname-hangzhou-65t9j-66fb4c9977-5d4sj
```

Then we test the service that with serviceTopology annotation. As we can see, its traffic can only be routed to the nodes that in shanghai nodepool.

```bash
$ kubectl exec -it nginx-test bash
root@nginx-test:/# curl svc-hostname-topology:80
ud-hostname-shanghai-ppdwk-6cf7b64669-c9d6b
root@nginx-test:/# curl svc-hostname-topology:80
ud-hostname-shanghai-ppdwk-6cf7b64669-c9d6b
root@nginx-test:/# curl svc-hostname-topology:80
ud-hostname-shanghai-ppdwk-6cf7b64669-c9d6b
root@nginx-test:/# curl svc-hostname-topology:80
ud-hostname-shanghai-ppdwk-6cf7b64669-79m9s
root@nginx-test:/# curl svc-hostname-topology:80
ud-hostname-shanghai-ppdwk-6cf7b64669-79m9s
root@nginx-test:/# curl svc-hostname-topology:80
ud-hostname-shanghai-ppdwk-6cf7b64669-79m9s
root@nginx-test:/# curl svc-hostname-topology:80
ud-hostname-shanghai-ppdwk-6cf7b64669-c9d6b
```

