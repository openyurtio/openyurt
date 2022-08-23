# Proposal-Optimize the pods recovery efficiency when edge nodes restart

## 1. Requirement Analysis

OpenYurt extend the cloud native ability to edge computing and IoT scenarios. The cloud nodes provide the ability to deploy the services on the edge nodes and realize the whole life-cycle management of the applications. However, edge node network is in the weak connection status.

Thereby, the frequent restart of the edge nodes involves the OS restart, Kubernetes components restart and OpenYurt components restart. The recovery of service applications costs almost 1 minute. To recover the edge nodes faster, we should optimize the pods recovery efficiency when edge nodes restart.

The specific requirements of the optimization are summarized as follows:

- The restart process will not be blocked, and all the components will be restarted successfully
- All the pods can restart successfully and satisfy the stable status of the applications
- Promote the restart efficiency and make the cost time less than 30s
- The solutions can be used in most hardwares

## 2. Question Analysis

Kubelet is the controller of the Kubernetes work node. It is responsible for the creation and management of Pods on the work node. For an OpenYurt cluster, the components are the pods deployed on the Kubernetes cluster. When the work node restarts, Kubelet will recover the pods (Openyurt components) and the service pods. Therefore, detect the time delay of the Kubelet operations should be the focus.

![](../img/pod-restart-step.png)

The operations of Kubelet component when the work node restart are shown in Figure X. After the work node restart, Kubelet will initialize first. Then, Kubelet will start the static pods, for instance, YurtHub, etc. After that, YurtHub start-up and load the local cache. In the weak connection condition of the network, the Kubelet list/watch the pods from the YurtHub cache. Lastly, the Kubelet will recover the other pods according to the cache.

According to the analysis above, YurtHub is the important component for the work node restart. The YurtHub recovery cost a period of time. Moreover, Kubelet will recover the service pods according to the local cache. The process depends on the CacheManager and StorageManager of YurtHub.

![](../img/yurthub-start-process-en.png)

Figure X shows the YurtHub init progress. According to the investigation of YurtHub source code, the start-up process are serialized. Each component part will start one by one. The time delay can be evaluated by setting logs.



From the above, the optimization will focus on the YurtHub start-up and Kubelet recover the service pods based on YurtHub local cache .

## 3. Experiments

### 3.1 Native Kubernetes Start-up Time Test

**Test Environment:**

1 master node, 1 work node

- Alibaba ECS 4C8G
- OS: CentOS 8
- K8s Version: v1.19.4
- Edge Network Connected

#### 3.1.1 Static pods and 10 service pods

*The 10 service pods are created by nginx-latest image

*The table records the **time-delay** from **OS restart** and the unit is ms

*The image pull strategy is **IfNotPresent**

*First Service Pod Recovery means the time point when the first nginx pod recover.

*Last Service Pod Recovery means the time point when the first nginx pod recover.

| Test Index | OS Restart Begin | Kubelet Start to work | First Service Pod Recovery | Last Service Pod Recovery |
| :--------: | :--------------: | :-------------------: | :------------------------: | :-----------------------: |
|     1      |        0         |         25498         |           32747            |           33030           |
|     2      |        0         |         27499         |           33701            |           34822           |
|     3      |        0         |         26776         |           33010            |           34112           |
|    Avg     |        0         |         26591         |           33152            |           33988           |
|    Diff    |        -         |         26591         |            6561            |            836            |

### 3.2 OpenYurt Start-up Time Test

**Test Environment:**

1 master node, 1 work node

- Alibaba ECS 4C8G
- OS: CentOS 8
- K8s Version: v1.19.4
- OpenYurt Version: v0.7.0

#### 3.2.1 Edge Network Connected, 10 service pods

*The table records the **time-delay** from **OS restart** and the unit is ms

*The image pull strategy is **IfNotPresent**

*First Service Pod Recovery means the time point when the first nginx pod recover.

*Last Service Pod Recovery means the time point when the last nginx pod recover.

| Test Index | OS Restart Begin | Kubelet Start to work | First Service Pod Recovery | Last Service Pod Recovery |
| :--------: | :--------------: | :-------------------: | :------------------------: | :-----------------------: |
|     1      |        0         |         34030         |           53890            |           54637           |
|     2      |        0         |         35499         |           53365            |           54050           |
|     3      |        0         |         35016         |           53665            |           54523           |
|    Avg     |        0         |         34848         |           53640            |           54403           |
|    Diff    |        -         |         34848         |           18792            |            763            |

#### 3.2.2 Edge Network Disconnected, 10 service pods, OpenYurt Edge

*The 10 service pods are created by nginx-latest image with ClusterIP service

*The table records the **time-delay** from **OS restart** and the unit is ms

*The image pull strategy is **IfNotPresent**

*First Service Pod Recovery means the time point when the first nginx pod recover.

*Last Service Pod Recovery means the time point when the last nginx pod recover.

| Test Index | OS Restart Begin | Kubelet Start to work | First Service Pod Recovery | Last Service Pod Recovery |
| :--------: | :--------------: | :-------------------: | :------------------------: | :-----------------------: |
|     1      |        0         |         36588         |           51845            |           52660           |
|     2      |        0         |         34502         |           50838            |           51622           |
|     3      |        0         |         34435         |           60179            |           60928           |
|    Avg     |        0         |         35175         |           54287            |           55070           |
|    Diff    |        -         |         35175         |           19112*           |            783            |

*According to the experiment, the time between Kubelet Start to work and First Service Pod Recovery is varied.

## 4. Time Delay Comparison between OpenYurt and Native Kubernetes

In this section, the time delay comparison between the OpenYurt Cluster (Network Connected and DIsconnected) and native Kubernetes are shown in the Table below.

Four time periods re defined for comparison.

**Period 1**: From OS restart begin to Kubelet start to work.

**Period 2**: From Kubelet start to work to first service pod recovery.

**Period 3**: From first service pod recovery to last service pod recovery.

**Whole Recovery Period**: From OS restart begin to last service pod recovery.

|                           | Native Kubernetes | OpenYurt Cluster (Network Connected) | OpenYurt Cluster (Network Disconnected) |
| :-----------------------: | :---------------: | :----------------------------------: | :-------------------------------------: |
|       **Period 1**        |      26.59s       |                34.85s                |                 35.17s                  |
|       **Period 2**        |       6.56s       |                18.79s                |                 19.11s                  |
|       **Period 3**        |       0.84s       |                0.76s                 |                  0.78s                  |
| **Whole Recovery Period** |      33.99s       |                54.40s                |                 55.07s                  |

According to the comparison table, OpenYurt Cluster spent 8.26s more than the time Native Kubernetes from OS restart begin to Kubelet start to work (Period 1). Additionally, OpenYurt Cluster spent 12.23s more than Native Kubernetes From Kubelet start to work to first service pod recovery (Period 2). In period 3, time between first service pod recovery to last service pod recovery are close to with the comparison.

Generally, when openyurt edge node restart, the overall pods restart time are 20.41s longer than native Kubernetes node restart.

## 5. Detailed Test on OpenYurt Edge Node

// In this section, I will investigate the added time in **Period 2**: from kubelet start to first pod start.

## 6. Detailed Analysis and Optimization

// Between Kubelet Work to YurtHub Start

​    //detailed analysis

// Between YurtHub Server Work to First service pod recovery

​    //detailed analysis