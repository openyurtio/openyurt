# Proposal-Optimize the pods recovery efficiency when edge nodes restart

## Requirement Analysis

OpenYurt extend the cloud native ability to edge computing and IoT scenarios. The cloud nodes provide the ability to deploy the services on the edge nodes and realize the whole life-cycle management of the applications. However, edge node network is in the weak connection status.

Thereby, the frequent restart of the edge nodes involves the OS restart, Kubernetes components restart and OpenYurt components restart. The recovery of service applications costs almost 1 minute. To recover the edge nodes faster, we should optimize the pods recovery efficiency when edge nodes restart.

The specific requirements of the optimization are summarized as follows:

- The restart process will not be blocked, and all the components will be restarted successfully
- All the pods can restart successfully and satisfy the stable status of the applications
- Promote the restart efficiency and make the cost time less than 30s
- The solutions can be used in most hardwares

## Question Analysis

Kubelet is the controller of the Kubernetes work node. It is responsible for the creation and management of Pods on the work node. For an OpenYurt cluster, the components are the pods deployed on the Kubernetes cluster. When the work node restarts, Kubelet will recover the pods (Openyurt components) and the service pods. Therefore, detect the time delay of the Kubelet operations should be the focus.

![](../img/pod-restart-step.png)

The operations of Kubelet component when the work node restart are shown in Figure X. After the work node restart, Kubelet will initialize first. Then, Kubelet will start the static pods, for instance, YurtHub, etc. After that, YurtHub start-up and load the local cache. In the weak connection condition of the network, the Kubelet list/watch the pods from the YurtHub cache. Lastly, the Kubelet will recover the other pods according to the cache.

According to the analysis above, YurtHub is the important component for the work node restart. The YurtHub recovery cost a period of time. Moreover, Kubelet will recover the service pods according to the local cache. The process depends on the CacheManager and StorageManager of YurtHub.

![](../img/yurthub-start-process-en.png)

Figure X shows the YurtHub init progress. According to the investigation of YurtHub source code, the start-up process are serialized. Each component part will start one by one. The time delay can be evaluated by setting logs.



From the above, the optimization will focus on the YurtHub start-up and Kubelet recover the service pods based on YurtHub local cache .

## Experiments

### Native Kubernetes Start-up Time Test

**Test Environment:**

1 master node, 1 work node

- Alibaba ECS 4C8G
- OS: CentOS 8
- K8s Version: v1.19.4
- Edge Network Connected

**Notations of Each Time Point:**

|                        Time Point                        | Notation                                                     |
| :------------------------------------------------------: | :----------------------------------------------------------- |
|                    *OS Restart Begin*                    | The time point when OS shut down and begins to restart. We donate this point as the zero point of the time delay detection. |
|                     *Kubelet Start*                      | After the OS recovery, the time point then Kubelet service start. |
| *Kubelet Start Watching API Server and Static Pods Info* | After the Kubelet initialization, basic functions will recover. The time point Kubelet start to watch API server (YurtHub Server) and  recover the static pods info. |
|              *Kubelet recover Static Pods*               | The time point kubelet recover all the static pods (CoreDNS, Flannel) on the edge node. |
|                     *YurtHub Start*                      | The time point YurtHub start.                                |
|                  *YurtHub Server Work*                   | The time point YurtHub server is recovered and it can handle the HTTP request. |
|               *First Service Pod Recovery*               | The time point first service pod start to recover.           |
|               *Last Service Pod Recovery*                | The time point last service pod start to recover.            |



#### Only static pods

*The table records the **time-delay** from **OS restart** and the unit is ms

| Test Index | OS Restart Begin | Kubelet Start | Kubelet Start Watching API Server and Static Pods Info | Kubelet recover Static Pods |
| :--------: | :--------------: | :-----------: | :----------------------------------------------------: | :-------------------------: |
|     01     |        0         |     21441     |                         22065                          |            24237            |
|     02     |        0         |     21500     |                         22102                          |            24362            |
|     02     |        0         |     21427     |                         22083                          |            24252            |
|   *Avg*    |        0         |     21456     |                         22083                          |            24284            |
|   *Diff*   |        0         |     21456     |                          627                           |            2200             |

#### Static pods and 10 service pods

*The 10 service pods are created by nginx-latest image with ClusterIP service

*The table records the **time-delay** from **OS restart** and the unit is ms

| Test Index | OS Restart Begin | Kubelet Start | Kubelet Start Watching API Server and Static Pods Info | Kubelet recover Static Pods | First Service Pod Recovery | Last Service Pod Recovery |
| :--------: | :--------------: | :-----------: | :----------------------------------------------------: | :-------------------------: | :------------------------: | :-----------------------: |
|     01     |        0         |     19954     |                         20711                          |            49167            |           51167            |          150167           |
|     02     |        0         |     21067     |                         21835                          |            50289            |           52227            |          152109           |
|     03     |        0         |     21072     |                         21897                          |            50272            |           52216            |          152209           |
|   *Avg*    |        0         |     20698     |                         21481                          |            49909            |           51870            |          151495           |
|   *Diff*   |        0         |     20698     |                          783                           |            28428            |            1961            |           99625           |

### OpenYurt Start-up Time Test

**Test Environment:**

1 master node, 1 work node

- Alibaba ECS 4C8G
- OS: CentOS 8
- K8s Version: v1.19.4
- OpenYurt Version: v0.7.0

#### Edge Network Connected, only static pods

*The table records the **time-delay** from **OS restart** and the unit is ms

| Test Index | OS Restart Begin | Kubelet Start | Kubelet Start Watching API Server and Static Pods Info | YurtHub Start | YurtHub Server Work | All static Pods Recovery |
| :--------: | :--------------: | :-----------: | :----------------------------------------------------: | :-----------: | :-----------------: | :----------------------: |
|     01     |        0         |     26545     |                         27547                          |     34973     |        35054        |          52423           |
|     02     |        0         |     24915     |                         25880                          |     33253     |        33337        |          50001           |
|     03     |        0         |     27936     |                         28673                          |     36027     |        36110        |          53012           |
|   *Avg*    |        0         |     26465     |                         27366                          |     34751     |        34833        |          51812           |
|   *Diff*   |        -         |     26465     |                          901                           |     7205      |         82          |          16979           |

#### Edge Network Connected, static pods and 10 service pods

*The 10 service pods are created by nginx-latest image with ClusterIP service

*The table records the **time-delay** from **OS restart** and the unit is ms

| Test Index | OS Restart Begin | Kubelet Start | Kubelet Start Watching API Server and Static Pods Info | YurtHub Start | YurtHub Server Work | All static Pods Recovery | First Service Pod Recovery | Last Service Pod Recovery |
| :--------: | :--------------: | :-----------: | :----------------------------------------------------: | :-----------: | :-----------------: | :----------------------: | :------------------------: | :-----------------------: |
|     01     |        0         |     26560     |                         27295                          |     34618     |        34697        |          94000*          |           74390            |          170126           |
|     02     |        0         |     28076     |                         29001                          |     36814     |        36928        |          54030           |           56920            |          153410           |
|     03     |        0         |     26398     |                         27427                          |     34834     |        34907        |          42000           |           58103            |          153875           |
|   *Avg*    |        0         |     27011     |                         27908                          |     35422     |        35511        |          63343           |           63138            |          159137           |
|   *Diff*   |        0         |     27011     |                          896                           |     7514      |         898         |          27833           |           -206**           |           95999           |

*It costs more time because waiting for the CNI Network ready

**The time between first service pod recovery and all static pods recovery is varied, and it is not reliable

#### Edge Network Disconnected, static pods and 10 service pods, OpenYurt Edge

*The 10 service pods are created by nginx-latest image with ClusterIP service

*The table records the **time-delay** from **OS restart** and the unit is ms

| Test Index | OS Restart Begin | Kubelet Start | Kubelet Start Watching API Server and Static Pods Info | YurtHub Start | YurtHub Server Work | All static Pods Recovery | First Service Pod Recovery | Last Service Pod Recovery |
| :--------: | :--------------: | :-----------: | :----------------------------------------------------: | :-----------: | :-----------------: | :----------------------: | :------------------------: | :-----------------------: |
|     01     |        0         |     29027     |                         30368                          |     37436     |        38893        |          59249           |           60420            |          170769           |
|     02     |        0         |     27396     |                         28246                          |     35565     |        37029        |          52780           |           58912            |          164756           |
|     03     |        0         |     27363     |                         28235                          |     35527     |        36990        |          52741           |           58879            |          164712           |
|   *Avg*    |        0         |     27929     |                         28950                          |     36176     |        37637        |          54923           |           59404            |          166746           |
|   *Diff*   |        0         |     27929     |                          1021                          |     7226      |        1461         |          17286           |            4480            |          107342           |

## Data analysis between different scenarios

- In both native Kubernetes and YurtHub network connected/disconnected environments, the time delay between first service pod recovery and last pod recovery is stable 100s.
- In the YurtHub mode, Kubelet will wait for YurtHub ready and start the other pods recover process. This period of time is almost 7s.
- Compared with time delay between YurtHub server work and first service pod recovery in network  connected and network disconnected scenarios, network disconnected scenarios can save almost 4s due to the local cache query.

## How to optimize the pods recovery efficiency?

1. **Time between Kubelet watching API and YurtHub init**
   - After the kubelet initializing, it will wait for YurtHub ready and start the other pods recover process. If the YurtHub can restart itself sync with Kubelet, the time delay will reduce.
   - The expect deduct time is 7000ms, the optimize efficiency may 8%-10%
2. **Time between the YurtHub server work and first pod recovery**
   - After the YurtHub server work, almost 23000ms will be cost before the first pod starts to recover. If YurtHub can read the cache files more faster, the time delay will be reduce.
   - This part can be optimized with the optimization of the YurtHub Cache Manager, the optimize efficiency may be estimated by the the further test.
3. **Time between the first service pod recovery and last service pod recovery**
   - The time between the first pod recovery and the last pod recovery is almost 100000ms.
   - This part was controlled by Kubelet and we can focus on the communication between the kubelet and YurtHub.