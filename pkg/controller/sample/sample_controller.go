/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package sample

import (
	"context"
	"flag"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appsv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	utilclient "github.com/openyurtio/openyurt/pkg/util/client"
	utildiscovery "github.com/openyurtio/openyurt/pkg/util/discovery"
)

func init() {
	flag.IntVar(&concurrentReconciles, "sample-workers", concurrentReconciles, "Max concurrent workers for Sample controller.")
}

var (
	concurrentReconciles = 3
	controllerKind       = appsv1beta1.SchemeGroupVersion.WithKind("Sample")
)

const (
	controllerName = "Sample-controller"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", controllerName, s)
}

// Add creates a new Sample Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) {
		return nil
	}
	return add(mgr, newReconciler(mgr))
}

var _ reconcile.Reconciler = &ReconcileSample{}

// ReconcileSample reconciles a Sample object
type ReconcileSample struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileSample{
		Client:   utilclient.NewClientFromManager(mgr, controllerName),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor(controllerName),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles,
	})
	if err != nil {
		return err
	}

	// Watch for changes to Sample
	err = c.Watch(&source.Kind{Type: &appsv1beta1.Sample{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// +kubebuilder:rbac:groups=apps.openyurt.io,resources=samples,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.openyurt.io,resources=samples/status,verbs=get;update;patch

// Reconcile reads that state of the cluster for a Sample object and makes changes based on the state read
// and what is in the Sample.Spec
func (r *ReconcileSample) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {

	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.Infof(Format("Reconcile Sample %s/%s", request.Namespace, request.Name))

	// Fetch the Sample instance
	instance := &appsv1beta1.Sample{}
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

	if instance.Spec.Foo != instance.Status.Foo {
		instance.Status.Foo = instance.Spec.Foo
		if err = r.Status().Update(context.TODO(), instance); err != nil {
			klog.Errorf(Format("Update Sample Status %s error %v", klog.KObj(instance), err))
			return reconcile.Result{Requeue: true}, err
		}
	}

	if err = r.Update(context.TODO(), instance); err != nil {
		klog.Errorf(Format("Update Sample %s error %v", klog.KObj(instance), err))
		return reconcile.Result{Requeue: true}, err
	}

	return reconcile.Result{}, nil
}
