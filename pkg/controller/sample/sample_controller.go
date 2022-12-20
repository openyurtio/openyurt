/*
Copyright 2022.

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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	appsv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
)

// SampleReconciler reconciles a Sample object
type SampleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=apps.openyurt.io,resources=samples,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps.openyurt.io,resources=samples/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps.openyurt.io,resources=samples/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Sample object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *SampleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}

func Add(mgr ctrl.Manager, ctx context.Context) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&appsv1beta1.Sample{}).
		Complete(&SampleReconciler{
			Client: mgr.GetClient(),
			Scheme: mgr.GetScheme(),
		})
}

