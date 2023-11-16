/*
Copyright 2021 The OpenYurt Authors.

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

package dns

import (
	"context"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	clientset "k8s.io/client-go/kubernetes"
	corelister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/constants"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/util"
)

const (
	maxRetries    = 15
	minSyncPeriod = 30

	dnatPortPrefix    = "dnat-"
	dnsControllerName = "tunnel-dns-controller"
)

var (
	yurttunnelDNSRecordConfigMapName = GetYurtTunnelDNSRecordConfigMapName()
)

func GetYurtTunnelDNSRecordConfigMapName() string {
	return fmt.Sprintf(constants.YurttunnelDNSRecordConfigMapName,
		strings.TrimRightFunc(projectinfo.GetProjectPrefix(), func(c rune) bool { return c == '-' }))
}

// DNSRecordController interface defines the method for synchronizing
// the node dns records with k8s DNS component(such as CoreDNS)
type DNSRecordController interface {
	Run(stopCh <-chan struct{})
}

// coreDNSRecordController implements the DNSRecordController
type coreDNSRecordController struct {
	lock                 sync.Mutex
	kubeClient           clientset.Interface
	sharedInformerFactor informers.SharedInformerFactory
	nodeLister           corelister.NodeLister
	nodeListerSynced     cache.InformerSynced
	svcInformerSynced    cache.InformerSynced
	cmInformerSynced     cache.InformerSynced
	queue                workqueue.RateLimitingInterface
	tunnelServerIP       string
	syncPeriod           int
	listenInsecureAddr   string
	listenSecureAddr     string
}

// NewCoreDNSRecordController create a CoreDNSRecordController that synchronizes node dns records with CoreDNS configuration
func NewCoreDNSRecordController(client clientset.Interface,
	informerFactory informers.SharedInformerFactory,
	listenInsecureAddr string,
	listenSecureAddr string,
	syncPeriod int) (DNSRecordController, error) {
	dnsctl := &coreDNSRecordController{
		kubeClient:           client,
		syncPeriod:           syncPeriod,
		listenInsecureAddr:   listenInsecureAddr,
		listenSecureAddr:     listenSecureAddr,
		sharedInformerFactor: informerFactory,
		queue:                workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "tunnel-dns"),
	}

	nodeInformer := informerFactory.Core().V1().Nodes()
	nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    dnsctl.addNode,
		UpdateFunc: dnsctl.updateNode,
		DeleteFunc: dnsctl.deleteNode,
	})
	dnsctl.nodeLister = nodeInformer.Lister()
	dnsctl.nodeListerSynced = nodeInformer.Informer().HasSynced

	svcInformer := informerFactory.Core().V1().Services().Informer()
	svcInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    dnsctl.addService,
		UpdateFunc: dnsctl.updateService,
		DeleteFunc: dnsctl.deleteService,
	})
	dnsctl.svcInformerSynced = svcInformer.HasSynced

	cmInformer := informerFactory.Core().V1().ConfigMaps().Informer()
	cmInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    dnsctl.addConfigMap,
		UpdateFunc: dnsctl.updateConfigMap,
		DeleteFunc: dnsctl.deleteConfigMap,
	})
	dnsctl.cmInformerSynced = cmInformer.HasSynced

	// override syncPeriod when the specified value is too small
	if dnsctl.syncPeriod < minSyncPeriod {
		dnsctl.syncPeriod = minSyncPeriod
	}

	return dnsctl, nil
}

func (dnsctl *coreDNSRecordController) Run(stopCh <-chan struct{}) {
	electionChecker := leaderelection.NewLeaderHealthzAdaptor(time.Second * 20)
	id, err := os.Hostname()
	if err != nil {
		klog.Fatalf("could not get hostname, %v", err)
	}
	rl, err := resourcelock.New("leases", metav1.NamespaceSystem, dnsControllerName,
		dnsctl.kubeClient.CoreV1(),
		dnsctl.kubeClient.CoordinationV1(),
		resourcelock.ResourceLockConfig{
			Identity: id + "_" + string(uuid.NewUUID()),
		})
	if err != nil {
		klog.Fatalf("error creating tunnel-dns-controller lock, %v", err)
	}

	leaderelection.RunOrDie(context.TODO(), leaderelection.LeaderElectionConfig{
		Lock:          rl,
		LeaseDuration: metav1.Duration{Duration: time.Second * time.Duration(15)}.Duration,
		RenewDeadline: metav1.Duration{Duration: time.Second * time.Duration(10)}.Duration,
		RetryPeriod:   metav1.Duration{Duration: time.Second * time.Duration(2)}.Duration,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				dnsctl.run(stopCh)
			},
			OnStoppedLeading: func() {
				klog.Fatalf("leaderelection lost")
			},
		},
		WatchDog: electionChecker,
		Name:     dnsControllerName,
	})
	panic("unreachable")
}

func (dnsctl *coreDNSRecordController) run(stopCh <-chan struct{}) {
	defer utilruntime.HandleCrash()
	defer dnsctl.queue.ShutDown()

	klog.Infof("starting tunnel dns controller")
	defer klog.Infof("shutting down tunnel dns controller")

	if !cache.WaitForNamedCacheSync(dnsControllerName, stopCh,
		dnsctl.nodeListerSynced, dnsctl.svcInformerSynced, dnsctl.cmInformerSynced) {
		return
	}

	if err := dnsctl.ensureCoreDNSRecordConfigMap(); err != nil {
		klog.Errorf("could not ensure dns record ConfigMap %v/%v, %v",
			constants.YurttunnelDNSRecordConfigMapNs, yurttunnelDNSRecordConfigMapName, err)
		return
	}

	go wait.Until(dnsctl.worker, time.Second, stopCh)

	// sync dns hosts as a whole
	go wait.Until(func() {
		if err := dnsctl.syncDNSRecordAsWhole(); err != nil {
			klog.Errorf("could not sync dns record, %v", err)
		}
	}, time.Duration(dnsctl.syncPeriod)*time.Second, stopCh)

	// sync tunnel server svc
	go wait.Until(func() {
		if err := dnsctl.syncTunnelServerServiceAsWhole(); err != nil {
			klog.Errorf("could not sync tunnel server service, %v", err)
		}
	}, time.Duration(dnsctl.syncPeriod)*time.Second, stopCh)

	<-stopCh
}

func (dnsctl *coreDNSRecordController) enqueue(obj interface{}, eventType EventType) {
	e := &Event{
		Obj:  obj,
		Type: eventType,
	}
	dnsctl.queue.Add(e)
}

func (dnsctl *coreDNSRecordController) worker() {
	for dnsctl.processNextWorkItem() {
	}
}

func (dnsctl *coreDNSRecordController) processNextWorkItem() bool {
	event, quit := dnsctl.queue.Get()
	if quit {
		return false
	}
	defer dnsctl.queue.Done(event)

	err := dnsctl.dispatch(event.(*Event))
	dnsctl.handleErr(err, event)

	return true
}

func (dnsctl *coreDNSRecordController) dispatch(event *Event) error {
	switch event.Type {
	case NodeAdd:
		return dnsctl.onNodeAdd(event.Obj.(*corev1.Node))
	case NodeUpdate:
		return dnsctl.onNodeUpdate(event.Obj.(*corev1.Node))
	case NodeDelete:
		return dnsctl.onNodeDelete(event.Obj.(*corev1.Node))
	case ServiceAdd:
		return dnsctl.onServiceAdd(event.Obj.(*corev1.Service))
	case ServiceUpdate:
		return dnsctl.onServiceUpdate(event.Obj.(*corev1.Service))
	case ServiceDelete:
		return dnsctl.onServiceDelete(event.Obj.(*corev1.Service))
	case ConfigMapAdd:
		return dnsctl.onConfigMapAdd(event.Obj.(*corev1.ConfigMap))
	case ConfigMapUpdate:
		return dnsctl.onConfigMapUpdate(event.Obj.(*corev1.ConfigMap))
	case ConfigMapDelete:
		return dnsctl.onConfigMapDelete(event.Obj.(*corev1.ConfigMap))
	default:
		return nil
	}
}

func (dnsctl *coreDNSRecordController) handleErr(err error, event interface{}) {
	if err == nil {
		dnsctl.queue.Forget(event)
		return
	}

	if dnsctl.queue.NumRequeues(event) < maxRetries {
		klog.Infof("error syncing event %v: %v", event, err)
		dnsctl.queue.AddRateLimited(event)
		return
	}

	utilruntime.HandleError(err)
	klog.Infof("dropping event %q out of the queue: %v", event, err)
	dnsctl.queue.Forget(event)
}

func (dnsctl *coreDNSRecordController) ensureCoreDNSRecordConfigMap() error {
	_, err := dnsctl.kubeClient.CoreV1().ConfigMaps(constants.YurttunnelServerServiceNs).
		Get(context.Background(), yurttunnelDNSRecordConfigMapName, metav1.GetOptions{})
	if err != nil && apierrors.IsNotFound(err) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      yurttunnelDNSRecordConfigMapName,
				Namespace: constants.YurttunnelServerServiceNs,
			},
			Data: map[string]string{
				constants.YurttunnelDNSRecordNodeDataKey: "",
			},
		}
		_, err = dnsctl.kubeClient.CoreV1().ConfigMaps(constants.YurttunnelServerServiceNs).Create(context.Background(), cm, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("could not create ConfigMap %v/%v, %w",
				constants.YurttunnelServerServiceNs, yurttunnelDNSRecordConfigMapName, err)
		}
	}
	return err
}

func (dnsctl *coreDNSRecordController) syncTunnelServerServiceAsWhole() error {
	klog.V(2).Info("sync tunnel server service as whole")
	dnatPorts, portMappings, err := util.GetConfiguredProxyPortsAndMappings(dnsctl.kubeClient, dnsctl.listenInsecureAddr, dnsctl.listenSecureAddr)
	if err != nil {
		return err
	}
	return dnsctl.updateTunnelServerSvcDnatPorts(dnatPorts, portMappings)
}

func (dnsctl *coreDNSRecordController) syncDNSRecordAsWhole() error {
	klog.V(2).Info("sync dns record as whole")

	dnsctl.lock.Lock()
	defer dnsctl.lock.Unlock()

	tunnelServerIP, err := dnsctl.getTunnelServerIP(false)
	if err != nil {
		klog.Errorf("could not sync dns record as whole, %v", err)
		return err
	}

	nodes, err := dnsctl.nodeLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("could not sync dns record as whole, %v", err)
		return err
	}

	records := make([]string, 0, len(nodes))
	for i := range nodes {
		ip, node := tunnelServerIP, nodes[i]
		if !isEdgeNode(node) {
			ip, err = getNodeHostIP(node)
			if err != nil {
				klog.Warningf("could not parse node address for %v, %v", node.Name, err)
				continue
			}
		}
		records = append(records, formatDNSRecord(ip, node.Name))
	}

	if err := dnsctl.updateDNSRecords(records); err != nil {
		klog.Errorf("could not sync dns record as whole, %v", err)
		return err
	}
	return nil
}

func (dnsctl *coreDNSRecordController) getTunnelServerIP(useCache bool) (string, error) {
	if useCache && len(dnsctl.tunnelServerIP) != 0 {
		return dnsctl.tunnelServerIP, nil
	}

	svc, err := dnsctl.kubeClient.CoreV1().Services(constants.YurttunnelServerServiceNs).
		Get(context.Background(), constants.YurttunnelServerInternalServiceName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("could not get %v/%v service, %w",
			constants.YurttunnelServerServiceNs, constants.YurttunnelServerInternalServiceName, err)
	}
	if len(svc.Spec.ClusterIP) == 0 {
		return "", fmt.Errorf("unable find ClusterIP from %s/%s service, %w",
			constants.YurttunnelServerServiceNs, constants.YurttunnelServerInternalServiceName, err)
	}

	// cache result
	dnsctl.tunnelServerIP = svc.Spec.ClusterIP

	return dnsctl.tunnelServerIP, nil
}

func (dnsctl *coreDNSRecordController) updateDNSRecords(records []string) error {
	// keep sorted
	sort.Strings(records)

	cm, err := dnsctl.kubeClient.CoreV1().ConfigMaps(constants.YurttunnelServerServiceNs).
		Get(context.Background(), yurttunnelDNSRecordConfigMapName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	cm.Data[constants.YurttunnelDNSRecordNodeDataKey] = strings.Join(records, "\n")
	if _, err := dnsctl.kubeClient.CoreV1().ConfigMaps(constants.YurttunnelServerServiceNs).Update(context.Background(), cm, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("could not update configmap %v/%v, %w",
			constants.YurttunnelServerServiceNs, yurttunnelDNSRecordConfigMapName, err)
	}
	return nil
}

func (dnsctl *coreDNSRecordController) updateTunnelServerSvcDnatPorts(ports []string, portMappings map[string]string) error {
	svc, err := dnsctl.kubeClient.CoreV1().Services(constants.YurttunnelServerServiceNs).
		Get(context.Background(), constants.YurttunnelServerInternalServiceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("could not sync tunnel server internal service, %w", err)
	}

	changed, updatedSvcPorts := resolveServicePorts(svc, ports, portMappings)
	if !changed {
		return nil
	}

	svc.Spec.Ports = updatedSvcPorts
	_, err = dnsctl.kubeClient.CoreV1().Services(constants.YurttunnelServerServiceNs).Update(context.Background(), svc, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("could not sync tunnel server service, %w", err)
	}
	return nil
}

// resolveServicePorts get service ports from specified service and ports.
func resolveServicePorts(svc *corev1.Service, ports []string, portMappings map[string]string) (bool, []corev1.ServicePort) {
	changed := false

	svcPortMap := make(map[string]corev1.ServicePort)
	for i := range svc.Spec.Ports {
		port := svc.Spec.Ports[i]
		svcPortMap[fmt.Sprintf("%s:%d", port.Protocol, port.Port)] = port
	}

	dnatPortMap := make(map[string]bool)
	for _, dnatPort := range ports {
		portInt, err := strconv.Atoi(dnatPort)
		if err != nil {
			klog.Errorf("could not parse dnat port %q, %v", dnatPort, err)
			continue
		}

		dst, ok := portMappings[dnatPort]
		if !ok {
			klog.Errorf("could not find proxy destination for port: %s", dnatPort)
			continue
		}

		_, targetPort, err := net.SplitHostPort(dst)
		if err != nil {
			klog.Errorf("could not split target port, %v", err)
			continue
		}
		targetPortInt, err := strconv.Atoi(targetPort)
		if err != nil {
			klog.Errorf("could not parse target port, %v", err)
			continue
		}

		tcpPort := fmt.Sprintf("%s:%s", corev1.ProtocolTCP, dnatPort)
		dnatPortMap[tcpPort] = true

		p, ok := svcPortMap[tcpPort]
		// new port or has not tcp protocol port, add a new port for service
		if !ok {
			svcPortMap[tcpPort] = corev1.ServicePort{
				Name:       fmt.Sprintf("%v%v", dnatPortPrefix, dnatPort),
				Port:       int32(portInt),
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.FromInt(targetPortInt),
			}
			changed = true
		} else if p.TargetPort.String() != targetPort { // target port is changed, overwrite the old port in service
			svcPortMap[tcpPort] = corev1.ServicePort{
				Name:       p.Name,
				Port:       p.Port,
				Protocol:   p.Protocol,
				TargetPort: intstr.FromInt(targetPortInt),
			}
			changed = true
		}
	}

	updatedSvcPorts := make([]corev1.ServicePort, 0, len(svc.Spec.Ports))
	for tcpPort, svcPort := range svcPortMap {
		if strings.HasPrefix(tcpPort, string(corev1.ProtocolTCP)) &&
			strings.HasPrefix(svcPort.Name, dnatPortPrefix) &&
			!dnatPortMap[tcpPort] {
			changed = true
			continue
		}
		updatedSvcPorts = append(updatedSvcPorts, svcPort)
	}

	return changed, updatedSvcPorts
}
