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

package iot

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	unitv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	devicev1alpha1 "github.com/openyurtio/openyurt/pkg/apis/device/v1alpha1"
	devicev1alpha2 "github.com/openyurtio/openyurt/pkg/apis/device/v1alpha2"
	"github.com/openyurtio/openyurt/pkg/controller/iot/config"
	util "github.com/openyurtio/openyurt/pkg/controller/iot/utils"
	utilclient "github.com/openyurtio/openyurt/pkg/util/client"
	utildiscovery "github.com/openyurtio/openyurt/pkg/util/discovery"
)

func init() {
	flag.IntVar(&concurrentReconciles, "iot-workers", concurrentReconciles, "Max concurrent workers for IoT controller.")
}

var (
	concurrentReconciles = 3
	controllerKind       = devicev1alpha2.SchemeGroupVersion.WithKind("IoT")
)

const (
	ControllerName = "IoT"

	LabelConfigmap  = "Configmap"
	LabelService    = "Service"
	LabelDeployment = "Deployment"

	AnnotationServiceTopologyKey           = "openyurt.io/topologyKeys"
	AnnotationServiceTopologyValueNodePool = "openyurt.io/nodepool"

	ConfigMapName = "common-variables"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", ControllerName, s)
}

// ReconcileIoT reconciles a IoT object
type ReconcileIoT struct {
	client.Client
	scheme       *runtime.Scheme
	recorder     record.EventRecorder
	Configration config.IoTControllerConfiguration
}

var _ reconcile.Reconciler = &ReconcileIoT{}

// Add creates a new IoT Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(c *appconfig.CompletedConfig, mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) {
		return nil
	}

	klog.Infof("iot-controller add controller %s", controllerKind.String())
	return add(mgr, newReconciler(c, mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *appconfig.CompletedConfig, mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileIoT{
		Client:       utilclient.NewClientFromManager(mgr, ControllerName),
		scheme:       mgr.GetScheme(),
		recorder:     mgr.GetEventRecorderFor(ControllerName),
		Configration: c.ComponentConfig.IoTController,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles,
	})
	if err != nil {
		return err
	}

	// Watch for changes to IoT
	err = c.Watch(&source.Kind{Type: &devicev1alpha2.IoT{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForOwner{
		IsController: false,
		OwnerType:    &devicev1alpha2.IoT{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &handler.EnqueueRequestForOwner{
		IsController: false,
		OwnerType:    &devicev1alpha2.IoT{},
	})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &unitv1alpha1.YurtAppSet{}}, &handler.EnqueueRequestForOwner{
		IsController: false,
		OwnerType:    &devicev1alpha2.IoT{},
	})
	if err != nil {
		return err
	}

	klog.Info("registering the field indexers of iot controller")
	if err := util.RegisterFieldIndexers(mgr.GetFieldIndexer()); err != nil {
		klog.Errorf("failed to register field indexers for iot controller, %v", err)
		return nil
	}

	return nil
}

// +kubebuilder:rbac:groups=device.openyurt.io,resources=iots,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=device.openyurt.io,resources=iots/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=device.openyurt.io,resources=iots/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps.openyurt.io,resources=yurtappsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.openyurt.io,resources=yurtappsets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps;services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps/status;services/status,verbs=get;update;patch

// Reconcile reads that state of the cluster for a IoT object and makes changes based on the state read
// and what is in the IoT.Spec
func (r *ReconcileIoT) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	klog.Infof(Format("Reconcile IoT %s/%s", request.Namespace, request.Name))

	// Fetch the IoT instance
	iot := &devicev1alpha2.IoT{}
	if err := r.Get(ctx, request.NamespacedName, iot); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	defer func() {
		if err := r.Status().Update(ctx, iot); err != nil {
			klog.Errorf(Format("Patch status to IoT %s error %v", klog.KObj(iot), err))
		}
	}()

	if iot.DeletionTimestamp != nil {
		return r.reconcileDelete(ctx, iot)
	}

	return r.reconcileNormal(ctx, iot)
}

func (r *ReconcileIoT) reconcileDelete(ctx context.Context, iot *devicev1alpha2.IoT) (reconcile.Result, error) {
	ud := &unitv1alpha1.YurtAppSet{}
	var desiredComponents []*config.Component
	if iot.Spec.Security {
		desiredComponents = r.Configration.SecurityComponents[iot.Spec.Version]
	} else {
		desiredComponents = r.Configration.NoSectyComponents[iot.Spec.Version]
	}

	additionalComponents, err := annotationToComponent(iot.Annotations)
	if err != nil {
		return reconcile.Result{}, err
	}
	desiredComponents = append(desiredComponents, additionalComponents...)

	//TODO: handle iot.Spec.Components

	for _, dc := range desiredComponents {
		if err := r.Get(
			ctx,
			types.NamespacedName{Namespace: iot.Namespace, Name: dc.Name},
			ud); err != nil {
			continue
		}

		for i, pool := range ud.Spec.Topology.Pools {
			if pool.Name == iot.Spec.PoolName {
				ud.Spec.Topology.Pools[i] = ud.Spec.Topology.Pools[len(ud.Spec.Topology.Pools)-1]
				ud.Spec.Topology.Pools = ud.Spec.Topology.Pools[:len(ud.Spec.Topology.Pools)-1]
			}
		}
		if err := r.Update(ctx, ud); err != nil {
			return reconcile.Result{}, err
		}
	}

	controllerutil.RemoveFinalizer(iot, devicev1alpha2.IoTFinalizer)

	return reconcile.Result{}, nil
}

func (r *ReconcileIoT) reconcileNormal(ctx context.Context, iot *devicev1alpha2.IoT) (reconcile.Result, error) {
	klog.Infof(Format("ReconcileNormal IoT %s/%s", iot.Namespace, iot.Name))
	controllerutil.AddFinalizer(iot, devicev1alpha2.IoTFinalizer)

	iot.Status.Initialized = true
	klog.Infof(Format("ReconcileConfigmap IoT %s/%s", iot.Namespace, iot.Name))
	if ok, err := r.reconcileConfigmap(ctx, iot); !ok {
		if err != nil {
			util.SetIoTCondition(&iot.Status, util.NewIoTCondition(devicev1alpha2.ConfigmapAvailableCondition, corev1.ConditionFalse, devicev1alpha2.ConfigmapProvisioningFailedReason, err.Error()))
			return reconcile.Result{}, errors.Wrapf(err,
				"unexpected error while reconciling configmap for %s", iot.Namespace+"/"+iot.Name)
		}
		util.SetIoTCondition(&iot.Status, util.NewIoTCondition(devicev1alpha2.ConfigmapAvailableCondition, corev1.ConditionFalse, devicev1alpha2.ConfigmapProvisioningReason, ""))
		return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
	}
	util.SetIoTCondition(&iot.Status, util.NewIoTCondition(devicev1alpha2.ConfigmapAvailableCondition, corev1.ConditionTrue, "", ""))

	klog.Infof(Format("ReconcileComponent IoT %s/%s", iot.Namespace, iot.Name))
	if ok, err := r.reconcileComponent(ctx, iot); !ok {
		if err != nil {
			util.SetIoTCondition(&iot.Status, util.NewIoTCondition(devicev1alpha2.ComponentAvailableCondition, corev1.ConditionFalse, devicev1alpha2.ComponentProvisioningReason, err.Error()))
			return reconcile.Result{}, errors.Wrapf(err,
				"unexpected error while reconciling Component for %s", iot.Namespace+"/"+iot.Name)
		}
		util.SetIoTCondition(&iot.Status, util.NewIoTCondition(devicev1alpha2.ComponentAvailableCondition, corev1.ConditionFalse, devicev1alpha2.ComponentProvisioningReason, ""))
		return reconcile.Result{RequeueAfter: 10 * time.Second}, nil
	}
	util.SetIoTCondition(&iot.Status, util.NewIoTCondition(devicev1alpha2.ConfigmapAvailableCondition, corev1.ConditionTrue, "", ""))

	iot.Status.Ready = true

	return reconcile.Result{}, nil
}

func (r *ReconcileIoT) removeOwner(ctx context.Context, iot *devicev1alpha2.IoT, obj client.Object) error {
	owners := obj.GetOwnerReferences()
	for i, owner := range owners {
		if owner.UID == iot.UID {
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

func (r *ReconcileIoT) reconcileConfigmap(ctx context.Context, iot *devicev1alpha2.IoT) (bool, error) {
	var configmaps []corev1.ConfigMap
	needConfigMaps := make(map[string]struct{})

	if iot.Spec.Security {
		configmaps = r.Configration.SecurityConfigMaps[iot.Spec.Version]
	} else {
		configmaps = r.Configration.NoSectyConfigMaps[iot.Spec.Version]
	}
	for _, configmap := range configmaps {
		// Supplement runtime information
		configmap.Namespace = iot.Namespace
		configmap.Labels = make(map[string]string)
		configmap.Labels[devicev1alpha2.LabelIoTGenerate] = LabelConfigmap

		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, &configmap, func() error {
			return controllerutil.SetOwnerReference(iot, &configmap, (r.Scheme()))
		})
		if err != nil {
			return false, err
		}

		needConfigMaps[configmap.Name] = struct{}{}
	}

	configmaplist := &corev1.ConfigMapList{}
	if err := r.List(ctx, configmaplist, client.InNamespace(iot.Namespace), client.MatchingLabels{devicev1alpha2.LabelIoTGenerate: LabelConfigmap}); err == nil {
		for _, c := range configmaplist.Items {
			if _, ok := needConfigMaps[c.Name]; !ok {
				r.removeOwner(ctx, iot, &c)
			}
		}
	}

	return true, nil
}

func (r *ReconcileIoT) reconcileComponent(ctx context.Context, iot *devicev1alpha2.IoT) (bool, error) {
	var desireComponents []*config.Component
	needComponents := make(map[string]struct{})
	var readyComponent int32 = 0

	if iot.Spec.Security {
		desireComponents = r.Configration.SecurityComponents[iot.Spec.Version]
	} else {
		desireComponents = r.Configration.NoSectyComponents[iot.Spec.Version]
	}

	additionalComponents, err := annotationToComponent(iot.Annotations)
	if err != nil {
		return false, err
	}
	desireComponents = append(desireComponents, additionalComponents...)

	//TODO: handle iot.Spec.Components

	defer func() {
		iot.Status.ReadyComponentNum = readyComponent
		iot.Status.UnreadyComponentNum = int32(len(desireComponents)) - readyComponent
	}()

NextC:
	for _, desireComponent := range desireComponents {
		readyService := false
		readyDeployment := false
		needComponents[desireComponent.Name] = struct{}{}

		if _, err := r.handleService(ctx, iot, desireComponent); err != nil {
			return false, err
		}
		readyService = true

		ud := &unitv1alpha1.YurtAppSet{}
		err := r.Get(
			ctx,
			types.NamespacedName{
				Namespace: iot.Namespace,
				Name:      desireComponent.Name},
			ud)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return false, err
			}
			_, err = r.handleYurtAppSet(ctx, iot, desireComponent)
			if err != nil {
				return false, err
			}
		} else {
			if _, ok := ud.Status.PoolReplicas[iot.Spec.PoolName]; ok {
				if ud.Status.ReadyReplicas == ud.Status.Replicas {
					readyDeployment = true
					if readyDeployment && readyService {
						readyComponent++
					}
				}
				continue NextC
			}
			pool := unitv1alpha1.Pool{
				Name:     iot.Spec.PoolName,
				Replicas: pointer.Int32Ptr(1),
			}
			pool.NodeSelectorTerm.MatchExpressions = append(pool.NodeSelectorTerm.MatchExpressions,
				corev1.NodeSelectorRequirement{
					Key:      unitv1alpha1.LabelCurrentNodePool,
					Operator: corev1.NodeSelectorOpIn,
					Values:   []string{iot.Spec.PoolName},
				})
			flag := false
			for _, up := range ud.Spec.Topology.Pools {
				if up.Name == pool.Name {
					flag = true
					break
				}
			}
			if !flag {
				ud.Spec.Topology.Pools = append(ud.Spec.Topology.Pools, pool)
			}
			if err := controllerutil.SetOwnerReference(iot, ud, r.Scheme()); err != nil {
				return false, err
			}
			if err := r.Update(ctx, ud); err != nil {
				return false, err
			}
		}
	}

	// Remove the service owner that we do not need
	servicelist := &corev1.ServiceList{}
	if err := r.List(ctx, servicelist, client.InNamespace(iot.Namespace), client.MatchingLabels{devicev1alpha2.LabelIoTGenerate: LabelService}); err == nil {
		for _, s := range servicelist.Items {
			if _, ok := needComponents[s.Name]; !ok {
				r.removeOwner(ctx, iot, &s)
			}
		}
	}

	// Remove the yurtappset owner that we do not need
	yurtappsetlist := &unitv1alpha1.YurtAppSetList{}
	if err := r.List(ctx, yurtappsetlist, client.InNamespace(iot.Namespace), client.MatchingLabels{devicev1alpha2.LabelIoTGenerate: LabelDeployment}); err == nil {
		for _, s := range yurtappsetlist.Items {
			if _, ok := needComponents[s.Name]; !ok {
				r.removeOwner(ctx, iot, &s)
			}
		}
	}

	return readyComponent == int32(len(desireComponents)), nil
}

func (r *ReconcileIoT) handleService(ctx context.Context, iot *devicev1alpha2.IoT, component *config.Component) (*corev1.Service, error) {
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
			Namespace:   iot.Namespace,
		},
		Spec: *component.Service,
	}
	service.Labels[devicev1alpha2.LabelIoTGenerate] = LabelService
	service.Annotations[AnnotationServiceTopologyKey] = AnnotationServiceTopologyValueNodePool

	_, err := controllerutil.CreateOrUpdate(
		ctx,
		r.Client,
		service,
		func() error {
			return controllerutil.SetOwnerReference(iot, service, r.Scheme())
		},
	)

	if err != nil {
		return nil, err
	}
	return service, nil
}

func (r *ReconcileIoT) handleYurtAppSet(ctx context.Context, iot *devicev1alpha2.IoT, component *config.Component) (*unitv1alpha1.YurtAppSet, error) {
	ud := &unitv1alpha1.YurtAppSet{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
			Name:        component.Name,
			Namespace:   iot.Namespace,
		},
		Spec: unitv1alpha1.YurtAppSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": component.Name},
			},
			WorkloadTemplate: unitv1alpha1.WorkloadTemplate{
				DeploymentTemplate: &unitv1alpha1.DeploymentTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{"app": component.Name},
					},
					Spec: *component.Deployment,
				},
			},
		},
	}

	ud.Labels[devicev1alpha2.LabelIoTGenerate] = LabelDeployment
	pool := unitv1alpha1.Pool{
		Name:     iot.Spec.PoolName,
		Replicas: pointer.Int32Ptr(1),
	}
	pool.NodeSelectorTerm.MatchExpressions = append(pool.NodeSelectorTerm.MatchExpressions,
		corev1.NodeSelectorRequirement{
			Key:      unitv1alpha1.LabelCurrentNodePool,
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{iot.Spec.PoolName},
		})
	ud.Spec.Topology.Pools = append(ud.Spec.Topology.Pools, pool)
	if err := controllerutil.SetControllerReference(iot, ud, r.Scheme()); err != nil {
		return nil, err
	}
	if err := r.Create(ctx, ud); err != nil {
		return nil, err
	}
	return ud, nil
}

// For version compatibility, v1alpha1's additionalservice and additionaldeployment are placed in
// v2alpha2's annotation, this function is to convert the annotation to component.
func annotationToComponent(annotation map[string]string) ([]*config.Component, error) {
	var components []*config.Component = []*config.Component{}
	var additionalDeployments []devicev1alpha1.DeploymentTemplateSpec = make([]devicev1alpha1.DeploymentTemplateSpec, 0)
	if _, ok := annotation["AdditionalDeployments"]; ok {
		err := json.Unmarshal([]byte(annotation["AdditionalDeployments"]), &additionalDeployments)
		if err != nil {
			return nil, err
		}
	}
	var additionalServices []devicev1alpha1.ServiceTemplateSpec = make([]devicev1alpha1.ServiceTemplateSpec, 0)
	if _, ok := annotation["AdditionalServices"]; ok {
		err := json.Unmarshal([]byte(annotation["AdditionalServices"]), &additionalServices)
		if err != nil {
			return nil, err
		}
	}
	if len(additionalDeployments) == 0 && len(additionalServices) == 0 {
		return components, nil
	}
	var services map[string]*corev1.ServiceSpec = make(map[string]*corev1.ServiceSpec)
	var usedServices map[string]struct{} = make(map[string]struct{})
	for _, additionalservice := range additionalServices {
		services[additionalservice.Name] = &additionalservice.Spec
	}
	for _, additionalDeployment := range additionalDeployments {
		var component config.Component
		component.Name = additionalDeployment.Name
		component.Deployment = &additionalDeployment.Spec
		service, ok := services[component.Name]
		if ok {
			component.Service = service
			usedServices[component.Name] = struct{}{}
		}
		components = append(components, &component)
	}
	if len(usedServices) < len(services) {
		for name, service := range services {
			_, ok := usedServices[name]
			if ok {
				continue
			}
			var component config.Component
			component.Name = name
			component.Service = service
			components = append(components, &component)
		}
	}

	return components, nil
}
