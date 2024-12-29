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

package endpointslice

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	discoveryv1beta1 "k8s.io/api/discovery/v1beta1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	yurtClient "github.com/openyurtio/openyurt/cmd/yurt-manager/app/client"
	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/servicetopology/adapter"
)

var (
	v1EndpointSliceGVR = discoveryv1.SchemeGroupVersion.WithResource("endpointslices")
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.ServiceTopologyEndpointSliceController, s)
}

// Add creates a new Servicetopology endpointslice Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, cfg *appconfig.CompletedConfig, mgr manager.Manager) error {
	r := newReconciler(cfg, mgr)
	c, err := controller.New(names.ServiceTopologyEndpointSliceController, mgr,
		controller.Options{Reconciler: r, MaxConcurrentReconciles: int(cfg.ComponentConfig.ServiceTopologyEndpointSliceController.ConcurrentEndpointSliceWorkers)})
	if err != nil {
		return err
	}

	// Watch for changes to Service
	if err := c.Watch(source.Kind[client.Object](mgr.GetCache(), &corev1.Service{}, &EnqueueEndpointsliceForService{
		endpointsliceAdapter: r.endpointsliceAdapter,
	})); err != nil {
		return err
	}

	klog.Infof("%s controller is added", names.ServiceTopologyEndpointSliceController)
	return nil
}

var _ reconcile.Reconciler = &ReconcileServiceTopologyEndpointSlice{}

// ReconcileServiceTopologyEndpointSlice reconciles a Example object
type ReconcileServiceTopologyEndpointSlice struct {
	client.Client
	endpointsliceAdapter     adapter.Adapter
	isSupportEndpointslicev1 bool
}

func newReconciler(_ *appconfig.CompletedConfig, mgr manager.Manager) *ReconcileServiceTopologyEndpointSlice {
	r := &ReconcileServiceTopologyEndpointSlice{
		Client: yurtClient.GetClientByControllerNameOrDie(mgr, names.ServiceTopologyEndpointSliceController),
	}
	if gvk, err := mgr.GetRESTMapper().KindFor(v1EndpointSliceGVR); err != nil {
		klog.Errorf("v1.EndpointSlice is not supported, %v", err)
		r.endpointsliceAdapter = adapter.NewEndpointsV1Beta1Adapter(r.Client)
		r.isSupportEndpointslicev1 = false
	} else {
		klog.Infof("%s is supported", gvk.String())
		r.endpointsliceAdapter = adapter.NewEndpointsV1Adapter(r.Client)
		r.isSupportEndpointslicev1 = true
	}

	return r
}

// +kubebuilder:rbac:groups=core,resources=services,verbs=get
// +kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;patch

// Reconcile reads that state of the cluster for endpointslice object and makes changes based on the state read
func (r *ReconcileServiceTopologyEndpointSlice) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {

	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.Info(Format("Reconcile Endpointslice %s/%s", request.Namespace, request.Name))

	// Fetch the Endpointslice instance
	if r.isSupportEndpointslicev1 {
		instance := &discoveryv1.EndpointSlice{}
		if err := r.Get(context.TODO(), request.NamespacedName, instance); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		if instance.DeletionTimestamp != nil {
			return reconcile.Result{}, nil
		}
	} else {
		instance := &discoveryv1beta1.EndpointSlice{}
		if err := r.Get(context.TODO(), request.NamespacedName, instance); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
		if instance.DeletionTimestamp != nil {
			return reconcile.Result{}, nil
		}
	}

	if err := r.syncEndpointslice(request.Namespace, request.Name); err != nil {
		klog.Error(Format("sync endpointslice %v failed with : %v", request.NamespacedName, err))
		return reconcile.Result{Requeue: true}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileServiceTopologyEndpointSlice) syncEndpointslice(namespace, name string) error {
	return r.endpointsliceAdapter.UpdateTriggerAnnotations(namespace, name)
}
