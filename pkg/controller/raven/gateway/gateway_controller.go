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
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"reflect"
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
	calicov3 "github.com/projectcalico/api/pkg/apis/projectcalico/v3"
)

var (
	controllerKind = ravenv1alpha1.SchemeGroupVersion.WithKind("Gateway")
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", common.GatewayController, s)
}

const (
	ActiveEndpointsName      = "ActiveEndpointName"
	ActiveEndpointsPublicIP  = "ActiveEndpointsPublicIP"
	ActiveEndpointsProxyType = "ActiveEndpointsProxyType"
)

// Add creates a new Gateway Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(c *appconfig.CompletedConfig, mgr manager.Manager) error {
	if !utildiscovery.DiscoverGVK(controllerKind) {
		return nil
	}
	// init global variables
	cfg := c.ComponentConfig.Generic
	ravenv1alpha1.ServiceNamespacedName.Namespace = cfg.WorkingNamespace

	klog.Infof("raven-gateway-controller add controller %s", controllerKind.String())
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
		Client:       utilclient.NewClientFromManager(mgr, common.GatewayController),
		scheme:       mgr.GetScheme(),
		recorder:     mgr.GetEventRecorderFor(common.GatewayController),
		Configration: c.ComponentConfig.GatewayController,
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(common.GatewayController, mgr, controller.Options{
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
	klog.V(1).Info(Format("list gateway %d node %v", len(nodeList.Items), nodeList.Items))

	// 1. try to elect an active endpoint if possible
	activeEp := r.electActiveEndpoint(nodeList, &gw)
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
		for _, aep := range activeEp {
			aep.PublicIP = svc.Status.LoadBalancer.Ingress[0].IP
		}
	}
	r.recordEndpointEvent(ctx, &gw, gw.Status.ActiveEndpoints, activeEp)
	gw.Status.ActiveEndpoints = activeEp

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

func (r *ReconcileGateway) recordEndpointEvent(ctx context.Context, sourceObj *ravenv1alpha1.Gateway, previous, current []*ravenv1alpha1.Endpoint) {
	sort.Slice(previous, func(i, j int) bool { return previous[i].NodeName < previous[j].NodeName })
	sort.Slice(current, func(i, j int) bool { return current[i].NodeName < current[j].NodeName })
	if len(current) != 0 && !reflect.DeepEqual(previous, current) {
		eps := getActiveEndpointsInfo(current)
		r.recorder.Event(sourceObj.DeepCopy(), corev1.EventTypeNormal,
			ravenv1alpha1.EventActiveEndpointElected,
			fmt.Sprintf("The endpoint hosted by node %s has been elected active endpoint, publicIP: %s", eps[ActiveEndpointsName], eps[ActiveEndpointsPublicIP]))
		klog.V(2).InfoS(Format("elected new active endpoint"), "nodeName", eps[ActiveEndpointsName], "publicIP", eps[ActiveEndpointsPublicIP])
		return
	}
	if len(previous) != 0 && !reflect.DeepEqual(previous, current) {
		eps := getActiveEndpointsInfo(previous)
		r.recorder.Event(sourceObj.DeepCopy(), corev1.EventTypeWarning,
			ravenv1alpha1.EventActiveEndpointLost,
			fmt.Sprintf("The active endpoint hosted by node %s was change, publicIP: %s", eps[ActiveEndpointsName], eps[ActiveEndpointsPublicIP]))
		klog.V(2).InfoS(Format("active endpoint lost"), "nodeName", eps[ActiveEndpointsName], "publicIP", eps[ActiveEndpointsPublicIP])
		return
	}
}

// electActiveEndpoint trys to elect an active Endpoint.
// If the current active endpoint remains valid, then we don't change it.
// Otherwise, try to elect a new one.
func (r *ReconcileGateway) electActiveEndpoint(nodeList corev1.NodeList, gw *ravenv1alpha1.Gateway) []*ravenv1alpha1.Endpoint {
	// get all ready nodes referenced by endpoints
	readyNodes := make(map[string]*corev1.Node)
	for _, v := range nodeList.Items {
		if isNodeReady(v) {
			readyNodes[v.Name] = &v
		}
	}
	klog.V(1).Infof(Format("Ready node has %d, node %v", len(readyNodes), readyNodes))
	// init a endpoints slice
	eps := make([]*ravenv1alpha1.Endpoint, 0)
	serverProxyGateway := make([]*ravenv1alpha1.Endpoint, 0)
	networkProxyGateway := make([]*ravenv1alpha1.Endpoint, 0)
	if gw.Spec.EnableNetworkProxy {
		networkProxyGateway = electEndpoints(gw, ravenv1alpha1.NetworkProxy, readyNodes)
	}

	if gw.Spec.EnableServerProxy {
		serverProxyGateway = electEndpoints(gw, ravenv1alpha1.ServerProxy, readyNodes)
	}
	eps = append(eps, networkProxyGateway...)
	eps = append(eps, serverProxyGateway...)
	return eps
}

func electEndpoints(gw *ravenv1alpha1.Gateway, proxyType string, readyNodes map[string]*corev1.Node) []*ravenv1alpha1.Endpoint {
	eps := make([]*ravenv1alpha1.Endpoint, 0)
	replicas := 1
	if proxyType == ravenv1alpha1.ServerProxy {
		replicas = gw.Spec.Replicas
	}

	checkCandidates := func(ep *ravenv1alpha1.Endpoint) bool {
		if _, ok := readyNodes[ep.NodeName]; ok && ep.ProxyType == proxyType {
			return true
		}
		return false
	}

	// the current active endpoint is still competent.
	candidates := make(map[string]*ravenv1alpha1.Endpoint, 0)
	for _, activeEndpoint := range gw.Status.ActiveEndpoints {
		if checkCandidates(activeEndpoint) {
			candidates[activeEndpoint.NodeName] = activeEndpoint.DeepCopy()
		}
	}
	for _, aep := range candidates {
		if len(eps) == replicas {
			klog.V(4).InfoS(Format("elect %d active endpoints %s for gateway %s/%s",
				len(eps), fmt.Sprintf("[%s]", getActiveEndpointsInfo(eps)[ActiveEndpointsName]), gw.GetNamespace(), gw.GetName()))
			return eps
		}
		klog.V(1).Infof(Format("node %s is active endpoints, type is %s", aep.NodeName, aep.ProxyType))
		klog.V(1).Infof(Format("add node %v", aep.DeepCopy()))
		eps = append(eps, aep.DeepCopy())
	}

	for _, ep := range gw.Spec.Endpoints {
		if _, ok := candidates[ep.NodeName]; !ok && checkCandidates(&ep) {
			if len(eps) == replicas {
				klog.V(4).InfoS(Format("elect %d active endpoints %s for gateway %s/%s",
					len(eps), fmt.Sprintf("[%s]", getActiveEndpointsInfo(eps)[ActiveEndpointsName]), gw.GetNamespace(), gw.GetName()))
				return eps
			}
			klog.V(1).Infof(Format("node %s is active endpoints, type is %s", ep.NodeName, ep.ProxyType))
			klog.V(1).Infof(Format("add node %v", ep.DeepCopy()))
			eps = append(eps, ep.DeepCopy())
		}
	}
	return eps
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

func getActiveEndpointsInfo(eps []*ravenv1alpha1.Endpoint) map[string]string {
	infos := make(map[string]string)
	infos[ActiveEndpointsName] = ""
	infos[ActiveEndpointsPublicIP] = ""
	if len(eps) == 0 {
		return infos
	}
	names := make([]string, 0)
	publicIPs := make([]string, 0)
	proxyTypes := make([]string, 0)
	for _, ep := range eps {
		names = append(names, ep.NodeName)
		publicIPs = append(publicIPs, ep.PublicIP)
		proxyTypes = append(proxyTypes, ep.ProxyType)
	}
	infos[ActiveEndpointsName] = fmt.Sprintf("[%s]", strings.Join(names, ","))
	infos[ActiveEndpointsPublicIP] = fmt.Sprintf("[%s]", strings.Join(publicIPs, ","))
	infos[ActiveEndpointsProxyType] = fmt.Sprintf("[%s]", strings.Join(proxyTypes, ","))
	return infos
}
