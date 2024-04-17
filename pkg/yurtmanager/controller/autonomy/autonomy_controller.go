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

package autonomy

import (
	"context"
	"fmt"
	"reflect"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	appsv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

var (
	controllerResource = v1.SchemeGroupVersion.WithKind("Node")
)

type ReconcileAutonomy struct {
	client.Client
}

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.AutonomyController, s)
}

func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
	klog.Infof(Format("autonomy-controller add controller %s", controllerResource.String()))
	return add(mgr, c, newReconciler(c, mgr))
}

func newReconciler(_ *appconfig.CompletedConfig, mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileAutonomy{Client: mgr.GetClient()}
}

func add(mgr manager.Manager, cfg *appconfig.CompletedConfig, r reconcile.Reconciler) error {
	c, err := controller.New(names.AutonomyController, mgr, controller.Options{
		Reconciler:              r,
		MaxConcurrentReconciles: int(cfg.Config.ComponentConfig.AutonomyController.ConcurrentAutonomyWorkers),
	})
	if err != nil {
		return err
	}

	nodePredicate := prepare()
	err = c.Watch(source.Kind(mgr.GetCache(), &v1.Node{}), &handler.EnqueueRequestForObject{}, nodePredicate)
	if err != nil {
		return err
	}
	return nil
}

func (r *ReconcileAutonomy) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	klog.V(2).Infof("Reconcile Node %s/%s Start.", req.Namespace, req.Name)
	var node v1.Node
	err := r.Client.Get(ctx, req.NamespacedName, &node)
	if err != nil {
		return reconcile.Result{}, err
	}
	for _, condition := range node.Status.Conditions {
		if condition.Type == appsv1beta1.NodeAutonomy {
			if node.Labels == nil {
				node.Labels = make(map[string]string)
			}
			if condition.Status == v1.ConditionTrue {
				if node.Labels[projectinfo.GetAutonomyStatusLabel()] == "true" {
					return reconcile.Result{}, nil
				}
				node.Labels[projectinfo.GetAutonomyStatusLabel()] = "true"
			} else if condition.Status == v1.ConditionFalse {
				if node.Labels[projectinfo.GetAutonomyStatusLabel()] == "false" {
					return reconcile.Result{}, nil
				}
				node.Labels[projectinfo.GetAutonomyStatusLabel()] = "false"
			} else if condition.Status == v1.ConditionUnknown {
				if node.Labels[projectinfo.GetAutonomyStatusLabel()] == "unknown" {
					return reconcile.Result{}, nil
				}
				node.Labels[projectinfo.GetAutonomyStatusLabel()] = "unknown"
			}
		}
	}
	if err := r.Client.Update(ctx, &node); err != nil {
		klog.Errorf("failed to update node %s/%s, err: %s", node.Namespace, node.Name, err)
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func prepare() predicate.Funcs {
	nodePredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			node := e.Object.(*v1.Node)
			if node.GetLabels()[projectinfo.GetEdgeWorkerLabelKey()] == "true" && node.GetAnnotations()[projectinfo.GetAutonomyAnnotation()] == "true" {
				return true
			}
			return false
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldNode := e.ObjectOld.(*v1.Node)
			newNode := e.ObjectNew.(*v1.Node)
			if reflect.DeepEqual(oldNode.Annotations, newNode.Annotations) {
				return false
			}
			if newNode.GetLabels()[projectinfo.GetEdgeWorkerLabelKey()] == "true" && newNode.GetAnnotations()[projectinfo.GetAutonomyAnnotation()] == "true" {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			return false
		},
	}
	return nodePredicate
}
