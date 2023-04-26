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

package gateway

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	calicov3 "github.com/projectcalico/api/pkg/apis/projectcalico/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
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
	"github.com/openyurtio/openyurt/pkg/apis/raven"
	ravenv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/raven/v1alpha1"
	common "github.com/openyurtio/openyurt/pkg/controller/raven"
	"github.com/openyurtio/openyurt/pkg/controller/raven/config"
	"github.com/openyurtio/openyurt/pkg/controller/raven/utils"
	nodeutil "github.com/openyurtio/openyurt/pkg/controller/util/node"
	utilclient "github.com/openyurtio/openyurt/pkg/util/client"
	utildiscovery "github.com/openyurtio/openyurt/pkg/util/discovery"
)

var (
	controllerKind = ravenv1alpha1.SchemeGroupVersion.WithKind("Gateway")
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s-gateway: %s", common.ControllerName, s)
}

// Add creates a new Gateway Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(c *appconfig.CompletedConfig, mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) {
		return nil
	}
	// init global variables
	cfg := c.ComponentConfig.Generic
	ravenv1alpha1.ServiceNamespacedName.Namespace = cfg.WorkingNamespace

	return add(mgr, newReconciler(c, mgr))
}

var _ reconcile.Reconciler = &ReconcileGateway{}

// ReconcileGateway reconciles a Gateway object
type ReconcileGateway struct {
	client.Client
	scheme       *runtime.Scheme
	recorder     record.EventRecorder
	Configration config.GatewayControllerConfiguration
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *appconfig.CompletedConfig, mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileGateway{
		Client:       utilclient.NewClientFromManager(mgr, common.ControllerName),
		scheme:       mgr.GetScheme(),
		recorder:     mgr.GetEventRecorderFor(common.ControllerName),
		Configration: c.ComponentConfig.GatewayController,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(fmt.Sprintf("%s-gateway", common.ControllerName), mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: common.ConcurrentReconciles,
	})
	if err != nil {
		return err
	}

	// Watch for changes to Gateway
	err = c.Watch(&source.Kind{Type: &ravenv1alpha1.Gateway{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to Nodes
	err = c.Watch(&source.Kind{Type: &corev1.Node{}}, &EnqueueGatewayForNode{})
	if err != nil {
		return err
	}

	return nil
}

//+kubebuilder:rbac:groups=raven.openyurt.io,resources=gateways,verbs=get;list;watch;
//+kubebuilder:rbac:groups=raven.openyurt.io,resources=gateways/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=raven.openyurt.io,resources=gateways/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=endpoints,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=crd.projectcalico.org,resources=blockaffinities,verbs=get;list;watch

// Reconcile reads that state of the cluster for a Gateway object and makes changes based on the state read
// and what is in the Gateway.Spec
func (r *ReconcileGateway) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {

	// Note !!!!!!!!!!
	// We strongly recommend use Format() to  encapsulation because Format() can print logs by module
	// @kadisi
	klog.V(4).Info(Format("started reconciling Gateway %s/%s", req.Namespace, req.Name))
	defer func() {
		klog.V(4).Info(Format("finished reconciling Gateway %s/%s", req.Namespace, req.Name))
	}()

	var gw ravenv1alpha1.Gateway
	if err := r.Get(ctx, req.NamespacedName, &gw); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// get all managed nodes
	var nodeList corev1.NodeList
	nodeSelector, err := labels.Parse(fmt.Sprintf(raven.LabelCurrentGateway+"=%s", gw.Name))
	if err != nil {
		return reconcile.Result{}, err
	}
	err = r.List(ctx, &nodeList, &client.ListOptions{
		LabelSelector: nodeSelector,
	})
	if err != nil {
		err = fmt.Errorf("unable to list nodes: %s", err)
		return reconcile.Result{}, err
	}

	// 1. try to elect an active endpoint if possible
	activeEp := r.electActiveEndpoint(nodeList, &gw)
	r.recordEndpointEvent(ctx, &gw, gw.Status.ActiveEndpoint, activeEp)
	if utils.IsGatewayExposeByLB(&gw) {
		var svc corev1.Service
		if err := r.Get(ctx, ravenv1alpha1.ServiceNamespacedName, &svc); err != nil {
			klog.V(2).Info(Format("waiting for service sync, error: %s", err))
			return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
		}
		if len(svc.Status.LoadBalancer.Ingress) == 0 {
			klog.V(2).Info("waiting for LB ingress sync")
			return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
		}
		activeEp.PublicIP = svc.Status.LoadBalancer.Ingress[0].IP
	}
	gw.Status.ActiveEndpoint = activeEp

	// 2. get nodeInfo list of nodes managed by the Gateway
	var nodes []ravenv1alpha1.NodeInfo
	for _, v := range nodeList.Items {
		podCIDRs, err := r.getPodCIDRs(ctx, v)
		if err != nil {
			klog.ErrorS(err, "unable to get podCIDR")
			return reconcile.Result{}, err
		}
		nodes = append(nodes, ravenv1alpha1.NodeInfo{
			NodeName:  v.Name,
			PrivateIP: utils.GetNodeInternalIP(v),
			Subnets:   podCIDRs,
		})
	}
	klog.V(4).Info(Format("managed node info list, nodes: %v", nodes))
	gw.Status.Nodes = nodes

	err = r.Status().Update(ctx, &gw)
	if err != nil {
		klog.V(4).ErrorS(err, Format("unable to Update Gateway.status"))
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileGateway) recordEndpointEvent(ctx context.Context, sourceObj *ravenv1alpha1.Gateway, previous, current *ravenv1alpha1.Endpoint) {
	if current != nil && !reflect.DeepEqual(previous, current) {
		r.recorder.Event(sourceObj.DeepCopy(), corev1.EventTypeNormal,
			ravenv1alpha1.EventActiveEndpointElected,
			fmt.Sprintf("The endpoint hosted by node %s has been elected active endpoint, publicIP: %s", current.NodeName, current.PublicIP))
		klog.V(2).InfoS(Format("elected new active endpoint"), "nodeName", current.NodeName, "publicIP", current.PublicIP)
		return
	}
	if current == nil && previous != nil {
		r.recorder.Event(sourceObj.DeepCopy(), corev1.EventTypeWarning,
			ravenv1alpha1.EventActiveEndpointLost,
			fmt.Sprintf("The active endpoint hosted by node %s was lost, publicIP: %s", previous.NodeName, previous.PublicIP))
		klog.V(2).InfoS(Format("active endpoint lost"), "nodeName", previous.NodeName, "publicIP", previous.PublicIP)
		return
	}
}

// electActiveEndpoint trys to elect an active Endpoint.
// If the current active endpoint remains valid, then we don't change it.
// Otherwise, try to elect a new one.
func (r *ReconcileGateway) electActiveEndpoint(nodeList corev1.NodeList, gw *ravenv1alpha1.Gateway) (ep *ravenv1alpha1.Endpoint) {
	// get all ready nodes referenced by endpoints
	readyNodes := make(map[string]corev1.Node)
	for _, v := range nodeList.Items {
		if isNodeReady(v) {
			readyNodes[v.Name] = v
		}
	}
	// checkActive check if the given endpoint is able to become the active endpoint.
	checkActive := func(ep *ravenv1alpha1.Endpoint) bool {
		if ep == nil {
			return false
		}
		// check if the node status is ready
		if _, ok := readyNodes[ep.NodeName]; ok {
			var inList bool
			// check if ep is in the Endpoint list
			for _, v := range gw.Spec.Endpoints {
				if reflect.DeepEqual(v, *ep) {
					inList = true
					break
				}
			}
			return inList
		}
		return false
	}

	// the current active endpoint is still competent.
	if checkActive(gw.Status.ActiveEndpoint) {
		for _, v := range gw.Spec.Endpoints {
			if v.NodeName == gw.Status.ActiveEndpoint.NodeName {
				return v.DeepCopy()
			}
		}
	}

	// try to elect an active endpoint.
	for _, v := range gw.Spec.Endpoints {
		if checkActive(&v) {
			return v.DeepCopy()
		}
	}
	return
}

// isNodeReady checks if the `node` is `corev1.NodeReady`
func isNodeReady(node corev1.Node) bool {
	_, nc := nodeutil.GetNodeCondition(&node.Status, corev1.NodeReady)
	// GetNodeCondition will return nil and -1 if the condition is not present
	return nc != nil && nc.Status == corev1.ConditionTrue
}

// getPodCIDRs returns the pod IP ranges assigned to the node.
func (r *ReconcileGateway) getPodCIDRs(ctx context.Context, node corev1.Node) ([]string, error) {
	podCIDRs := make([]string, 0)
	for key := range node.Annotations {
		if strings.Contains(key, "projectcalico.org") {
			var blockAffinityList calicov3.BlockAffinityList
			err := r.List(ctx, &blockAffinityList)
			if err != nil {
				err = fmt.Errorf(Format("unable to list calico blockaffinity: %s", err))
				return nil, err
			}
			for _, v := range blockAffinityList.Items {
				if v.Spec.Node != node.Name || v.Spec.State != "confirmed" {
					continue
				}
				podCIDRs = append(podCIDRs, v.Spec.CIDR)
			}
			return podCIDRs, nil
		}
	}
	return append(podCIDRs, node.Spec.PodCIDR), nil
}
