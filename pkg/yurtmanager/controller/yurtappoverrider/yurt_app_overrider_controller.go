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

package yurtappoverrider

import (
	"context"
	"fmt"
	"reflect"
	"time"

	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	yurtClient "github.com/openyurtio/openyurt/cmd/yurt-manager/app/client"
	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtappoverrider/config"
)

var (
	controllerResource = appsv1alpha1.SchemeGroupVersion.WithResource("yurtappoverriders")
)

const (
	ControllerName = "yurtappoverrider"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", ControllerName, s)
}

// Add creates a new YurtAppOverrider Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
	if _, err := mgr.GetRESTMapper().KindFor(controllerResource); err != nil {
		klog.Infof("resource %s doesn't exist", controllerResource.String())
		return err
	}

	return add(mgr, c, newReconciler(c, mgr))
}

var _ reconcile.Reconciler = &ReconcileYurtAppOverrider{}

// ReconcileYurtAppOverrider reconciles a YurtAppOverrider object
type ReconcileYurtAppOverrider struct {
	client.Client
	Configuration     config.YurtAppOverriderControllerConfiguration
	CacheOverriderMap map[string]*appsv1alpha1.YurtAppOverrider
	recorder          record.EventRecorder
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *appconfig.CompletedConfig, mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileYurtAppOverrider{
		Client:            yurtClient.GetClientByControllerNameOrDie(mgr, names.YurtAppOverriderController),
		Configuration:     c.ComponentConfig.YurtAppOverriderController,
		CacheOverriderMap: make(map[string]*appsv1alpha1.YurtAppOverrider),
		recorder:          mgr.GetEventRecorderFor(names.YurtAppOverriderController),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, cfg *appconfig.CompletedConfig, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(ControllerName, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: int(cfg.ComponentConfig.YurtAppOverriderController.ConcurrentYurtAppOverriderWorkers),
	})
	if err != nil {
		return err
	}

	// Watch for changes to YurtAppOverrider
	err = c.Watch(source.Kind[client.Object](mgr.GetCache(), &appsv1alpha1.YurtAppOverrider{}, &handler.EnqueueRequestForObject{}))
	if err != nil {
		return err
	}

	return nil
}

// +kubebuilder:rbac:groups=apps.openyurt.io,resources=yurtappoverriders,verbs=get
// +kubebuilder:rbac:groups=apps.openyurt.io,resources=yurtappsets,verbs=get
// +kubebuilder:rbac:groups=apps.openyurt.io,resources=yurtappdaemons,verbs=get
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=update

// Reconcile reads that state of the cluster for a YurtAppOverrider object and makes changes based on the state read
// and what is in the YurtAppOverrider.Spec
func (r *ReconcileYurtAppOverrider) Reconcile(_ context.Context, request reconcile.Request) (reconcile.Result, error) {

	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.Info(Format("Reconcile YurtAppOverrider %s/%s", request.Namespace, request.Name))

	// Fetch the YurtAppOverrider instance
	instance := &appsv1alpha1.YurtAppOverrider{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			delete(r.CacheOverriderMap, request.Namespace+"/"+request.Name)
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	if instance.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	switch instance.Subject.Kind {
	case "YurtAppSet":
		appSet := &appsv1alpha1.YurtAppSet{}
		if err := r.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Subject.Name}, appSet); err != nil {
			return reconcile.Result{}, err
		}
		if appSet.Spec.WorkloadTemplate.StatefulSetTemplate != nil {
			r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("unable to override statefulset workload of %s", appSet.Name), "It is not supported to overrider statefulset now")
			return reconcile.Result{}, nil
		}
	case "YurtAppDaemon":
		appDaemon := &appsv1alpha1.YurtAppDaemon{}
		if err := r.Get(context.TODO(), client.ObjectKey{Namespace: instance.Namespace, Name: instance.Subject.Name}, appDaemon); err != nil {
			return reconcile.Result{}, err
		}
		if appDaemon.Spec.WorkloadTemplate.StatefulSetTemplate != nil {
			r.recorder.Event(instance.DeepCopy(), corev1.EventTypeWarning, fmt.Sprintf("unable to override statefulset workload of %s", appDaemon.Name), "It is not supported to overrider statefulset now")
			return reconcile.Result{}, nil
		}
	default:
		return reconcile.Result{}, fmt.Errorf("unsupported kind: %s", instance.Subject.Kind)
	}

	if cacheOverrider, ok := r.CacheOverriderMap[instance.Namespace+"/"+instance.Name]; ok {
		if reflect.DeepEqual(cacheOverrider.Entries, instance.Entries) {
			return reconcile.Result{}, nil
		}
	}
	r.CacheOverriderMap[instance.Namespace+"/"+instance.Name] = instance.DeepCopy()

	deployments := v1.DeploymentList{}
	if err := r.List(context.TODO(), &deployments); err != nil {
		return reconcile.Result{}, err
	}

	for _, deployment := range deployments.Items {
		if len(deployment.OwnerReferences) != 0 {
			if deployment.OwnerReferences[0].Kind == instance.Subject.Kind && deployment.OwnerReferences[0].Name == instance.Subject.Name {
				if deployment.Annotations == nil {
					deployment.Annotations = make(map[string]string)
				}
				deployment.Annotations["LastOverrideTime"] = time.Now().String()
				if err := r.Client.Update(context.TODO(), &deployment); err != nil {
					return reconcile.Result{}, err
				}
			}
		}
	}

	return reconcile.Result{}, nil
}
