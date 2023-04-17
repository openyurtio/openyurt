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

package endpoints

import (
	"context"
	"flag"
	"fmt"
	"github.com/openyurtio/openyurt/pkg/controller/servicetopology/adapter"
	corev1 "k8s.io/api/core/v1"
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
	utilclient "github.com/openyurtio/openyurt/pkg/util/client"
	utildiscovery "github.com/openyurtio/openyurt/pkg/util/discovery"
)

func init() {
	flag.IntVar(&concurrentReconciles, "servicetopology-endpoints-workers", concurrentReconciles, "Max concurrent workers for Servicetopology-endpoints controller.")
}

var (
	concurrentReconciles = 3
	controllerKind       = corev1.SchemeGroupVersion.WithKind("Endpoints")
)

const (
	controllerName = "Servicetopology-endpoints-controller"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", controllerName, s)
}

// Add creates a new Servicetopology endpoints Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(c *appconfig.CompletedConfig, mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) {
		return nil
	}
	return add(mgr, newReconciler(c, mgr))
}

var _ reconcile.Reconciler = &ReconcileServicetopologyEndpoints{}

// ReconcileServicetopologyEndpoints reconciles a endpoints object
type ReconcileServicetopologyEndpoints struct {
	client.Client
	scheme           *runtime.Scheme
	recorder         record.EventRecorder
	endpointsAdapter adapter.Adapter
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(_ *appconfig.CompletedConfig, mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileServicetopologyEndpoints{
		Client:   utilclient.NewClientFromManager(mgr, controllerName),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor(controllerName),
	}
}

func (r *ReconcileServicetopologyEndpoints) InjectConfig(cfg *rest.Config) error {
	c, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Errorf(Format("failed to create kube client, %v", err))
		return err
	}
	r.endpointsAdapter = adapter.NewEndpointsAdapter(c, r.Client)
	return nil
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{Reconciler: r, MaxConcurrentReconciles: concurrentReconciles})
	if err != nil {
		return err
	}

	// Watch for changes to Service
	if err := c.Watch(&source.Kind{Type: &corev1.Service{}}, &EnqueueEndpointsForService{
		endpointsAdapter: r.(*ReconcileServicetopologyEndpoints).endpointsAdapter,
	}); err != nil {
		return err
	}

	// Watch for changes to NodePool
	if err := c.Watch(&source.Kind{Type: &appsv1beta1.NodePool{}}, &EnqueueEndpointsForNodePool{
		endpointsAdapter: r.(*ReconcileServicetopologyEndpoints).endpointsAdapter,
		client:           r.(*ReconcileServicetopologyEndpoints).Client,
	}); err != nil {
		return err
	}

	return nil
}

// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps.openyurt.io,resources=nodepools,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=endpoints,verbs=get;list;watch;patch

// Reconcile reads that state of the cluster for endpoints object and makes changes based on the state read
func (r *ReconcileServicetopologyEndpoints) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {

	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.Infof(Format("Reconcile Endpoints %s/%s", request.Namespace, request.Name))

	// Fetch the Endpoints instance
	instance := &corev1.Endpoints{}
	if err := r.Get(context.TODO(), request.NamespacedName, instance); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if instance.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	if err := r.syncEndpoints(request.Namespace, request.Name); err != nil {
		klog.Errorf(Format("sync endpoints %v failed with : %v", request.NamespacedName, err))
		return reconcile.Result{Requeue: true}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileServicetopologyEndpoints) syncEndpoints(namespace, name string) error {
	return r.endpointsAdapter.UpdateTriggerAnnotations(namespace, name)
}
