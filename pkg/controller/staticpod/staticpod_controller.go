/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package staticpod

import (
	"context"
	"flag"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	kerr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/controller/staticpod/config"
	"github.com/openyurtio/openyurt/pkg/controller/staticpod/upgradeinfo"
	"github.com/openyurtio/openyurt/pkg/controller/staticpod/util"
	utilclient "github.com/openyurtio/openyurt/pkg/util/client"
	utildiscovery "github.com/openyurtio/openyurt/pkg/util/discovery"
)

func init() {
	flag.IntVar(&concurrentReconciles, "staticpod-workers", concurrentReconciles, "Max concurrent workers for StaticPod controller.")
}

var (
	concurrentReconciles = 3
	controllerKind       = appsv1alpha1.SchemeGroupVersion.WithKind("StaticPod")
	True                 = true
)

const (
	controllerName = "StaticPod-controller"

	StaticPodHashAnnotation     = "openyurt.io/static-pod-hash"
	OTALatestManifestAnnotation = "openyurt.io/ota-latest-version"

	hostPathVolumeName       = "hostpath"
	hostPathVolumeMountPath  = "/etc/kubernetes/manifests/"
	configMapVolumeName      = "configmap"
	configMapVolumeMountPath = "/data"
	hostPathVolumeSourcePath = hostPathVolumeMountPath

	// UpgradeWorkerPodPrefix is the name prefix of worker pod which used for static pod upgrade
	UpgradeWorkerPodPrefix     = "yurt-static-pod-upgrade-worker-"
	UpgradeWorkerContainerName = "upgrade-worker"

	ArgTmpl = "/usr/local/bin/node-servant static-pod-upgrade --name=%s --namespace=%s --manifest=%s --hash=%s --mode=%s"
)

// upgradeWorker is the pod template used for static pod upgrade
// Fields need be set
// 1. name of worker pod: `yurt-static-pod-upgrade-worker-node-hash`
// 2. node of worker pod
// 3. image of worker pod, default is "openyurt/node-servant:latest"
// 4. annotation `openyurt.io/static-pod-hash`
// 5. the corresponding configmap
var (
	upgradeWorker = &corev1.Pod{
		Spec: corev1.PodSpec{
			HostPID:       true,
			HostNetwork:   true,
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:    UpgradeWorkerContainerName,
				Command: []string{"/bin/sh", "-c"},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      hostPathVolumeName,
						MountPath: hostPathVolumeMountPath,
					},
					{
						Name:      configMapVolumeName,
						MountPath: configMapVolumeMountPath,
					},
				},
				ImagePullPolicy: corev1.PullIfNotPresent,
				SecurityContext: &corev1.SecurityContext{
					Privileged: &True,
				},
			}},
			Volumes: []corev1.Volume{{
				Name: hostPathVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: hostPathVolumeSourcePath,
					},
				}},
			},
		},
	}
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", controllerName, s)
}

// Add creates a new StaticPod Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(c *appconfig.CompletedConfig, mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) {
		return nil
	}
	return add(mgr, newReconciler(c, mgr))
}

var _ reconcile.Reconciler = &ReconcileStaticPod{}

// ReconcileStaticPod reconciles a StaticPod object
type ReconcileStaticPod struct {
	client.Client
	scheme        *runtime.Scheme
	recorder      record.EventRecorder
	Configuration config.StaticPodControllerConfiguration
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *appconfig.CompletedConfig, mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileStaticPod{
		Client:        utilclient.NewClientFromManager(mgr, controllerName),
		scheme:        mgr.GetScheme(),
		recorder:      mgr.GetEventRecorderFor(controllerName),
		Configuration: c.ComponentConfig.StaticPodController,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: concurrentReconciles})
	if err != nil {
		return err
	}

	// 1. Watch for changes to StaticPod
	if err := c.Watch(&source.Kind{Type: &appsv1alpha1.StaticPod{}}, &handler.EnqueueRequestForObject{}); err != nil {
		return err
	}

	// 2. Watch for changes to node
	// When node turn ready, reconcile all StaticPod instances
	// nodeReadyPredicate filter events which are node turn ready
	nodeReadyPredicate := predicate.Funcs{
		CreateFunc: func(evt event.CreateEvent) bool {
			return false
		},
		DeleteFunc: func(evt event.DeleteEvent) bool {
			return false
		},
		UpdateFunc: func(evt event.UpdateEvent) bool {
			return nodeTurnReady(evt)
		},
		GenericFunc: func(evt event.GenericEvent) bool {
			return false
		},
	}

	reconcileAllStaticPods := func(c client.Client) []reconcile.Request {
		staticPodList := &appsv1alpha1.StaticPodList{}
		if err := c.List(context.TODO(), staticPodList); err != nil {
			return nil
		}
		var requests []reconcile.Request
		for _, staticPod := range staticPodList.Items {
			requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
				Namespace: staticPod.Namespace,
				Name:      staticPod.Name,
			}})
		}
		return requests
	}

	if err := c.Watch(&source.Kind{Type: &corev1.Node{}},
		handler.EnqueueRequestsFromMapFunc(
			func(client.Object) []reconcile.Request {
				return reconcileAllStaticPods(mgr.GetClient())
			}), nodeReadyPredicate); err != nil {
		return err
	}

	// 3. Watch for changes to upgrade worker pods which are created by static-pod-controller
	if err := c.Watch(&source.Kind{Type: &corev1.Pod{}}, &handler.EnqueueRequestForOwner{IsController: true, OwnerType: &appsv1alpha1.StaticPod{}}); err != nil {
		return err
	}

	return nil
}

// nodeTurnReady filter events: old node is not-ready or unknown, new node is ready
func nodeTurnReady(evt event.UpdateEvent) bool {
	if _, ok := evt.ObjectOld.(*corev1.Node); !ok {
		return false
	}

	oldNode := evt.ObjectOld.(*corev1.Node)
	newNode := evt.ObjectNew.(*corev1.Node)

	_, onc := util.GetNodeCondition(&oldNode.Status, corev1.NodeReady)
	_, nnc := util.GetNodeCondition(&newNode.Status, corev1.NodeReady)

	oldReady := (onc != nil) && ((onc.Status == corev1.ConditionFalse) || (onc.Status == corev1.ConditionUnknown))
	newReady := (nnc != nil) && (nnc.Status == corev1.ConditionTrue)

	return oldReady && newReady
}

//+kubebuilder:rbac:groups=apps.openyurt.io,resources=staticpods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps.openyurt.io,resources=staticpods/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps.openyurt.io,resources=staticpods/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile reads that state of the cluster for a StaticPod object and makes changes based on the state read
// and what is in the StaticPod.Spec
func (r *ReconcileStaticPod) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {

	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.V(4).Infof(Format("Reconcile StaticPod %s", request.Name))

	// Fetch the StaticPod instance
	instance := &appsv1alpha1.StaticPod{}
	if err := r.Get(context.TODO(), request.NamespacedName, instance); err != nil {
		klog.Errorf("Fail to get StaticPod %v, %v", request.NamespacedName, err)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if instance.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	var (
		// totalNumber represents the total number of nodes running the target static pod
		totalNumber int32

		// readyNumber represents the number of ready static pods
		readyNumber int32

		// upgradedNumber represents the number of nodes that have been upgraded
		upgradedNumber int32
	)

	// The later upgrade operation is conducted based on upgradeInfos
	upgradeInfos, err := upgradeinfo.New(r.Client, instance, UpgradeWorkerPodPrefix)
	if err != nil {
		klog.Errorf(Format("Fail to get static pod and worker pod upgrade info for nodes of StaticPod %v, %v",
			request.NamespacedName, err))
		return ctrl.Result{}, err
	}
	totalNumber = int32(len(upgradeInfos))
	// There are no nodes running target static pods in the cluster
	if totalNumber == 0 {
		klog.Infof(Format("No static pods need to be upgraded of StaticPod %v", request.NamespacedName))
		return r.updateStaticPodStatus(instance, totalNumber, totalNumber, totalNumber)
	}

	// The latest hash value for static pod spec
	// This hash value is used in three places
	// 1. Automatically added to the annotation of static pods to facilitate checking if the running static pods are up-to-date
	// 2. Automatically added to the annotation of worker pods to facilitate checking if the worker pods are up-to-date
	// 3. Added to static pods' corresponding configmap to facilitate checking if the configmap is up-to-date
	latestHash := util.ComputeHash(&instance.Spec.Template)

	// The latest static pod manifest generated from user-specified template
	// The above hash value will be added to the annotation
	latestManifest, err := util.GenStaticPodManifest(&instance.Spec.Template, latestHash)
	if err != nil {
		klog.Errorf(Format("Fail to generate static pod manifest of StaticPod %v, %v", request.NamespacedName, err))
		return ctrl.Result{}, err
	}

	// Sync the corresponding configmap to the latest state
	if err := r.syncConfigMap(instance, latestHash, latestManifest); err != nil {
		klog.Errorf(Format("Fail to sync the corresponding configmap of StaticPod %v, %v", request.NamespacedName, err))
		return ctrl.Result{}, err
	}

	// Complete upgrade info
	{
		// Count the number of upgraded nodes
		upgradedNumber = upgradeinfo.SetUpgradeNeededInfos(upgradeInfos, latestHash)

		readyNumber = upgradeinfo.ReadyStaticPodsNumber(upgradeInfos)

		// Set node ready info
		if err := checkReadyNodes(r.Client, upgradeInfos); err != nil {
			klog.Errorf(Format("Fail to check node ready status of StaticPod %v,%v", request.NamespacedName, err))
			return ctrl.Result{}, err
		}
	}

	// Sync worker pods
	allSucceeded := true
	deletePods := make([]*corev1.Pod, 0)
	{
		for node, info := range upgradeInfos {
			if info.WorkerPod == nil {
				continue
			}

			hash := info.WorkerPod.Annotations[StaticPodHashAnnotation]
			// If the worker pod is not up-to-date, then it can be recreated directly
			if hash != latestHash {
				deletePods = append(deletePods, info.WorkerPod)
				continue
			}
			// If the worker pod is up-to-date, there are three possible situations
			// 1. The worker pod is failed, then some irreparable failure has occurred. Just stop reconcile and update status
			// 2. The worker pod is succeeded, then this node must be up-to-date. Just delete this worker pod
			// 3. The worker pod is running, pending or unknown, then just wait
			switch info.WorkerPod.Status.Phase {
			case corev1.PodFailed:
				klog.Errorf("Fail to continue upgrade, cause worker pod %s of StaticPod %v in node %s failed",
					info.WorkerPod.Name, request.NamespacedName, node)
				return reconcile.Result{},
					fmt.Errorf("fail to continue upgrade, cause worker pod %s of StaticPod %v in node %s failed",
						info.WorkerPod.Name, request.NamespacedName, node)
			case corev1.PodSucceeded:
				deletePods = append(deletePods, info.WorkerPod)
			default:
				// In this node, the latest worker pod is still running, and we don't need to create new worker for it.
				info.WorkerPodRunning = true
				allSucceeded = false
			}
		}
	}

	// Clean up unused pods
	if err := r.removeUnusedPods(deletePods); err != nil {
		klog.Errorf(Format("Fail to remove unused pods of StaticPod %v, %v", request.NamespacedName, err))
		return reconcile.Result{}, err
	}

	// If all nodes have been upgraded, just return
	// Put this here because we need to clean up the worker pods first
	if totalNumber == upgradedNumber {
		klog.Infof(Format("All static pods have been upgraded of StaticPod %v", request.NamespacedName))
		return r.updateStaticPodStatus(instance, totalNumber, readyNumber, upgradedNumber)
	}

	switch instance.Spec.UpgradeStrategy.Type {
	// Auto Upgrade is to automate the upgrade process for the target static pods on ready nodes
	// It supports rolling update and the max-unavailable number can be specified by users
	case appsv1alpha1.AutoStaticPodUpgradeStrategyType:
		if !allSucceeded {
			klog.V(5).Infof(Format("Wait last round auto upgrade to finish of StaticPod %v", request.NamespacedName))
			return r.updateStaticPodStatus(instance, totalNumber, readyNumber, upgradedNumber)
		}

		if err := r.autoUpgrade(instance, upgradeInfos, latestHash); err != nil {
			klog.Errorf(Format("Fail to auto upgrade of StaticPod %v, %v", request.NamespacedName, err))
			return ctrl.Result{}, err
		}
		return r.updateStaticPodStatus(instance, totalNumber, readyNumber, upgradedNumber)

	// OTA Upgrade can help users control the timing of static pods upgrade
	// It will set PodNeedUpgrade condition and work with YurtHub component
	case appsv1alpha1.OTAStaticPodUpgradeStrategyType:
		if err := r.otaUpgrade(instance, upgradeInfos, latestHash); err != nil {
			klog.Errorf(Format("Fail to ota upgrade of StaticPod %v, %v", request.NamespacedName, err))
			return ctrl.Result{}, err
		}
		return r.updateStaticPodStatus(instance, totalNumber, readyNumber, upgradedNumber)
	}

	return ctrl.Result{}, nil
}

// syncConfigMap moves the target static pod's corresponding configmap to the latest state
func (r *ReconcileStaticPod) syncConfigMap(instance *appsv1alpha1.StaticPod, hash, data string) error {
	cmName := util.WithConfigMapPrefix(util.Hyphen(instance.Namespace, instance.Name))
	cm := &corev1.ConfigMap{}
	if err := r.Get(context.TODO(), types.NamespacedName{Name: cmName, Namespace: instance.Namespace}, cm); err != nil {
		// if the configmap does not exist, then create a new one
		if kerr.IsNotFound(err) {
			cm = &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      cmName,
					Namespace: instance.Namespace,
					Annotations: map[string]string{
						StaticPodHashAnnotation: hash,
					},
				},

				Data: map[string]string{
					instance.Spec.StaticPodManifest: data,
				},
			}
			if err := r.Create(context.TODO(), cm, &client.CreateOptions{}); err != nil {
				return err
			}
			return nil
		}
		return err
	}

	// if the hash value in the annotation of the cm does not match the latest hash, then update the data in the cm
	if cm.Annotations[StaticPodHashAnnotation] != hash {
		cm.Annotations[StaticPodHashAnnotation] = hash
		cm.Data[instance.Spec.StaticPodManifest] = data

		if err := r.Update(context.TODO(), cm, &client.UpdateOptions{}); err != nil {
			return err
		}
	}

	return nil
}

// autoUpgrade automatically rolling upgrade the target static pods in cluster
func (r *ReconcileStaticPod) autoUpgrade(instance *appsv1alpha1.StaticPod, infos map[string]*upgradeinfo.UpgradeInfo, hash string) error {
	// readyUpgradeWaitingNodes represents nodes that need to create worker pods
	readyUpgradeWaitingNodes := upgradeinfo.ReadyUpgradeWaitingNodes(infos)

	waitingNumber := len(readyUpgradeWaitingNodes)
	if waitingNumber == 0 {
		return nil
	}

	// max is the maximum number of nodes can be upgraded in current round in auto upgrade mode
	max, err := util.UnavailableCount(&instance.Spec.UpgradeStrategy, len(infos))
	if err != nil {
		return err
	}

	if waitingNumber < max {
		max = waitingNumber
	}

	readyUpgradeWaitingNodes = readyUpgradeWaitingNodes[:max]
	if err := createUpgradeWorker(r.Client, instance, readyUpgradeWaitingNodes, hash,
		string(appsv1alpha1.AutoStaticPodUpgradeStrategyType), r.Configuration.UpgradeWorkerImage); err != nil {
		return err
	}
	return nil
}

// otaUpgrade adds condition PodNeedUpgrade to the target static pods and issue the latest manifest to corresponding nodes
func (r *ReconcileStaticPod) otaUpgrade(instance *appsv1alpha1.StaticPod, infos map[string]*upgradeinfo.UpgradeInfo, hash string) error {
	upgradeNeededNodes := upgradeinfo.UpgradeNeededNodes(infos)
	upgradedNodes := upgradeinfo.UpgradedNodes(infos)

	// Set condition for upgrade needed static pods
	for _, n := range upgradeNeededNodes {
		if err := util.SetPodUpgradeCondition(r.Client, corev1.ConditionTrue, infos[n].StaticPod); err != nil {
			return err
		}
	}

	// Set condition for upgraded static pods
	for _, n := range upgradedNodes {
		if err := util.SetPodUpgradeCondition(r.Client, corev1.ConditionFalse, infos[n].StaticPod); err != nil {
			return err
		}
	}

	// Create worker pod to issue the latest manifest to ready node
	readyUpgradeWaitingNodes := upgradeinfo.OTAReadyUpgradeWaitingNodes(infos, hash)
	if err := createUpgradeWorker(r.Client, instance, readyUpgradeWaitingNodes, hash,
		string(appsv1alpha1.OTAStaticPodUpgradeStrategyType), r.Configuration.UpgradeWorkerImage); err != nil {
		return err
	}

	if err := r.setLatestManifestHash(instance, readyUpgradeWaitingNodes, hash); err != nil {
		return err
	}

	return nil
}

// setLatestManifestHash set the latest manifest hash value to target static pod annotation
// TODO: In ota mode, it's hard for controller to check whether the latest manifest file has been issued to nodes
// TODO: Use annotation `openyurt.io/ota-latest-version` to indicate the version of manifest issued to nodes
func (r *ReconcileStaticPod) setLatestManifestHash(instance *appsv1alpha1.StaticPod, nodes []string, hash string) error {
	pod := &corev1.Pod{}
	for _, node := range nodes {
		if err := r.Client.Get(context.TODO(), types.NamespacedName{Namespace: instance.Namespace,
			Name: util.Hyphen(instance.Name, node)}, pod); err != nil {
			return err
		}

		metav1.SetMetaDataAnnotation(&pod.ObjectMeta, OTALatestManifestAnnotation, hash)
		if err := r.Client.Update(context.TODO(), pod, &client.UpdateOptions{}); err != nil {
			return err
		}
	}
	return nil
}

// removeUnusedPods delete pods, include two situations: out-of-date worker pods and succeeded worker pods
func (r *ReconcileStaticPod) removeUnusedPods(pods []*corev1.Pod) error {
	for _, pod := range pods {
		if err := r.Delete(context.TODO(), pod, &client.DeleteOptions{}); err != nil {
			return err
		}
		klog.V(4).Infof(Format("Delete upgrade worker pod %v", pod.Name))
	}
	return nil
}

// createUpgradeWorker creates static pod upgrade worker to the given nodes
func createUpgradeWorker(c client.Client, instance *appsv1alpha1.StaticPod, nodes []string, hash, mode, img string) error {
	for _, node := range nodes {
		pod := upgradeWorker.DeepCopy()
		pod.Name = UpgradeWorkerPodPrefix + util.Hyphen(node, hash)
		pod.Namespace = instance.Namespace
		pod.Spec.NodeName = node
		metav1.SetMetaDataAnnotation(&pod.ObjectMeta, StaticPodHashAnnotation, hash)
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: configMapVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: util.WithConfigMapPrefix(util.Hyphen(instance.Namespace, instance.Name)),
					},
				},
			},
		})
		pod.Spec.Containers[0].Args = []string{fmt.Sprintf(ArgTmpl, util.Hyphen(instance.Name, node), instance.Namespace,
			instance.Spec.StaticPodManifest, hash, mode)}
		pod.Spec.Containers[0].Image = img
		if err := controllerutil.SetControllerReference(instance, pod, c.Scheme()); err != nil {
			return err
		}

		if err := c.Create(context.TODO(), pod, &client.CreateOptions{}); err != nil {
			return err
		}
		klog.Infof(Format("Create static pod upgrade worker %s of StaticPod %s", pod.Name, instance.Name))
	}

	return nil
}

// checkReadyNodes checks and sets the ready status for every node which has the target static pod
func checkReadyNodes(client client.Client, infos map[string]*upgradeinfo.UpgradeInfo) error {
	for node, info := range infos {
		ready, err := util.NodeReadyByName(client, node)
		if err != nil {
			return err
		}
		info.Ready = ready
	}
	return nil
}

// updateStatus set the status of instance to the given values
func (r *ReconcileStaticPod) updateStaticPodStatus(instance *appsv1alpha1.StaticPod, totalNum, readyNum, upgradedNum int32) (reconcile.Result, error) {
	instance.Status.TotalNumber = totalNum
	instance.Status.ReadyNumber = readyNum
	instance.Status.UpgradedNumber = upgradedNum

	if err := r.Client.Status().Update(context.TODO(), instance); err != nil {
		return reconcile.Result{Requeue: true}, err
	}

	return reconcile.Result{}, nil
}
