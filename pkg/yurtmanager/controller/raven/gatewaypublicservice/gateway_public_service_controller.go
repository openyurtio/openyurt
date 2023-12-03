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
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
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
func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

var _ reconcile.Reconciler = &ReconcileService{}

type serviceRecord struct {
	data map[string]string
}

func newServiceRecord() *serviceRecord {
	return &serviceRecord{data: make(map[string]string, 0)}
}
func (s *serviceRecord) write(key, val string) {
	s.data[key] = val
}

func (s *serviceRecord) read(key string) string {
	return s.data[key]
}

// ReconcileService reconciles a Gateway object
type ReconcileService struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileService{
		Client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor(names.GatewayPublicServiceController),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(names.GatewayPublicServiceController, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: util.ConcurrentReconciles,
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
			if cm.GetNamespace() != util.WorkingNamespace {
				return false
			}
			if cm.GetName() != util.RavenAgentConfig {
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
	enableProxy, enableTunnel := util.CheckServer(ctx, r.Client)
	gw, err := r.getGateway(ctx, req)
	if err != nil {
		if apierrs.IsNotFound(err) {
			gw = &ravenv1beta1.Gateway{ObjectMeta: metav1.ObjectMeta{Name: req.Name}}
			enableTunnel = false
			enableProxy = false
		} else {
			klog.Error(Format("could not get gateway %s, error %s", req.Name, err.Error()))
			return reconcile.Result{}, err
		}
	}
	svcRecord := newServiceRecord()
	if err := r.reconcileService(ctx, gw.DeepCopy(), svcRecord, enableTunnel, enableProxy); err != nil {
		err = fmt.Errorf(Format("unable to reconcile service: %s", err))
		klog.Error(err.Error())
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
	}

	if err := r.reconcileEndpoints(ctx, gw.DeepCopy(), svcRecord, enableTunnel, enableProxy); err != nil {
		err = fmt.Errorf(Format("unable to reconcile endpoint: %s", err))
		klog.Error(err.Error())
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileService) getGateway(ctx context.Context, req reconcile.Request) (*ravenv1beta1.Gateway, error) {
	var gw ravenv1beta1.Gateway
	err := r.Get(ctx, req.NamespacedName, &gw)
	if err != nil {
		return nil, err
	}
	return gw.DeepCopy(), nil
}

func recordServiceNames(services []corev1.Service, record *serviceRecord) {
	for _, svc := range services {
		epName := svc.Labels[util.LabelCurrentGatewayEndpoints]
		epType := svc.Labels[raven.LabelCurrentGatewayType]
		if epName == "" || epType == "" {
			continue
		}
		record.write(formatKey(epName, epType), svc.GetName())
	}
	return
}

func (r *ReconcileService) reconcileService(ctx context.Context, gw *ravenv1beta1.Gateway, record *serviceRecord, enableTunnel, enableProxy bool) error {
	if enableProxy {
		if err := r.manageService(ctx, gw, ravenv1beta1.Proxy, record); err != nil {
			return fmt.Errorf("could not manage service for proxy, error %s", err.Error())
		}
	} else {
		if err := r.clearService(ctx, gw.GetName(), ravenv1beta1.Proxy); err != nil {
			return fmt.Errorf("could not clear service for proxy, error %s", err.Error())
		}
	}

	if enableTunnel {
		if err := r.manageService(ctx, gw, ravenv1beta1.Tunnel, record); err != nil {
			return fmt.Errorf("could not manage service for tunnel, error %s", err.Error())
		}
	} else {
		if err := r.clearService(ctx, gw.GetName(), ravenv1beta1.Tunnel); err != nil {
			return fmt.Errorf("could not clear service for tunnel, error %s", err.Error())
		}
	}
	return nil
}

func (r *ReconcileService) reconcileEndpoints(ctx context.Context, gw *ravenv1beta1.Gateway, record *serviceRecord, enableTunnel, enableProxy bool) error {
	if enableProxy {
		if err := r.manageEndpoints(ctx, gw, ravenv1beta1.Proxy, record); err != nil {
			return fmt.Errorf("could not manage endpoints for proxy server %s", err.Error())
		}
	} else {
		if err := r.clearEndpoints(ctx, gw.GetName(), ravenv1beta1.Proxy); err != nil {
			return fmt.Errorf("could not clear endpoints for proxy server %s", err.Error())
		}
	}
	if enableTunnel {
		if err := r.manageEndpoints(ctx, gw, ravenv1beta1.Tunnel, record); err != nil {
			return fmt.Errorf("could not manage endpoints for tunnel server %s", err.Error())
		}
	} else {
		if err := r.clearEndpoints(ctx, gw.GetName(), ravenv1beta1.Tunnel); err != nil {
			return fmt.Errorf("could not clear endpoints for tunnel server %s", err.Error())
		}
	}
	return nil
}

func (r *ReconcileService) clearService(ctx context.Context, gatewayName, gatewayType string) error {
	svcList, err := r.listService(ctx, gatewayName, gatewayType)
	if err != nil {
		return fmt.Errorf("could not list service for gateway %s", gatewayName)
	}
	for _, svc := range svcList.Items {
		err := r.Delete(ctx, svc.DeepCopy())
		if err != nil {
			r.recorder.Event(svc.DeepCopy(), corev1.EventTypeWarning, ServiceDeleteFailed,
				fmt.Sprintf("The gateway %s %s server is not need to exposed by loadbalancer, could not delete service %s/%s",
					gatewayName, gatewayType, svc.GetNamespace(), svc.GetName()))
			continue
		}
	}
	return nil
}

func (r *ReconcileService) clearEndpoints(ctx context.Context, gatewayName, gatewayType string) error {
	epsList, err := r.listEndpoints(ctx, gatewayName, gatewayType)
	if err != nil {
		return fmt.Errorf("could not list endpoints for gateway %s", gatewayName)
	}
	for _, eps := range epsList.Items {
		err := r.Delete(ctx, eps.DeepCopy())
		if err != nil {
			r.recorder.Event(eps.DeepCopy(), corev1.EventTypeWarning, ServiceDeleteFailed,
				fmt.Sprintf("The gateway %s %s server is not need to exposed by loadbalancer, could not delete endpoints %s/%s",
					gatewayName, gatewayType, eps.GetNamespace(), eps.GetName()))
			continue
		}
	}
	return nil
}

func (r *ReconcileService) manageService(ctx context.Context, gateway *ravenv1beta1.Gateway, gatewayType string, record *serviceRecord) error {
	curSvcList, err := r.listService(ctx, gateway.GetName(), gatewayType)
	if err != nil {
		return fmt.Errorf("failed list service for gateway %s type %s , error %s", gateway.GetName(), gatewayType, err.Error())
	}
	proxyPort, tunnelPort := r.getTargetPort()
	specSvcList := acquiredSpecService(gateway, gatewayType, proxyPort, tunnelPort)
	addSvc, updateSvc, deleteSvc := classifyService(curSvcList, specSvcList)
	recordServiceNames(specSvcList.Items, record)
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

func (r *ReconcileService) manageEndpoints(ctx context.Context, gateway *ravenv1beta1.Gateway, gatewayType string, record *serviceRecord) error {
	currEpsList, err := r.listEndpoints(ctx, gateway.GetName(), gatewayType)
	if err != nil {
		return fmt.Errorf("failed list service for gateway %s type %s , error %s", gateway.GetName(), gatewayType, err.Error())
	}
	specEpsList := r.acquiredSpecEndpoints(ctx, gateway, gatewayType, record)
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
	err := r.Get(context.TODO(), types.NamespacedName{Namespace: util.WorkingNamespace, Name: util.RavenAgentConfig}, &cm)
	if err != nil {
		return
	}
	if cm.Data == nil {
		return
	}
	_, proxyExposedPort, err := net.SplitHostPort(cm.Data[util.ProxyServerExposedPortKey])
	if err == nil {
		proxy, _ := strconv.Atoi(proxyExposedPort)
		proxyPort = int32(proxy)
	}
	_, tunnelExposedPort, err := net.SplitHostPort(cm.Data[util.VPNServerExposedPortKey])
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

func (r *ReconcileService) acquiredSpecEndpoints(ctx context.Context, gateway *ravenv1beta1.Gateway, gatewayType string, record *serviceRecord) *corev1.EndpointsList {
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
			name := record.read(formatKey(aep.NodeName, ravenv1beta1.Proxy))
			if name == "" {
				continue
			}
			endpoints = append(endpoints, corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: util.WorkingNamespace,
					Labels: map[string]string{
						raven.LabelCurrentGateway:         gateway.GetName(),
						raven.LabelCurrentGatewayType:     ravenv1beta1.Proxy,
						util.LabelCurrentGatewayEndpoints: aep.NodeName,
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
			name := record.read(formatKey(aep.NodeName, ravenv1beta1.Tunnel))
			if name == "" {
				continue
			}
			endpoints = append(endpoints, corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: util.WorkingNamespace,
					Labels: map[string]string{
						raven.LabelCurrentGateway:         gateway.GetName(),
						raven.LabelCurrentGatewayType:     ravenv1beta1.Tunnel,
						util.LabelCurrentGatewayEndpoints: aep.NodeName,
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
		klog.Errorf(Format("could not get node %s for get active endpoints address, error %s", name, err.Error()))
		return nil, err
	}
	return &corev1.EndpointAddress{NodeName: func(n corev1.Node) *string { return &n.Name }(node), IP: util.GetNodeInternalIP(node)}, nil
}

func acquiredSpecService(gateway *ravenv1beta1.Gateway, gatewayType string, proxyPort, tunnelPort int32) *corev1.ServiceList {
	services := make([]corev1.Service, 0)
	if gateway == nil {
		return &corev1.ServiceList{Items: services}
	}
	if gateway.Spec.ExposeType != ravenv1beta1.ExposeTypeLoadBalancer {
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
					Name:      util.FormatName(fmt.Sprintf("%s-%s", util.GatewayProxyServiceNamePrefix, gateway.GetName())),
					Namespace: util.WorkingNamespace,
					Labels: map[string]string{
						raven.LabelCurrentGateway:         gateway.GetName(),
						raven.LabelCurrentGatewayType:     ravenv1beta1.Proxy,
						util.LabelCurrentGatewayEndpoints: aep.NodeName,
					},
					Annotations: map[string]string{"svc.openyurt.io/discard": "true"},
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
					Name:      util.FormatName(fmt.Sprintf("%s-%s", util.GatewayTunnelServiceNamePrefix, gateway.GetName())),
					Namespace: util.WorkingNamespace,
					Labels: map[string]string{
						raven.LabelCurrentGateway:         gateway.GetName(),
						raven.LabelCurrentGatewayType:     ravenv1beta1.Tunnel,
						util.LabelCurrentGatewayEndpoints: aep.NodeName,
					},
					Annotations: map[string]string{"svc.openyurt.io/discard": "true"},
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
		epName := svc.Labels[util.LabelCurrentGatewayEndpoints]
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
		epName := ep.Labels[util.LabelCurrentGatewayEndpoints]
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
