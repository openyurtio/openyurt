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

package platformadmin

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kjson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/kubectl/pkg/scheme"
	"k8s.io/utils/ptr"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	yurtClient "github.com/openyurtio/openyurt/cmd/yurt-manager/app/client"
	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	appsv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	iotv1beta1 "github.com/openyurtio/openyurt/pkg/apis/iot/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/platformadmin/config"
	util "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/platformadmin/utils"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.PlatformAdminController, s)
}

var (
	controllerResource = iotv1beta1.SchemeGroupVersion.WithResource("platformadmins")
)

const (
	LabelConfigmap  = "Configmap"
	LabelService    = "Service"
	LabelDeployment = "Deployment"
	LabelFramework  = "Framework"

	AnnotationServiceTopologyKey           = "openyurt.io/topologyKeys"
	AnnotationServiceTopologyValueNodePool = "openyurt.io/nodepool"

	ConfigMapName      = "common-variables"
	FrameworkName      = "platformadmin-framework"
	FrameworkFinalizer = "kubernetes.io/platformadmin-framework"
)

// PlatformAdminFramework is the framework of platformadmin,
// it contains all configs of configmaps, services and yurtappsets.
// PlatformAdmin will customize the configuration based on this structure.
type PlatformAdminFramework struct {
	runtime.TypeMeta `json:",inline"`

	name       string
	security   bool
	Components []*config.Component `yaml:"components,omitempty" json:"components,omitempty"`
	ConfigMaps []corev1.ConfigMap  `yaml:"configMaps,omitempty" json:"configMaps,omitempty"`
}

// A function written to implement the yaml serializer interface, which is not actually useful
func (p *PlatformAdminFramework) DeepCopyObject() runtime.Object {
	copy := p.DeepCopy()
	return &copy
}

// A function written to implement the yaml serializer interface, which is not actually useful
func (p *PlatformAdminFramework) DeepCopy() PlatformAdminFramework {
	newObj := *p
	return newObj
}

// ReconcilePlatformAdmin reconciles a PlatformAdmin object.
type ReconcilePlatformAdmin struct {
	client.Client
	scheme         *runtime.Scheme
	recorder       record.EventRecorder
	yamlSerializer *kjson.Serializer
	Configuration  config.PlatformAdminControllerConfiguration
}

var _ reconcile.Reconciler = &ReconcilePlatformAdmin{}

// Add creates a new PlatformAdmin Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
	if _, err := mgr.GetRESTMapper().KindFor(controllerResource); err != nil {
		klog.Infof("resource %s doesn't exist", controllerResource.String())
		return err
	}

	klog.Infof("platformadmin-controller add controller %s", controllerResource.String())
	return add(mgr, c, newReconciler(c, mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *appconfig.CompletedConfig, mgr manager.Manager) reconcile.Reconciler {
	return &ReconcilePlatformAdmin{
		Client:         yurtClient.GetClientByControllerNameOrDie(mgr, names.PlatformAdminController),
		scheme:         mgr.GetScheme(),
		recorder:       mgr.GetEventRecorderFor(names.PlatformAdminController),
		yamlSerializer: kjson.NewSerializerWithOptions(kjson.DefaultMetaFactory, scheme.Scheme, scheme.Scheme, kjson.SerializerOptions{Yaml: true, Pretty: true}),
		Configuration:  c.ComponentConfig.PlatformAdminController,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, cfg *appconfig.CompletedConfig, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(names.PlatformAdminController, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: int(cfg.ComponentConfig.PlatformAdminController.ConcurrentPlatformAdminWorkers),
	})
	if err != nil {
		return err
	}

	// Watch for changes to PlatformAdmin
	err = c.Watch(source.Kind[client.Object](mgr.GetCache(), &iotv1beta1.PlatformAdmin{}, &handler.EnqueueRequestForObject{}))
	if err != nil {
		return err
	}

	err = c.Watch(source.Kind[client.Object](mgr.GetCache(), &corev1.ConfigMap{},
		handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &iotv1beta1.PlatformAdmin{}, handler.OnlyControllerOwner())))
	if err != nil {
		return err
	}

	err = c.Watch(source.Kind[client.Object](mgr.GetCache(), &corev1.Service{},
		handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &iotv1beta1.PlatformAdmin{}, handler.OnlyControllerOwner())))
	if err != nil {
		return err
	}

	err = c.Watch(source.Kind[client.Object](mgr.GetCache(), &appsv1beta1.YurtAppSet{},
		handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &iotv1beta1.PlatformAdmin{}, handler.OnlyControllerOwner())))
	if err != nil {
		return err
	}

	klog.V(4).Info(Format("registering the field indexers of platformadmin controller"))
	if err := util.RegisterFieldIndexers(mgr.GetFieldIndexer()); err != nil {
		klog.Error(Format("could not register field indexers for platformadmin controller, %v", err))
		return nil
	}

	return nil
}

// +kubebuilder:rbac:groups=iot.openyurt.io,resources=platformadmins,verbs=get;create;update;patch;delete
// +kubebuilder:rbac:groups=iot.openyurt.io,resources=platformadmins/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=iot.openyurt.io,resources=platformadmins/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps.openyurt.io,resources=yurtappsets,verbs=get;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.openyurt.io,resources=yurtappsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps/status,verbs=get;update;patch;watch
// +kubebuilder:rbac:groups=core,resources=services/status,verbs=get;update;patch

// Reconcile reads that state of the cluster for a PlatformAdmin object and makes changes based on the state read
// and what is in the PlatformAdmin.Spec
func (r *ReconcilePlatformAdmin) Reconcile(ctx context.Context, request reconcile.Request) (_ reconcile.Result, reterr error) {
	klog.Info(Format("Reconcile PlatformAdmin %s/%s", request.Namespace, request.Name))

	// Fetch the PlatformAdmin instance
	platformAdmin := &iotv1beta1.PlatformAdmin{}
	if err := r.Get(ctx, request.NamespacedName, platformAdmin); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		klog.Error(Format("Get PlatformAdmin %s/%s error %v", request.Namespace, request.Name, err))
		return reconcile.Result{}, err
	}

	platformAdminStatus := platformAdmin.Status.DeepCopy()
	isDeleted := false

	// Always issue a patch when exiting this function so changes to the
	// resource are patched back to the API server.
	defer func(isDeleted *bool) {
		if !*isDeleted {
			// Finally check whether PlatformAdmin is Ready
			platformAdminStatus.Ready = true
			if cond := util.GetPlatformAdminCondition(*platformAdminStatus, iotv1beta1.ConfigmapAvailableCondition); cond.Status == corev1.ConditionFalse {
				platformAdminStatus.Ready = false
			}
			if cond := util.GetPlatformAdminCondition(*platformAdminStatus, iotv1beta1.ComponentAvailableCondition); cond.Status == corev1.ConditionFalse {
				platformAdminStatus.Ready = false
			}
			if platformAdminStatus.UnreadyComponentNum != 0 {
				platformAdminStatus.Ready = false
			}

			// Finally update the status of PlatformAdmin
			platformAdmin.Status = *platformAdminStatus
			if err := r.Status().Update(ctx, platformAdmin); err != nil {
				klog.Error(Format("Update the status of PlatformAdmin %s/%s failed", platformAdmin.Namespace, platformAdmin.Name))
				reterr = kerrors.NewAggregate([]error{reterr, err})
			}

			if reterr != nil {
				klog.ErrorS(reterr, Format("Reconcile PlatformAdmin %s/%s failed", platformAdmin.Namespace, platformAdmin.Name))
			}
		}
	}(&isDeleted)

	if platformAdmin.DeletionTimestamp != nil {
		isDeleted = true
		return r.reconcileDelete(ctx, platformAdmin)
	}

	return r.reconcileNormal(ctx, platformAdmin, platformAdminStatus)
}

func (r *ReconcilePlatformAdmin) reconcileDelete(ctx context.Context, platformAdmin *iotv1beta1.PlatformAdmin) (reconcile.Result, error) {
	klog.V(4).Info(Format("ReconcileDelete PlatformAdmin %s/%s", platformAdmin.Namespace, platformAdmin.Name))
	yas := &appsv1beta1.YurtAppSet{}

	platformAdminFramework, err := r.readFramework(ctx, platformAdmin)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "unexpected error while synchronizing customize framework for %s", platformAdmin.Namespace+"/"+platformAdmin.Name)
	}
	desiredComponents := platformAdminFramework.Components

	for _, dc := range desiredComponents {
		if err := r.Get(
			ctx,
			types.NamespacedName{Namespace: platformAdmin.Namespace, Name: dc.Name},
			yas); err != nil {
			klog.V(4).ErrorS(err, Format("Get YurtAppSet %s/%s error", platformAdmin.Namespace, dc.Name))
			continue
		}

		oldYas := yas.DeepCopy()

		newPools := make([]string, 0)
		for _, poolName := range yas.Spec.Pools {
			if !util.Contains(platformAdmin.Spec.NodePools, poolName) {
				newPools = append(newPools, poolName)
			}
		}
		yas.Spec.Pools = newPools

		newTweaks := make([]appsv1beta1.WorkloadTweak, 0)
		for _, tweak := range yas.Spec.Workload.WorkloadTweaks {
			newTweakPools := make([]string, 0)
			for _, poolName := range tweak.Pools {
				if !util.Contains(platformAdmin.Spec.NodePools, poolName) {
					newTweakPools = append(newTweakPools, poolName)
				}
			}
			if len(newTweakPools) > 0 {
				newTweaks = append(newTweaks, appsv1beta1.WorkloadTweak{
					Pools:  newTweakPools,
					Tweaks: tweak.Tweaks,
				})
			}
		}
		yas.Spec.Workload.WorkloadTweaks = newTweaks

		if err := r.Client.Patch(ctx, yas, client.MergeFrom(oldYas)); err != nil {
			klog.V(4).ErrorS(err, Format("Patch YurtAppSet %s/%s error", platformAdmin.Namespace, dc.Name))
			return reconcile.Result{}, err
		}
	}

	controllerutil.RemoveFinalizer(platformAdmin, iotv1beta1.PlatformAdminFinalizer)
	if err := r.Client.Update(ctx, platformAdmin); err != nil {
		klog.Error(Format("Update PlatformAdmin %s error %v", klog.KObj(platformAdmin), err))
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcilePlatformAdmin) reconcileNormal(ctx context.Context, platformAdmin *iotv1beta1.PlatformAdmin, platformAdminStatus *iotv1beta1.PlatformAdminStatus) (reconcile.Result, error) {
	klog.V(4).Info(Format("ReconcileNormal PlatformAdmin %s/%s", platformAdmin.Namespace, platformAdmin.Name))
	controllerutil.AddFinalizer(platformAdmin, iotv1beta1.PlatformAdminFinalizer)

	platformAdminStatus.Initialized = true

	// Note that this configmap is different from the one below, which is used to customize the edgex framework
	// Sync configmap of edgex confiruation during initialization
	// This framework pointer is needed to synchronize user-modified edgex configurations
	platformAdminFramework, err := r.readFramework(ctx, platformAdmin)
	if err != nil {
		return reconcile.Result{}, errors.Wrapf(err, "unexpected error while synchronizing customize framework for %s", platformAdmin.Namespace+"/"+platformAdmin.Name)
	}

	// Reconcile configmap of edgex confiruation
	klog.V(4).Info(Format("ReconcileConfigmap PlatformAdmin %s/%s", platformAdmin.Namespace, platformAdmin.Name))
	if ok, err := r.reconcileConfigmap(ctx, platformAdmin, platformAdminStatus, platformAdminFramework); !ok {
		if err != nil {
			util.SetPlatformAdminCondition(platformAdminStatus, util.NewPlatformAdminCondition(iotv1beta1.ConfigmapAvailableCondition, corev1.ConditionFalse, iotv1beta1.ConfigmapProvisioningFailedReason, err.Error()))
			return reconcile.Result{}, errors.Wrapf(err,
				"unexpected error while reconciling configmap for %s", platformAdmin.Namespace+"/"+platformAdmin.Name)
		}
		util.SetPlatformAdminCondition(platformAdminStatus, util.NewPlatformAdminCondition(iotv1beta1.ConfigmapAvailableCondition, corev1.ConditionFalse, iotv1beta1.ConfigmapProvisioningReason, ""))
		return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
	}
	util.SetPlatformAdminCondition(platformAdminStatus, util.NewPlatformAdminCondition(iotv1beta1.ConfigmapAvailableCondition, corev1.ConditionTrue, "", ""))

	// Reconcile component of edgex confiruation
	klog.V(4).Info(Format("ReconcileComponent PlatformAdmin %s/%s", platformAdmin.Namespace, platformAdmin.Name))
	if ok, err := r.reconcileComponent(ctx, platformAdmin, platformAdminStatus, platformAdminFramework); !ok {
		if err != nil {
			util.SetPlatformAdminCondition(platformAdminStatus, util.NewPlatformAdminCondition(iotv1beta1.ComponentAvailableCondition, corev1.ConditionFalse, iotv1beta1.ComponentProvisioningReason, err.Error()))
			return reconcile.Result{}, errors.Wrapf(err,
				"unexpected error while reconciling component for %s", platformAdmin.Namespace+"/"+platformAdmin.Name)
		}
		util.SetPlatformAdminCondition(platformAdminStatus, util.NewPlatformAdminCondition(iotv1beta1.ComponentAvailableCondition, corev1.ConditionFalse, iotv1beta1.ComponentProvisioningReason, ""))
		return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
	}
	util.SetPlatformAdminCondition(platformAdminStatus, util.NewPlatformAdminCondition(iotv1beta1.ComponentAvailableCondition, corev1.ConditionTrue, "", ""))

	// Update the metadata of PlatformAdmin
	if err := r.Client.Update(ctx, platformAdmin); err != nil {
		klog.Error(Format("Update PlatformAdmin %s error %v", klog.KObj(platformAdmin), err))
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcilePlatformAdmin) reconcileConfigmap(ctx context.Context, platformAdmin *iotv1beta1.PlatformAdmin, _ *iotv1beta1.PlatformAdminStatus, platformAdminFramework *PlatformAdminFramework) (bool, error) {
	var configmaps []corev1.ConfigMap
	needConfigMaps := make(map[string]struct{})
	configmaps = platformAdminFramework.ConfigMaps

	for i, configmap := range configmaps {
		configmap.Namespace = platformAdmin.Namespace
		configmap.Labels = make(map[string]string)
		configmap.Labels[iotv1beta1.LabelPlatformAdminGenerate] = LabelConfigmap
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, &configmap, func() error {
			configmap.Data = platformAdminFramework.ConfigMaps[i].Data
			return controllerutil.SetOwnerReference(platformAdmin, &configmap, (r.Scheme()))
		})
		if err != nil {
			return false, err
		}

		needConfigMaps[configmap.Name] = struct{}{}
	}

	configmaplist := &corev1.ConfigMapList{}
	if err := r.List(ctx, configmaplist, client.InNamespace(platformAdmin.Namespace), client.MatchingLabels{iotv1beta1.LabelPlatformAdminGenerate: LabelConfigmap}); err == nil {
		for _, c := range configmaplist.Items {
			if _, ok := needConfigMaps[c.Name]; !ok {
				r.removeOwner(ctx, platformAdmin, &c)
			}
		}
	}

	return true, nil
}

func (r *ReconcilePlatformAdmin) reconcileComponent(ctx context.Context, platformAdmin *iotv1beta1.PlatformAdmin, platformAdminStatus *iotv1beta1.PlatformAdminStatus, platformAdminFramework *PlatformAdminFramework) (bool, error) {
	var (
		readyComponent int32 = 0
		needComponents       = make(map[string]struct{})
		needServices         = make(map[string]struct{})
	)

	// Users can configure components in the framework,
	// or they can choose to configure optional components directly in spec,
	// which combines the two approaches and tells the controller if the framework needs to be updated.
	needWriteFramework := r.calculateDesiredComponents(platformAdmin, platformAdminFramework)

	defer func() {
		platformAdminStatus.ReadyComponentNum = readyComponent
		platformAdminStatus.UnreadyComponentNum = int32(len(platformAdminFramework.Components)) - readyComponent
	}()

	// The component in spec that does not exist in the framework, so the framework needs to be updated.
	if needWriteFramework {
		if err := r.writeFramework(ctx, platformAdmin, platformAdminFramework); err != nil {
			return false, err
		}
	}

	// Update the yurtappsets based on the desired components
	for _, desiredComponent := range platformAdminFramework.Components {
		readyService := false
		readyDeployment := false
		needServices[desiredComponent.Name] = struct{}{}
		needComponents[platformAdmin.Name+"-"+desiredComponent.Name] = struct{}{}

		if _, err := r.handleService(ctx, platformAdmin, desiredComponent); err != nil {
			return false, err
		}
		readyService = true

		yas := &appsv1beta1.YurtAppSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      platformAdmin.Name + "-" + desiredComponent.Name,
				Namespace: platformAdmin.Namespace,
			},
		}

		err := r.Get(
			ctx,
			types.NamespacedName{
				Namespace: platformAdmin.Namespace,
				Name:      platformAdmin.Name + "-" + desiredComponent.Name},
			yas)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return false, err
			}
			_, err = r.handleYurtAppSet(ctx, platformAdmin, desiredComponent)
			if err != nil {
				return false, err
			}
		} else {
			oldYas := yas.DeepCopy()

			// Refresh the YurtAppSet according to the user-defined configuration
			yas.Spec.WorkloadTemplate.DeploymentTemplate.Spec = *desiredComponent.Deployment

			for _, poolName := range platformAdmin.Spec.NodePools {
				if slices.Contains(yas.Spec.Pools, poolName) {
					if yas.Status.ReadyWorkloads == yas.Status.TotalWorkloads {
						readyDeployment = true
						if readyDeployment && readyService {
							readyComponent++
						}
					}
				}

				pools := []string{poolName}
				tweaks := []appsv1beta1.WorkloadTweak{
					{
						Pools: []string{poolName},
						Tweaks: appsv1beta1.Tweaks{
							Replicas: ptr.To[int32](1),
						},
					},
				}
				flag := false
				for _, name := range yas.Spec.Pools {
					if name == poolName {
						flag = true
						break
					}
				}
				if !flag {
					yas.Spec.Pools = append(yas.Spec.Pools, pools...)
					yas.Spec.Workload.WorkloadTweaks = append(yas.Spec.Workload.WorkloadTweaks, tweaks...)
				}
			}
			if err := controllerutil.SetOwnerReference(platformAdmin, yas, r.Scheme()); err != nil {
				return false, err
			}
			if err := r.Client.Patch(ctx, yas, client.MergeFrom(oldYas)); err != nil {
				klog.Error(Format("Patch yurtappset %s/%s failed: %v", yas.Namespace, yas.Name, err))
				return false, err
			}
		}
	}

	// Remove the service owner that we do not need
	servicelist := &corev1.ServiceList{}
	if err := r.List(ctx, servicelist, client.InNamespace(platformAdmin.Namespace), client.MatchingLabels{iotv1beta1.LabelPlatformAdminGenerate: LabelService}); err == nil {
		for _, s := range servicelist.Items {
			if _, ok := needServices[s.Name]; !ok {
				r.removeOwner(ctx, platformAdmin, &s)
			}
		}
	}

	// Remove the yurtappset owner that we do not need
	yurtappsetlist := &appsv1beta1.YurtAppSetList{}
	if err := r.List(ctx, yurtappsetlist, client.InNamespace(platformAdmin.Namespace), client.MatchingLabels{iotv1beta1.LabelPlatformAdminGenerate: LabelDeployment}); err == nil {
		for _, s := range yurtappsetlist.Items {
			if _, ok := needComponents[s.Name]; !ok {
				r.removeOwner(ctx, platformAdmin, &s)
			}
		}
	}

	return readyComponent == int32(len(platformAdminFramework.Components)), nil
}

func (r *ReconcilePlatformAdmin) handleService(ctx context.Context, platformAdmin *iotv1beta1.PlatformAdmin, component *config.Component) (*corev1.Service, error) {
	// It is possible that the component does not need service.
	// Therefore, you need to be careful when calling this function.
	// It is still possible for service to be nil when there is no error!
	if component.Service == nil {
		return nil, nil
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
			Name:        component.Name,
			Namespace:   platformAdmin.Namespace,
		},
	}
	service.Labels[iotv1beta1.LabelPlatformAdminGenerate] = LabelService
	service.Annotations[AnnotationServiceTopologyKey] = AnnotationServiceTopologyValueNodePool

	_, err := controllerutil.CreateOrUpdate(
		ctx,
		r.Client,
		service,
		func() error {
			service.Spec = *component.Service
			return controllerutil.SetOwnerReference(platformAdmin, service, r.Scheme())
		},
	)
	if err != nil {
		return nil, err
	}
	return service, nil
}

func (r *ReconcilePlatformAdmin) handleYurtAppSet(ctx context.Context, platformAdmin *iotv1beta1.PlatformAdmin, component *config.Component) (*appsv1beta1.YurtAppSet, error) {
	// It is possible that the component does not need deployment.
	// Therefore, you need to be careful when calling this function.
	// It is still possible for deployment to be nil when there is no error!
	if component.Deployment == nil {
		return nil, nil
	}

	yas := &appsv1beta1.YurtAppSet{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
			Name:        platformAdmin.Name + "-" + component.Name,
			Namespace:   platformAdmin.Namespace,
		},
		Spec: appsv1beta1.YurtAppSetSpec{
			Workload: appsv1beta1.Workload{
				WorkloadTemplate: appsv1beta1.WorkloadTemplate{
					DeploymentTemplate: &appsv1beta1.DeploymentTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{"app": component.Name},
						},
						Spec: *component.Deployment,
					},
				},
			},
		},
	}

	yas.Labels[iotv1beta1.LabelPlatformAdminGenerate] = LabelDeployment
	yas.Spec.Pools = platformAdmin.Spec.NodePools
	for _, nodePool := range platformAdmin.Spec.NodePools {
		exists := false
		for _, pool := range yas.Spec.Pools {
			if pool == nodePool {
				exists = true
				break
			}
		}
		if !exists {
			yas.Spec.Pools = append(yas.Spec.Pools, nodePool)
		}
	}
	yas.Spec.Workload.WorkloadTweaks = []appsv1beta1.WorkloadTweak{
		{
			Pools: yas.Spec.Pools,
			Tweaks: appsv1beta1.Tweaks{
				Replicas: ptr.To[int32](1),
			},
		},
	}
	if err := controllerutil.SetControllerReference(platformAdmin, yas, r.Scheme()); err != nil {
		return nil, err
	}
	if err := r.Create(ctx, yas); err != nil {
		return nil, err
	}
	return yas, nil
}

func (r *ReconcilePlatformAdmin) removeOwner(ctx context.Context, platformAdmin *iotv1beta1.PlatformAdmin, obj client.Object) error {
	owners := obj.GetOwnerReferences()

	for i, owner := range owners {
		if owner.UID == platformAdmin.UID {
			owners[i] = owners[len(owners)-1]
			owners = owners[:len(owners)-1]

			if len(owners) == 0 {
				return r.Delete(ctx, obj)
			} else {
				obj.SetOwnerReferences(owners)
				return r.Update(ctx, obj)
			}
		}
	}
	return nil
}

func (r *ReconcilePlatformAdmin) readFramework(ctx context.Context, platformAdmin *iotv1beta1.PlatformAdmin) (*PlatformAdminFramework, error) {
	klog.V(6).Info(Format("Synchronize the customize framework information for PlatformAdmin %s/%s", platformAdmin.Namespace, platformAdmin.Name))

	// Try to get the configmap that represents the framework
	platformAdminFramework := &PlatformAdminFramework{
		// The configmap that represents framework is named with the framework prefix and the version name
		name: FrameworkName,
	}

	// Check if the configmap that represents framework is found
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: platformAdmin.Namespace, Name: platformAdminFramework.name}, cm); err != nil {
		if apierrors.IsNotFound(err) {
			// If the configmap that represents framework is not found,
			// need to create it by standard configuration
			err = r.initFramework(ctx, platformAdmin, platformAdminFramework)
			if err != nil {
				klog.Error(Format("Init framework for PlatformAdmin %s/%s error %v", platformAdmin.Namespace, platformAdmin.Name, err))
				return nil, err
			}
			return platformAdminFramework, nil
		}
		klog.Error(Format("Get framework for PlatformAdmin %s/%s error %v", platformAdmin.Namespace, platformAdmin.Name, err))
		return nil, err
	}

	// For better serialization, the serialization method of the Kubernetes runtime library is used
	err := runtime.DecodeInto(r.yamlSerializer, []byte(cm.Data["framework"]), platformAdminFramework)
	if err != nil {
		klog.Error(Format("Decode framework for PlatformAdmin %s/%s error %v", platformAdmin.Namespace, platformAdmin.Name, err))
		return nil, err
	}

	// If PlatformAdmin is about to be deleted, remove Finalizer from the framework.
	// If not deleted, the owner reference is synchronized.
	if platformAdmin.DeletionTimestamp != nil {
		_, err = controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
			// During the deletion phase, ensure that data in the framework is read before deletion
			// The following code removes the finalizer, allowing the framework to be deleted (since we read out its data above).
			controllerutil.RemoveFinalizer(cm, FrameworkFinalizer)
			return nil
		})
		if err != nil {
			klog.Error(Format("could not remove finalizer of framework configmap for PlatformAdmin %s/%s", platformAdmin.Namespace, platformAdmin.Name))
			return nil, err
		}
	} else {
		hasOwnerReference := false
		for _, ref := range cm.ObjectMeta.OwnerReferences {
			if ref.Kind == platformAdmin.Kind && ref.Name == platformAdmin.Name {
				hasOwnerReference = true
			}
		}
		if !hasOwnerReference {
			_, err = controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
				return controllerutil.SetOwnerReference(platformAdmin, cm, r.scheme)
			})
			if err != nil {
				klog.Error(Format("could not add owner reference of framework configmap for PlatformAdmin %s/%s", platformAdmin.Namespace, platformAdmin.Name))
				return nil, err
			}
		}
	}

	return platformAdminFramework, nil
}

func (r *ReconcilePlatformAdmin) writeFramework(ctx context.Context, platformAdmin *iotv1beta1.PlatformAdmin, platformAdminFramework *PlatformAdminFramework) error {
	// For better serialization, the serialization method of the Kubernetes runtime library is used
	data, err := runtime.Encode(r.yamlSerializer, platformAdminFramework)
	if err != nil {
		klog.Error(Format("could not marshal framework for PlatformAdmin %s/%s", platformAdmin.Namespace, platformAdmin.Name))
		return err
	}

	// Check if the configmap that represents framework is found
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: platformAdmin.Namespace, Name: platformAdminFramework.name}, cm); err != nil {
		if apierrors.IsNotFound(err) {
			// If the configmap that represents framework is not found,
			// need to create it by standard configuration
			err = r.initFramework(ctx, platformAdmin, platformAdminFramework)
			if err != nil {
				klog.Error(Format("Init framework for PlatformAdmin %s/%s error %v", platformAdmin.Namespace, platformAdmin.Name, err))
				return err
			}
			return nil
		}
		klog.Error(Format("Get framework for PlatformAdmin %s/%s error %v", platformAdmin.Namespace, platformAdmin.Name, err))
		return err
	}

	// Creates configmap on behalf of the framework, which is called only once upon creation
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		cm.Data["framework"] = string(data)
		return controllerutil.SetOwnerReference(platformAdmin, cm, r.Scheme())
	})
	if err != nil {
		klog.Error(Format("could not write framework configmap for PlatformAdmin %s/%s", platformAdmin.Namespace, platformAdmin.Name))
		return err
	}
	return nil
}

// initFramework initializes the framework information for PlatformAdmin
func (r *ReconcilePlatformAdmin) initFramework(ctx context.Context, platformAdmin *iotv1beta1.PlatformAdmin, platformAdminFramework *PlatformAdminFramework) error {
	klog.V(6).Info(Format("Initializes the standard framework information for PlatformAdmin %s/%s", platformAdmin.Namespace, platformAdmin.Name))

	// Use standard configurations to build the framework
	platformAdminFramework.security = platformAdmin.Spec.Security
	if platformAdminFramework.security {
		platformAdminFramework.ConfigMaps = r.Configuration.SecurityConfigMaps[platformAdmin.Spec.Version]
		r.calculateDesiredComponents(platformAdmin, platformAdminFramework)
	} else {
		platformAdminFramework.ConfigMaps = r.Configuration.NoSectyConfigMaps[platformAdmin.Spec.Version]
		r.calculateDesiredComponents(platformAdmin, platformAdminFramework)
	}

	// For better serialization, the serialization method of the Kubernetes runtime library is used
	data, err := runtime.Encode(r.yamlSerializer, platformAdminFramework)
	if err != nil {
		klog.Error(Format("could not marshal framework for PlatformAdmin %s/%s", platformAdmin.Namespace, platformAdmin.Name))
		return err
	}

	// Create the configmap that represents framework
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      platformAdminFramework.name,
			Namespace: platformAdmin.Namespace,
		},
	}
	cm.Labels = make(map[string]string)
	cm.Labels[iotv1beta1.LabelPlatformAdminGenerate] = LabelFramework
	cm.Data = make(map[string]string)
	cm.Data["framework"] = string(data)
	// Creates configmap on behalf of the framework, which is called only once upon creation
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		// We need to control the deletion time of the framework,
		// because we must ensure that its data is read before deleting it.
		controllerutil.AddFinalizer(cm, FrameworkFinalizer)
		return controllerutil.SetOwnerReference(platformAdmin, cm, r.Scheme())
	})
	if err != nil {
		klog.Error(Format("could not init framework configmap for PlatformAdmin %s/%s", platformAdmin.Namespace, platformAdmin.Name))
		return err
	}
	return nil
}

// calculateDesiredComponents calculates the components that need to be added and determines whether the framework needs to be rewritten
func (r *ReconcilePlatformAdmin) calculateDesiredComponents(platformAdmin *iotv1beta1.PlatformAdmin, platformAdminFramework *PlatformAdminFramework) bool {
	needWriteFramework := false
	desiredComponents := []*config.Component{}

	// Find all the required components from spec and manifest
	requiredComponentSet := config.ExtractRequiredComponentsName(&r.Configuration.Manifest, platformAdmin.Spec.Version)
	for _, component := range platformAdmin.Spec.Components {
		requiredComponentSet.Insert(component.Name)
	}

	// Find all existing components and filter removed components
	frameworkComponentSet := sets.NewString()
	for _, component := range platformAdminFramework.Components {
		if requiredComponentSet.Has(component.Name) {
			frameworkComponentSet.Insert(component.Name)
			desiredComponents = append(desiredComponents, component)
		} else {
			needWriteFramework = true
		}
	}

	// Calculate all the components that need to be added or removed and determine whether need to rewrite the framework
	addedComponentSet := sets.NewString()
	for _, componentName := range requiredComponentSet.UnsortedList() {
		if !frameworkComponentSet.Has(componentName) {
			addedComponentSet.Insert(componentName)
			needWriteFramework = true
		}
	}

	// If a component needs to be added,
	// check whether the corresponding template exists in the standard configuration library
	if platformAdmin.Spec.Security {
		for _, component := range r.Configuration.SecurityComponents[platformAdmin.Spec.Version] {
			if addedComponentSet.Has(component.Name) {
				desiredComponents = append(desiredComponents, component)
			}
		}
	} else {
		for _, component := range r.Configuration.NoSectyComponents[platformAdmin.Spec.Version] {
			if addedComponentSet.Has(component.Name) {
				desiredComponents = append(desiredComponents, component)
			}
		}
	}

	// The yurt-iot-dock is maintained by openyurt and is not obtained through an auto-collector.
	// Therefore, it needs to be handled separately
	if addedComponentSet.Has(util.IotDockName) {
		yurtIotDock, err := newYurtIoTDockComponent(platformAdmin, platformAdminFramework)
		if err != nil {
			klog.Error(Format("newYurtIoTDockComponent error %v", err))
		}
		desiredComponents = append(desiredComponents, yurtIotDock)
	}

	platformAdminFramework.Components = desiredComponents

	return needWriteFramework
}
