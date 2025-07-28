节点级别提供prepull接口，controller下发prepull job实现镜像拉取

背景
1. OpenYurt提供了Daemonset的OTA升级能力，用户可以通过API 自主控制应用程序是否更新。
2. 通过annotions配置和updateStrategy配置开启OTA能力
# filename: simple-daemonset.yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: simple-daemonset
  annotations:
  # 开启OTA能力
    apps.openyurt.io/update-strategy: OTA
  labels:
    app: simple-daemon
spec:
  # OTA依赖onDelete updateStrategy策略
  updateStrategy:
    type: OnDelete
  selector:
    matchLabels:
      app: simple-daemon
  template:
    metadata:
      labels:
        app: simple-daemon
    spec:
      tolerations:
      - key: "node-role.kubernetes.io/control-plane"
        operator: "Exists"
        effect: "NoSchedule"
      - key: "node-role.kubernetes.io/master"
        operator: "Exists"
        effect: "NoSchedule" # 旧版本 Kubernetes 使用 master

      containers:
      - name: simple-daemon-container
        image: registry-cn-hangzhou.ack.aliyuncs.com/dev/busybox:1.34
        command: ["sh", "-c"]
        args:
        - echo "Hello from simple daemon on this node!" && sleep infinity
3. 具体实现提供了两个API
  a. GET /pods HTTP/1.1
  b. POST /openyurt.io/v1/namespaces/{ns}/pods/{podname}/upgrade  HTTP/1.1
4. 整体流程如下
@startuml

actor User
participant OTA
participant KAS
participant Kubelet

== 1 Get 更新信息 ==

User -> OTA: 1 Query update status
OTA -> OTA: Get latest workload version (From Hub Cache)
note right of OTA: determine if updates\nare available
OTA --> User: Query return

== 2 Update 执行 ==

User -> OTA: 2 Update Apply
OTA -> KAS: Delete Pod(Etcd)
KAS -> Kubelet: Sync Pod
Kubelet -> Kubelet: Delete Pod(Real)
Kubelet --> KAS: Report back
KAS -> KAS: Recreate Pod
KAS -> Kubelet: Sync Pod
Kubelet -> Kubelet: Create Pod(Real)

@enduml

问题
在OTA升级流程中，从用户执行删除到应用启动中间存在一大段时间gap（旧Pod删除，新Pod创建，镜像拉取，新Pod启动），当镜像体积很大时，镜像拉取花费过多时间，导致应用升级服务中断时间过长。


设计

@startuml
!theme mars

title OpenYurt DaemonSet OTA 升级流程 (优化版)

actor "集群管理员" as Admin
actor "节点侧Pod用户" as PodUser

box "Kubernetes Control Plane" #LightBlue
  participant "APIServer" as API
  participant "OTA Controller\n(Yurt-Manager)" as OTC
  participant "Job Controller\n(Kube-Controller-Manager)" as JobC
end box

box "边缘节点 (Node)" #LightGreen
  participant "Kubelet" as Kubelet
  participant "OTA Module" as NodeOTAM
  participant "CRI" as JobPodInstance
end box

participant "Image Registry" as Registry

autonumber

==1. 管理员更新DS==

Admin -> API: 发布Daemonset新版本 (更新DaemonSet对象)

activate OTC
loop list/watch DaemonSet
  OTC <-- API: 检测到DaemonSet spec更新
end
OTC -> API: Patch Pod状态以及更新信息\n(PodNeedUpgrade=true，镜像新版本/imagepullsecret)
note right of API: 更新特定Pod对象的Status Condition

activate Kubelet
loop list/watch Pod spec
  Kubelet <-- API: (持续) 获取Pod对象及状态
  Kubelet -> API: (持续) 报告Pod运行时状态
end
deactivate Kubelet

==2. 节点用户触发镜像拉取==

activate NodeOTAM
PodUser -> NodeOTAM: 调用Get接口，获取可升级Pod列表
NodeOTAM --> PodUser: 返回可升级Pod列表 (根据PodNeedUpgrade=true过滤)
deactivate NodeOTAM

PodUser -> NodeOTAM: 调用ImagePull接口，指定某个可升级Pod拉取镜像

activate NodeOTAM
NodeOTAM -> API: Patch Pod状态\n(PodImageState=initial)
note right of API: 更新特定Pod对象的Status Condition
deactivate NodeOTAM

activate OTC
loop list/watch Pod status conditions
  OTC <-- API: 检测到Pod状态变化\n(PodImageState=initial)
end
OTC -> API: 创建ImagePull Job对象\n(指定Pod、镜像信息)
deactivate OTC

activate JobC
loop list/watch Job objects
  JobC <-- API: 检测到新Job对象创建
end
JobC -> API: 创建Job Pod对象\n(Pod调度到对应节点)

activate Kubelet
Kubelet <-- API: Kubelet获取到Job Pod对象spec
Kubelet -> JobPodInstance: 启动Job Pod容器
activate JobPodInstance
JobPodInstance -> Registry: 拉取指定镜像
Registry --> JobPodInstance: 镜像拉取完成
JobPodInstance -> Kubelet: 报告容器退出状态
deactivate JobPodInstance
Kubelet -> API: Kubelet更新Job Pod状态为Succeeded\n(进而Job对象状态为Completed)
deactivate Kubelet
deactivate JobC

activate OTC
loop list/watch Job objects and Pod objects
  OTC <-- API: 检测到Job对象执行成功\n(Job Pod状态为Succeeded)
end
OTC -> API: Patch Pod状态\n(PodImageState=ready)
note right of API: 更新特定Pod对象的Status Condition
deactivate OTC

==3. 节点用户更新Pod==
...

@enduml


OTA Pod状态流转图:

@startuml
Title Pod 生命周期的状态流转 (DaemonSet管理)

hide empty description

state "正常Pod" as NormalPod
state "待升级Pod" as PendingUpgradePod
state "已升级 (正常) Pod" as UpgradedNormalPod

state "镜像状态" as ImageStates {
    state "镜像init Pod" as ImageNotReadyPod
    state "镜像ready Pod" as ImageReadyPod
    state "镜像fail Pod" as ImageFailPod
}

[*] --> NormalPod : Pod启动 / Daemonset初始部署

NormalPod --> PendingUpgradePod : 管理员更新Daemonset (onDelete策略)

PendingUpgradePod --> UpgradedNormalPod : 节点用户执行update命令，删除旧Pod (视为升级成功)

PendingUpgradePod --> ImageNotReadyPod : 节点用户执行imagepull命令，开始拉取镜像

ImageStates.ImageNotReadyPod --> ImageStates.ImageReadyPod : 镜像拉取成功

ImageStates.ImageNotReadyPod --> ImageStates.ImageFailPod : 镜像拉取失败

ImageStates.ImageFailPod --> ImageStates.ImageNotReadyPod : 节点用户重试imagepull命令

ImageStates --> PendingUpgradePod : 管理员再次更新Daemonset镜像

ImageStates --> UpgradedNormalPod : 节点用户执行update命令，删除旧Pod (镜像状态不影响升级)


UpgradedNormalPod --> [*] : Pod生命周期结束 (已被新Pod取代)

@enduml

1. 正常Pod（由Daemonset管理，与Daemonset版本匹配）
2. 正常Pod -> 待升级Pod（Daemonset更新触发，由于onDelete升级策略，变为旧版本Pod）
3. 待升级Pod -> 已升级（正常）Pod（节点侧用户执行update命令触发，删除Pod，认为升级成功）
4. 待升级Pod -> 镜像int Pod （节点侧用户执行imagepull命令触发，开始为升级Pod下发拉取镜像任务）
5. 镜像init Pod -> 镜像 ready Pod（镜像拉取任务执行成功）
6. 镜像init Pod -> 镜像 fail Pod （镜像拉取任务执行失败）
7. 镜像fail Pod -> 镜像 init Pod（节点侧用户重试执行imagepull命令）
8. 镜像ready/init/fail -> 待升级 Pod（Daemonset镜像再次被更新）
9. 镜像ready/init/fail -> 已升级（正常）Pod（节点侧用户执行update命令触发，删除该Pod，镜像不阻塞升级流程）


实现
Pod状态实现：
const PodNeedUpgrade corev1.PodConditionType = "PodNeedUpgrade"
const PodImageReady corev1.PodConditionType = "PodImageReady"
正常Pod：PodNeedUpgrade为空，PodImageReady为空
待升级Pod：PodNeedUpgrade=true，PodImageReady为空
镜像init Pod：PodNeedUpgrade=true，PodImageReady=false/Reason=imagepull start
镜像fail Pod: PodNeedUpgrade=true，PodImageReady=false/Reason=imagepull fail
镜像ready Pod：PodNeedUpgrade=true，PodImageReady=true

ImagePullJob的模板大致如下
apiVersion: batch/v1
kind: Job
metadata:
  name: image-pre-pull
  namespace: test
  labels:
    app: image-prepull
spec:
  activeDeadlineSeconds: 600 # 可选：Job的最长运行时间（秒），超过此时间Job会被终止
  ttlSecondsAfterFinished: 300 # 可选：Job完成后，自动删除Job对象及其Pod（秒）。建议设置，避免残留。
  template:
    metadata:
      labels:
        app: image-prepull-pod
    spec:
      # 如果你的镜像位于私有仓库，需要配置imagePullSecrets
      # imagePullSecrets:
      #   - name: your-private-registry-secret
      
      kubernetes.io/hostname: your-specific-node-name

      containers:
        # 容器：预热 Nginx 镜像
        - name: prepull-nginx-image
          image: nginx:1.9.1  # 你想预热的Nginx镜像版本
          command: ["sleep", "30"] # 容器启动后简单执行一个命令并等待，重要的是拉取行为
          imagePullPolicy: Always # 确保总是尝试拉取最新的镜像，即使本地有缓存
          
      restartPolicy: OnFailure # Pod失败时会尝试重启

具体需要做的事：
1. 更新yurthub中的OTA模块，用户在执行upgrade之前，可以选择调用pullImage接口，提前把镜像下载到节点上，具体做的事情是给pod/status添加一个PodImageReady=false/Reason=imagepull start的状态
  a. POST /openyurt.io/v1/namespaces/{ns}/pods/{podname}/imagePull HTTP/1.1
2. yurt-manager中新增image_pull_controller，listwatch pod，当pod/status中PodImageReady=false/Reason=imagepull start时，创建一个imagepull job，job的image相关信息可以在待升级pod status message里找到，运行在对应节点上拉取镜像