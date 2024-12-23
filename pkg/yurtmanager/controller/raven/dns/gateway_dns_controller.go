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
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	yurtClient "github.com/openyurtio/openyurt/cmd/yurt-manager/app/client"
	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/raven/util"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.GatewayDNSController, s)
}

// Add creates a new Ravendns Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, c *appconfig.CompletedConfig, mgr manager.Manager) error {
	return add(mgr, c, newReconciler(mgr))
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
		Client:   yurtClient.GetClientByControllerNameOrDie(mgr, names.GatewayDNSController),
		scheme:   mgr.GetScheme(),
		recorder: mgr.GetEventRecorderFor(names.GatewayDNSController),
	}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, cfg *appconfig.CompletedConfig, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New(names.GatewayDNSController, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: int(cfg.ComponentConfig.GatewayDNSController.ConcurrentGatewayDNSWorkers),
	})
	if err != nil {
		return err
	}

	// Watch for changes to service
	err = c.Watch(source.Kind[client.Object](mgr.GetCache(), &corev1.Service{}, &EnqueueRequestForServiceEvent{}, predicate.NewPredicateFuncs(
		func(obj client.Object) bool {
			svc, ok := obj.(*corev1.Service)
			if !ok {
				return false
			}
			if svc.Spec.Type != corev1.ServiceTypeClusterIP {
				return false
			}
			return svc.Namespace == util.WorkingNamespace && svc.Name == util.GatewayProxyInternalService
		})))
	if err != nil {
		return err
	}
	//Watch for changes to nodes
	err = c.Watch(source.Kind[client.Object](mgr.GetCache(), &corev1.Node{}, &EnqueueRequestForNodeEvent{}))
	if err != nil {
		return err
	}

	return nil
}

// +kubebuilder:rbac:groups=core,resources=services,verbs=get
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;create;update;delete
// +kubebuilder:rbac:groups=apps.openyurt.io,resources=nodepools,verbs=get

func (r *ReconcileDns) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	klog.V(4).Info(Format("Reconcile DNS configMap for gateway %s", req.Name))
	defer func() {
		klog.V(4).Info(Format("finished DNS configMap for gateway %s", req.Name))
	}()
	var proxyAddress = ""
	//1. ensure configmap to record dns
	cm, err := r.getProxyDNS(ctx, client.ObjectKey{Namespace: util.WorkingNamespace, Name: util.RavenProxyNodesConfig})
	if err != nil {
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
	}

	// 2. acquired raven global config to check whether the proxy s enabled
	enableProxy, _ := util.CheckServer(ctx, r.Client)
	if !enableProxy {
		klog.Infoln(Format("the proxy feature is not enabled for the cluster"))
	} else {
		svc, err := r.getService(ctx, types.NamespacedName{Namespace: util.WorkingNamespace, Name: util.GatewayProxyInternalService})
		if err != nil && !apierrors.IsNotFound(err) {
			klog.Error(Format("could not get service %s/%s", util.WorkingNamespace, util.GatewayProxyInternalService))
			return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
		}
		if apierrors.IsNotFound(err) || svc.DeletionTimestamp != nil {
			klog.Infoln(Format("the proxy feature lacks service %s/%s", util.WorkingNamespace, util.GatewayProxyInternalService))
		}
		if svc != nil {
			if svc.Spec.ClusterIP == "" {
				klog.Infof("the service %s/%s cluster IP is empty", util.WorkingNamespace, util.GatewayProxyInternalService)
			} else {
				proxyAddress = svc.Spec.ClusterIP
			}
		}
	}

	//3. update dns record
	nodeList := corev1.NodeList{}
	err = r.Client.List(ctx, &nodeList, &client.ListOptions{})
	if err != nil {
		klog.Error(Format("could not list node, error %s", err.Error()))
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
	}
	cm.Data[util.ProxyNodesKey] = buildDNSRecords(&nodeList, enableProxy, proxyAddress)
	err = r.updateDNS(cm)
	if err != nil {
		klog.Error(Format("could not update configmap %s/%s, error %s",
			cm.GetNamespace(), cm.GetName(), err.Error()))
		return reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second}, err
	}
	return reconcile.Result{}, nil
}

func (r ReconcileDns) getProxyDNS(ctx context.Context, objKey client.ObjectKey) (*corev1.ConfigMap, error) {
	var cm corev1.ConfigMap
	waitErr := wait.PollUntilContextTimeout(ctx, 5*time.Second, time.Minute, true, func(ctx context.Context) (done bool, err error) {
		err = r.Client.Get(ctx, objKey, &cm)
		if err != nil {
			if apierrors.IsNotFound(err) {
				err = r.buildRavenDNSConfigMap()
				if err != nil {
					klog.Error(err.Error())
				}
			} else {
				klog.Error(Format("could not get configmap %s, error %s", objKey.String(), err.Error()))
			}
			return false, nil
		}
		return true, nil
	})

	if waitErr != nil {
		return nil, fmt.Errorf("could not get ConfigMap %s, error %s", objKey.String(), waitErr.Error())
	}
	return cm.DeepCopy(), nil
}

func (r *ReconcileDns) buildRavenDNSConfigMap() error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.RavenProxyNodesConfig,
			Namespace: util.WorkingNamespace,
		},
		Data: map[string]string{
			util.ProxyNodesKey: "",
		},
	}
	err := r.Client.Create(context.TODO(), cm, &client.CreateOptions{})
	if err != nil {
		return fmt.Errorf("could not create ConfigMap %s/%s, error %s", cm.GetNamespace(), cm.GetName(), err.Error())
	}
	return nil
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
		return fmt.Errorf("could not update configmap %s/%s, %s", cm.GetNamespace(), cm.GetName(), err.Error())
	}
	return nil
}

func buildDNSRecords(nodeList *corev1.NodeList, needProxy bool, proxyIp string) string {
	// record node name <-> ip address
	if needProxy && proxyIp == "" {
		klog.Infoln(Format("internal proxy address is empty for dns record, redirect node internal address"))
		needProxy = false
	}
	var err error
	dns := make([]string, 0, len(nodeList.Items))
	for _, node := range nodeList.Items {
		ip := proxyIp
		if !needProxy {
			ip, err = getHostIP(&node)
			if err != nil {
				klog.Error(Format("could not parse node address for %s, %s", node.Name, err.Error()))
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
