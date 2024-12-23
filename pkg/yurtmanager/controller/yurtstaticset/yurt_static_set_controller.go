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

package yurtstaticset

import (
	"context"
	"fmt"
	"strings"

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

	yurtClient "github.com/openyurtio/openyurt/cmd/yurt-manager/app/client"
	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	nodeutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/node"
	podutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/pod"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtstaticset/config"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtstaticset/upgradeinfo"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtstaticset/util"
)

var (
	controllerResource = appsv1alpha1.SchemeGroupVersion.WithResource("yurtstaticsets")
	True               = true
)

const (
	StaticPodHashAnnotation = "openyurt.io/static-pod-hash"

	hostPathVolumeName       = "hostpath"
	hostPathVolumeMountPath  = "/etc/kubernetes/manifests/"
	configMapVolumeName      = "configmap"
	configMapVolumeMountPath = "/data"
	hostPathVolumeSourcePath = hostPathVolumeMountPath

	// UpgradeWorkerPodPrefix is the name prefix of worker pod which used for static pod upgrade
	UpgradeWorkerPodPrefix     = "yss-upgrade-worker-"
	UpgradeWorkerContainerName = "upgrade-worker"

	ArgTmpl = "/usr/local/bin/node-servant static-pod-upgrade --name=%s --namespace=%s --manifest=%s --hash=%s --mode=%s"
)

// upgradeWorker is the pod template used for static pod upgrade
// Fields need be set
// 1. name of worker pod: `yurt-static-set-upgrade-worker-node-hash`
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
	return fmt.Sprintf("%s: %s", names.YurtStaticSetController, s)
}

// Add creates a new YurtStaticSet Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
	if _, err := mgr.GetRESTMapper().KindFor(controllerResource); err != nil {
		klog.Infof("resource %s doesn't exist", controllerResource.String())
		return err
	}

	klog.Infof("yurtstaticset-controller add controller %s", controllerResource.String())
	return add(mgr, c, newReconciler(c, mgr))
}

var _ reconcile.Reconciler = &ReconcileYurtStaticSet{}

// ReconcileYurtStaticSet reconciles a YurtStaticSet object
type ReconcileYurtStaticSet struct {
	client.Client
	scheme        *runtime.Scheme
	recorder      record.EventRecorder
	Configuration config.YurtStaticSetControllerConfiguration
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *appconfig.CompletedConfig, mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileYurtStaticSet{
		Client:        yurtClient.GetClientByControllerNameOrDie(mgr, names.YurtStaticSetController),
		scheme:        mgr.GetScheme(),
		recorder:      mgr.GetEventRecorderFor(names.YurtStaticSetController),
		Configuration: c.ComponentConfig.YurtStaticSetController,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, cfg *appconfig.CompletedConfig, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(names.YurtStaticSetController, mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: int(cfg.ComponentConfig.YurtStaticSetController.ConcurrentYurtStaticSetWorkers)})
	if err != nil {
		return err
	}

	// 1. Watch for changes to YurtStaticSet
	if err := c.Watch(source.Kind[client.Object](mgr.GetCache(), &appsv1alpha1.YurtStaticSet{}, &handler.EnqueueRequestForObject{})); err != nil {
		return err
	}

	// 2. Watch for changes to node
	// When node turn ready, reconcile all YurtStaticSet instances
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

	reconcileAllYurtStaticSets := func(c client.Client) []reconcile.Request {
		yurtStaticSetList := &appsv1alpha1.YurtStaticSetList{}
		if err := c.List(context.TODO(), yurtStaticSetList); err != nil {
			return nil
		}
		var requests []reconcile.Request
		for _, yss := range yurtStaticSetList.Items {
			requests = append(requests, reconcile.Request{NamespacedName: types.NamespacedName{
				Namespace: yss.Namespace,
				Name:      yss.Name,
			}})
		}
		return requests
	}

	if err := c.Watch(source.Kind[client.Object](mgr.GetCache(), &corev1.Node{},
		handler.EnqueueRequestsFromMapFunc(
			func(context.Context, client.Object) []reconcile.Request {
				return reconcileAllYurtStaticSets(mgr.GetClient())
			}), nodeReadyPredicate)); err != nil {
		return err
	}

	// 3. Watch for changes to upgrade worker pods which are created by yurt-static-set-controller
	if err := c.Watch(source.Kind[client.Object](mgr.GetCache(), &corev1.Pod{},
		handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &appsv1alpha1.YurtStaticSet{}, handler.OnlyControllerOwner()))); err != nil {
		return err
	}

	// 4. Watch for changes of static pods
	reconcileYurtStaticSetForStaticPod := func(obj client.Object) []reconcile.Request {
		var reqs []reconcile.Request
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			return reqs
		}

		if !podutil.IsStaticPod(pod) {
			return reqs
		}

		yurtStaticSetName := strings.TrimSuffix(pod.Name, fmt.Sprintf("-%s", pod.Spec.NodeName))
		reqs = append(reqs, reconcile.Request{NamespacedName: types.NamespacedName{
			Namespace: pod.Namespace,
			Name:      yurtStaticSetName,
		}})

		return reqs
	}
	if err := c.Watch(source.Kind[client.Object](mgr.GetCache(), &corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(
		func(ctx context.Context, obj client.Object) []reconcile.Request {
			return reconcileYurtStaticSetForStaticPod(obj)
		}))); err != nil {
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

	_, onc := nodeutil.GetNodeCondition(&oldNode.Status, corev1.NodeReady)
	_, nnc := nodeutil.GetNodeCondition(&newNode.Status, corev1.NodeReady)

	oldReady := (onc != nil) && ((onc.Status == corev1.ConditionFalse) || (onc.Status == corev1.ConditionUnknown))
	newReady := (nnc != nil) && (nnc.Status == corev1.ConditionTrue)

	return oldReady && newReady
}

//+kubebuilder:rbac:groups=apps.openyurt.io,resources=yurtstaticsets,verbs=get;create;update;patch;delete
//+kubebuilder:rbac:groups=apps.openyurt.io,resources=yurtstaticsets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps.openyurt.io,resources=yurtstaticsets/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods/status,verbs=update;patch
//+kubebuilder:rbac:groups=core,resources=nodes,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;create;update;patch;delete

// Reconcile reads that state of the cluster for a YurtStaticSet object and makes changes based on the state read
// and what is in the YurtStaticSet.Spec
func (r *ReconcileYurtStaticSet) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {

	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.V(4).Info(Format("Reconcile YurtStaticSet %s", request.Name))

	// Fetch the YurtStaticSet instance
	instance := &appsv1alpha1.YurtStaticSet{}
	if err := r.Get(context.TODO(), request.NamespacedName, instance); err != nil {
		// if the yurtStaticSet does not exist, delete the specified configmap if exist.
		if kerr.IsNotFound(err) {
			return reconcile.Result{}, r.deleteConfigMap(request.Name, request.Namespace)
		}
		klog.Errorf("could not get YurtStaticSet %v, %v", request.NamespacedName, err)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if instance.DeletionTimestamp != nil {
		// handle the deletion event
		// delete the configMap which is created by yurtStaticSet
		return reconcile.Result{}, r.deleteConfigMap(request.Name, request.Namespace)
	}

	var (
		// totalNumber represents the total number of nodes running the target static pod
		totalNumber int32

		// readyNumber represents the number of ready static pods
		readyNumber int32

		// upgradedNumber represents the number of nodes that have been upgraded
		upgradedNumber int32

		// check whether all worker pod is finished
		allSucceeded bool

		// the worker pod need to be deleted
		deletePods []*corev1.Pod
	)

	// The latest hash value for static pod spec
	// This hash value is used in three places
	// 1. Automatically added to the annotation of static pods to facilitate checking if the running static pods are up-to-date
	// 2. Automatically added to the annotation of worker pods to facilitate checking if the worker pods are up-to-date
	// 3. Added to YurtStaticSet's corresponding configmap to facilitate checking if the configmap is up-to-date
	latestHash := util.ComputeHash(&instance.Spec.Template)

	// The latest static pod manifest generated from user-specified template
	// The above hash value will be added to the annotation
	latestManifest, err := util.GenStaticPodManifest(&instance.Spec.Template, latestHash)
	if err != nil {
		klog.Error(Format("could not generate static pod manifest of YurtStaticSet %v, %v", request.NamespacedName, err))
		return ctrl.Result{}, err
	}

	// Sync the corresponding configmap to the latest state
	if err := r.syncConfigMap(instance, latestHash, latestManifest); err != nil {
		klog.Error(Format("could not sync the corresponding configmap of YurtStaticSet %v, %v", request.NamespacedName, err))
		return ctrl.Result{}, err
	}

	// The later upgrade operation is conducted based on upgradeInfos
	upgradeInfos, err := upgradeinfo.New(r.Client, instance, UpgradeWorkerPodPrefix, latestHash)
	if err != nil {
		// The worker pod is failed, then some irreparable failure has occurred. Just stop reconcile and update status
		if strings.Contains(err.Error(), "could not init worker pod") {
			r.recorder.Eventf(instance, corev1.EventTypeWarning, "YurtStaticSet Upgrade Failed", err.Error())
			klog.Error(err.Error())
			return reconcile.Result{}, err
		}

		klog.Error(Format("could not get static pod and worker pod upgrade info for nodes of YurtStaticSet %v, %v",
			request.NamespacedName, err))
		return ctrl.Result{}, err
	}
	totalNumber = int32(len(upgradeInfos))
	// There are no nodes running target static pods in the cluster
	if totalNumber == 0 {
		klog.Info(Format("No static pods need to be upgraded of YurtStaticSet %v", request.NamespacedName))
		return r.updateYurtStaticSetStatus(instance, totalNumber, totalNumber, totalNumber)
	}

	// Count the number of ready static pods and upgraded nodes, the delete pods list and whether all worker is finished
	upgradedNumber, readyNumber, allSucceeded, deletePods = upgradeinfo.CalculateOperateInfoFromUpgradeInfoMap(upgradeInfos)

	// Clean up unused pods
	if err := r.removeUnusedPods(deletePods); err != nil {
		klog.Error(Format("could not remove unused pods of YurtStaticSet %v, %v", request.NamespacedName, err))
		return reconcile.Result{}, err
	}

	// If all nodes have been upgraded, just return
	// Put this here because we need to clean up the worker pods first
	if totalNumber == upgradedNumber {
		klog.Info(Format("All static pods have been upgraded of YurtStaticSet %v", request.NamespacedName))
		return r.updateYurtStaticSetStatus(instance, totalNumber, readyNumber, upgradedNumber)
	}

	switch strings.ToLower(string(instance.Spec.UpgradeStrategy.Type)) {
	// AdvancedRollingUpdate Upgrade is to automate the upgrade process for the target static pods on ready nodes
	// It supports rolling update and the max-unavailable number can be specified by users
	case strings.ToLower(string(appsv1alpha1.AdvancedRollingUpdateUpgradeStrategyType)):
		if !allSucceeded {
			klog.V(5).Info(Format("Wait last round AdvancedRollingUpdate upgrade to finish of YurtStaticSet %v", request.NamespacedName))
			return r.updateYurtStaticSetStatus(instance, totalNumber, readyNumber, upgradedNumber)
		}

		if err := r.advancedRollingUpdate(instance, upgradeInfos, latestHash); err != nil {
			klog.Error(Format("could not AdvancedRollingUpdate upgrade of YurtStaticSet %v, %v", request.NamespacedName, err))
			return ctrl.Result{}, err
		}
		return r.updateYurtStaticSetStatus(instance, totalNumber, readyNumber, upgradedNumber)

	// OTA Upgrade can help users control the timing of static pods upgrade
	// It will set PodNeedUpgrade condition and work with YurtHub component
	case strings.ToLower(string(appsv1alpha1.OTAUpgradeStrategyType)):
		if err := r.otaUpgrade(upgradeInfos); err != nil {
			klog.Error(Format("could not OTA upgrade of YurtStaticSet %v, %v", request.NamespacedName, err))
			return ctrl.Result{}, err
		}
		return r.updateYurtStaticSetStatus(instance, totalNumber, readyNumber, upgradedNumber)
	}

	return ctrl.Result{}, nil
}

// syncConfigMap moves the target yurtstaticset's corresponding configmap to the latest state
func (r *ReconcileYurtStaticSet) syncConfigMap(instance *appsv1alpha1.YurtStaticSet, hash, data string) error {
	cmName := util.WithConfigMapPrefix(instance.Name)
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

// advancedRollingUpdate automatically rolling upgrade the target static pods in cluster
func (r *ReconcileYurtStaticSet) advancedRollingUpdate(instance *appsv1alpha1.YurtStaticSet, infos map[string]*upgradeinfo.UpgradeInfo, hash string) error {
	// readyUpgradeWaitingNodes represents nodes that need to create worker pods
	readyUpgradeWaitingNodes := upgradeinfo.ReadyUpgradeWaitingNodes(infos)

	waitingNumber := len(readyUpgradeWaitingNodes)
	if waitingNumber == 0 {
		return nil
	}

	// max is the maximum number of nodes can be upgraded in current round in AdvancedRollingUpdate upgrade mode
	max, err := util.UnavailableCount(&instance.Spec.UpgradeStrategy, len(infos))
	if err != nil {
		return err
	}

	if waitingNumber < max {
		max = waitingNumber
	}

	readyUpgradeWaitingNodes = readyUpgradeWaitingNodes[:max]
	if err := createUpgradeWorker(r.Client, instance, readyUpgradeWaitingNodes, hash,
		string(appsv1alpha1.AdvancedRollingUpdateUpgradeStrategyType), r.Configuration.UpgradeWorkerImage); err != nil {
		return err
	}
	return nil
}

// otaUpgrade adds condition PodNeedUpgrade to the target static pods
func (r *ReconcileYurtStaticSet) otaUpgrade(infos map[string]*upgradeinfo.UpgradeInfo) error {
	upgradeNeededNodes, upgradedNodes := upgradeinfo.ListOutUpgradeNeededNodesAndUpgradedNodes(infos)

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

	return nil
}

// removeUnusedPods delete pods, include two situations: out-of-date worker pods and succeeded worker pods
func (r *ReconcileYurtStaticSet) removeUnusedPods(pods []*corev1.Pod) error {
	for _, pod := range pods {
		if err := r.Delete(context.TODO(), pod, &client.DeleteOptions{}); err != nil {
			return err
		}
		klog.V(4).Info(Format("Delete upgrade worker pod %v", pod.Name))
	}
	return nil
}

// createUpgradeWorker creates static pod upgrade worker to the given nodes
func createUpgradeWorker(c client.Client, instance *appsv1alpha1.YurtStaticSet, nodes []string, hash, mode, img string) error {
	for _, node := range nodes {
		pod := upgradeWorker.DeepCopy()
		pod.Name = UpgradeWorkerPodPrefix + instance.Name + "-" + util.Hyphen(node, hash)
		pod.Namespace = instance.Namespace
		pod.Spec.NodeName = node
		metav1.SetMetaDataAnnotation(&pod.ObjectMeta, StaticPodHashAnnotation, hash)
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: configMapVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: util.WithConfigMapPrefix(instance.Name),
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
		klog.Info(Format("Create static pod upgrade worker %s of YurtStaticSet %s", pod.Name, instance.Name))
	}

	return nil
}

// updateYurtStaticSetStatus set the status of instance to the given values
func (r *ReconcileYurtStaticSet) updateYurtStaticSetStatus(instance *appsv1alpha1.YurtStaticSet, totalNum, readyNum, upgradedNum int32) (reconcile.Result, error) {
	instance.Status.TotalNumber = totalNum
	instance.Status.ReadyNumber = readyNum
	instance.Status.UpgradedNumber = upgradedNum

	if err := r.Client.Status().Update(context.TODO(), instance); err != nil {
		return reconcile.Result{Requeue: true}, err
	}

	return reconcile.Result{}, nil
}

// deleteConfigMap delete the configMap if YurtStaticSet is deleting
func (r *ReconcileYurtStaticSet) deleteConfigMap(name, namespace string) error {
	cmName := util.WithConfigMapPrefix(name)
	configMap := &corev1.ConfigMap{}
	if err := r.Get(context.TODO(), types.NamespacedName{Name: cmName, Namespace: namespace}, configMap); err != nil {
		if kerr.IsNotFound(err) {
			return nil
		}
		return err
	}
	if err := r.Delete(context.TODO(), configMap, &client.DeleteOptions{}); err != nil {
		return err
	}
	klog.Info(Format("Delete ConfigMap %s from YurtStaticSet %s", configMap.Name, name))
	return nil
}
