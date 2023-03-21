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

package dns

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	common "github.com/openyurtio/openyurt/pkg/controller/raven"
	"github.com/openyurtio/openyurt/pkg/controller/raven/utils"
	utilclient "github.com/openyurtio/openyurt/pkg/util/client"
)

var (
	concurrentReconciles = 3
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", common.DnsController, s)
}

// Add creates a new Ravendns Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(c *appconfig.CompletedConfig, mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

var _ reconcile.Reconciler = &ReconcileDns{}

type ReconcileDns struct {
	client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileDns{
		Client:   utilclient.NewClientFromManager(mgr, common.DnsController),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor(common.DnsController),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(common.DnsController, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles,
	})
	if err != nil {
		return err
	}

	// Watch for changes to service
	err = c.Watch(&source.Kind{Type: &corev1.Service{}}, &EnqueueRequestForServiceEvent{}, predicate.NewPredicateFuncs(
		func(obj client.Object) bool {
			svc, ok := obj.(*corev1.Service)
			if !ok {
				return false
			}
			if svc.Spec.Type != corev1.ServiceTypeClusterIP {
				return false
			}
			return svc.Namespace == utils.WorkingNamespace && svc.Name == utils.GatewayProxyInternalService
		}))
	if err != nil {
		return err
	}
	//Watch for changes to nodes
	err = c.Watch(&source.Kind{Type: &corev1.Node{}}, &EnqueueRequestForNodeEvent{})
	if err != nil {
		return err
	}

	return nil
}

// +kubebuilder:rbac:groups=apps.openyurt.io,resources=ravendnss,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.openyurt.io,resources=ravendnss/status,verbs=get;update;patch

func (r *ReconcileDns) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	klog.V(4).Info(Format("Reconcile DNS configMap for gateway %s", req.Name))
	defer func() {
		klog.V(4).Info(Format("finished DNS configMap for gateway %s", req.Name))
	}()
	var needProxy = true
	var proxyAddress = ""
	//1. ensure configmap to record dns
	cm, err := r.getConfigMap(ctx, types.NamespacedName{Namespace: utils.WorkingNamespace, Name: utils.RavenProxyNodesConfig})
	if err != nil && !apierrors.IsNotFound(err) {
		return reconcile.Result{}, err
	}
	if apierrors.IsNotFound(err) {
		if err = r.buildRavenDNSConfigMap(); err != nil {
			return reconcile.Result{Requeue: true}, err
		}
		cm, err = r.getConfigMap(ctx, types.NamespacedName{Namespace: utils.WorkingNamespace, Name: utils.RavenProxyNodesConfig})
		if err != nil {
			return reconcile.Result{Requeue: true}, err
		}
	}
	// 2. acquired raven global config to check whether the proxy s enabled
	ravenConfig, err := r.getConfigMap(ctx, types.NamespacedName{Namespace: utils.WorkingNamespace, Name: utils.RavenGlobalConfig})
	if err != nil && !apierrors.IsNotFound(err) {
		klog.V(2).Infof(Format("failed to get configmap %s/%s", utils.WorkingNamespace, utils.RavenGlobalConfig))
		return reconcile.Result{}, err
	}
	if apierrors.IsNotFound(err) || ravenConfig.Data[utils.RavenEnableProxy] != "true" {
		r.recorder.Event(cm.DeepCopy(), corev1.EventTypeNormal, "MaintainDNSRecord", "The Raven Layer 7 proxy feature is not enabled for the cluster")
		needProxy = false
	}
	//3. acquired cluster IP from x-raven-proxy-internal-svc
	svc, err := r.getService(ctx, types.NamespacedName{Namespace: utils.WorkingNamespace, Name: utils.GatewayProxyInternalService})
	if err != nil && !apierrors.IsNotFound(err) {
		klog.V(2).Infof(Format("failed to get service %s/%s", utils.WorkingNamespace, utils.GatewayProxyInternalService))
		return reconcile.Result{}, err
	}
	if apierrors.IsNotFound(err) || svc.DeletionTimestamp != nil {
		r.recorder.Event(cm.DeepCopy(), corev1.EventTypeNormal, "MaintainDNSRecord",
			fmt.Sprintf("The Raven Layer 7 proxy lacks service %s/%s", utils.WorkingNamespace, utils.GatewayProxyInternalService))
		needProxy = false
	}
	if svc != nil {
		if svc.Spec.ClusterIP == "" {
			r.recorder.Event(cm.DeepCopy(), corev1.EventTypeNormal, "MaintainDNSRecord",
				fmt.Sprintf("The service %s/%s cluster IP is empty", utils.WorkingNamespace, utils.GatewayProxyInternalService))
			return reconcile.Result{}, err
		} else {
			proxyAddress = svc.Spec.ClusterIP
		}
	}
	//4. update dns record
	nodeList := new(corev1.NodeList)
	err = r.Client.List(ctx, nodeList, &client.ListOptions{})
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to list node, error %s", err.Error())
	}
	cm.Data[utils.ProxyNodesKey] = buildDNSRecords(nodeList, needProxy, proxyAddress)
	err = r.updateDNS(cm)
	if err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("failed to update configmap %s/%s, error %s",
			cm.GetNamespace(), cm.GetName(), err.Error())
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileDns) buildRavenDNSConfigMap() error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.RavenProxyNodesConfig,
			Namespace: utils.WorkingNamespace,
		},
		Data: map[string]string{
			utils.ProxyNodesKey: "",
		},
	}
	err := r.Client.Create(context.TODO(), cm, &client.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create ConfigMap %s/%s, error %s", cm.GetNamespace(), cm.GetName(), err.Error())
	}
	return nil
}

func (r *ReconcileDns) getConfigMap(ctx context.Context, objectKey client.ObjectKey) (*corev1.ConfigMap, error) {
	cm := corev1.ConfigMap{}
	err := r.Client.Get(ctx, objectKey, &cm)
	if err != nil {
		return nil, err
	}
	return cm.DeepCopy(), nil
}

func (r *ReconcileDns) getService(ctx context.Context, objectKey client.ObjectKey) (*corev1.Service, error) {
	svc := corev1.Service{}
	err := r.Client.Get(ctx, objectKey, &svc)
	if err != nil {
		return nil, err
	}
	return svc.DeepCopy(), nil
}

func (r *ReconcileDns) updateDNS(cm *corev1.ConfigMap) error {
	err := r.Client.Update(context.TODO(), cm, &client.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update configmap %s/%s, %s", cm.GetNamespace(), cm.GetName(), err.Error())
	}
	return nil
}

func buildDNSRecords(nodeList *corev1.NodeList, needProxy bool, proxyIp string) string {
	// record node name <-> ip address
	var err error
	dns := make([]string, 0, len(nodeList.Items))
	for _, node := range nodeList.Items {
		ip := proxyIp
		if !needProxy {
			ip, err = getHostIP(&node)
			if err != nil {
				klog.Errorf("failed to parse node address for %s, %s", node.Name, err.Error())
				continue
			}
		}
		dns = append(dns, fmt.Sprintf("%s\t%s", ip, node.Name))
	}
	sort.Strings(dns)
	return strings.Join(dns, "\n")
}

func getHostIP(node *corev1.Node) (string, error) {
	// get InternalIPs first and then ExternalIPs
	var internalIP, externalIP net.IP
	for _, addr := range node.Status.Addresses {
		switch addr.Type {
		case corev1.NodeInternalIP:
			ip := net.ParseIP(addr.Address)
			if ip != nil {
				return ip.String(), nil
			}
		case corev1.NodeExternalIP:
			ip := net.ParseIP(addr.Address)
			if ip != nil {
				externalIP = ip
			}
		}
	}
	if internalIP == nil && externalIP == nil {
		return "", fmt.Errorf("host IP unknown; known addresses: %v", node.Status.Addresses)
	}
	return externalIP.String(), nil
}
