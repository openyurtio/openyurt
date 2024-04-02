/*
Copyright 2024 The OpenYurt Authors.

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

package poolservice

import (
	"context"
	"flag"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	netv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/poolservice/config"
)

func init() {
	flag.IntVar(&concurrentReconciles, "poolservice-workers", concurrentReconciles, "Max concurrent workers for PoolService controller.")
}

var (
	concurrentReconciles = 3
	controllerKind       = netv1alpha1.SchemeGroupVersion.WithKind("PoolService")
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.PoolServiceController, s)
}

// Add creates a new PoolService Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
	klog.Infof(Format("poolservice-controller add controller %s", controllerKind.String()))
	return add(mgr, newReconciler(c, mgr))
}

var _ reconcile.Reconciler = &ReconcilePoolService{}

// ReconcilePoolService reconciles a PoolService object
type ReconcilePoolService struct {
	client.Client
	scheme       *runtime.Scheme
	recorder     record.EventRecorder
	Configration config.PoolServiceControllerConfiguration
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *appconfig.CompletedConfig, mgr manager.Manager) reconcile.Reconciler {
	return &ReconcilePoolService{
		Client:       mgr.GetClient(),
		scheme:       mgr.GetScheme(),
		recorder:     mgr.GetEventRecorderFor(names.PoolServiceController),
		Configration: c.ComponentConfig.PoolServiceController,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(names.PoolServiceController, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles,
	})
	if err != nil {
		return err
	}

	// Watch for changes to PoolService
	err = c.Watch(&source.Kind{Type: &netv1alpha1.PoolService{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// +kubebuilder:rbac:groups=net.openyurt.io,resources=poolservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=net.openyurt.io,resources=poolservices/status,verbs=get;update;patch

// Reconcile reads that state of the cluster for a PoolService object and makes changes based on the state read
// and what is in the PoolService.Spec
func (r *ReconcilePoolService) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {

	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.Infof(Format("Reconcile PoolService %s/%s", request.Namespace, request.Name))

	// Fetch the PoolService instance
	instance := &netv1alpha1.PoolService{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if instance.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	// Update Status

	// Update Instance
	//if err = r.Update(context.TODO(), instance); err != nil {
	//	klog.Errorf(Format("Update PoolService %s error %v", klog.KObj(instance), err))
	//	return reconcile.Result{Requeue: true}, err
	//}

	return reconcile.Result{}, nil
}
