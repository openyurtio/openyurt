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

package service

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	ravenv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/raven/v1alpha1"
	common "github.com/openyurtio/openyurt/pkg/controller/raven"
	"github.com/openyurtio/openyurt/pkg/controller/raven/utils"
	utilclient "github.com/openyurtio/openyurt/pkg/util/client"
	utildiscovery "github.com/openyurtio/openyurt/pkg/util/discovery"
)

var (
	controllerKind = corev1.SchemeGroupVersion.WithKind("Service")
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s-service: %s", common.ServiceController, s)
}

// Add creates a new Service Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(c *appconfig.CompletedConfig, mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) {
		return nil
	}

	klog.Infof("raven-service-controller add controller %s", controllerKind.String())
	return add(mgr, newReconciler(c, mgr))
}

var _ reconcile.Reconciler = &ReconcileService{}

// ReconcileService reconciles a Gateway object
type ReconcileService struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *appconfig.CompletedConfig, mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileService{
		Client:   utilclient.NewClientFromManager(mgr, common.ServiceController),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor(common.ServiceController),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(common.ServiceController, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: common.ConcurrentReconciles,
	})
	if err != nil {
		return err
	}

	// Watch for changes to Service
	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, handler.EnqueueRequestsFromMapFunc(mapServiceToRequest))
	if err != nil {
		return err
	}
	// Watch for changes to Gateway
	err = c.Watch(&source.Kind{Type: &ravenv1alpha1.Gateway{}}, handler.EnqueueRequestsFromMapFunc(mapGatewayToRequest))
	if err != nil {
		return err
	}

	// Watch for changes to Endpoints
	err = c.Watch(&source.Kind{Type: &corev1.Endpoints{}}, handler.EnqueueRequestsFromMapFunc(mapEndpointToRequest))
	if err != nil {
		return err
	}

	return nil
}

// Reconcile reads that state of the cluster for a Gateway object and makes changes based on the state read
// and what is in the Gateway.Spec
func (r *ReconcileService) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	klog.V(4).Info(Format("started reconciling Service %s/%s", req.Namespace, req.Name))
	defer func() {
		klog.V(4).Info(Format("finished reconciling Service %s/%s", req.Namespace, req.Name))
	}()

	var gatewayList ravenv1alpha1.GatewayList
	if err := r.List(ctx, &gatewayList); err != nil {
		err = fmt.Errorf(Format("unable to list gateways: %s", err))
		return reconcile.Result{}, err
	}

	if err := r.reconcileService(ctx, req, &gatewayList); err != nil {
		err = fmt.Errorf(Format("unable to reconcile service: %s", err))
		return reconcile.Result{}, err
	}

	if err := r.reconcileEndpoint(ctx, req, &gatewayList); err != nil {
		err = fmt.Errorf(Format("unable to reconcile endpoint: %s", err))
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileService) reconcileService(ctx context.Context, req ctrl.Request, gatewayList *ravenv1alpha1.GatewayList) error {
	for _, gw := range gatewayList.Items {
		if utils.IsGatewayExposeByLB(&gw) {
			return r.ensureService(ctx, req)
		}
	}
	return r.cleanService(ctx, req)
}

func (r *ReconcileService) cleanService(ctx context.Context, req ctrl.Request) error {
	if err := r.Delete(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
	}); err != nil && !apierrs.IsNotFound(err) {
		return err
	}
	return nil
}

func (r *ReconcileService) ensureService(ctx context.Context, req ctrl.Request) error {
	var service corev1.Service
	if err := r.Get(ctx, req.NamespacedName, &service); err != nil {
		if apierrs.IsNotFound(err) {
			klog.V(2).InfoS(Format("create service"), "name", req.Name, "namespace", req.Namespace)
			return r.Create(ctx, &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      req.Name,
					Namespace: req.Namespace,
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:       4500,
							Protocol:   corev1.ProtocolUDP,
							TargetPort: intstr.FromInt(4500),
						},
					},
					Type:                  corev1.ServiceTypeLoadBalancer,
					ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeLocal,
				},
			})

		}
	}
	return nil
}

func (r *ReconcileService) reconcileEndpoint(ctx context.Context, req ctrl.Request, gatewayList *ravenv1alpha1.GatewayList) error {
	exposedByLB := false
	for _, gw := range gatewayList.Items {
		if utils.IsGatewayExposeByLB(&gw) {
			exposedByLB = true
			if len(gw.Status.ActiveEndpoints) != 0 {
				nodes := make([]*corev1.Node, 0)
				for _, ep := range gw.Status.ActiveEndpoints {
					var node corev1.Node
					err := r.Get(ctx, types.NamespacedName{Name: ep.NodeName}, &node)
					if err != nil {
						return err
					}
					nodes = append(nodes, &node)
				}
				return r.ensureEndpoint(ctx, req, nodes)
			}
		}
	}

	if !exposedByLB {
		return r.cleanEndpoint(ctx, req)
	}

	return nil
}

func (r *ReconcileService) cleanEndpoint(ctx context.Context, req ctrl.Request) error {
	if err := r.Delete(ctx, &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
	}); err != nil && !apierrs.IsNotFound(err) {
		return err
	}
	return nil
}

func (r *ReconcileService) ensureEndpoint(ctx context.Context, req ctrl.Request, nodes []*corev1.Node) error {
	addresses := make([]corev1.EndpointAddress, 0)
	for _, node := range nodes {
		addresses = append(addresses, corev1.EndpointAddress{
			IP:       utils.GetNodeInternalIP(*node),
			NodeName: func(n corev1.Node) *string { return &n.Name }(*node),
		})
	}

	var serviceEndpoint corev1.Endpoints
	newSubnets := []corev1.EndpointSubset{
		{
			Addresses: addresses,
			Ports: []corev1.EndpointPort{
				{
					Port:     4500,
					Protocol: corev1.ProtocolUDP,
				},
			},
		},
	}
	if err := r.Get(ctx, req.NamespacedName, &serviceEndpoint); err != nil {
		if apierrs.IsNotFound(err) {
			klog.V(2).InfoS(Format("create endpoint"), "name", req.Name, "namespace", req.Namespace)
			return r.Create(ctx, &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      req.Name,
					Namespace: req.Namespace,
				},
				Subsets: newSubnets,
			})
		}
		return err
	}

	if !reflect.DeepEqual(serviceEndpoint.Subsets, newSubnets) {
		klog.V(2).InfoS(Format("update endpoint"), "name", req.Name, "namespace", req.Namespace)
		serviceEndpoint.Subsets = newSubnets
		return r.Update(ctx, &serviceEndpoint)
	}
	klog.V(2).InfoS(Format("skip to update endpoint"), "name", req.Name, "namespace", req.Namespace)
	return nil
}

// mapGatewayToRequest maps the given Gateway object to reconcile.Request.
func mapGatewayToRequest(object client.Object) []reconcile.Request {
	return []reconcile.Request{}
}

// mapEndpointToRequest maps the given Endpoint object to reconcile.Request.
func mapEndpointToRequest(object client.Object) []reconcile.Request {
	return []reconcile.Request{}
}

// mapEndpointToRequest maps the given Endpoint object to reconcile.Request.
func mapServiceToRequest(object client.Object) []reconcile.Request {
	return []reconcile.Request{}
}
