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
	"flag"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	discoveryv1beta1 "k8s.io/api/discovery/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	appsv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	common "github.com/openyurtio/openyurt/pkg/controller/servicetopology"
	"github.com/openyurtio/openyurt/pkg/controller/servicetopology/adapter"
	utilclient "github.com/openyurtio/openyurt/pkg/util/client"
	utildiscovery "github.com/openyurtio/openyurt/pkg/util/discovery"
)

func init() {
	flag.IntVar(&concurrentReconciles, "servicetopology-endpointslice-workers", concurrentReconciles, "Max concurrent workers for Servicetopology-endpointslice controller.")
}

var (
	concurrentReconciles  = 3
	controllerV1Kind      = discoveryv1.SchemeGroupVersion.WithKind("EndpointSlice")
	controllerV1beta1Kind = discoveryv1beta1.SchemeGroupVersion.WithKind("EndpointSlice")
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s-endpointslice: %s", common.ControllerName, s)
}

// Add creates a new Servicetopology endpointslice Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(c *appconfig.CompletedConfig, mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerV1Kind) && !utildiscovery.DiscoverGVK(controllerV1beta1Kind) {
		return nil
	}

	klog.Infof("servicetopology-endpointslice-controller add controller %s and %s", controllerV1Kind.String(), controllerV1beta1Kind.String())
	return add(mgr, newReconciler(c, mgr))
}

var _ reconcile.Reconciler = &ReconcileServiceTopologyEndpointSlice{}

// ReconcileServiceTopologyEndpointSlice reconciles a Example object
type ReconcileServiceTopologyEndpointSlice struct {
	client.Client
	scheme                   *runtime.Scheme
	recorder                 record.EventRecorder
	endpointsliceAdapter     adapter.Adapter
	isSupportEndpointslicev1 bool
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(_ *appconfig.CompletedConfig, mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileServiceTopologyEndpointSlice{
		Client:   utilclient.NewClientFromManager(mgr, common.ControllerName),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor(common.ControllerName),
	}
}

func (r *ReconcileServiceTopologyEndpointSlice) InjectConfig(cfg *rest.Config) error {
	c, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Errorf(Format("failed to create kube client, %v", err))
		return err
	}
	endpointSliceAdapter, isSupportEndpointslicev1, err := getEndpointSliceAdapter(c, r.Client)
	if err != nil {
		return err
	}
	r.endpointsliceAdapter = endpointSliceAdapter
	r.isSupportEndpointslicev1 = isSupportEndpointslicev1
	return nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(fmt.Sprintf("%s-endpointslice", common.ControllerName), mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: concurrentReconciles})
	if err != nil {
		return err
	}

	// Watch for changes to Service
	if err := c.Watch(&source.Kind{Type: &corev1.Service{}}, &EnqueueEndpointsliceForService{
		endpointsliceAdapter: r.(*ReconcileServiceTopologyEndpointSlice).endpointsliceAdapter,
	}); err != nil {
		return err
	}

	// Watch for changes to NodePool
	if err := c.Watch(&source.Kind{Type: &appsv1beta1.NodePool{}}, &EnqueueEndpointsliceForNodePool{
		endpointsliceAdapter: r.(*ReconcileServiceTopologyEndpointSlice).endpointsliceAdapter,
		client:               r.(*ReconcileServiceTopologyEndpointSlice).Client,
	}); err != nil {
		return err
	}

	return nil
}

// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps.openyurt.io,resources=nodepools,verbs=get;list;watch
// +kubebuilder:rbac:groups=discovery.k8s.io,resources=endpointslices,verbs=get;list;watch;patch

// Reconcile reads that state of the cluster for endpointslice object and makes changes based on the state read
func (r *ReconcileServiceTopologyEndpointSlice) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {

	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.Infof(Format("Reconcile Endpointslice %s/%s", request.Namespace, request.Name))

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
		klog.Errorf(Format("sync endpointslice %v failed with : %v", request.NamespacedName, err))
		return reconcile.Result{Requeue: true}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileServiceTopologyEndpointSlice) syncEndpointslice(namespace, name string) error {
	return r.endpointsliceAdapter.UpdateTriggerAnnotations(namespace, name)
}

func getEndpointSliceAdapter(kubeClient kubernetes.Interface, client client.Client) (adapter.Adapter, bool, error) {
	_, err := kubeClient.DiscoveryV1().EndpointSlices(corev1.NamespaceAll).List(context.TODO(), metav1.ListOptions{})

	if err == nil {
		klog.Infof(Format("v1.EndpointSlice is supported."))
		return adapter.NewEndpointsV1Adapter(kubeClient, client), true, nil
	}
	if errors.IsNotFound(err) {
		klog.Infof(Format("fall back to v1beta1.EndpointSlice."))
		return adapter.NewEndpointsV1Beta1Adapter(kubeClient, client), false, nil
	}
	return nil, false, err
}
