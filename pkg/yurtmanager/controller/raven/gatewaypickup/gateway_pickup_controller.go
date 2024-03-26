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

package gatewaypickup

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	calicov3 "github.com/openyurtio/openyurt/pkg/apis/calico/v3"
	"github.com/openyurtio/openyurt/pkg/apis/raven"
	ravenv1beta1 "github.com/openyurtio/openyurt/pkg/apis/raven/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/gatewaypickup/config"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
	nodeutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/node"
)

var (
	controllerResource = ravenv1beta1.SchemeGroupVersion.WithResource("gateways")
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.GatewayPickupController, s)
}

const (
	ActiveEndpointsName     = "ActiveEndpointName"
	ActiveEndpointsPublicIP = "ActiveEndpointsPublicIP"
	ActiveEndpointsType     = "ActiveEndpointsType"
)

// Add creates a new Gateway Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
	if _, err := mgr.GetRESTMapper().KindFor(controllerResource); err != nil {
		klog.Infof("resource %s doesn't exist", controllerResource.String())
		return err
	}
	klog.Infof("raven-gateway-controller add controller %s", controllerResource.String())
	return add(mgr, newReconciler(c, mgr))
}

var _ reconcile.Reconciler = &ReconcileGateway{}

// ReconcileGateway reconciles a Gateway object
type ReconcileGateway struct {
	client.Client
	scheme       *runtime.Scheme
	recorder     record.EventRecorder
	Configration config.GatewayPickupControllerConfiguration
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(c *appconfig.CompletedConfig, mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileGateway{
		Client:       mgr.GetClient(),
		scheme:       mgr.GetScheme(),
		recorder:     mgr.GetEventRecorderFor(names.GatewayPickupController),
		Configration: c.ComponentConfig.GatewayPickupController,
	}
}

// add is used to add a new Controller to mgr
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(names.GatewayPickupController, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: util.ConcurrentReconciles,
	})
	if err != nil {
		return err
	}

	// Watch for changes to Gateway
	err = c.Watch(&source.Kind{Type: &ravenv1beta1.Gateway{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	// Watch for changes to Nodes
	err = c.Watch(&source.Kind{Type: &corev1.Node{}}, &EnqueueGatewayForNode{})
	if err != nil {
		return err
	}

	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &EnqueueGatewayForRavenConfig{client: mgr.GetClient()}, predicate.NewPredicateFuncs(
		func(object client.Object) bool {
			cm, ok := object.(*corev1.ConfigMap)
			if !ok {
				return false
			}
			if cm.GetNamespace() != util.WorkingNamespace {
				return false
			}
			if cm.GetName() != util.RavenGlobalConfig {
				return false
			}
			return true
		}))
	if err != nil {
		return err
	}
	return nil
}

//+kubebuilder:rbac:groups=raven.openyurt.io,resources=gateways,verbs=get;list;watch;create;delete;update
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

	var gw ravenv1beta1.Gateway
	if err := r.Get(ctx, req.NamespacedName, &gw); err != nil {
		klog.Error(Format("unable get gateway %s, error %s", req.String(), err.Error()))
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// get all managed nodes
	var nodeList corev1.NodeList
	nodeSelector, err := labels.Parse(fmt.Sprintf(raven.LabelCurrentGateway+"=%s", gw.Name))
	if err != nil {
		return reconcile.Result{}, err
	}
	err = r.List(ctx, &nodeList, &client.ListOptions{LabelSelector: nodeSelector})
	if err != nil {
		klog.Error(Format("unable to list node error %s", err.Error()))
		return reconcile.Result{}, err
	}

	// 1. try to elect an active endpoint if possible
	activeEp := r.electActiveEndpoint(nodeList, &gw)
	r.recordEndpointEvent(&gw, gw.Status.ActiveEndpoints, activeEp)
	gw.Status.ActiveEndpoints = activeEp
	r.configEndpoints(ctx, &gw)
	// 2. get nodeInfo list of nodes managed by the Gateway
	var nodes []ravenv1beta1.NodeInfo
	for _, v := range nodeList.Items {
		podCIDRs, err := r.getPodCIDRs(ctx, v)
		if err != nil {
			klog.Error(Format("unable to get podCIDR for node %s error %s", v.GetName(), err.Error()))
			return reconcile.Result{}, err
		}
		nodes = append(nodes, ravenv1beta1.NodeInfo{
			NodeName:  v.Name,
			PrivateIP: util.GetNodeInternalIP(v),
			Subnets:   podCIDRs,
		})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].NodeName < nodes[j].NodeName })
	gw.Status.Nodes = nodes
	r.addExtraAllowedSubnet(&gw)
	err = r.Status().Update(ctx, &gw)
	if err != nil {
		if apierrs.IsConflict(err) {
			klog.Warning(err, Format("unable to update gateway.status, error %s", err.Error()))
			return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, nil
		}
		klog.Error(Format("unable to update %s gateway.status, error %s", gw.GetName(), err.Error()))
		return reconcile.Result{Requeue: true, RequeueAfter: 5 * time.Second}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileGateway) recordEndpointEvent(sourceObj *ravenv1beta1.Gateway, previous, current []*ravenv1beta1.Endpoint) {
	sort.Slice(previous, func(i, j int) bool { return previous[i].NodeName < previous[j].NodeName })
	sort.Slice(current, func(i, j int) bool { return current[i].NodeName < current[j].NodeName })
	if len(current) != 0 && !reflect.DeepEqual(previous, current) {
		eps, num := getActiveEndpointsInfo(current)
		for i := 0; i < num; i++ {
			r.recorder.Event(sourceObj.DeepCopy(), corev1.EventTypeNormal,
				ravenv1beta1.EventActiveEndpointElected,
				fmt.Sprintf("The endpoint hosted by node %s has been elected active endpoint, publicIP: %s, type: %s", eps[ActiveEndpointsName][i], eps[ActiveEndpointsPublicIP][i], eps[ActiveEndpointsType][i]))
		}

		klog.V(2).InfoS(Format("elected new active endpoint"), "nodeName", eps[ActiveEndpointsName], "publicIP", eps[ActiveEndpointsPublicIP], "type", eps[ActiveEndpointsType])
		return
	}
	if len(previous) != 0 && !reflect.DeepEqual(previous, current) {
		eps, num := getActiveEndpointsInfo(previous)
		for i := 0; i < num; i++ {
			r.recorder.Event(sourceObj.DeepCopy(), corev1.EventTypeWarning,
				ravenv1beta1.EventActiveEndpointLost,
				fmt.Sprintf("The active endpoint hosted by node %s was change, publicIP: %s, type :%s", eps[ActiveEndpointsName][i], eps[ActiveEndpointsPublicIP][i], eps[ActiveEndpointsType][i]))
		}
		klog.V(2).InfoS(Format("active endpoint lost"), "nodeName", eps[ActiveEndpointsName], "publicIP", eps[ActiveEndpointsPublicIP], "type", eps[ActiveEndpointsType])
		return
	}
}

// electActiveEndpoint trys to elect an active Endpoint.
// If the current active endpoint remains valid, then we don't change it.
// Otherwise, try to elect a new one.
func (r *ReconcileGateway) electActiveEndpoint(nodeList corev1.NodeList, gw *ravenv1beta1.Gateway) []*ravenv1beta1.Endpoint {
	// get all ready nodes referenced by endpoints
	readyNodes := make(map[string]*corev1.Node)
	for _, v := range nodeList.Items {
		if isNodeReady(v) {
			readyNodes[v.Name] = &v
		}
	}
	klog.V(1).Infof(Format("Ready node has %d, node %v", len(readyNodes), readyNodes))
	// init a endpoints slice
	enableProxy, enableTunnel := util.CheckServer(context.TODO(), r.Client)
	eps := make([]*ravenv1beta1.Endpoint, 0)
	if enableProxy {
		eps = append(eps, electEndpoints(gw, ravenv1beta1.Proxy, readyNodes)...)
	}
	if enableTunnel {
		eps = append(eps, electEndpoints(gw, ravenv1beta1.Tunnel, readyNodes)...)
	}
	sort.Slice(eps, func(i, j int) bool { return eps[i].NodeName < eps[j].NodeName })
	return eps
}

func electEndpoints(gw *ravenv1beta1.Gateway, endpointType string, readyNodes map[string]*corev1.Node) []*ravenv1beta1.Endpoint {
	eps := make([]*ravenv1beta1.Endpoint, 0)
	var replicas int
	switch endpointType {
	case ravenv1beta1.Proxy:
		replicas = gw.Spec.ProxyConfig.Replicas
	case ravenv1beta1.Tunnel:
		replicas = gw.Spec.TunnelConfig.Replicas
	default:
		replicas = 1
	}

	checkCandidates := func(ep *ravenv1beta1.Endpoint) bool {
		if _, ok := readyNodes[ep.NodeName]; ok && ep.Type == endpointType {
			return true
		}
		return false
	}

	// the current active endpoint is still competent.
	candidates := make(map[string]*ravenv1beta1.Endpoint, 0)
	for _, activeEndpoint := range gw.Status.ActiveEndpoints {
		if checkCandidates(activeEndpoint) {
			for _, ep := range gw.Spec.Endpoints {
				if ep.NodeName == activeEndpoint.NodeName && ep.Type == activeEndpoint.Type {
					candidates[activeEndpoint.NodeName] = ep.DeepCopy()
				}
			}
		}
	}
	for _, aep := range candidates {
		if len(eps) == replicas {
			aepInfo, _ := getActiveEndpointsInfo(eps)
			klog.V(4).InfoS(Format("elect %d active endpoints %s for gateway %s/%s",
				len(eps), fmt.Sprintf("[%s]", strings.Join(aepInfo[ActiveEndpointsName], ",")), gw.GetNamespace(), gw.GetName()))
			return eps
		}
		klog.V(1).Infof(Format("node %s is active endpoints, type is %s", aep.NodeName, aep.Type))
		klog.V(1).Infof(Format("add node %v", aep.DeepCopy()))
		eps = append(eps, aep.DeepCopy())
	}

	for _, ep := range gw.Spec.Endpoints {
		if _, ok := candidates[ep.NodeName]; !ok && checkCandidates(&ep) {
			if len(eps) == replicas {
				aepInfo, _ := getActiveEndpointsInfo(eps)
				klog.V(4).InfoS(Format("elect %d active endpoints %s for gateway %s/%s",
					len(eps), fmt.Sprintf("[%s]", strings.Join(aepInfo[ActiveEndpointsName], ",")), gw.GetNamespace(), gw.GetName()))
				return eps
			}
			klog.V(1).Infof(Format("node %s is active endpoints, type is %s", ep.NodeName, ep.Type))
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

func getActiveEndpointsInfo(eps []*ravenv1beta1.Endpoint) (map[string][]string, int) {
	infos := make(map[string][]string)
	infos[ActiveEndpointsName] = make([]string, 0)
	infos[ActiveEndpointsPublicIP] = make([]string, 0)
	infos[ActiveEndpointsType] = make([]string, 0)
	if len(eps) == 0 {
		return infos, 0
	}
	for _, ep := range eps {
		infos[ActiveEndpointsName] = append(infos[ActiveEndpointsName], ep.NodeName)
		infos[ActiveEndpointsPublicIP] = append(infos[ActiveEndpointsPublicIP], ep.PublicIP)
		infos[ActiveEndpointsType] = append(infos[ActiveEndpointsType], ep.Type)
	}
	return infos, len(infos[ActiveEndpointsName])
}

func (r *ReconcileGateway) configEndpoints(ctx context.Context, gw *ravenv1beta1.Gateway) {
	enableProxy, enableTunnel := util.CheckServer(ctx, r.Client)
	for idx, val := range gw.Status.ActiveEndpoints {
		if gw.Status.ActiveEndpoints[idx].Config == nil {
			gw.Status.ActiveEndpoints[idx].Config = make(map[string]string)
		}
		switch val.Type {
		case ravenv1beta1.Proxy:
			gw.Status.ActiveEndpoints[idx].Config[util.RavenEnableProxy] = strconv.FormatBool(enableProxy)
		case ravenv1beta1.Tunnel:
			gw.Status.ActiveEndpoints[idx].Config[util.RavenEnableTunnel] = strconv.FormatBool(enableTunnel)
		default:
		}
	}
	return
}

func (r *ReconcileGateway) addExtraAllowedSubnet(gw *ravenv1beta1.Gateway) {
	if gw.Annotations == nil || gw.Annotations[util.ExtraAllowedSourceCIDRs] == "" {
		return
	}
	subnets := strings.Split(gw.Annotations[util.ExtraAllowedSourceCIDRs], ",")
	var gatewayName string
	for _, aep := range gw.Status.ActiveEndpoints {
		if aep.Type == ravenv1beta1.Tunnel {
			gatewayName = aep.NodeName
			break
		}
	}
	for idx, node := range gw.Status.Nodes {
		if node.NodeName == gatewayName {
			gw.Status.Nodes[idx].Subnets = append(gw.Status.Nodes[idx].Subnets, subnets...)
			break
		}
	}
}
