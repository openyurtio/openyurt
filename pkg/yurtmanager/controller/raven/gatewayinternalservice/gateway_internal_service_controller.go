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

package gatewayinternalservice

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	ravenv1beta1 "github.com/openyurtio/openyurt/pkg/apis/raven/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
)

const (
	HTTPPorts  = "http"
	HTTPSPorts = "https"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.GatewayInternalServiceController, s)
}

// Add creates a new Service Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
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
		Client:   mgr.GetClient(),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor(names.GatewayInternalServiceController),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(names.GatewayInternalServiceController, mgr, controller.Options{
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
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &EnqueueRequestForConfigEvent{}, predicate.NewPredicateFuncs(
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

	gwList, err := r.listExposedGateway(ctx)
	if err != nil {
		return reconcile.Result{Requeue: true}, err
	}

	enableProxy, _ := util.CheckServer(ctx, r.Client)
	if err = r.reconcileService(ctx, req, gwList, enableProxy); err != nil {
		err = fmt.Errorf(Format("unable to reconcile service: %s", err))
		klog.Errorln(err.Error())
		return reconcile.Result{}, err
	}

	if err = r.reconcileEndpoint(ctx, req, gwList, enableProxy); err != nil {
		err = fmt.Errorf(Format("unable to reconcile endpoint: %s", err))
		klog.Errorln(err.Error())
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileService) listExposedGateway(ctx context.Context) ([]*ravenv1beta1.Gateway, error) {
	var gatewayList ravenv1beta1.GatewayList
	if err := r.List(ctx, &gatewayList); err != nil {
		return nil, fmt.Errorf(Format("unable to list gateways: %s", err))
	}
	exposedGateways := make([]*ravenv1beta1.Gateway, 0)
	for _, gw := range gatewayList.Items {
		switch gw.Spec.ExposeType {
		case ravenv1beta1.ExposeTypePublicIP:
			exposedGateways = append(exposedGateways, gw.DeepCopy())
		case ravenv1beta1.ExposeTypeLoadBalancer:
			exposedGateways = append(exposedGateways, gw.DeepCopy())
		default:
			continue
		}
	}
	return exposedGateways, nil
}

func (r *ReconcileService) reconcileService(ctx context.Context, req ctrl.Request, gatewayList []*ravenv1beta1.Gateway, enableProxy bool) error {
	if len(gatewayList) == 0 || !enableProxy {
		if err := r.cleanService(ctx, req); err != nil {
			return fmt.Errorf("clear service %s error: %s", req.String(), err.Error())
		}
		return nil
	}
	if err := r.updateService(ctx, req, gatewayList); err != nil {
		return fmt.Errorf("update service %s error: %s", req.String(), err.Error())
	}
	return nil
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

func generateService(req ctrl.Request) corev1.Service {
	return corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
			Labels: map[string]string{
				"app": "raven",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

func (r *ReconcileService) updateService(ctx context.Context, req ctrl.Request, gatewayList []*ravenv1beta1.Gateway) error {
	insecurePort, securePort := r.getTargetPort()
	servicePorts := acquiredSpecPorts(gatewayList, insecurePort, securePort)
	sort.Slice(servicePorts, func(i, j int) bool {
		return servicePorts[i].Name < servicePorts[j].Name
	})

	var svc corev1.Service
	err := r.Get(ctx, req.NamespacedName, &svc)
	if err != nil && !apierrs.IsNotFound(err) {
		return err
	}
	if apierrs.IsNotFound(err) {
		klog.V(2).InfoS(Format("create service"), "name", req.Name, "namespace", req.Namespace)
		svc = generateService(req)
		svc.Spec.Ports = servicePorts
		return r.Create(ctx, &svc)
	}
	svc.Spec.Ports = servicePorts
	return r.Update(ctx, &svc)
}

func (r *ReconcileService) getTargetPort() (insecurePort, securePort int32) {
	insecurePort = ravenv1beta1.DefaultProxyServerInsecurePort
	securePort = ravenv1beta1.DefaultProxyServerSecurePort
	var cm corev1.ConfigMap
	err := r.Get(context.TODO(), types.NamespacedName{Namespace: util.WorkingNamespace, Name: util.RavenAgentConfig}, &cm)
	if err != nil {
		return
	}
	if cm.Data == nil {
		return
	}
	_, internalInsecurePort, err := net.SplitHostPort(cm.Data[util.ProxyServerInsecurePortKey])
	if err == nil {
		insecure, _ := strconv.Atoi(internalInsecurePort)
		insecurePort = int32(insecure)
	}

	_, internalSecurePort, err := net.SplitHostPort(cm.Data[util.ProxyServerSecurePortKey])
	if err == nil {
		secure, _ := strconv.Atoi(internalSecurePort)
		securePort = int32(secure)
	}
	return
}

func acquiredSpecPorts(gatewayList []*ravenv1beta1.Gateway, insecurePort, securePort int32) []corev1.ServicePort {
	specPorts := make([]corev1.ServicePort, 0)
	for _, gw := range gatewayList {
		specPorts = append(specPorts, generateServicePorts(gw.Spec.ProxyConfig.ProxyHTTPPort, HTTPPorts, insecurePort)...)
		specPorts = append(specPorts, generateServicePorts(gw.Spec.ProxyConfig.ProxyHTTPSPort, HTTPSPorts, securePort)...)
	}
	return specPorts
}

func generateServicePorts(ports, namePrefix string, targetPort int32) []corev1.ServicePort {
	svcPorts := make([]corev1.ServicePort, 0)
	for _, port := range splitPorts(ports) {
		p, err := strconv.Atoi(port)
		if err != nil {
			continue
		}
		svcPorts = append(svcPorts, corev1.ServicePort{
			Name:       fmt.Sprintf("%s-%s", namePrefix, port),
			Protocol:   corev1.ProtocolTCP,
			Port:       int32(p),
			TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: targetPort},
		})
	}
	return svcPorts
}

func splitPorts(str string) []string {
	ret := make([]string, 0)
	for _, val := range strings.Split(str, ",") {
		ret = append(ret, strings.TrimSpace(val))
	}
	return ret
}

func (r *ReconcileService) reconcileEndpoint(ctx context.Context, req ctrl.Request, gatewayList []*ravenv1beta1.Gateway, enableProxy bool) error {
	var service corev1.Service
	err := r.Get(ctx, req.NamespacedName, &service)
	if err != nil && !apierrs.IsNotFound(err) {
		return fmt.Errorf("get service %s, error: %s", req.String(), err.Error())
	}
	if apierrs.IsNotFound(err) || service.DeletionTimestamp != nil || len(gatewayList) == 0 || !enableProxy {
		if err = r.cleanEndpoint(ctx, req); err != nil {
			return fmt.Errorf("clear endpoints %s, error: %s", req.String(), err.Error())
		}
		return nil
	}
	if err = r.updateEndpoint(ctx, req, &service, gatewayList); err != nil {
		return fmt.Errorf("update endpoints %s, error: %s", req.String(), err.Error())
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

func generateEndpoint(req ctrl.Request) corev1.Endpoints {
	klog.V(2).InfoS(Format("create endpoint"), "name", req.Name, "namespace", req.Namespace)
	return corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		Subsets: []corev1.EndpointSubset{},
	}
}

func (r *ReconcileService) updateEndpoint(ctx context.Context, req ctrl.Request, service *corev1.Service, gatewayList []*ravenv1beta1.Gateway) error {

	subsets := []corev1.EndpointSubset{
		{
			Addresses: r.ensureSpecEndpoints(ctx, gatewayList),
			Ports:     ensureSpecPorts(service),
		},
	}
	if len(subsets[0].Addresses) < 1 || len(subsets[0].Ports) < 1 {
		klog.Warning(Format("endpoints %s/%s miss available node address or port, get node %d and port %d",
			req.Namespace, req.Name, len(subsets[0].Addresses), len(subsets[0].Ports)))
		return nil
	}
	var eps corev1.Endpoints
	err := r.Get(ctx, req.NamespacedName, &eps)
	if err != nil && !apierrs.IsNotFound(err) {
		return err
	}
	if apierrs.IsNotFound(err) {
		eps = generateEndpoint(req)
		eps.Subsets = subsets
		return r.Create(ctx, &eps)
	}
	eps.Subsets = subsets
	return r.Update(ctx, &eps)
}

func (r *ReconcileService) ensureSpecEndpoints(ctx context.Context, gateways []*ravenv1beta1.Gateway) []corev1.EndpointAddress {
	specAddresses := make([]corev1.EndpointAddress, 0)
	for _, gw := range gateways {

		if len(gw.Status.ActiveEndpoints) < 1 {
			newGw, err := r.waitElectEndpoints(ctx, gw.Name)
			if err == nil {
				gw = newGw
			}
		}
		for _, aep := range gw.Status.ActiveEndpoints {
			if aep.Type != ravenv1beta1.Proxy {
				continue
			}
			var node corev1.Node
			err := r.Get(ctx, types.NamespacedName{Name: aep.NodeName}, &node)
			if err != nil {
				continue
			}
			specAddresses = append(specAddresses, corev1.EndpointAddress{
				IP:       util.GetNodeInternalIP(node),
				NodeName: func(n corev1.Node) *string { return &n.Name }(node),
			})
		}
	}
	return specAddresses
}

func (r *ReconcileService) waitElectEndpoints(ctx context.Context, gwName string) (*ravenv1beta1.Gateway, error) {
	var gw ravenv1beta1.Gateway
	err := wait.PollImmediate(time.Second*5, time.Minute, func() (done bool, err error) {
		err = r.Get(ctx, types.NamespacedName{Name: gwName}, &gw)
		if err != nil {
			return false, err
		}
		if len(gw.Status.ActiveEndpoints) < 1 {
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	return gw.DeepCopy(), nil
}

func ensureSpecPorts(svc *corev1.Service) []corev1.EndpointPort {
	specPorts := make([]corev1.EndpointPort, 0)
	for _, port := range svc.Spec.Ports {
		specPorts = append(specPorts, corev1.EndpointPort{
			Name:     port.Name,
			Port:     int32(port.TargetPort.IntValue()),
			Protocol: port.Protocol,
		})
	}
	return specPorts
}
