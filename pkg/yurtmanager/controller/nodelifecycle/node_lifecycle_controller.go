/*
Copyright 2017 The Kubernetes Authors.
Copyright 2023 The OpenYurt Authors.

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

// The Controller sets tainted annotations on nodes.
// Tainted nodes should not be used for new work loads and
// some effort should be given to getting existing work
// loads off of tainted nodes.

package nodelifecycle

import (
	"context"
	"fmt"
	"sync"
	"time"

	apps "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	coordv1 "k8s.io/api/coordination/v1"
	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/flowcontrol"
	"k8s.io/client-go/util/workqueue"
	nodetopology "k8s.io/component-helpers/node/topology"
	"k8s.io/klog/v2"
	kubeletapis "k8s.io/kubelet/pkg/apis"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	taintutils "github.com/openyurtio/openyurt/pkg/util/taints"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/nodelifecycle/scheduler"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util"
	controllerutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/node"
	nodeutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/node"
)

func init() {
	// Register prometheus metrics
	Register()
}

var (
	// UnreachableTaintTemplate is the taint for when a node becomes unreachable.
	UnreachableTaintTemplate = &v1.Taint{
		Key:    v1.TaintNodeUnreachable,
		Effect: v1.TaintEffectNoExecute,
	}

	// NotReadyTaintTemplate is the taint for when a node is not ready for
	// executing pods
	NotReadyTaintTemplate = &v1.Taint{
		Key:    v1.TaintNodeNotReady,
		Effect: v1.TaintEffectNoExecute,
	}

	// map {NodeConditionType: {ConditionStatus: TaintKey}}
	// represents which NodeConditionType under which ConditionStatus should be
	// tainted with which TaintKey
	// for certain NodeConditionType, there are multiple {ConditionStatus,TaintKey} pairs
	nodeConditionToTaintKeyStatusMap = map[v1.NodeConditionType]map[v1.ConditionStatus]string{
		v1.NodeReady: {
			v1.ConditionFalse:   v1.TaintNodeNotReady,
			v1.ConditionUnknown: v1.TaintNodeUnreachable,
		},
		v1.NodeMemoryPressure: {
			v1.ConditionTrue: v1.TaintNodeMemoryPressure,
		},
		v1.NodeDiskPressure: {
			v1.ConditionTrue: v1.TaintNodeDiskPressure,
		},
		v1.NodeNetworkUnavailable: {
			v1.ConditionTrue: v1.TaintNodeNetworkUnavailable,
		},
		v1.NodePIDPressure: {
			v1.ConditionTrue: v1.TaintNodePIDPressure,
		},
	}

	taintKeyToNodeConditionMap = map[string]v1.NodeConditionType{
		v1.TaintNodeNotReady:           v1.NodeReady,
		v1.TaintNodeUnreachable:        v1.NodeReady,
		v1.TaintNodeNetworkUnavailable: v1.NodeNetworkUnavailable,
		v1.TaintNodeMemoryPressure:     v1.NodeMemoryPressure,
		v1.TaintNodeDiskPressure:       v1.NodeDiskPressure,
		v1.TaintNodePIDPressure:        v1.NodePIDPressure,
	}
)

// ZoneState is the state of a given zone.
type ZoneState string

const (
	stateInitial           = ZoneState("Initial")
	stateNormal            = ZoneState("Normal")
	stateFullDisruption    = ZoneState("FullDisruption")
	statePartialDisruption = ZoneState("PartialDisruption")
)

const (
	// The amount of time the nodecontroller should sleep between retrying node health updates
	retrySleepTime   = 20 * time.Millisecond
	nodeNameKeyIndex = "spec.nodeName"
	// podUpdateWorkerSizes assumes that in most cases pod will be handled by monitorNodeHealth pass.
	// Pod update workers will only handle lagging cache pods. 4 workers should be enough.
	podUpdateWorkerSize = 4
)

// labelReconcileInfo lists Node labels to reconcile, and how to reconcile them.
// primaryKey and secondaryKey are keys of labels to reconcile.
//   - If both keys exist, but their values don't match. Use the value from the
//     primaryKey as the source of truth to reconcile.
//   - If ensureSecondaryExists is true, and the secondaryKey does not
//     exist, secondaryKey will be added with the value of the primaryKey.
var labelReconcileInfo = []struct {
	primaryKey            string
	secondaryKey          string
	ensureSecondaryExists bool
}{
	{
		// Reconcile the beta and the stable OS label using the stable label as the source of truth.
		// TODO(#89477): no earlier than 1.22: drop the beta labels if they differ from the GA labels
		primaryKey:            v1.LabelOSStable,
		secondaryKey:          kubeletapis.LabelOS,
		ensureSecondaryExists: true,
	},
	{
		// Reconcile the beta and the stable arch label using the stable label as the source of truth.
		// TODO(#89477): no earlier than 1.22: drop the beta labels if they differ from the GA labels
		primaryKey:            v1.LabelArchStable,
		secondaryKey:          kubeletapis.LabelArch,
		ensureSecondaryExists: true,
	},
}

type nodeHealthData struct {
	probeTimestamp           metav1.Time
	readyTransitionTimestamp metav1.Time
	status                   *v1.NodeStatus
	lease                    *coordv1.Lease
}

func (n *nodeHealthData) deepCopy() *nodeHealthData {
	if n == nil {
		return nil
	}
	return &nodeHealthData{
		probeTimestamp:           n.probeTimestamp,
		readyTransitionTimestamp: n.readyTransitionTimestamp,
		status:                   n.status.DeepCopy(),
		lease:                    n.lease.DeepCopy(),
	}
}

type nodeHealthMap struct {
	lock        sync.RWMutex
	nodeHealths map[string]*nodeHealthData
}

func newNodeHealthMap() *nodeHealthMap {
	return &nodeHealthMap{
		nodeHealths: make(map[string]*nodeHealthData),
	}
}

// getDeepCopy - returns copy of node health data.
// It prevents data being changed after retrieving it from the map.
func (n *nodeHealthMap) getDeepCopy(name string) *nodeHealthData {
	n.lock.RLock()
	defer n.lock.RUnlock()
	return n.nodeHealths[name].deepCopy()
}

func (n *nodeHealthMap) set(name string, data *nodeHealthData) {
	n.lock.Lock()
	defer n.lock.Unlock()
	n.nodeHealths[name] = data
}

type podUpdateItem struct {
	namespace string
	name      string
}

// ReconcileNodeLifeCycle is the controller that manages node's life cycle.
type ReconcileNodeLifeCycle struct {
	controllerRuntimeClient client.Client
	taintManager            *scheduler.NoExecuteTaintManager
	kubeClient              clientset.Interface

	// This timestamp is to be used instead of LastProbeTime stored in Condition. We do this
	// to avoid the problem with time skew across the cluster.
	now func() metav1.Time

	enterPartialDisruptionFunc func(nodeNum int) float32
	enterFullDisruptionFunc    func(nodeNum int) float32
	computeZoneStateFunc       func(nodeConditions []*v1.NodeCondition) (int, ZoneState)

	knownNodeSet map[string]*v1.Node
	// per Node map storing last observed health together with a local time when it was observed.
	nodeHealthMap *nodeHealthMap

	// evictorLock protects zonePodEvictor and zoneNoExecuteTainter.
	evictorLock sync.Mutex
	// workers that are responsible for tainting nodes.
	zoneNoExecuteTainter map[string]*scheduler.RateLimitedTimedQueue

	nodesToRetry sync.Map

	zoneStates map[string]ZoneState

	getPodsAssignedToNode func(nodeName string) ([]*v1.Pod, error)

	recorder record.EventRecorder

	// Value controlling Controller monitoring period, i.e. how often does Controller
	// check node health signal posted from kubelet. This value should be lower than
	// nodeMonitorGracePeriod.
	// TODO: Change node health monitor to watch based.
	nodeMonitorPeriod time.Duration

	// When node is just created, e.g. cluster bootstrap or node creation, we give
	// a longer grace period.
	nodeStartupGracePeriod time.Duration

	// Controller will not proactively sync node health, but will monitor node
	// health signal updated from kubelet. There are 2 kinds of node healthiness
	// signals: NodeStatus and NodeLease. If it doesn't receive update for this amount
	// of time, it will start posting "NodeReady==ConditionUnknown". The amount of
	// time before which Controller start evicting pods is controlled via flag
	// 'pod-eviction-timeout'.
	// Note: be cautious when changing the constant, it must work with
	// nodeStatusUpdateFrequency in kubelet and renewInterval in NodeLease
	// controller. The node health signal update frequency is the minimal of the
	// two.
	// There are several constraints:
	// 1. nodeMonitorGracePeriod must be N times more than  the node health signal
	//    update frequency, where N means number of retries allowed for kubelet to
	//    post node status/lease. It is pointless to make nodeMonitorGracePeriod
	//    be less than the node health signal update frequency, since there will
	//    only be fresh values from Kubelet at an interval of node health signal
	//    update frequency.
	// 2. nodeMonitorGracePeriod can't be too large for user experience - larger
	//    value takes longer for user to see up-to-date node health.
	nodeMonitorGracePeriod time.Duration

	// Number of workers Controller uses to process node monitor health updates.
	// Defaults to scheduler.UpdateWorkerSize.
	nodeUpdateWorkerSize int

	evictionLimiterQPS          float32
	secondaryEvictionLimiterQPS float32
	largeClusterThreshold       int32
	unhealthyZoneThreshold      float32

	nodeUpdateQueue workqueue.Interface
	podUpdateQueue  workqueue.RateLimitingInterface
}

// +kubebuilder:rbac:groups=core,resources=nodes/status,verbs=get;list;watch;update;patch

// Add creates a new CsrApprover Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(ctx context.Context, cfg *appconfig.CompletedConfig, mgr manager.Manager) error {
	nc, err := newReconciler(cfg, mgr)
	if err != nil {
		return err
	}
	// Create a new controller
	c, err := util.NewNoReconcileController(names.NodeLifeCycleController, mgr, controller.Options{})
	if err != nil {
		return err
	}

	podsPredicate := predicate.Funcs{
		CreateFunc: func(evt event.CreateEvent) bool {
			pod := evt.Object.(*v1.Pod)
			nc.podUpdated(nil, pod)
			if nc.taintManager != nil {
				nc.taintManager.PodUpdated(nil, pod)
			}
			return false
		},
		UpdateFunc: func(evt event.UpdateEvent) bool {
			prevPod := evt.ObjectOld.(*v1.Pod)
			newPod := evt.ObjectNew.(*v1.Pod)
			nc.podUpdated(prevPod, newPod)
			if nc.taintManager != nil {
				nc.taintManager.PodUpdated(prevPod, newPod)
			}
			return false
		},
		DeleteFunc: func(evt event.DeleteEvent) bool {
			pod := evt.Object.(*v1.Pod)
			nc.podUpdated(pod, nil)
			if nc.taintManager != nil {
				nc.taintManager.PodUpdated(pod, nil)
			}
			return false
		},
		GenericFunc: func(evt event.GenericEvent) bool {
			return false
		},
	}
	c.Watch(&source.Kind{Type: &v1.Pod{}}, &handler.Funcs{}, podsPredicate)

	nc.taintManager = scheduler.NewNoExecuteTaintManager(nc.recorder, nc.controllerRuntimeClient, nc.getPodsAssignedToNode)
	nodesTaintManagerPredicate := predicate.Funcs{
		CreateFunc: func(evt event.CreateEvent) bool {
			node := evt.Object.(*v1.Node).DeepCopy()
			nc.taintManager.NodeUpdated(nil, node)
			return false
		},
		UpdateFunc: func(evt event.UpdateEvent) bool {
			oldNode := evt.ObjectOld.(*v1.Node).DeepCopy()
			newNode := evt.ObjectNew.(*v1.Node).DeepCopy()
			nc.taintManager.NodeUpdated(oldNode, newNode)
			return false
		},
		DeleteFunc: func(evt event.DeleteEvent) bool {
			node := evt.Object.(*v1.Node).DeepCopy()
			nc.taintManager.NodeUpdated(node, nil)
			return false
		},
		GenericFunc: func(evt event.GenericEvent) bool {
			return false
		},
	}
	c.Watch(&source.Kind{Type: &v1.Node{}}, &handler.Funcs{}, nodesTaintManagerPredicate)

	nodesUpdateQueuePredicate := predicate.Funcs{
		CreateFunc: func(evt event.CreateEvent) bool {
			node := evt.Object.(*v1.Node)
			nc.nodeUpdateQueue.Add(node.Name)
			return false
		},
		UpdateFunc: func(evt event.UpdateEvent) bool {
			newNode := evt.ObjectNew.(*v1.Node)
			nc.nodeUpdateQueue.Add(newNode.Name)
			return false
		},
		DeleteFunc: func(evt event.DeleteEvent) bool {
			node := evt.Object.(*v1.Node)
			nc.nodesToRetry.Delete(node.Name)
			return false
		},
		GenericFunc: func(evt event.GenericEvent) bool {
			return false
		},
	}
	c.Watch(&source.Kind{Type: &v1.Node{}}, &handler.Funcs{}, nodesUpdateQueuePredicate)
	c.Watch(&source.Kind{Type: &apps.DaemonSet{}}, &handler.Funcs{})
	c.Watch(&source.Kind{Type: &coordinationv1.Lease{}}, &handler.Funcs{})

	go nc.Run(ctx, c.WaitForStarted)
	return nil
}

func GenGetPodsAssignedToNode(c client.Client) func(string) ([]*v1.Pod, error) {
	return func(name string) ([]*v1.Pod, error) {
		listOptions := &client.ListOptions{
			FieldSelector: fields.SelectorFromSet(fields.Set{
				nodeNameKeyIndex: name,
			}),
		}

		podList := &v1.PodList{}
		err := c.List(context.TODO(), podList, listOptions)
		if err != nil {
			klog.Errorf("could not get podList for node(%s), %v", name, err)
			return nil, err
		}

		pods := make([]*v1.Pod, len(podList.Items))
		for i := range podList.Items {
			pods[i] = &podList.Items[i]
		}
		return pods, nil
	}
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(cfg *appconfig.CompletedConfig, mgr manager.Manager) (*ReconcileNodeLifeCycle, error) {
	nc := &ReconcileNodeLifeCycle{
		controllerRuntimeClient:     mgr.GetClient(),
		recorder:                    mgr.GetEventRecorderFor(names.NodeLifeCycleController),
		now:                         metav1.Now,
		knownNodeSet:                make(map[string]*v1.Node),
		nodeHealthMap:               newNodeHealthMap(),
		nodeUpdateWorkerSize:        scheduler.UpdateWorkerSize,
		zoneNoExecuteTainter:        make(map[string]*scheduler.RateLimitedTimedQueue),
		nodesToRetry:                sync.Map{},
		zoneStates:                  make(map[string]ZoneState),
		nodeMonitorPeriod:           metav1.Duration{Duration: 5 * time.Second}.Duration,
		nodeStartupGracePeriod:      cfg.ComponentConfig.NodeLifeCycleController.NodeStartupGracePeriod.Duration,
		nodeMonitorGracePeriod:      cfg.ComponentConfig.NodeLifeCycleController.NodeMonitorGracePeriod.Duration,
		evictionLimiterQPS:          cfg.ComponentConfig.NodeLifeCycleController.NodeEvictionRate,
		secondaryEvictionLimiterQPS: cfg.ComponentConfig.NodeLifeCycleController.SecondaryNodeEvictionRate,
		largeClusterThreshold:       cfg.ComponentConfig.NodeLifeCycleController.LargeClusterSizeThreshold,
		unhealthyZoneThreshold:      cfg.ComponentConfig.NodeLifeCycleController.UnhealthyZoneThreshold,
		nodeUpdateQueue:             workqueue.NewNamed("node_lifecycle_controller"),
		podUpdateQueue:              workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "node_lifecycle_controller_pods"),
	}
	nc.getPodsAssignedToNode = GenGetPodsAssignedToNode(nc.controllerRuntimeClient)
	nc.enterPartialDisruptionFunc = nc.ReducedQPSFunc
	nc.enterFullDisruptionFunc = nc.HealthyQPSFunc
	nc.computeZoneStateFunc = nc.ComputeZoneState
	kubeClient, err := clientset.NewForConfig(mgr.GetConfig())
	if err != nil {
		klog.Errorf("could not create kube client, %v", err)
		return nil, err
	}
	nc.kubeClient = kubeClient
	return nc, nil
}

// Run starts an asynchronous loop that monitors the status of cluster nodes.
func (nc *ReconcileNodeLifeCycle) Run(ctx context.Context, waitForControllerStarted func(ctx context.Context) bool) {
	defer utilruntime.HandleCrash()

	// Close node update queue to cleanup go routine.
	defer nc.nodeUpdateQueue.ShutDown()
	defer nc.podUpdateQueue.ShutDown()

	klog.Info("Starting node controller")
	defer klog.Info("Shutting down node controller")

	if !waitForControllerStarted(ctx) {
		return
	}

	go nc.taintManager.Run(ctx)

	// Start workers to reconcile labels and/or update NoSchedule taint for nodes.
	for i := 0; i < scheduler.UpdateWorkerSize; i++ {
		// Thanks to "workqueue", each worker just need to get item from queue, because
		// the item is flagged when got from queue: if new event come, the new item will
		// be re-queued until "Done", so no more than one worker handle the same item and
		// no event missed.
		go wait.UntilWithContext(ctx, nc.doNodeProcessingPassWorker, time.Second)
	}

	for i := 0; i < podUpdateWorkerSize; i++ {
		go wait.UntilWithContext(ctx, nc.doPodProcessingWorker, time.Second)
	}

	// Handling taint based evictions. Because we don't want a dedicated logic in TaintManager for NC-originated
	// taints and we normally don't rate limit evictions caused by taints, we need to rate limit adding taints.
	go wait.UntilWithContext(ctx, nc.doNoExecuteTaintingPass, scheduler.NodeEvictionPeriod)

	// Incorporate the results of node health signal pushed from kubelet to master.
	go wait.UntilWithContext(ctx, func(ctx context.Context) {
		if err := nc.monitorNodeHealth(ctx); err != nil {
			klog.Errorf("Error monitoring node health, %v", err)
		}
	}, nc.nodeMonitorPeriod)

	<-ctx.Done()
}

func (nc *ReconcileNodeLifeCycle) doNodeProcessingPassWorker(ctx context.Context) {
	for {
		obj, shutdown := nc.nodeUpdateQueue.Get()
		// "nodeUpdateQueue" will be shutdown when "stopCh" closed;
		// we do not need to re-check "stopCh" again.
		if shutdown {
			return
		}
		nodeName := obj.(string)
		if err := nc.doNoScheduleTaintingPass(ctx, nodeName); err != nil {
			klog.ErrorS(err, "could not taint NoSchedule on node, requeue it", "node", klog.KRef("", nodeName))
			// TODO(k82cn): Add nodeName back to the queue
		}
		// TODO: re-evaluate whether there are any labels that need to be
		// reconcile in 1.19. Remove this function if it's no longer necessary.
		if err := nc.reconcileNodeLabels(ctx, nodeName); err != nil {
			klog.ErrorS(err, "could not reconcile labels for node, requeue it", "node", klog.KRef("", nodeName))
			// TODO(yujuhong): Add nodeName back to the queue
		}
		nc.nodeUpdateQueue.Done(nodeName)
	}
}

func (nc *ReconcileNodeLifeCycle) doNoScheduleTaintingPass(ctx context.Context, nodeName string) error {
	//node, err := nc.nodeLister.Get(nodeName)
	node := new(v1.Node)
	err := nc.controllerRuntimeClient.Get(ctx, types.NamespacedName{Name: nodeName}, node)
	if err != nil {
		// If node not found, just ignore it.
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	// Map node's condition to Taints.
	var taints []v1.Taint
	for _, condition := range node.Status.Conditions {
		if taintMap, found := nodeConditionToTaintKeyStatusMap[condition.Type]; found {
			if taintKey, found := taintMap[condition.Status]; found {
				taints = append(taints, v1.Taint{
					Key:    taintKey,
					Effect: v1.TaintEffectNoSchedule,
				})
			}
		}
	}
	if node.Spec.Unschedulable {
		// If unschedulable, append related taint.
		taints = append(taints, v1.Taint{
			Key:    v1.TaintNodeUnschedulable,
			Effect: v1.TaintEffectNoSchedule,
		})
	}

	// Get exist taints of node.
	nodeTaints := taintutils.TaintSetFilter(node.Spec.Taints, func(t *v1.Taint) bool {
		// only NoSchedule taints are candidates to be compared with "taints" later
		if t.Effect != v1.TaintEffectNoSchedule {
			return false
		}
		// Find unschedulable taint of node.
		if t.Key == v1.TaintNodeUnschedulable {
			return true
		}
		// Find node condition taints of node.
		_, found := taintKeyToNodeConditionMap[t.Key]
		return found
	})
	taintsToAdd, taintsToDel := taintutils.TaintSetDiff(taints, nodeTaints)
	// If nothing to add or delete, return true directly.
	if len(taintsToAdd) == 0 && len(taintsToDel) == 0 {
		return nil
	}
	if !controllerutil.SwapNodeControllerTaint(ctx, nc.kubeClient, taintsToAdd, taintsToDel, node) {
		return fmt.Errorf("could not swap taints of node %+v", node)
	}
	return nil
}

func (nc *ReconcileNodeLifeCycle) doNoExecuteTaintingPass(ctx context.Context) {
	// Extract out the keys of the map in order to not hold
	// the evictorLock for the entire function and hold it
	// only when nescessary.
	var zoneNoExecuteTainterKeys []string
	func() {
		nc.evictorLock.Lock()
		defer nc.evictorLock.Unlock()

		zoneNoExecuteTainterKeys = make([]string, 0, len(nc.zoneNoExecuteTainter))
		for k := range nc.zoneNoExecuteTainter {
			zoneNoExecuteTainterKeys = append(zoneNoExecuteTainterKeys, k)
		}
	}()
	for _, k := range zoneNoExecuteTainterKeys {
		var zoneNoExecuteTainterWorker *scheduler.RateLimitedTimedQueue
		func() {
			nc.evictorLock.Lock()
			defer nc.evictorLock.Unlock()
			// Extracting the value without checking if the key
			// exists or not is safe to do here since zones do
			// not get removed, and consequently pod evictors for
			// these zones also do not get removed, only added.
			zoneNoExecuteTainterWorker = nc.zoneNoExecuteTainter[k]
		}()
		// Function should return 'false' and a time after which it should be retried, or 'true' if it shouldn't (it succeeded).
		zoneNoExecuteTainterWorker.Try(func(value scheduler.TimedValue) (bool, time.Duration) {
			//node, err := nc.nodeLister.Get(value.Value)
			node := new(v1.Node)
			err := nc.controllerRuntimeClient.Get(ctx, types.NamespacedName{Name: value.Value}, node)
			if apierrors.IsNotFound(err) {
				klog.InfoS("Node no longer present in nodeLister", "node", klog.KRef("", value.Value))
				return true, 0
			} else if err != nil {
				klog.InfoS("could not get Node from the nodeLister", "node", klog.KRef("", value.Value), "err", err)
				// retry in 50 millisecond
				return false, 50 * time.Millisecond
			}
			_, condition := controllerutil.GetNodeCondition(&node.Status, v1.NodeReady)
			// Because we want to mimic NodeStatus.Condition["Ready"] we make "unreachable" and "not ready" taints mutually exclusive.
			taintToAdd := v1.Taint{}
			oppositeTaint := v1.Taint{}
			switch condition.Status {
			case v1.ConditionFalse:
				taintToAdd = *NotReadyTaintTemplate
				oppositeTaint = *UnreachableTaintTemplate
			case v1.ConditionUnknown:
				taintToAdd = *UnreachableTaintTemplate
				oppositeTaint = *NotReadyTaintTemplate
			default:
				// It seems that the Node is ready again, so there's no need to taint it.
				klog.V(4).InfoS("Node was in a taint queue, but it's ready now. Ignoring taint request", "node", klog.KRef("", value.Value))
				return true, 0
			}
			result := controllerutil.SwapNodeControllerTaint(ctx, nc.kubeClient, []*v1.Taint{&taintToAdd}, []*v1.Taint{&oppositeTaint}, node)
			if result {
				// Count the number of evictions.
				zone := nodetopology.GetZoneKey(node)
				evictionsTotal.WithLabelValues(zone).Inc()
			}

			return result, 0
		})
	}
}

// monitorNodeHealth verifies node health are constantly updated by kubelet, and if not, post "NodeReady==ConditionUnknown".
// This function will
//   - add nodes which are not ready or not reachable for a long period of time to a rate-limited
//     queue so that NoExecute taints can be added by the goroutine running the doNoExecuteTaintingPass function,
//   - update the PodReady condition Pods according to the state of the Node Ready condition.
func (nc *ReconcileNodeLifeCycle) monitorNodeHealth(ctx context.Context) error {
	start := nc.now()
	defer func() {
		updateAllNodesHealthDuration.Observe(time.Since(start.Time).Seconds())
	}()

	// We are listing nodes from local cache as we can tolerate some small delays
	// comparing to state from etcd and there is eventual consistency anyway.
	//nodes, err := nc.nodeLister.List(labels.Everything())
	nodeList := new(v1.NodeList)
	err := nc.controllerRuntimeClient.List(ctx, nodeList, &client.ListOptions{})
	if err != nil {
		return err
	}
	nodes := make([]*v1.Node, len(nodeList.Items), len(nodeList.Items))
	for i := range nodeList.Items {
		nodes[i] = &nodeList.Items[i]
	}

	added, deleted, newZoneRepresentatives := nc.classifyNodes(nodes)
	for i := range newZoneRepresentatives {
		nc.addPodEvictorForNewZone(newZoneRepresentatives[i])
	}

	for i := range added {
		klog.V(1).InfoS("Controller observed a new Node", "node", klog.KRef("", added[i].Name))
		controllerutil.RecordNodeEvent(ctx, nc.recorder, added[i].Name, string(added[i].UID), v1.EventTypeNormal, "RegisteredNode", fmt.Sprintf("Registered Node %v in Controller", added[i].Name))
		nc.knownNodeSet[added[i].Name] = added[i]
		nc.addPodEvictorForNewZone(added[i])
		nc.markNodeAsReachable(ctx, added[i])
	}

	for i := range deleted {
		klog.V(1).InfoS("Controller observed a Node deletion", "node", klog.KRef("", deleted[i].Name))
		controllerutil.RecordNodeEvent(ctx, nc.recorder, deleted[i].Name, string(deleted[i].UID), v1.EventTypeNormal, "RemovingNode", fmt.Sprintf("Removing Node %v from Controller", deleted[i].Name))
		delete(nc.knownNodeSet, deleted[i].Name)
	}

	var zoneToNodeConditionsLock sync.Mutex
	zoneToNodeConditions := map[string][]*v1.NodeCondition{}
	updateNodeFunc := func(piece int) {
		start := nc.now()
		defer func() {
			updateNodeHealthDuration.Observe(time.Since(start.Time).Seconds())
		}()

		var observedReadyCondition v1.NodeCondition
		var currentReadyCondition *v1.NodeCondition
		node := nodes[piece].DeepCopy()

		if err := wait.PollImmediate(retrySleepTime, retrySleepTime*scheduler.NodeHealthUpdateRetry, func() (bool, error) {
			var err error
			_, observedReadyCondition, currentReadyCondition, err = nc.tryUpdateNodeHealth(ctx, node)
			if err == nil {
				return true, nil
			}
			name := node.Name
			node, err = nc.kubeClient.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				klog.ErrorS(err, "Failed while getting a Node to retry updating node health. Probably Node was deleted", "node", klog.KRef("", name))
				return false, err
			}
			return false, nil
		}); err != nil {
			klog.ErrorS(err, "Update health of Node from Controller error, Skipping - no pods will be evicted", "node", klog.KObj(node))
			return
		}

		// Some nodes may be excluded from disruption checking
		if !isNodeExcludedFromDisruptionChecks(node) {
			zoneToNodeConditionsLock.Lock()
			zoneToNodeConditions[nodetopology.GetZoneKey(node)] = append(zoneToNodeConditions[nodetopology.GetZoneKey(node)], currentReadyCondition)
			zoneToNodeConditionsLock.Unlock()
		}

		if currentReadyCondition != nil {
			pods, err := nc.getPodsAssignedToNode(node.Name)
			if err != nil {
				utilruntime.HandleError(fmt.Errorf("unable to list pods of node %v: %v", node.Name, err))
				if currentReadyCondition.Status != v1.ConditionTrue && observedReadyCondition.Status == v1.ConditionTrue {
					// If error happened during node status transition (Ready -> NotReady)
					// we need to mark node for retry to force MarkPodsNotReady execution
					// in the next iteration.
					nc.nodesToRetry.Store(node.Name, struct{}{})
				}
				return
			}
			nc.processTaintBaseEviction(ctx, node, &observedReadyCondition)

			_, needsRetry := nc.nodesToRetry.Load(node.Name)
			switch {
			case currentReadyCondition.Status != v1.ConditionTrue && observedReadyCondition.Status == v1.ConditionTrue:
				// Report node event only once when status changed.
				controllerutil.RecordNodeStatusChange(nc.recorder, node, "NodeNotReady")
				fallthrough
			case needsRetry && observedReadyCondition.Status != v1.ConditionTrue:
				// Ignore mark the pods NotReady if the node has bounded to node.
				if nodeutil.IsPodBoundenToNode(node) {
					return
				}

				if err = controllerutil.MarkPodsNotReady(ctx, nc.controllerRuntimeClient, nc.recorder, pods, node.Name); err != nil {
					utilruntime.HandleError(fmt.Errorf("unable to mark all pods NotReady on node %v: %v; queuing for retry", node.Name, err))
					nc.nodesToRetry.Store(node.Name, struct{}{})
					return
				}
			}
		}
		nc.nodesToRetry.Delete(node.Name)
	}

	// Marking the pods not ready on a node requires looping over them and
	// updating each pod's status one at a time. This is performed serially, and
	// can take a while if we're processing each node serially as well. So we
	// process them with bounded concurrency instead, since most of the time is
	// spent waiting on io.
	workqueue.ParallelizeUntil(ctx, nc.nodeUpdateWorkerSize, len(nodes), updateNodeFunc)

	nc.handleDisruption(ctx, zoneToNodeConditions, nodes)

	return nil
}

func (nc *ReconcileNodeLifeCycle) processTaintBaseEviction(ctx context.Context, node *v1.Node, observedReadyCondition *v1.NodeCondition) {
	decisionTimestamp := nc.now()
	// Check eviction timeout against decisionTimestamp
	switch observedReadyCondition.Status {
	case v1.ConditionFalse:
		// We want to update the taint straight away if Node is already tainted with the UnreachableTaint
		if taintutils.TaintExists(node.Spec.Taints, UnreachableTaintTemplate) {
			taintToAdd := *NotReadyTaintTemplate
			if !controllerutil.SwapNodeControllerTaint(ctx, nc.kubeClient, []*v1.Taint{&taintToAdd}, []*v1.Taint{UnreachableTaintTemplate}, node) {
				klog.ErrorS(nil, "could not instantly swap UnreachableTaint to NotReadyTaint. Will try again in the next cycle")
			}
		} else if nc.markNodeForTainting(node, v1.ConditionFalse) {
			klog.V(2).InfoS("Node is NotReady. Adding it to the Taint queue", "node", klog.KObj(node), "timeStamp", decisionTimestamp)
		}
	case v1.ConditionUnknown:
		// We want to update the taint straight away if Node is already tainted with the UnreachableTaint
		if taintutils.TaintExists(node.Spec.Taints, NotReadyTaintTemplate) {
			taintToAdd := *UnreachableTaintTemplate
			if !controllerutil.SwapNodeControllerTaint(ctx, nc.kubeClient, []*v1.Taint{&taintToAdd}, []*v1.Taint{NotReadyTaintTemplate}, node) {
				klog.ErrorS(nil, "could not instantly swap NotReadyTaint to UnreachableTaint. Will try again in the next cycle")
			}
		} else if nc.markNodeForTainting(node, v1.ConditionUnknown) {
			klog.V(2).InfoS("Node is unresponsive. Adding it to the Taint queue", "node", klog.KObj(node), "timeStamp", decisionTimestamp)
		}
	case v1.ConditionTrue:
		removed, err := nc.markNodeAsReachable(ctx, node)
		if err != nil {
			klog.ErrorS(nil, "could not remove taints from node. Will retry in next iteration", "node", klog.KObj(node))
		}
		if removed {
			klog.V(2).InfoS("Node is healthy again, removing all taints", "node", klog.KObj(node))
		}
	}
}

// labelNodeDisruptionExclusion is a label on nodes that controls whether they are
// excluded from being considered for disruption checks by the node controller.
const labelNodeDisruptionExclusion = "node.kubernetes.io/exclude-disruption"

func isNodeExcludedFromDisruptionChecks(node *v1.Node) bool {
	if _, ok := node.Labels[labelNodeDisruptionExclusion]; ok {
		return true
	}
	return false
}

// tryUpdateNodeHealth checks a given node's conditions and tries to update it. Returns grace period to
// which given node is entitled, state of current and last observed Ready Condition, and an error if it occurred.
func (nc *ReconcileNodeLifeCycle) tryUpdateNodeHealth(ctx context.Context, node *v1.Node) (time.Duration, v1.NodeCondition, *v1.NodeCondition, error) {
	nodeHealth := nc.nodeHealthMap.getDeepCopy(node.Name)
	defer func() {
		nc.nodeHealthMap.set(node.Name, nodeHealth)
	}()

	var gracePeriod time.Duration
	var observedReadyCondition v1.NodeCondition
	_, currentReadyCondition := controllerutil.GetNodeCondition(&node.Status, v1.NodeReady)
	if currentReadyCondition == nil {
		// If ready condition is nil, then kubelet (or nodecontroller) never posted node status.
		// A fake ready condition is created, where LastHeartbeatTime and LastTransitionTime is set
		// to node.CreationTimestamp to avoid handle the corner case.
		observedReadyCondition = v1.NodeCondition{
			Type:               v1.NodeReady,
			Status:             v1.ConditionUnknown,
			LastHeartbeatTime:  node.CreationTimestamp,
			LastTransitionTime: node.CreationTimestamp,
		}
		gracePeriod = nc.nodeStartupGracePeriod
		if nodeHealth != nil {
			nodeHealth.status = &node.Status
		} else {
			nodeHealth = &nodeHealthData{
				status:                   &node.Status,
				probeTimestamp:           node.CreationTimestamp,
				readyTransitionTimestamp: node.CreationTimestamp,
			}
		}
	} else {
		// If ready condition is not nil, make a copy of it, since we may modify it in place later.
		observedReadyCondition = *currentReadyCondition
		gracePeriod = nc.nodeMonitorGracePeriod
	}
	// There are following cases to check:
	// - both saved and new status have no Ready Condition set - we leave everything as it is,
	// - saved status have no Ready Condition, but current one does - Controller was restarted with Node data already present in etcd,
	// - saved status have some Ready Condition, but current one does not - it's an error, but we fill it up because that's probably a good thing to do,
	// - both saved and current statuses have Ready Conditions and they have the same LastProbeTime - nothing happened on that Node, it may be
	//   unresponsive, so we leave it as it is,
	// - both saved and current statuses have Ready Conditions, they have different LastProbeTimes, but the same Ready Condition State -
	//   everything's in order, no transition occurred, we update only probeTimestamp,
	// - both saved and current statuses have Ready Conditions, different LastProbeTimes and different Ready Condition State -
	//   Ready Condition changed it state since we last seen it, so we update both probeTimestamp and readyTransitionTimestamp.
	// TODO: things to consider:
	//   - if 'LastProbeTime' have gone back in time its probably an error, currently we ignore it,
	//   - currently only correct Ready State transition outside of Node Controller is marking it ready by Kubelet, we don't check
	//     if that's the case, but it does not seem necessary.
	var savedCondition *v1.NodeCondition
	var savedLease *coordv1.Lease
	if nodeHealth != nil {
		_, savedCondition = controllerutil.GetNodeCondition(nodeHealth.status, v1.NodeReady)
		savedLease = nodeHealth.lease
	}

	if nodeHealth == nil {
		klog.InfoS("Missing timestamp for Node. Assuming now as a timestamp", "node", klog.KObj(node))
		nodeHealth = &nodeHealthData{
			status:                   &node.Status,
			probeTimestamp:           nc.now(),
			readyTransitionTimestamp: nc.now(),
		}
	} else if savedCondition == nil && currentReadyCondition != nil {
		klog.V(1).InfoS("Creating timestamp entry for newly observed Node", "node", klog.KObj(node))
		nodeHealth = &nodeHealthData{
			status:                   &node.Status,
			probeTimestamp:           nc.now(),
			readyTransitionTimestamp: nc.now(),
		}
	} else if savedCondition != nil && currentReadyCondition == nil {
		klog.ErrorS(nil, "ReadyCondition was removed from Status of Node", "node", klog.KObj(node))
		// TODO: figure out what to do in this case. For now we do the same thing as above.
		nodeHealth = &nodeHealthData{
			status:                   &node.Status,
			probeTimestamp:           nc.now(),
			readyTransitionTimestamp: nc.now(),
		}
	} else if savedCondition != nil && currentReadyCondition != nil && savedCondition.LastHeartbeatTime != currentReadyCondition.LastHeartbeatTime {
		var transitionTime metav1.Time
		// If ReadyCondition changed since the last time we checked, we update the transition timestamp to "now",
		// otherwise we leave it as it is.
		if savedCondition.LastTransitionTime != currentReadyCondition.LastTransitionTime {
			klog.V(3).InfoS("ReadyCondition for Node transitioned from savedCondition to currentReadyCondition", "node", klog.KObj(node), "savedCondition", savedCondition, "currentReadyCondition", currentReadyCondition)
			transitionTime = nc.now()
		} else {
			transitionTime = nodeHealth.readyTransitionTimestamp
		}
		if loggerV := klog.V(5); loggerV.Enabled() {
			loggerV.Info("Node ReadyCondition updated. Updating timestamp", "node", klog.KObj(node), "nodeHealthStatus", nodeHealth.status, "nodeStatus", node.Status)
		} else {
			klog.V(3).InfoS("Node ReadyCondition updated. Updating timestamp", "node", klog.KObj(node))
		}
		nodeHealth = &nodeHealthData{
			status:                   &node.Status,
			probeTimestamp:           nc.now(),
			readyTransitionTimestamp: transitionTime,
		}
	}
	// Always update the probe time if node lease is renewed.
	// Note: If kubelet never posted the node status, but continues renewing the
	// heartbeat leases, the node controller will assume the node is healthy and
	// take no action.
	//observedLease, _ := nc.leaseLister.Leases(v1.NamespaceNodeLease).Get(node.Name)
	observedLease := new(coordinationv1.Lease)
	err := nc.controllerRuntimeClient.Get(ctx, types.NamespacedName{Namespace: v1.NamespaceNodeLease, Name: node.Name}, observedLease)
	if err == nil && observedLease != nil && (savedLease == nil || savedLease.Spec.RenewTime.Before(observedLease.Spec.RenewTime)) {
		nodeHealth.lease = observedLease
		nodeHealth.probeTimestamp = nc.now()
	}

	if nc.now().After(nodeHealth.probeTimestamp.Add(gracePeriod)) {
		// NodeReady condition or lease was last set longer ago than gracePeriod, so
		// update it to Unknown (regardless of its current value) in the master.

		nodeConditionTypes := []v1.NodeConditionType{
			v1.NodeReady,
			v1.NodeMemoryPressure,
			v1.NodeDiskPressure,
			v1.NodePIDPressure,
			// We don't change 'NodeNetworkUnavailable' condition, as it's managed on a control plane level.
			// v1.NodeNetworkUnavailable,
		}

		nowTimestamp := nc.now()
		for _, nodeConditionType := range nodeConditionTypes {
			_, currentCondition := controllerutil.GetNodeCondition(&node.Status, nodeConditionType)
			if currentCondition == nil {
				klog.V(2).InfoS("Condition of node was never updated by kubelet", "nodeConditionType", nodeConditionType, "node", klog.KObj(node))
				node.Status.Conditions = append(node.Status.Conditions, v1.NodeCondition{
					Type:               nodeConditionType,
					Status:             v1.ConditionUnknown,
					Reason:             "NodeStatusNeverUpdated",
					Message:            "Kubelet never posted node status.",
					LastHeartbeatTime:  node.CreationTimestamp,
					LastTransitionTime: nowTimestamp,
				})
			} else {
				klog.V(2).InfoS("Node hasn't been updated",
					"node", klog.KObj(node), "duration", nc.now().Time.Sub(nodeHealth.probeTimestamp.Time), "nodeConditionType", nodeConditionType, "currentCondition", currentCondition)
				if currentCondition.Status != v1.ConditionUnknown {
					currentCondition.Status = v1.ConditionUnknown
					currentCondition.Reason = "NodeStatusUnknown"
					currentCondition.Message = "Kubelet stopped posting node status."
					currentCondition.LastTransitionTime = nowTimestamp
				}
			}
		}
		// We need to update currentReadyCondition due to its value potentially changed.
		_, currentReadyCondition = controllerutil.GetNodeCondition(&node.Status, v1.NodeReady)

		if !apiequality.Semantic.DeepEqual(currentReadyCondition, &observedReadyCondition) {
			//if _, err := nc.kubeClient.CoreV1().Nodes().UpdateStatus(ctx, node, metav1.UpdateOptions{}); err != nil {
			if err := nc.controllerRuntimeClient.Status().Update(ctx, node, &client.UpdateOptions{}); err != nil {
				klog.ErrorS(err, "Error updating node", "node", klog.KObj(node))
				return gracePeriod, observedReadyCondition, currentReadyCondition, err
			}
			nodeHealth = &nodeHealthData{
				status:                   &node.Status,
				probeTimestamp:           nodeHealth.probeTimestamp,
				readyTransitionTimestamp: nc.now(),
				lease:                    observedLease,
			}
			return gracePeriod, observedReadyCondition, currentReadyCondition, nil
		}
	}

	return gracePeriod, observedReadyCondition, currentReadyCondition, nil
}

func (nc *ReconcileNodeLifeCycle) handleDisruption(ctx context.Context, zoneToNodeConditions map[string][]*v1.NodeCondition, nodes []*v1.Node) {
	newZoneStates := map[string]ZoneState{}
	allAreFullyDisrupted := true
	for k, v := range zoneToNodeConditions {
		zoneSize.WithLabelValues(k).Set(float64(len(v)))
		unhealthy, newState := nc.computeZoneStateFunc(v)
		zoneHealth.WithLabelValues(k).Set(float64(100*(len(v)-unhealthy)) / float64(len(v)))
		unhealthyNodes.WithLabelValues(k).Set(float64(unhealthy))
		if newState != stateFullDisruption {
			allAreFullyDisrupted = false
		}
		newZoneStates[k] = newState
		if _, had := nc.zoneStates[k]; !had {
			klog.ErrorS(nil, "Setting initial state for unseen zone", "zone", k)
			nc.zoneStates[k] = stateInitial
		}
	}

	allWasFullyDisrupted := true
	for k, v := range nc.zoneStates {
		if _, have := zoneToNodeConditions[k]; !have {
			zoneSize.WithLabelValues(k).Set(0)
			zoneHealth.WithLabelValues(k).Set(100)
			unhealthyNodes.WithLabelValues(k).Set(0)
			delete(nc.zoneStates, k)
			continue
		}
		if v != stateFullDisruption {
			allWasFullyDisrupted = false
			break
		}
	}

	// At least one node was responding in previous pass or in the current pass. Semantics is as follows:
	// - if the new state is "partialDisruption" we call a user defined function that returns a new limiter to use,
	// - if the new state is "normal" we resume normal operation (go back to default limiter settings),
	// - if new state is "fullDisruption" we restore normal eviction rate,
	//   - unless all zones in the cluster are in "fullDisruption" - in that case we stop all evictions.
	if !allAreFullyDisrupted || !allWasFullyDisrupted {
		// We're switching to full disruption mode
		if allAreFullyDisrupted {
			klog.Info("Controller detected that all Nodes are not-Ready. Entering master disruption mode")
			for i := range nodes {
				_, err := nc.markNodeAsReachable(ctx, nodes[i])
				if err != nil {
					klog.ErrorS(nil, "could not remove taints from Node", "node", klog.KObj(nodes[i]))
				}
			}
			// We stop all evictions.
			for k := range nc.zoneStates {
				nc.zoneNoExecuteTainter[k].SwapLimiter(0)
			}
			for k := range nc.zoneStates {
				nc.zoneStates[k] = stateFullDisruption
			}
			// All rate limiters are updated, so we can return early here.
			return
		}
		// We're exiting full disruption mode
		if allWasFullyDisrupted {
			klog.Info("Controller detected that some Nodes are Ready. Exiting master disruption mode")
			// When exiting disruption mode update probe timestamps on all Nodes.
			now := nc.now()
			for i := range nodes {
				v := nc.nodeHealthMap.getDeepCopy(nodes[i].Name)
				v.probeTimestamp = now
				v.readyTransitionTimestamp = now
				nc.nodeHealthMap.set(nodes[i].Name, v)
			}
			// We reset all rate limiters to settings appropriate for the given state.
			for k := range nc.zoneStates {
				nc.setLimiterInZone(k, len(zoneToNodeConditions[k]), newZoneStates[k])
				nc.zoneStates[k] = newZoneStates[k]
			}
			return
		}
		// We know that there's at least one not-fully disrupted so,
		// we can use default behavior for rate limiters
		for k, v := range nc.zoneStates {
			newState := newZoneStates[k]
			if v == newState {
				continue
			}
			klog.InfoS("Controller detected that zone is now in new state", "zone", k, "newState", newState)
			nc.setLimiterInZone(k, len(zoneToNodeConditions[k]), newState)
			nc.zoneStates[k] = newState
		}
	}
}

func (nc *ReconcileNodeLifeCycle) podUpdated(oldPod, newPod *v1.Pod) {
	if newPod == nil {
		return
	}
	if len(newPod.Spec.NodeName) != 0 && (oldPod == nil || newPod.Spec.NodeName != oldPod.Spec.NodeName) {
		podItem := podUpdateItem{newPod.Namespace, newPod.Name}
		nc.podUpdateQueue.Add(podItem)
	}
}

func (nc *ReconcileNodeLifeCycle) doPodProcessingWorker(ctx context.Context) {
	for {
		obj, shutdown := nc.podUpdateQueue.Get()
		// "podUpdateQueue" will be shutdown when "stopCh" closed;
		// we do not need to re-check "stopCh" again.
		if shutdown {
			return
		}

		podItem := obj.(podUpdateItem)
		nc.processPod(ctx, podItem)
	}
}

// processPod is processing events of assigning pods to nodes. In particular:
// 1. for NodeReady=true node, taint eviction for this pod will be cancelled
// 2. for NodeReady=false or unknown node, taint eviction of pod will happen and pod will be marked as not ready
// 3. if node doesn't exist in cache, it will be skipped.
func (nc *ReconcileNodeLifeCycle) processPod(ctx context.Context, podItem podUpdateItem) {
	defer nc.podUpdateQueue.Done(podItem)
	//pod, err := nc.podLister.Pods(podItem.namespace).Get(podItem.name)
	pod := new(v1.Pod)
	err := nc.controllerRuntimeClient.Get(ctx, types.NamespacedName{Namespace: podItem.namespace, Name: podItem.name}, pod)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the pod was deleted, there is no need to requeue.
			return
		}
		klog.InfoS("could not read pod", "pod", klog.KRef(podItem.namespace, podItem.name), "err", err)
		nc.podUpdateQueue.AddRateLimited(podItem)
		return
	}

	nodeName := pod.Spec.NodeName

	nodeHealth := nc.nodeHealthMap.getDeepCopy(nodeName)
	if nodeHealth == nil {
		// Node data is not gathered yet or node has been removed in the meantime.
		return
	}

	node := new(v1.Node)
	//_, err = nc.nodeLister.Get(nodeName)
	err = nc.controllerRuntimeClient.Get(ctx, types.NamespacedName{Name: nodeName}, node)
	if err != nil {
		klog.InfoS("could not read node", "node", klog.KRef("", nodeName), "err", err)
		nc.podUpdateQueue.AddRateLimited(podItem)
		return
	}

	// Ignore mark the pods NotReady if the node has bounded to node.
	if nodeutil.IsPodBoundenToNode(node) {
		return
	}

	_, currentReadyCondition := controllerutil.GetNodeCondition(nodeHealth.status, v1.NodeReady)
	if currentReadyCondition == nil {
		// Lack of NodeReady condition may only happen after node addition (or if it will be maliciously deleted).
		// In both cases, the pod will be handled correctly (evicted if needed) during processing
		// of the next node update event.
		return
	}

	pods := []*v1.Pod{pod}
	if currentReadyCondition.Status != v1.ConditionTrue {
		if err := controllerutil.MarkPodsNotReady(ctx, nc.controllerRuntimeClient, nc.recorder, pods, nodeName); err != nil {
			klog.InfoS("Unable to mark pod NotReady on node", "pod", klog.KRef(podItem.namespace, podItem.name), "node", klog.KRef("", nodeName), "err", err)
			nc.podUpdateQueue.AddRateLimited(podItem)
		}
	}
}

func (nc *ReconcileNodeLifeCycle) setLimiterInZone(zone string, zoneSize int, state ZoneState) {
	switch state {
	case stateNormal:
		nc.zoneNoExecuteTainter[zone].SwapLimiter(nc.evictionLimiterQPS)
	case statePartialDisruption:
		nc.zoneNoExecuteTainter[zone].SwapLimiter(
			nc.enterPartialDisruptionFunc(zoneSize))
	case stateFullDisruption:
		nc.zoneNoExecuteTainter[zone].SwapLimiter(
			nc.enterFullDisruptionFunc(zoneSize))
	}
}

// classifyNodes classifies the allNodes to three categories:
//  1. added: the nodes that in 'allNodes', but not in 'knownNodeSet'
//  2. deleted: the nodes that in 'knownNodeSet', but not in 'allNodes'
//  3. newZoneRepresentatives: the nodes that in both 'knownNodeSet' and 'allNodes', but no zone states
func (nc *ReconcileNodeLifeCycle) classifyNodes(allNodes []*v1.Node) (added, deleted, newZoneRepresentatives []*v1.Node) {
	for i := range allNodes {
		if _, has := nc.knownNodeSet[allNodes[i].Name]; !has {
			added = append(added, allNodes[i])
		} else {
			// Currently, we only consider new zone as updated.
			zone := nodetopology.GetZoneKey(allNodes[i])
			if _, found := nc.zoneStates[zone]; !found {
				newZoneRepresentatives = append(newZoneRepresentatives, allNodes[i])
			}
		}
	}

	// If there's a difference between lengths of known Nodes and observed nodes
	// we must have removed some Node.
	if len(nc.knownNodeSet)+len(added) != len(allNodes) {
		knowSetCopy := map[string]*v1.Node{}
		for k, v := range nc.knownNodeSet {
			knowSetCopy[k] = v
		}
		for i := range allNodes {
			delete(knowSetCopy, allNodes[i].Name)
		}
		for i := range knowSetCopy {
			deleted = append(deleted, knowSetCopy[i])
		}
	}
	return
}

// HealthyQPSFunc returns the default value for cluster eviction rate - we take
// nodeNum for consistency with ReducedQPSFunc.
func (nc *ReconcileNodeLifeCycle) HealthyQPSFunc(_ int) float32 {
	return nc.evictionLimiterQPS
}

// ReducedQPSFunc returns the QPS for when a the cluster is large make
// evictions slower, if they're small stop evictions altogether.
func (nc *ReconcileNodeLifeCycle) ReducedQPSFunc(nodeNum int) float32 {
	if int32(nodeNum) > nc.largeClusterThreshold {
		return nc.secondaryEvictionLimiterQPS
	}
	return 0
}

// addPodEvictorForNewZone checks if new zone appeared, and if so add new evictor.
func (nc *ReconcileNodeLifeCycle) addPodEvictorForNewZone(node *v1.Node) {
	nc.evictorLock.Lock()
	defer nc.evictorLock.Unlock()
	zone := nodetopology.GetZoneKey(node)
	if _, found := nc.zoneStates[zone]; !found {
		nc.zoneStates[zone] = stateInitial
		nc.zoneNoExecuteTainter[zone] =
			scheduler.NewRateLimitedTimedQueue(
				flowcontrol.NewTokenBucketRateLimiter(nc.evictionLimiterQPS, scheduler.EvictionRateLimiterBurst))
		// Init the metric for the new zone.
		klog.InfoS("Initializing eviction metric for zone", "zone", zone)
		evictionsTotal.WithLabelValues(zone).Add(0)
	}
}

func (nc *ReconcileNodeLifeCycle) markNodeForTainting(node *v1.Node, status v1.ConditionStatus) bool {
	nc.evictorLock.Lock()
	defer nc.evictorLock.Unlock()
	if status == v1.ConditionFalse {
		if !taintutils.TaintExists(node.Spec.Taints, NotReadyTaintTemplate) {
			nc.zoneNoExecuteTainter[nodetopology.GetZoneKey(node)].Remove(node.Name)
		}
	}

	if status == v1.ConditionUnknown {
		if !taintutils.TaintExists(node.Spec.Taints, UnreachableTaintTemplate) {
			nc.zoneNoExecuteTainter[nodetopology.GetZoneKey(node)].Remove(node.Name)
		}
	}

	return nc.zoneNoExecuteTainter[nodetopology.GetZoneKey(node)].Add(node.Name, string(node.UID))
}

func (nc *ReconcileNodeLifeCycle) markNodeAsReachable(ctx context.Context, node *v1.Node) (bool, error) {
	err := controllerutil.RemoveTaintOffNode(ctx, nc.kubeClient, node.Name, node, UnreachableTaintTemplate)
	if err != nil {
		klog.ErrorS(err, "could not remove taint from node", "node", klog.KObj(node))
		return false, err
	}
	err = controllerutil.RemoveTaintOffNode(ctx, nc.kubeClient, node.Name, node, NotReadyTaintTemplate)
	if err != nil {
		klog.ErrorS(err, "could not remove taint from node", "node", klog.KObj(node))
		return false, err
	}
	nc.evictorLock.Lock()
	defer nc.evictorLock.Unlock()

	return nc.zoneNoExecuteTainter[nodetopology.GetZoneKey(node)].Remove(node.Name), nil
}

// ComputeZoneState returns a slice of NodeReadyConditions for all Nodes in a given zone.
// The zone is considered:
// - fullyDisrupted if there're no Ready Nodes,
// - partiallyDisrupted if at least than nc.unhealthyZoneThreshold percent of Nodes are not Ready,
// - normal otherwise
func (nc *ReconcileNodeLifeCycle) ComputeZoneState(nodeReadyConditions []*v1.NodeCondition) (int, ZoneState) {
	readyNodes := 0
	notReadyNodes := 0
	for i := range nodeReadyConditions {
		if nodeReadyConditions[i] != nil && nodeReadyConditions[i].Status == v1.ConditionTrue {
			readyNodes++
		} else {
			notReadyNodes++
		}
	}
	switch {
	case readyNodes == 0 && notReadyNodes > 0:
		return notReadyNodes, stateFullDisruption
	case notReadyNodes > 2 && float32(notReadyNodes)/float32(notReadyNodes+readyNodes) >= nc.unhealthyZoneThreshold:
		return notReadyNodes, statePartialDisruption
	default:
		return notReadyNodes, stateNormal
	}
}

// reconcileNodeLabels reconciles node labels.
func (nc *ReconcileNodeLifeCycle) reconcileNodeLabels(ctx context.Context, nodeName string) error {
	//node, err := nc.nodeLister.Get(nodeName)
	node := new(v1.Node)
	err := nc.controllerRuntimeClient.Get(ctx, types.NamespacedName{Name: nodeName}, node)
	if err != nil {
		// If node not found, just ignore it.
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	if node.Labels == nil {
		// Nothing to reconcile.
		return nil
	}

	labelsToUpdate := map[string]string{}
	for _, r := range labelReconcileInfo {
		primaryValue, primaryExists := node.Labels[r.primaryKey]
		secondaryValue, secondaryExists := node.Labels[r.secondaryKey]

		if !primaryExists {
			// The primary label key does not exist. This should not happen
			// within our supported version skew range, when no external
			// components/factors modifying the node object. Ignore this case.
			continue
		}
		if secondaryExists && primaryValue != secondaryValue {
			// Secondary label exists, but not consistent with the primary
			// label. Need to reconcile.
			labelsToUpdate[r.secondaryKey] = primaryValue

		} else if !secondaryExists && r.ensureSecondaryExists {
			// Apply secondary label based on primary label.
			labelsToUpdate[r.secondaryKey] = primaryValue
		}
	}

	if len(labelsToUpdate) == 0 {
		return nil
	}
	if !controllerutil.AddOrUpdateLabelsOnNode(ctx, nc.kubeClient, labelsToUpdate, node) {
		return fmt.Errorf("failed update labels for node %+v", node)
	}
	return nil
}
