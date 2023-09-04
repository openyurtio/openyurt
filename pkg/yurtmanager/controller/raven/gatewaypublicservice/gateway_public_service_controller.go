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

package gatewaypublicservice

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	"github.com/openyurtio/openyurt/pkg/apis/raven"
	ravenv1beta1 "github.com/openyurtio/openyurt/pkg/apis/raven/v1beta1"
	common "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/utils"
)

const (
	ServiceDeleteFailed = "DeleteServiceFail"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.GatewayPublicServiceController, s)
}

// Add creates a new Service Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(c *appconfig.CompletedConfig, mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

var _ reconcile.Reconciler = &ReconcileService{}

type serviceInformation struct {
	mu   sync.Mutex
	data map[string]string
}

func newServiceInfo() *serviceInformation {
	return &serviceInformation{data: make(map[string]string, 0)}
}
func (s *serviceInformation) write(key, val string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = val
}

func (s *serviceInformation) read(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data[key]
}

func (s *serviceInformation) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = make(map[string]string)
}

// ReconcileService reconciles a Gateway object
type ReconcileService struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
	option   utils.Option
	svcInfo  *serviceInformation
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileService{
		Client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor(names.GatewayPublicServiceController),
		option:   utils.NewOption(),
		svcInfo:  newServiceInfo(),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(names.GatewayPublicServiceController, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: common.ConcurrentReconciles,
	})
	if err != nil {
		return err
	}

	// Watch for changes to Gateway
	err = c.Watch(&source.Kind{Type: &ravenv1beta1.Gateway{}}, &EnqueueRequestForGatewayEvent{})
	if err != nil {
		return err
	}

	//Watch for changes to raven agent
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &EnqueueRequestForConfigEvent{client: mgr.GetClient()}, predicate.NewPredicateFuncs(
		func(object client.Object) bool {
			cm, ok := object.(*corev1.ConfigMap)
			if !ok {
				return false
			}
			if cm.GetNamespace() != utils.WorkingNamespace {
				return false
			}
			if cm.GetName() != utils.RavenAgentConfig {
				return false
			}
			return true
		},
	))
	if err != nil {
		return err
	}
	return nil
}

// Reconcile reads that state of the cluster for a Gateway object and makes changes based on the state read
// and what is in the Gateway.Spec
func (r *ReconcileService) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	klog.V(2).Info(Format("started reconciling public service for gateway %s", req.Name))
	defer klog.V(2).Info(Format("finished reconciling public service for gateway %s", req.Name))
	gw, err := r.getGateway(ctx, req)
	if err != nil && !apierrs.IsNotFound(err) {
		klog.Error(Format("failed to get gateway %s, error %s", req.Name, err.Error()))
		return reconcile.Result{}, err
	}
	if apierrs.IsNotFound(err) {
		gw = &ravenv1beta1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: req.Name}}
	}
	r.svcInfo.cleanup()
	r.setOptions(ctx, gw, apierrs.IsNotFound(err))
	if err := r.reconcileService(ctx, gw.DeepCopy()); err != nil {
		err = fmt.Errorf(Format("unable to reconcile service: %s", err))
		klog.Error(err.Error())
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
	}

	if err := r.reconcileEndpoints(ctx, gw.DeepCopy()); err != nil {
		err = fmt.Errorf(Format("unable to reconcile endpoint: %s", err))
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileService) setOptions(ctx context.Context, gw *ravenv1beta1.Gateway, isNotFound bool) {
	r.option.SetProxyOption(true)
	r.option.SetTunnelOption(true)
	if isNotFound {
		r.option.SetProxyOption(false)
		r.option.SetTunnelOption(false)
		klog.V(4).Info(Format("set option for proxy (%t) and tunnel (%t), reason gateway %s is not found",
			false, false, gw.GetName()))
		return
	}

	if gw.DeletionTimestamp != nil {
		r.option.SetProxyOption(false)
		r.option.SetTunnelOption(false)
		klog.V(4).Info(Format("set option for proxy (%t) and tunnel (%t), reason: gateway %s is deleted ",
			false, false, gw.GetName()))
		return
	}

	if gw.Spec.ExposeType != ravenv1beta1.ExposeTypeLoadBalancer {
		r.option.SetProxyOption(false)
		r.option.SetTunnelOption(false)
		klog.V(4).Info(Format("set option for proxy (%t) and tunnel (%t), reason: gateway %s exposed type is %s ",
			false, false, gw.GetName(), gw.Spec.ExposeType))
		return
	}

	enableProxy, enableTunnel := utils.CheckServer(ctx, r.Client)
	if !enableTunnel {
		r.option.SetTunnelOption(enableTunnel)
		klog.V(4).Info(Format("set option for tunnel (%t), reason: raven-cfg close tunnel ", false))
	}

	if !enableProxy {
		r.option.SetProxyOption(enableProxy)
		klog.V(4).Info(Format("set option for tunnel (%t), reason: raven-cfg close proxy ", false))
	}
	return
}

func (r *ReconcileService) getGateway(ctx context.Context, req reconcile.Request) (*ravenv1beta1.Gateway, error) {
	var gw ravenv1beta1.Gateway
	err := r.Get(ctx, req.NamespacedName, &gw)
	if err != nil {
		return nil, err
	}
	return gw.DeepCopy(), nil
}

func (r *ReconcileService) generateServiceName(services []corev1.Service) {
	for _, svc := range services {
		epName := svc.Labels[utils.LabelCurrentGatewayEndpoints]
		epType := svc.Labels[raven.LabelCurrentGatewayType]
		if epName == "" || epType == "" {
			continue
		}
		r.svcInfo.write(formatKey(epName, epType), svc.GetName())
	}
	return
}

func (r *ReconcileService) reconcileService(ctx context.Context, gw *ravenv1beta1.Gateway) error {
	enableProxy := r.option.GetProxyOption()
	if enableProxy {
		klog.V(2).Info(Format("start manage proxy service for gateway %s", gw.GetName()))
		defer klog.V(2).Info(Format("finish manage proxy service for gateway %s", gw.GetName()))
		if err := r.manageService(ctx, gw, ravenv1beta1.Proxy); err != nil {
			return fmt.Errorf("failed to manage service for proxy server %s", err.Error())
		}
	} else {
		klog.V(2).Info(Format("start clear proxy service for gateway %s", gw.GetName()))
		defer klog.V(2).Info(Format("finish clear proxy service for gateway %s", gw.GetName()))
		if err := r.clearService(ctx, gw.GetName(), ravenv1beta1.Proxy); err != nil {
			return fmt.Errorf("failed to clear service for proxy server %s", err.Error())
		}
	}

	enableTunnel := r.option.GetTunnelOption()
	if enableTunnel {
		klog.V(2).Info(Format("start manage tunnel service for gateway %s", gw.GetName()))
		defer klog.V(2).Info(Format("finish manage tunnel service for gateway %s", gw.GetName()))
		if err := r.manageService(ctx, gw, ravenv1beta1.Tunnel); err != nil {
			return fmt.Errorf("failed to manage service for tunnel server %s", err.Error())
		}
	} else {
		klog.V(2).Info(Format("start clear tunnel service for gateway %s", gw.GetName()))
		defer klog.V(2).Info(Format("finish clear tunnel service for gateway %s", gw.GetName()))
		if err := r.clearService(ctx, gw.GetName(), ravenv1beta1.Tunnel); err != nil {
			return fmt.Errorf("failed to clear service for tunnel server %s", err.Error())
		}
	}
	return nil
}

func (r *ReconcileService) reconcileEndpoints(ctx context.Context, gw *ravenv1beta1.Gateway) error {
	enableProxy := r.option.GetProxyOption()
	if enableProxy {
		klog.V(2).Info(Format("start manage proxy service endpoints for gateway %s", gw.GetName()))
		defer klog.V(2).Info(Format("finish manage proxy service endpoints for gateway %s", gw.GetName()))
		if err := r.manageEndpoints(ctx, gw, ravenv1beta1.Proxy); err != nil {
			return fmt.Errorf("failed to manage endpoints for proxy server %s", err.Error())
		}
	} else {
		klog.V(2).Info(Format("start clear proxy service endpoints for gateway %s", gw.GetName()))
		defer klog.V(2).Info(Format("finish clear proxy service endpoints for gateway %s", gw.GetName()))
		if err := r.clearEndpoints(ctx, gw.GetName(), ravenv1beta1.Proxy); err != nil {
			return fmt.Errorf("failed to clear endpoints for proxy server %s", err.Error())
		}
	}
	enableTunnel := r.option.GetTunnelOption()
	if enableTunnel {
		klog.V(2).Info(Format("start manage tunnel service endpoints for gateway %s", gw.GetName()))
		defer klog.V(2).Info(Format("finish manage tunnel service endpoints for gateway %s", gw.GetName()))
		if err := r.manageEndpoints(ctx, gw, ravenv1beta1.Tunnel); err != nil {
			return fmt.Errorf("failed to manage endpoints for tunnel server %s", err.Error())
		}
	} else {
		klog.V(2).Info(Format("start clear tunnel service endpoints for gateway %s", gw.GetName()))
		defer klog.V(2).Info(Format("finish clear tunnel service endpoints for gateway %s", gw.GetName()))
		if err := r.clearEndpoints(ctx, gw.GetName(), ravenv1beta1.Tunnel); err != nil {
			return fmt.Errorf("failed to clear endpoints for tunnel server %s", err.Error())
		}
	}
	return nil
}

func (r *ReconcileService) clearService(ctx context.Context, gatewayName, gatewayType string) error {
	svcList, err := r.listService(ctx, gatewayName, gatewayType)
	if err != nil {
		return fmt.Errorf("failed to list service for gateway %s", gatewayName)
	}
	for _, svc := range svcList.Items {
		err := r.Delete(ctx, svc.DeepCopy())
		if err != nil {
			r.recorder.Event(svc.DeepCopy(), corev1.EventTypeWarning, ServiceDeleteFailed,
				fmt.Sprintf("The gateway %s %s server is not need to exposed by loadbalancer, failed to delete service %s/%s",
					gatewayName, gatewayType, svc.GetNamespace(), svc.GetName()))
			continue
		}
	}
	return nil
}

func (r *ReconcileService) clearEndpoints(ctx context.Context, gatewayName, gatewayType string) error {
	epsList, err := r.listEndpoints(ctx, gatewayName, gatewayType)
	if err != nil {
		return fmt.Errorf("failed to list endpoints for gateway %s", gatewayName)
	}
	for _, eps := range epsList.Items {
		err := r.Delete(ctx, eps.DeepCopy())
		if err != nil {
			r.recorder.Event(eps.DeepCopy(), corev1.EventTypeWarning, ServiceDeleteFailed,
				fmt.Sprintf("The gateway %s %s server is not need to exposed by loadbalancer, failed to delete endpoints %s/%s",
					gatewayName, gatewayType, eps.GetNamespace(), eps.GetName()))
			continue
		}
	}
	return nil
}

func (r *ReconcileService) manageService(ctx context.Context, gateway *ravenv1beta1.Gateway, gatewayType string) error {
	curSvcList, err := r.listService(ctx, gateway.GetName(), gatewayType)
	if err != nil {
		return fmt.Errorf("failed list service for gateway %s type %s , error %s", gateway.GetName(), gatewayType, err.Error())
	}
	proxyPort, tunnelPort := r.getTargetPort()
	specSvcList := acquiredSpecService(gateway, gatewayType, proxyPort, tunnelPort)
	addSvc, updateSvc, deleteSvc := classifyService(curSvcList, specSvcList)
	r.generateServiceName(specSvcList.Items)
	for i := 0; i < len(addSvc); i++ {
		if err := r.Create(ctx, addSvc[i]); err != nil {
			if apierrs.IsAlreadyExists(err) {
				klog.V(2).Info(Format("service %s/%s has already exist, ignore creating it", addSvc[i].GetNamespace(), addSvc[i].GetName()))
				return nil
			}
			return fmt.Errorf("failed create service for gateway %s type %s , error %s", gateway.GetName(), gatewayType, err.Error())
		}
	}
	for i := 0; i < len(updateSvc); i++ {
		if err := r.Update(ctx, updateSvc[i]); err != nil {
			return fmt.Errorf("failed update service for gateway %s type %s , error %s", gateway.GetName(), gatewayType, err.Error())
		}
	}
	for i := 0; i < len(deleteSvc); i++ {
		if err := r.Delete(ctx, deleteSvc[i]); err != nil {
			return fmt.Errorf("failed delete service for gateway %s type %s , error %s", gateway.GetName(), gatewayType, err.Error())
		}
	}
	return nil
}

func (r *ReconcileService) manageEndpoints(ctx context.Context, gateway *ravenv1beta1.Gateway, gatewayType string) error {
	currEpsList, err := r.listEndpoints(ctx, gateway.GetName(), gatewayType)
	if err != nil {
		return fmt.Errorf("failed list service for gateway %s type %s , error %s", gateway.GetName(), gatewayType, err.Error())
	}
	specEpsList := r.acquiredSpecEndpoints(ctx, gateway, gatewayType)
	addEps, updateEps, deleteEps := classifyEndpoints(currEpsList, specEpsList)
	for i := 0; i < len(addEps); i++ {
		if err := r.Create(ctx, addEps[i]); err != nil {
			if apierrs.IsAlreadyExists(err) {
				klog.V(2).Info(Format("endpoints %s/%s has already exist, ignore creating it", addEps[i].GetNamespace(), addEps[i].GetName()))
				return nil
			}
			return fmt.Errorf("failed create endpoints for gateway %s type %s , error %s", gateway.GetName(), gatewayType, err.Error())
		}
	}
	for i := 0; i < len(updateEps); i++ {
		if err := r.Update(ctx, updateEps[i]); err != nil {
			return fmt.Errorf("failed update endpoints for gateway %s type %s , error %s", gateway.GetName(), gatewayType, err.Error())
		}
	}
	for i := 0; i < len(deleteEps); i++ {
		if err := r.Delete(ctx, deleteEps[i]); err != nil {
			return fmt.Errorf("failed delete endpoints for gateway %s type %s , error %s", gateway.GetName(), gatewayType, err.Error())
		}
	}
	return nil
}

func (r *ReconcileService) getTargetPort() (proxyPort, tunnelPort int32) {
	proxyPort = ravenv1beta1.DefaultProxyServerExposedPort
	tunnelPort = ravenv1beta1.DefaultTunnelServerExposedPort
	var cm corev1.ConfigMap
	err := r.Get(context.TODO(), types.NamespacedName{Namespace: utils.WorkingNamespace, Name: utils.RavenAgentConfig}, &cm)
	if err != nil {
		return
	}
	if cm.Data == nil {
		return
	}
	_, proxyExposedPort, err := net.SplitHostPort(cm.Data[utils.ProxyServerExposedPortKey])
	if err == nil {
		proxy, _ := strconv.Atoi(proxyExposedPort)
		proxyPort = int32(proxy)
	}
	_, tunnelExposedPort, err := net.SplitHostPort(cm.Data[utils.VPNServerExposedPortKey])
	if err == nil {
		tunnel, _ := strconv.Atoi(tunnelExposedPort)
		tunnelPort = int32(tunnel)
	}
	return
}

func (r *ReconcileService) listService(ctx context.Context, gatewayName, gatewayType string) (*corev1.ServiceList, error) {
	var svcList corev1.ServiceList
	err := r.List(ctx, &svcList, &client.ListOptions{
		LabelSelector: labels.Set{
			raven.LabelCurrentGateway:     gatewayName,
			raven.LabelCurrentGatewayType: gatewayType,
		}.AsSelector(),
	})
	if err != nil {
		return nil, err
	}
	newList := make([]corev1.Service, 0)
	for _, val := range svcList.Items {
		if val.Spec.Type == corev1.ServiceTypeLoadBalancer {
			newList = append(newList, val)
		}
	}
	svcList.Items = newList
	return &svcList, nil
}

func (r *ReconcileService) listEndpoints(ctx context.Context, gatewayName, gatewayType string) (*corev1.EndpointsList, error) {
	var epsList corev1.EndpointsList
	err := r.List(ctx, &epsList, &client.ListOptions{
		LabelSelector: labels.Set{
			raven.LabelCurrentGateway:     gatewayName,
			raven.LabelCurrentGatewayType: gatewayType,
		}.AsSelector()})
	if err != nil {
		return nil, err
	}
	return &epsList, nil
}

func (r *ReconcileService) acquiredSpecEndpoints(ctx context.Context, gateway *ravenv1beta1.Gateway, gatewayType string) *corev1.EndpointsList {
	proxyPort, tunnelPort := r.getTargetPort()
	endpoints := make([]corev1.Endpoints, 0)
	for _, aep := range gateway.Status.ActiveEndpoints {
		if aep.Type != gatewayType {
			continue
		}
		address, err := r.getEndpointsAddress(ctx, aep.NodeName)
		if err != nil {
			continue
		}
		switch aep.Type {
		case ravenv1beta1.Proxy:
			name := r.svcInfo.read(formatKey(aep.NodeName, ravenv1beta1.Proxy))
			if name == "" {
				continue
			}
			endpoints = append(endpoints, corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: utils.WorkingNamespace,
					Labels: map[string]string{
						raven.LabelCurrentGateway:          gateway.GetName(),
						raven.LabelCurrentGatewayType:      ravenv1beta1.Proxy,
						utils.LabelCurrentGatewayEndpoints: aep.NodeName,
					},
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{*address},
						Ports: []corev1.EndpointPort{
							{
								Port:     proxyPort,
								Protocol: corev1.ProtocolTCP,
							},
						},
					},
				},
			})
		case ravenv1beta1.Tunnel:
			name := r.svcInfo.read(formatKey(aep.NodeName, ravenv1beta1.Tunnel))
			if name == "" {
				continue
			}
			endpoints = append(endpoints, corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: utils.WorkingNamespace,
					Labels: map[string]string{
						raven.LabelCurrentGateway:          gateway.GetName(),
						raven.LabelCurrentGatewayType:      ravenv1beta1.Tunnel,
						utils.LabelCurrentGatewayEndpoints: aep.NodeName,
					},
				},
				Subsets: []corev1.EndpointSubset{
					{
						Addresses: []corev1.EndpointAddress{*address},
						Ports: []corev1.EndpointPort{
							{
								Port:     tunnelPort,
								Protocol: corev1.ProtocolUDP,
							},
						},
					},
				},
			})
		}
	}
	return &corev1.EndpointsList{Items: endpoints}
}

func (r *ReconcileService) getEndpointsAddress(ctx context.Context, name string) (*corev1.EndpointAddress, error) {
	var node corev1.Node
	err := r.Get(ctx, types.NamespacedName{Name: name}, &node)
	if err != nil {
		klog.Errorf(Format("failed to get node %s for get active endpoints address, error %s", name, err.Error()))
		return nil, err
	}
	return &corev1.EndpointAddress{NodeName: func(n corev1.Node) *string { return &n.Name }(node), IP: utils.GetNodeInternalIP(node)}, nil
}

func acquiredSpecService(gateway *ravenv1beta1.Gateway, gatewayType string, proxyPort, tunnelPort int32) *corev1.ServiceList {
	services := make([]corev1.Service, 0)
	if gateway == nil {
		return &corev1.ServiceList{Items: services}
	}
	for _, aep := range gateway.Status.ActiveEndpoints {
		if aep.Type != gatewayType {
			continue
		}
		if aep.Port < 1 || aep.Port > 65535 {
			continue
		}
		switch aep.Type {
		case ravenv1beta1.Proxy:
			services = append(services, corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      utils.FormatName(fmt.Sprintf("%s-%s", utils.GatewayProxyServiceNamePrefix, gateway.GetName())),
					Namespace: utils.WorkingNamespace,
					Labels: map[string]string{
						raven.LabelCurrentGateway:          gateway.GetName(),
						raven.LabelCurrentGatewayType:      ravenv1beta1.Proxy,
						utils.LabelCurrentGatewayEndpoints: aep.NodeName,
					},
				},
				Spec: corev1.ServiceSpec{
					Type:                  corev1.ServiceTypeLoadBalancer,
					ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeLocal,
					Ports: []corev1.ServicePort{
						{
							Protocol: corev1.ProtocolTCP,
							Port:     int32(aep.Port),
							TargetPort: intstr.IntOrString{
								Type:   intstr.Int,
								IntVal: proxyPort,
							},
						},
					},
				},
			})
		case ravenv1beta1.Tunnel:
			services = append(services, corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      utils.FormatName(fmt.Sprintf("%s-%s", utils.GatewayTunnelServiceNamePrefix, gateway.GetName())),
					Namespace: utils.WorkingNamespace,
					Labels: map[string]string{
						raven.LabelCurrentGateway:          gateway.GetName(),
						raven.LabelCurrentGatewayType:      ravenv1beta1.Tunnel,
						utils.LabelCurrentGatewayEndpoints: aep.NodeName,
					},
				},
				Spec: corev1.ServiceSpec{
					Type:                  corev1.ServiceTypeLoadBalancer,
					ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeLocal,
					Ports: []corev1.ServicePort{
						{
							Protocol: corev1.ProtocolUDP,
							Port:     int32(aep.Port),
							TargetPort: intstr.IntOrString{
								Type:   intstr.Int,
								IntVal: tunnelPort,
							},
						},
					},
				},
			})
		}
	}
	return &corev1.ServiceList{Items: services}
}

func classifyService(current, spec *corev1.ServiceList) (added, updated, deleted []*corev1.Service) {
	added = make([]*corev1.Service, 0)
	updated = make([]*corev1.Service, 0)
	deleted = make([]*corev1.Service, 0)

	getKey := func(svc *corev1.Service) string {
		epType := svc.Labels[raven.LabelCurrentGatewayType]
		epName := svc.Labels[utils.LabelCurrentGatewayEndpoints]
		if epType == "" {
			return ""
		}
		if epName == "" {
			return ""
		}
		return formatKey(epName, epType)
	}

	r := make(map[string]int)
	for idx, val := range current.Items {
		if key := getKey(&val); key != "" {
			r[key] = idx
		}
	}
	for _, val := range spec.Items {
		if key := getKey(&val); key != "" {
			if idx, ok := r[key]; ok {
				updatedService := current.Items[idx].DeepCopy()
				updatedService.Spec = *val.Spec.DeepCopy()
				updated = append(updated, updatedService)
				delete(r, key)
			} else {
				added = append(added, val.DeepCopy())
			}
		}

	}
	for key, val := range r {
		deleted = append(deleted, current.Items[val].DeepCopy())
		delete(r, key)
	}
	return added, updated, deleted
}

func classifyEndpoints(current, spec *corev1.EndpointsList) (added, updated, deleted []*corev1.Endpoints) {
	added = make([]*corev1.Endpoints, 0)
	updated = make([]*corev1.Endpoints, 0)
	deleted = make([]*corev1.Endpoints, 0)

	getKey := func(ep *corev1.Endpoints) string {
		epType := ep.Labels[raven.LabelCurrentGatewayType]
		epName := ep.Labels[utils.LabelCurrentGatewayEndpoints]
		if epType == "" {
			return ""
		}
		if epName == "" {
			return ""
		}
		return formatKey(epName, epType)
	}

	r := make(map[string]int)
	for idx, val := range current.Items {
		if key := getKey(&val); key != "" {
			r[key] = idx
		}
	}
	for _, val := range spec.Items {
		if key := getKey(&val); key != "" {
			if idx, ok := r[key]; ok {
				updatedEndpoints := current.Items[idx].DeepCopy()
				updatedEndpoints.Subsets = val.DeepCopy().Subsets
				updated = append(updated, updatedEndpoints)
				delete(r, key)
			} else {
				added = append(added, val.DeepCopy())
			}
		}
	}
	for key, val := range r {
		deleted = append(deleted, current.Items[val].DeepCopy())
		delete(r, key)
	}
	return added, updated, deleted
}

func formatKey(endpointName, endpointType string) string {
	return fmt.Sprintf("%s-%s", endpointName, endpointType)
}
