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

package nodelifecycle

import (
	"context"
	"fmt"
	goruntime "runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	coordv1 "k8s.io/api/coordination/v1"
	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	testcore "k8s.io/client-go/testing"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	kubeletapis "k8s.io/kubelet/pkg/apis"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	taintutils "github.com/openyurtio/openyurt/pkg/util/taints"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/nodelifecycle/scheduler"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/testutil"
	nodeutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/node"
)

const (
	testNodeMonitorGracePeriod = 40 * time.Second
	testNodeStartupGracePeriod = 60 * time.Second
	testNodeMonitorPeriod      = 5 * time.Second
	testRateLimiterQPS         = float32(100000)
	testLargeClusterThreshold  = 20
	testUnhealthyThreshold     = float32(0.55)
)

func createNodeLease(nodeName string, renewTime metav1.MicroTime) *coordv1.Lease {
	return &coordv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodeName,
			Namespace: v1.NamespaceNodeLease,
		},
		Spec: coordv1.LeaseSpec{
			HolderIdentity: pointer.String(nodeName),
			RenewTime:      &renewTime,
		},
	}
}

func newNodeLifecycleControllerFromClient(
	ctx context.Context,
	handler *testutil.ImprovedFakeNodeHandler,
	evictionLimiterQPS float32,
	secondaryEvictionLimiterQPS float32,
	largeClusterThreshold int32,
	unhealthyZoneThreshold float32,
	nodeMonitorGracePeriod time.Duration,
	nodeStartupGracePeriod time.Duration,
	nodeMonitorPeriod time.Duration,
) (*ReconcileNodeLifeCycle, error) {

	nc := &ReconcileNodeLifeCycle{
		controllerRuntimeClient:     handler,
		kubeClient:                  handler,
		recorder:                    testutil.NewFakeRecorder(),
		now:                         metav1.Now,
		knownNodeSet:                make(map[string]*v1.Node),
		nodeHealthMap:               newNodeHealthMap(),
		nodeUpdateWorkerSize:        scheduler.UpdateWorkerSize,
		zoneNoExecuteTainter:        make(map[string]*scheduler.RateLimitedTimedQueue),
		nodesToRetry:                sync.Map{},
		zoneStates:                  make(map[string]ZoneState),
		nodeMonitorPeriod:           nodeMonitorPeriod,
		nodeStartupGracePeriod:      nodeStartupGracePeriod,
		nodeMonitorGracePeriod:      nodeMonitorGracePeriod,
		evictionLimiterQPS:          evictionLimiterQPS,
		secondaryEvictionLimiterQPS: secondaryEvictionLimiterQPS,
		largeClusterThreshold:       largeClusterThreshold,
		unhealthyZoneThreshold:      unhealthyZoneThreshold,
		nodeUpdateQueue:             workqueue.NewNamed("node_lifecycle_controller"),
		podUpdateQueue:              workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "node_lifecycle_controller_pods"),
	}
	nc.enterPartialDisruptionFunc = nc.ReducedQPSFunc
	nc.enterFullDisruptionFunc = nc.HealthyQPSFunc
	nc.computeZoneStateFunc = nc.ComputeZoneState
	nc.getPodsAssignedToNode = GenGetPodsAssignedToNode(handler)

	return nc, nil
}

func TestMonitorNodeHealth(t *testing.T) {
	fakeNow := metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC)
	timeToPass := 60 * time.Minute
	healthyNodeNewStatus := v1.NodeStatus{
		Conditions: []v1.NodeCondition{
			{
				Type:               v1.NodeReady,
				Status:             v1.ConditionTrue,
				LastHeartbeatTime:  metav1.NewTime(fakeNow.Add(timeToPass)),
				LastTransitionTime: fakeNow,
			},
		},
	}
	unhealthyNodeNewStatus := v1.NodeStatus{
		Conditions: []v1.NodeCondition{
			{
				Type:   v1.NodeReady,
				Status: v1.ConditionUnknown,
				// Node status was updated by nodecontroller timeToPass ago
				LastHeartbeatTime:  fakeNow,
				LastTransitionTime: fakeNow,
			},
		},
	}

	tests := map[string]struct {
		nodeList                []*v1.Node
		updatedNodeStatuses     map[string]v1.NodeStatus
		expectedInitialStates   map[string]ZoneState
		expectedFollowingStates map[string]ZoneState
	}{
		"No Disruption: Node created recently without failure domain labels (happens only at cluster startup)": {
			nodeList: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: fakeNow,
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionTrue,
								LastHeartbeatTime:  fakeNow,
								LastTransitionTime: fakeNow,
							},
						},
					},
				},
			},
			updatedNodeStatuses: map[string]v1.NodeStatus{
				"node0": healthyNodeNewStatus,
			},
			expectedInitialStates: map[string]ZoneState{
				"": stateNormal,
			},
			expectedFollowingStates: map[string]ZoneState{
				"": stateNormal,
			},
		},
		"No Disruption: Initially both zones down, one comes back": {
			nodeList: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: fakeNow,
						Labels: map[string]string{
							v1.LabelTopologyRegion:          "region1",
							v1.LabelTopologyZone:            "zone1",
							v1.LabelFailureDomainBetaRegion: "region1",
							v1.LabelFailureDomainBetaZone:   "zone1",
						},
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionUnknown,
								LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
								LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node1",
						CreationTimestamp: fakeNow,
						Labels: map[string]string{
							v1.LabelTopologyRegion:          "region1",
							v1.LabelTopologyZone:            "zone2",
							v1.LabelFailureDomainBetaRegion: "region1",
							v1.LabelFailureDomainBetaZone:   "zone2",
						},
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionUnknown,
								LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
								LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							},
						},
					},
				},
			},
			updatedNodeStatuses: map[string]v1.NodeStatus{
				"node0": unhealthyNodeNewStatus,
				"node1": healthyNodeNewStatus,
			},
			expectedInitialStates: map[string]ZoneState{
				testutil.CreateZoneID("region1", "zone1"): stateFullDisruption,
				testutil.CreateZoneID("region1", "zone2"): stateFullDisruption,
			},
			expectedFollowingStates: map[string]ZoneState{
				testutil.CreateZoneID("region1", "zone1"): stateFullDisruption,
				testutil.CreateZoneID("region1", "zone2"): stateNormal,
			},
		},
		"Partial Disruption: Nodes created recently without status conditions (happens only at cluster startup)": {
			nodeList: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: fakeNow,
						Labels: map[string]string{
							v1.LabelTopologyRegion:          "region1",
							v1.LabelTopologyZone:            "zone1",
							v1.LabelFailureDomainBetaRegion: "region1",
							v1.LabelFailureDomainBetaZone:   "zone1",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node1",
						CreationTimestamp: fakeNow,
						Labels: map[string]string{
							v1.LabelTopologyRegion:          "region1",
							v1.LabelTopologyZone:            "zone1",
							v1.LabelFailureDomainBetaRegion: "region1",
							v1.LabelFailureDomainBetaZone:   "zone1",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node2",
						CreationTimestamp: fakeNow,
						Labels: map[string]string{
							v1.LabelTopologyRegion:          "region1",
							v1.LabelTopologyZone:            "zone1",
							v1.LabelFailureDomainBetaRegion: "region1",
							v1.LabelFailureDomainBetaZone:   "zone1",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node3",
						CreationTimestamp: fakeNow,
						Labels: map[string]string{
							v1.LabelTopologyRegion:          "region1",
							v1.LabelTopologyZone:            "zone1",
							v1.LabelFailureDomainBetaRegion: "region1",
							v1.LabelFailureDomainBetaZone:   "zone1",
						},
					},
				},
			},
			updatedNodeStatuses: map[string]v1.NodeStatus{
				"node0": unhealthyNodeNewStatus,
				"node1": unhealthyNodeNewStatus,
				"node2": unhealthyNodeNewStatus,
				"node3": healthyNodeNewStatus,
			},
			expectedInitialStates: map[string]ZoneState{
				// we've not received any status for the nodes yet
				// so the controller assumes the zones is fully disrupted
				testutil.CreateZoneID("region1", "zone1"): stateFullDisruption,
			},
			expectedFollowingStates: map[string]ZoneState{
				testutil.CreateZoneID("region1", "zone1"): statePartialDisruption,
			},
		},
		"Partial Disruption: one Node failed leading to the number of healthy Nodes to exceed the configured threshold": {
			nodeList: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: fakeNow,
						Labels: map[string]string{
							v1.LabelTopologyRegion:          "region1",
							v1.LabelTopologyZone:            "zone1",
							v1.LabelFailureDomainBetaRegion: "region1",
							v1.LabelFailureDomainBetaZone:   "zone1",
						},
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionUnknown,
								LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
								LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node1",
						CreationTimestamp: fakeNow,
						Labels: map[string]string{
							v1.LabelTopologyRegion:          "region1",
							v1.LabelTopologyZone:            "zone1",
							v1.LabelFailureDomainBetaRegion: "region1",
							v1.LabelFailureDomainBetaZone:   "zone1",
						},
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionUnknown,
								LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
								LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node2",
						CreationTimestamp: fakeNow,
						Labels: map[string]string{
							v1.LabelTopologyRegion:          "region1",
							v1.LabelTopologyZone:            "zone1",
							v1.LabelFailureDomainBetaRegion: "region1",
							v1.LabelFailureDomainBetaZone:   "zone1",
						},
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionTrue,
								LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
								LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node3",
						CreationTimestamp: fakeNow,
						Labels: map[string]string{
							v1.LabelTopologyRegion:          "region1",
							v1.LabelTopologyZone:            "zone1",
							v1.LabelFailureDomainBetaRegion: "region1",
							v1.LabelFailureDomainBetaZone:   "zone1",
						},
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionTrue,
								LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
								LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node4",
						CreationTimestamp: fakeNow,
						Labels: map[string]string{
							v1.LabelTopologyRegion:          "region1",
							v1.LabelTopologyZone:            "zone1",
							v1.LabelFailureDomainBetaRegion: "region1",
							v1.LabelFailureDomainBetaZone:   "zone1",
						},
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionTrue,
								LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
								LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							},
						},
					},
				},
			},
			updatedNodeStatuses: map[string]v1.NodeStatus{
				"node0": unhealthyNodeNewStatus,
				"node1": unhealthyNodeNewStatus,
				"node2": unhealthyNodeNewStatus,
				"node3": healthyNodeNewStatus,
				"node4": healthyNodeNewStatus,
			},
			expectedInitialStates: map[string]ZoneState{
				testutil.CreateZoneID("region1", "zone1"): stateNormal,
			},
			expectedFollowingStates: map[string]ZoneState{
				testutil.CreateZoneID("region1", "zone1"): statePartialDisruption,
			},
		},
		"Full Disruption: the zone has less than 2 Nodes down, the last healthy Node has failed": {
			nodeList: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: fakeNow,
						Labels: map[string]string{
							v1.LabelTopologyRegion:          "region1",
							v1.LabelTopologyZone:            "zone1",
							v1.LabelFailureDomainBetaRegion: "region1",
							v1.LabelFailureDomainBetaZone:   "zone1",
						},
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionUnknown,
								LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
								LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node1",
						CreationTimestamp: fakeNow,
						Labels: map[string]string{
							v1.LabelTopologyRegion:          "region1",
							v1.LabelTopologyZone:            "zone1",
							v1.LabelFailureDomainBetaRegion: "region1",
							v1.LabelFailureDomainBetaZone:   "zone1",
						},
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionUnknown,
								LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
								LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node2",
						CreationTimestamp: fakeNow,
						Labels: map[string]string{
							v1.LabelTopologyRegion:          "region1",
							v1.LabelTopologyZone:            "zone1",
							v1.LabelFailureDomainBetaRegion: "region1",
							v1.LabelFailureDomainBetaZone:   "zone1",
						},
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionTrue,
								LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
								LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							},
						},
					},
				},
			},
			updatedNodeStatuses: map[string]v1.NodeStatus{
				"node0": unhealthyNodeNewStatus,
				"node1": unhealthyNodeNewStatus,
				"node2": unhealthyNodeNewStatus,
			},
			expectedInitialStates: map[string]ZoneState{
				// if a zone has a number of unhealthy nodes less or equal to 2
				// the controller will consider it normal regardless on
				// the ration of healthy vs unhealthy nodes
				testutil.CreateZoneID("region1", "zone1"): stateNormal,
			},
			expectedFollowingStates: map[string]ZoneState{
				testutil.CreateZoneID("region1", "zone1"): stateFullDisruption,
			},
		},
		"Full Disruption: all the Nodes in one zone are down": {
			nodeList: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: fakeNow,
						Labels: map[string]string{
							v1.LabelTopologyRegion:          "region1",
							v1.LabelTopologyZone:            "zone1",
							v1.LabelFailureDomainBetaRegion: "region1",
							v1.LabelFailureDomainBetaZone:   "zone1",
						},
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionUnknown,
								LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
								LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node1",
						CreationTimestamp: fakeNow,
						Labels: map[string]string{
							v1.LabelTopologyRegion:          "region1",
							v1.LabelTopologyZone:            "zone2",
							v1.LabelFailureDomainBetaRegion: "region1",
							v1.LabelFailureDomainBetaZone:   "zone2",
						},
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionTrue,
								LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
								LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							},
						},
					},
				},
			},
			updatedNodeStatuses: map[string]v1.NodeStatus{
				"node0": unhealthyNodeNewStatus,
				"node1": healthyNodeNewStatus,
			},
			expectedInitialStates: map[string]ZoneState{
				testutil.CreateZoneID("region1", "zone1"): stateFullDisruption,
				testutil.CreateZoneID("region1", "zone2"): stateNormal,
			},
			expectedFollowingStates: map[string]ZoneState{
				testutil.CreateZoneID("region1", "zone1"): stateFullDisruption,
				testutil.CreateZoneID("region1", "zone2"): stateNormal,
			},
		},
		"Full Disruption: all the Nodes in both the zones are down": {
			nodeList: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: fakeNow,
						Labels: map[string]string{
							v1.LabelTopologyRegion:          "region1",
							v1.LabelTopologyZone:            "zone1",
							v1.LabelFailureDomainBetaRegion: "region1",
							v1.LabelFailureDomainBetaZone:   "zone1",
						},
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionUnknown,
								LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
								LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node1",
						CreationTimestamp: fakeNow,
						Labels: map[string]string{
							v1.LabelTopologyRegion:          "region2",
							v1.LabelTopologyZone:            "zone2",
							v1.LabelFailureDomainBetaRegion: "region2",
							v1.LabelFailureDomainBetaZone:   "zone2",
						},
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionUnknown,
								LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
								LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							},
						},
					},
				},
			},

			updatedNodeStatuses: map[string]v1.NodeStatus{
				"node0": unhealthyNodeNewStatus,
				"node1": unhealthyNodeNewStatus,
			},
			expectedInitialStates: map[string]ZoneState{
				testutil.CreateZoneID("region1", "zone1"): stateFullDisruption,
				testutil.CreateZoneID("region2", "zone2"): stateFullDisruption,
			},
			expectedFollowingStates: map[string]ZoneState{
				testutil.CreateZoneID("region1", "zone1"): stateFullDisruption,
				testutil.CreateZoneID("region2", "zone2"): stateFullDisruption,
			},
		},
		"Full Disruption: Ready condition removed from the Node": {
			nodeList: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: fakeNow,
						Labels: map[string]string{
							v1.LabelTopologyRegion:          "region1",
							v1.LabelTopologyZone:            "zone1",
							v1.LabelFailureDomainBetaRegion: "region1",
							v1.LabelFailureDomainBetaZone:   "zone1",
						},
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionTrue,
								LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
								LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							},
						},
					},
				},
			},

			updatedNodeStatuses: map[string]v1.NodeStatus{
				"node0": {
					Conditions: []v1.NodeCondition{},
				},
			},
			expectedInitialStates: map[string]ZoneState{
				testutil.CreateZoneID("region1", "zone1"): stateNormal,
			},
			expectedFollowingStates: map[string]ZoneState{
				testutil.CreateZoneID("region1", "zone1"): stateFullDisruption,
			},
		},
		"Full Disruption: the only available Node has the node.kubernetes.io/exclude-disruption label": {
			nodeList: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: fakeNow,
						Labels: map[string]string{
							v1.LabelTopologyRegion:          "region1",
							v1.LabelTopologyZone:            "zone1",
							v1.LabelFailureDomainBetaRegion: "region1",
							v1.LabelFailureDomainBetaZone:   "zone1",
						},
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionUnknown,
								LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
								LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node-master",
						CreationTimestamp: fakeNow,
						Labels: map[string]string{
							v1.LabelTopologyRegion:          "region1",
							v1.LabelTopologyZone:            "zone1",
							v1.LabelFailureDomainBetaRegion: "region1",
							v1.LabelFailureDomainBetaZone:   "zone1",
							labelNodeDisruptionExclusion:    "",
						},
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionTrue,
								LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
								LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							},
						},
					},
				},
			},
			updatedNodeStatuses: map[string]v1.NodeStatus{
				"node0":       unhealthyNodeNewStatus,
				"node-master": healthyNodeNewStatus,
			},
			expectedInitialStates: map[string]ZoneState{
				testutil.CreateZoneID("region1", "zone1"): stateFullDisruption,
			},
			expectedFollowingStates: map[string]ZoneState{
				testutil.CreateZoneID("region1", "zone1"): stateFullDisruption,
			},
		},
	}

	for testName, tt := range tests {
		t.Run(testName, func(t *testing.T) {
			fakeNodeHandler := testutil.NewImprovedFakeNodeHandler(tt.nodeList, nil)
			nodeController, _ := newNodeLifecycleControllerFromClient(
				context.TODO(),
				fakeNodeHandler,
				testRateLimiterQPS,
				testRateLimiterQPS,
				testLargeClusterThreshold,
				testUnhealthyThreshold,
				testNodeMonitorGracePeriod,
				testNodeStartupGracePeriod,
				testNodeMonitorPeriod)
			nodeController.recorder = testutil.NewFakeRecorder()
			nodeController.enterPartialDisruptionFunc = func(nodeNum int) float32 {
				return testRateLimiterQPS
			}
			nodeController.enterFullDisruptionFunc = func(nodeNum int) float32 {
				return testRateLimiterQPS
			}

			syncAndDiffZoneState := func(wanted map[string]ZoneState) {
				//if err := nodeController.syncNodeStore(fakeNodeHandler); err != nil {
				//	t.Errorf("unexpected error: %v", err)
				//}
				if err := nodeController.monitorNodeHealth(context.TODO()); err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if diff := cmp.Diff(wanted, nodeController.zoneStates); diff != "" {
					t.Errorf("unexpected zone state (-want +got):\n%s", diff)
				}
			}

			// initial zone state
			nodeController.now = func() metav1.Time { return fakeNow }
			syncAndDiffZoneState(tt.expectedInitialStates)

			// following zone state
			nodeController.now = func() metav1.Time { return metav1.Time{Time: fakeNow.Add(timeToPass)} }
			//for i := range tt.updatedNodeStatuses {
			//	fakeNodeHandler.Existing[i].Status = tt.updatedNodeStatuses[i]
			//}
			if err := fakeNodeHandler.UpdateNodeStatuses(tt.updatedNodeStatuses); err != nil {
				t.Errorf("failed to update node status, %v", err)
			}
			syncAndDiffZoneState(tt.expectedFollowingStates)
		})
	}
}

func TestPodStatusChange(t *testing.T) {
	fakeNow := metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC)

	// Because of the logic that prevents NC from evicting anything when all Nodes are NotReady
	// we need second healthy node in tests. Because of how the tests are written we need to update
	// the status of this Node.
	healthyNodeNewStatus := v1.NodeStatus{
		Conditions: []v1.NodeCondition{
			{
				Type:   v1.NodeReady,
				Status: v1.ConditionTrue,
				// Node status has just been updated, and is NotReady for 10min.
				LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 9, 0, 0, time.UTC),
				LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
			},
		},
	}

	// Node created long time ago, node controller posted Unknown for a long period of time.
	table := []struct {
		nodes               []*v1.Node
		pods                *v1.PodList
		timeToPass          time.Duration
		newNodeStatus       v1.NodeStatus
		secondNodeNewStatus v1.NodeStatus
		expectedPodUpdate   bool
		expectedReason      string
		description         string
	}{
		{
			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
						Labels: map[string]string{
							v1.LabelTopologyRegion:          "region1",
							v1.LabelTopologyZone:            "zone1",
							v1.LabelFailureDomainBetaRegion: "region1",
							v1.LabelFailureDomainBetaZone:   "zone1",
						},
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionUnknown,
								LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
								LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							},
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node1",
						CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
						Labels: map[string]string{
							v1.LabelFailureDomainBetaRegion: "region1",
							v1.LabelFailureDomainBetaZone:   "zone1",
						},
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionTrue,
								LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
								LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							},
						},
					},
				},
			},
			pods:       &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}},
			timeToPass: 60 * time.Minute,
			newNodeStatus: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type:   v1.NodeReady,
						Status: v1.ConditionUnknown,
						// Node status was updated by nodecontroller 1hr ago
						LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
						LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
					},
				},
			},
			secondNodeNewStatus: healthyNodeNewStatus,
			expectedPodUpdate:   true,
			expectedReason:      nodeutil.NodeUnreachablePodReason,
			description: "Node created long time ago, node controller posted Unknown for a " +
				"long period of time, the pod status must include reason for termination.",
		},
	}

	for _, item := range table {
		fakeNodeHandler := testutil.NewImprovedFakeNodeHandler(item.nodes, item.pods)
		nodeController, _ := newNodeLifecycleControllerFromClient(
			context.TODO(),
			fakeNodeHandler,
			testRateLimiterQPS,
			testRateLimiterQPS,
			testLargeClusterThreshold,
			testUnhealthyThreshold,
			testNodeMonitorGracePeriod,
			testNodeStartupGracePeriod,
			testNodeMonitorPeriod,
		)
		nodeController.now = func() metav1.Time { return fakeNow }
		nodeController.recorder = testutil.NewFakeRecorder()
		//nodeController.getPodsAssignedToNode = fakeGetPodsAssignedToNode(item.fakeNodeHandler.Clientset)
		//if err := nodeController.syncNodeStore(item.fakeNodeHandler); err != nil {
		//	t.Errorf("unexpected error: %v", err)
		//}
		if err := nodeController.monitorNodeHealth(context.TODO()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if item.timeToPass > 0 {
			nodeController.now = func() metav1.Time { return metav1.Time{Time: fakeNow.Add(item.timeToPass)} }
			//item.fakeNodeHandler.Existing[0].Status = item.newNodeStatus
			//item.fakeNodeHandler.Existing[1].Status = item.secondNodeNewStatus
			fakeNodeHandler.UpdateNodeStatuses(map[string]v1.NodeStatus{
				"node0": item.newNodeStatus,
				"node1": item.secondNodeNewStatus,
			})
		}
		//if err := nodeController.syncNodeStore(item.fakeNodeHandler); err != nil {
		//	t.Errorf("unexpected error: %v", err)
		//}
		if err := nodeController.monitorNodeHealth(context.TODO()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		zones := testutil.GetZones(fakeNodeHandler)
		for _, zone := range zones {
			nodeController.zoneNoExecuteTainter[zone].Try(func(value scheduler.TimedValue) (bool, time.Duration) {
				nodeUID, _ := value.UID.(string)
				pods, err := nodeController.getPodsAssignedToNode(value.Value)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				nodeutil.DeletePods(context.TODO(), fakeNodeHandler, pods, nodeController.recorder, value.Value, nodeUID)
				return true, 0
			})
		}

		podReasonUpdate := false
		for _, action := range fakeNodeHandler.Actions() {
			if action.GetVerb() == "update" && action.GetResource().Resource == "pods" {
				updateReason := action.(testcore.UpdateActionImpl).GetObject().(*v1.Pod).Status.Reason
				podReasonUpdate = true
				if updateReason != item.expectedReason {
					t.Errorf("expected pod status reason: %+v, got %+v for %+v", item.expectedReason, updateReason, item.description)
				}
			}
		}

		if podReasonUpdate != item.expectedPodUpdate {
			t.Errorf("expected pod update: %+v, got %+v for %+v", item.expectedPodUpdate, podReasonUpdate, item.description)
		}
	}
}

func TestMonitorNodeHealthUpdateStatus(t *testing.T) {
	fakeNow := metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC)
	table := []struct {
		nodes                   []*v1.Node
		pods                    *v1.PodList
		timeToPass              time.Duration
		newNodeStatus           v1.NodeStatus
		expectedRequestCount    int
		expectedNodes           []*v1.Node
		expectedPodStatusUpdate bool
	}{
		// Node created long time ago, without status:
		// Expect Unknown status posted from node controller.
		{

			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
					},
				},
			},
			pods:                 &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}},
			expectedRequestCount: 2, // List+Update
			expectedNodes: []*v1.Node{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Node",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionUnknown,
								Reason:             "NodeStatusNeverUpdated",
								Message:            "Kubelet never posted node status.",
								LastHeartbeatTime:  metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
								LastTransitionTime: fakeNow,
							},
							{
								Type:               v1.NodeMemoryPressure,
								Status:             v1.ConditionUnknown,
								Reason:             "NodeStatusNeverUpdated",
								Message:            "Kubelet never posted node status.",
								LastHeartbeatTime:  metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
								LastTransitionTime: fakeNow,
							},
							{
								Type:               v1.NodeDiskPressure,
								Status:             v1.ConditionUnknown,
								Reason:             "NodeStatusNeverUpdated",
								Message:            "Kubelet never posted node status.",
								LastHeartbeatTime:  metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
								LastTransitionTime: fakeNow,
							},
							{
								Type:               v1.NodePIDPressure,
								Status:             v1.ConditionUnknown,
								Reason:             "NodeStatusNeverUpdated",
								Message:            "Kubelet never posted node status.",
								LastHeartbeatTime:  metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
								LastTransitionTime: fakeNow,
							},
						},
					},
				},
			},
			expectedPodStatusUpdate: false, // Pod was never scheduled
		},
		// Node created recently, without status.
		// Expect no action from node controller (within startup grace period).
		{
			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: fakeNow,
					},
				},
			},
			pods:                    &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}},
			expectedRequestCount:    1, // List
			expectedNodes:           nil,
			expectedPodStatusUpdate: false,
		},
		// Node created long time ago, with status updated by kubelet exceeds grace period.
		// Expect Unknown status posted from node controller.
		{

			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:   v1.NodeReady,
								Status: v1.ConditionTrue,
								// Node status hasn't been updated for 1hr.
								LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
								LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							},
						},
						Capacity: v1.ResourceList{
							v1.ResourceName(v1.ResourceCPU):    resource.MustParse("10"),
							v1.ResourceName(v1.ResourceMemory): resource.MustParse("10G"),
						},
					},
				},
			},
			pods:                 &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}},
			expectedRequestCount: 3, // (List+)List+Update
			timeToPass:           time.Hour,
			newNodeStatus: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type:   v1.NodeReady,
						Status: v1.ConditionTrue,
						// Node status hasn't been updated for 1hr.
						LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
						LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
					},
				},
				Capacity: v1.ResourceList{
					v1.ResourceName(v1.ResourceCPU):    resource.MustParse("10"),
					v1.ResourceName(v1.ResourceMemory): resource.MustParse("10G"),
				},
			},
			expectedNodes: []*v1.Node{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Node",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionUnknown,
								Reason:             "NodeStatusUnknown",
								Message:            "Kubelet stopped posting node status.",
								LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
								LastTransitionTime: metav1.Time{Time: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC).Add(time.Hour)},
							},
							{
								Type:               v1.NodeMemoryPressure,
								Status:             v1.ConditionUnknown,
								Reason:             "NodeStatusNeverUpdated",
								Message:            "Kubelet never posted node status.",
								LastHeartbeatTime:  metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC), // should default to node creation time if condition was never updated
								LastTransitionTime: metav1.Time{Time: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC).Add(time.Hour)},
							},
							{
								Type:               v1.NodeDiskPressure,
								Status:             v1.ConditionUnknown,
								Reason:             "NodeStatusNeverUpdated",
								Message:            "Kubelet never posted node status.",
								LastHeartbeatTime:  metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC), // should default to node creation time if condition was never updated
								LastTransitionTime: metav1.Time{Time: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC).Add(time.Hour)},
							},
							{
								Type:               v1.NodePIDPressure,
								Status:             v1.ConditionUnknown,
								Reason:             "NodeStatusNeverUpdated",
								Message:            "Kubelet never posted node status.",
								LastHeartbeatTime:  metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC), // should default to node creation time if condition was never updated
								LastTransitionTime: metav1.Time{Time: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC).Add(time.Hour)},
							},
						},
						Capacity: v1.ResourceList{
							v1.ResourceName(v1.ResourceCPU):    resource.MustParse("10"),
							v1.ResourceName(v1.ResourceMemory): resource.MustParse("10G"),
						},
					},
				},
			},
			expectedPodStatusUpdate: true,
		},
		// Node created long time ago, with status updated recently.
		// Expect no action from node controller (within monitor grace period).
		{
			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:   v1.NodeReady,
								Status: v1.ConditionTrue,
								// Node status has just been updated.
								LastHeartbeatTime:  fakeNow,
								LastTransitionTime: fakeNow,
							},
						},
						Capacity: v1.ResourceList{
							v1.ResourceName(v1.ResourceCPU):    resource.MustParse("10"),
							v1.ResourceName(v1.ResourceMemory): resource.MustParse("10G"),
						},
					},
				},
			},
			pods:                    &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}},
			expectedRequestCount:    1, // List
			expectedNodes:           nil,
			expectedPodStatusUpdate: false,
		},
	}
	for i, item := range table {
		fakeNodeHandler := testutil.NewImprovedFakeNodeHandler(item.nodes, item.pods)
		nodeController, _ := newNodeLifecycleControllerFromClient(
			context.TODO(),
			fakeNodeHandler,
			testRateLimiterQPS,
			testRateLimiterQPS,
			testLargeClusterThreshold,
			testUnhealthyThreshold,
			testNodeMonitorGracePeriod,
			testNodeStartupGracePeriod,
			testNodeMonitorPeriod,
		)
		nodeController.now = func() metav1.Time { return fakeNow }
		nodeController.recorder = testutil.NewFakeRecorder()
		if err := nodeController.monitorNodeHealth(context.TODO()); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if item.timeToPass > 0 {
			nodeController.now = func() metav1.Time { return metav1.Time{Time: fakeNow.Add(item.timeToPass)} }
			//item.fakeNodeHandler.Existing[0].Status = item.newNodeStatus
			//if err := nodeController.syncNodeStore(item.fakeNodeHandler); err != nil {
			//	t.Errorf("unexpected error: %v", err)
			//}
			fakeNodeHandler.UpdateNodeStatuses(map[string]v1.NodeStatus{
				"node0": item.newNodeStatus,
			})
			if err := nodeController.monitorNodeHealth(context.TODO()); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		}
		if item.expectedRequestCount != fakeNodeHandler.RequestCount {
			t.Errorf("expected %v call, but got %v.", item.expectedRequestCount, fakeNodeHandler.RequestCount)
		}

		if len(fakeNodeHandler.UpdatedNodes) > 0 && !apiequality.Semantic.DeepEqual(item.expectedNodes, fakeNodeHandler.UpdatedNodes) {
			t.Errorf("Case[%d] unexpected nodes, expected nodes: %#+v\n, got nodes: %#+v", i, item.expectedNodes[0], fakeNodeHandler.UpdatedNodes[0])
		}

		if len(fakeNodeHandler.UpdatedNodeStatuses) > 0 && !apiequality.Semantic.DeepEqual(item.expectedNodes, fakeNodeHandler.UpdatedNodeStatuses) {
			t.Errorf("Case[%d] unexpected node status: expected %#+v\n, but got: %#+v", i, item.expectedNodes[0], fakeNodeHandler.UpdatedNodeStatuses[0])
		}

		podStatusUpdated := false
		for _, action := range fakeNodeHandler.Actions() {
			if action.GetVerb() == "update" && action.GetResource().Resource == "pods" && action.GetSubresource() == "status" {
				podStatusUpdated = true
			}
		}
		if podStatusUpdated != item.expectedPodStatusUpdate {
			t.Errorf("Case[%d] expect pod status updated to be %v, but got %v", i, item.expectedPodStatusUpdate, podStatusUpdated)
		}
	}
}

func TestMonitorNodeHealthUpdateNodeAndPodStatusWithLease(t *testing.T) {
	nodeCreationTime := metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC)
	fakeNow := metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC)
	testcases := []struct {
		description string
		//fakeNodeHandler         *testutil.FakeNodeHandler
		nodes                   []*v1.Node
		pods                    *v1.PodList
		lease                   *coordv1.Lease
		timeToPass              time.Duration
		newNodeStatus           map[string]v1.NodeStatus
		newLease                *coordv1.Lease
		expectedRequestCount    int
		expectedNodes           []*v1.Node
		expectedPodStatusUpdate bool
	}{
		// Node created recently, without status. Node lease is missing.
		// Expect no action from node controller (within startup grace period).
		{
			description: "Node created recently, without status. Node lease is missing.",

			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: fakeNow,
					},
				},
			},
			pods:                    &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}},
			expectedRequestCount:    1, // List
			expectedNodes:           nil,
			expectedPodStatusUpdate: false,
		},
		// Node created recently, without status. Node lease is renewed recently.
		// Expect no action from node controller (within startup grace period).
		{
			description: "Node created recently, without status. Node lease is renewed recently.",

			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: fakeNow,
					},
				},
			},
			pods:                    &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}},
			lease:                   createNodeLease("node0", metav1.NewMicroTime(fakeNow.Time)),
			expectedRequestCount:    1, // List
			expectedNodes:           nil,
			expectedPodStatusUpdate: false,
		},
		// Node created long time ago, without status. Node lease is missing.
		// Expect Unknown status posted from node controller.
		{
			description: "Node created long time ago, without status. Node lease is missing.",
			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: nodeCreationTime,
					},
				},
			},
			pods:                 &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}},
			expectedRequestCount: 2, // List+Update
			expectedNodes: []*v1.Node{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Node",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: nodeCreationTime,
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionUnknown,
								Reason:             "NodeStatusNeverUpdated",
								Message:            "Kubelet never posted node status.",
								LastHeartbeatTime:  nodeCreationTime,
								LastTransitionTime: fakeNow,
							},
							{
								Type:               v1.NodeMemoryPressure,
								Status:             v1.ConditionUnknown,
								Reason:             "NodeStatusNeverUpdated",
								Message:            "Kubelet never posted node status.",
								LastHeartbeatTime:  nodeCreationTime,
								LastTransitionTime: fakeNow,
							},
							{
								Type:               v1.NodeDiskPressure,
								Status:             v1.ConditionUnknown,
								Reason:             "NodeStatusNeverUpdated",
								Message:            "Kubelet never posted node status.",
								LastHeartbeatTime:  nodeCreationTime,
								LastTransitionTime: fakeNow,
							},
							{
								Type:               v1.NodePIDPressure,
								Status:             v1.ConditionUnknown,
								Reason:             "NodeStatusNeverUpdated",
								Message:            "Kubelet never posted node status.",
								LastHeartbeatTime:  nodeCreationTime,
								LastTransitionTime: fakeNow,
							},
						},
					},
				},
			},
			expectedPodStatusUpdate: false, // Pod was never scheduled because the node was never ready.
		},
		// Node created long time ago, without status. Node lease is renewed recently.
		// Expect no action from node controller (within monitor grace period).
		{
			description: "Node created long time ago, without status. Node lease is renewed recently.",
			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: nodeCreationTime,
					},
				},
			},
			pods:                 &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}},
			lease:                createNodeLease("node0", metav1.NewMicroTime(fakeNow.Time)),
			timeToPass:           time.Hour,
			newLease:             createNodeLease("node0", metav1.NewMicroTime(fakeNow.Time.Add(time.Hour))), // Lease is renewed after 1 hour.
			expectedRequestCount: 2,                                                                          // List+List
			expectedNodes: []*v1.Node{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Node",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: nodeCreationTime,
					},
				},
			},
			expectedPodStatusUpdate: false,
		},
		// Node created long time ago, without status. Node lease is expired.
		// Expect Unknown status posted from node controller.
		{
			description: "Node created long time ago, without status. Node lease is expired.",
			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: nodeCreationTime,
					},
				},
			},
			pods:                 &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}},
			lease:                createNodeLease("node0", metav1.NewMicroTime(fakeNow.Time)),
			timeToPass:           time.Hour,
			newLease:             createNodeLease("node0", metav1.NewMicroTime(fakeNow.Time)), // Lease is not renewed after 1 hour.
			expectedRequestCount: 3,                                                           // List+List+Update
			expectedNodes: []*v1.Node{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Node",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: nodeCreationTime,
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionUnknown,
								Reason:             "NodeStatusNeverUpdated",
								Message:            "Kubelet never posted node status.",
								LastHeartbeatTime:  nodeCreationTime,
								LastTransitionTime: metav1.Time{Time: fakeNow.Add(time.Hour)},
							},
							{
								Type:               v1.NodeMemoryPressure,
								Status:             v1.ConditionUnknown,
								Reason:             "NodeStatusNeverUpdated",
								Message:            "Kubelet never posted node status.",
								LastHeartbeatTime:  nodeCreationTime,
								LastTransitionTime: metav1.Time{Time: fakeNow.Add(time.Hour)},
							},
							{
								Type:               v1.NodeDiskPressure,
								Status:             v1.ConditionUnknown,
								Reason:             "NodeStatusNeverUpdated",
								Message:            "Kubelet never posted node status.",
								LastHeartbeatTime:  nodeCreationTime,
								LastTransitionTime: metav1.Time{Time: fakeNow.Add(time.Hour)},
							},
							{
								Type:               v1.NodePIDPressure,
								Status:             v1.ConditionUnknown,
								Reason:             "NodeStatusNeverUpdated",
								Message:            "Kubelet never posted node status.",
								LastHeartbeatTime:  nodeCreationTime,
								LastTransitionTime: metav1.Time{Time: fakeNow.Add(time.Hour)},
							},
						},
					},
				},
			},
			expectedPodStatusUpdate: false,
		},
		// Node created long time ago, with status updated by kubelet exceeds grace period. Node lease is renewed.
		// Expect no action from node controller (within monitor grace period).
		{
			description: "Node created long time ago, with status updated by kubelet exceeds grace period. Node lease is renewed.",
			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: nodeCreationTime,
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionTrue,
								LastHeartbeatTime:  fakeNow,
								LastTransitionTime: fakeNow,
							},
							{
								Type:               v1.NodeDiskPressure,
								Status:             v1.ConditionFalse,
								LastHeartbeatTime:  fakeNow,
								LastTransitionTime: fakeNow,
							},
						},
						Capacity: v1.ResourceList{
							v1.ResourceName(v1.ResourceCPU):    resource.MustParse("10"),
							v1.ResourceName(v1.ResourceMemory): resource.MustParse("10G"),
						},
					},
				},
			},
			pods:                 &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}},
			lease:                createNodeLease("node0", metav1.NewMicroTime(fakeNow.Time)),
			expectedRequestCount: 2, // List+List
			timeToPass:           time.Hour,
			newNodeStatus: map[string]v1.NodeStatus{
				// Node status hasn't been updated for 1 hour.
				"node0": {
					Conditions: []v1.NodeCondition{
						{
							Type:               v1.NodeReady,
							Status:             v1.ConditionTrue,
							LastHeartbeatTime:  fakeNow,
							LastTransitionTime: fakeNow,
						},
						{
							Type:               v1.NodeDiskPressure,
							Status:             v1.ConditionFalse,
							LastHeartbeatTime:  fakeNow,
							LastTransitionTime: fakeNow,
						},
					},
					Capacity: v1.ResourceList{
						v1.ResourceName(v1.ResourceCPU):    resource.MustParse("10"),
						v1.ResourceName(v1.ResourceMemory): resource.MustParse("10G"),
					},
				},
			},
			newLease: createNodeLease("node0", metav1.NewMicroTime(fakeNow.Time.Add(time.Hour))), // Lease is renewed after 1 hour.
			expectedNodes: []*v1.Node{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Node",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: nodeCreationTime,
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionTrue,
								LastHeartbeatTime:  fakeNow,
								LastTransitionTime: fakeNow,
							},
							{
								Type:               v1.NodeDiskPressure,
								Status:             v1.ConditionFalse,
								LastHeartbeatTime:  fakeNow,
								LastTransitionTime: fakeNow,
							},
						},
						Capacity: v1.ResourceList{
							v1.ResourceName(v1.ResourceCPU):    resource.MustParse("10"),
							v1.ResourceName(v1.ResourceMemory): resource.MustParse("10G"),
						},
					},
				},
			},
			expectedPodStatusUpdate: false,
		},
		// Node created long time ago, with status updated by kubelet recently. Node lease is expired.
		// Expect no action from node controller (within monitor grace period).
		{
			description: "Node created long time ago, with status updated by kubelet recently. Node lease is expired.",
			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: nodeCreationTime,
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionTrue,
								LastHeartbeatTime:  fakeNow,
								LastTransitionTime: fakeNow,
							},
							{
								Type:               v1.NodeDiskPressure,
								Status:             v1.ConditionFalse,
								LastHeartbeatTime:  fakeNow,
								LastTransitionTime: fakeNow,
							},
						},
						Capacity: v1.ResourceList{
							v1.ResourceName(v1.ResourceCPU):    resource.MustParse("10"),
							v1.ResourceName(v1.ResourceMemory): resource.MustParse("10G"),
						},
					},
				},
			},
			pods:                 &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}},
			lease:                createNodeLease("node0", metav1.NewMicroTime(fakeNow.Time)),
			expectedRequestCount: 2, // List+List
			timeToPass:           time.Hour,
			newNodeStatus: map[string]v1.NodeStatus{
				// Node status is updated after 1 hour.
				"node0": {
					Conditions: []v1.NodeCondition{
						{
							Type:               v1.NodeReady,
							Status:             v1.ConditionTrue,
							LastHeartbeatTime:  metav1.Time{Time: fakeNow.Add(time.Hour)},
							LastTransitionTime: fakeNow,
						},
						{
							Type:               v1.NodeDiskPressure,
							Status:             v1.ConditionFalse,
							LastHeartbeatTime:  metav1.Time{Time: fakeNow.Add(time.Hour)},
							LastTransitionTime: fakeNow,
						},
					},
					Capacity: v1.ResourceList{
						v1.ResourceName(v1.ResourceCPU):    resource.MustParse("10"),
						v1.ResourceName(v1.ResourceMemory): resource.MustParse("10G"),
					},
				},
			},
			newLease: createNodeLease("node0", metav1.NewMicroTime(fakeNow.Time)), // Lease is not renewed after 1 hour.
			expectedNodes: []*v1.Node{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Node",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: nodeCreationTime,
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionTrue,
								LastHeartbeatTime:  metav1.Time{Time: fakeNow.Add(time.Hour)},
								LastTransitionTime: fakeNow,
							},
							{
								Type:               v1.NodeDiskPressure,
								Status:             v1.ConditionFalse,
								LastHeartbeatTime:  metav1.Time{Time: fakeNow.Add(time.Hour)},
								LastTransitionTime: fakeNow,
							},
						},
						Capacity: v1.ResourceList{
							v1.ResourceName(v1.ResourceCPU):    resource.MustParse("10"),
							v1.ResourceName(v1.ResourceMemory): resource.MustParse("10G"),
						},
					},
				},
			},
			expectedPodStatusUpdate: false,
		},
		// Node created long time ago, with status updated by kubelet exceeds grace period. Node lease is also expired.
		// Expect Unknown status posted from node controller.
		{
			description: "Node created long time ago, with status updated by kubelet exceeds grace period. Node lease is also expired.",
			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: nodeCreationTime,
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionTrue,
								LastHeartbeatTime:  fakeNow,
								LastTransitionTime: fakeNow,
							},
						},
						Capacity: v1.ResourceList{
							v1.ResourceName(v1.ResourceCPU):    resource.MustParse("10"),
							v1.ResourceName(v1.ResourceMemory): resource.MustParse("10G"),
						},
					},
				},
			},
			pods:                 &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}},
			lease:                createNodeLease("node0", metav1.NewMicroTime(fakeNow.Time)),
			expectedRequestCount: 3, // List+List+Update
			timeToPass:           time.Hour,
			newNodeStatus: map[string]v1.NodeStatus{
				// Node status hasn't been updated for 1 hour.
				"node0": {
					Conditions: []v1.NodeCondition{
						{
							Type:               v1.NodeReady,
							Status:             v1.ConditionTrue,
							LastHeartbeatTime:  fakeNow,
							LastTransitionTime: fakeNow,
						},
					},
					Capacity: v1.ResourceList{
						v1.ResourceName(v1.ResourceCPU):    resource.MustParse("10"),
						v1.ResourceName(v1.ResourceMemory): resource.MustParse("10G"),
					},
				},
			},
			newLease: createNodeLease("node0", metav1.NewMicroTime(fakeNow.Time)), // Lease is not renewed after 1 hour.
			expectedNodes: []*v1.Node{
				{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Node",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: nodeCreationTime,
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:               v1.NodeReady,
								Status:             v1.ConditionUnknown,
								Reason:             "NodeStatusUnknown",
								Message:            "Kubelet stopped posting node status.",
								LastHeartbeatTime:  fakeNow,
								LastTransitionTime: metav1.Time{Time: fakeNow.Add(time.Hour)},
							},
							{
								Type:               v1.NodeMemoryPressure,
								Status:             v1.ConditionUnknown,
								Reason:             "NodeStatusNeverUpdated",
								Message:            "Kubelet never posted node status.",
								LastHeartbeatTime:  nodeCreationTime, // should default to node creation time if condition was never updated
								LastTransitionTime: metav1.Time{Time: fakeNow.Add(time.Hour)},
							},
							{
								Type:               v1.NodeDiskPressure,
								Status:             v1.ConditionUnknown,
								Reason:             "NodeStatusNeverUpdated",
								Message:            "Kubelet never posted node status.",
								LastHeartbeatTime:  nodeCreationTime, // should default to node creation time if condition was never updated
								LastTransitionTime: metav1.Time{Time: fakeNow.Add(time.Hour)},
							},
							{
								Type:               v1.NodePIDPressure,
								Status:             v1.ConditionUnknown,
								Reason:             "NodeStatusNeverUpdated",
								Message:            "Kubelet never posted node status.",
								LastHeartbeatTime:  nodeCreationTime, // should default to node creation time if condition was never updated
								LastTransitionTime: metav1.Time{Time: fakeNow.Add(time.Hour)},
							},
						},
						Capacity: v1.ResourceList{
							v1.ResourceName(v1.ResourceCPU):    resource.MustParse("10"),
							v1.ResourceName(v1.ResourceMemory): resource.MustParse("10G"),
						},
					},
				},
			},
			expectedPodStatusUpdate: true,
		},
	}

	for i, item := range testcases {
		t.Run(item.description, func(t *testing.T) {
			fakeNodeHandler := testutil.NewImprovedFakeNodeHandler(item.nodes, item.pods)
			nodeController, _ := newNodeLifecycleControllerFromClient(
				context.TODO(),
				fakeNodeHandler,
				testRateLimiterQPS,
				testRateLimiterQPS,
				testLargeClusterThreshold,
				testUnhealthyThreshold,
				testNodeMonitorGracePeriod,
				testNodeStartupGracePeriod,
				testNodeMonitorPeriod,
			)
			nodeController.now = func() metav1.Time { return fakeNow }
			nodeController.recorder = testutil.NewFakeRecorder()
			//nodeController.getPodsAssignedToNode = fakeGetPodsAssignedToNode(item.fakeNodeHandler.Clientset)
			//if err := nodeController.syncNodeStore(item.fakeNodeHandler); err != nil {
			//	t.Fatalf("unexpected error: %v", err)
			//}
			//if err := nodeController.syncLeaseStore(item.lease); err != nil {
			if err := fakeNodeHandler.UpdateLease(item.lease); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if err := nodeController.monitorNodeHealth(context.TODO()); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if item.timeToPass > 0 {
				nodeController.now = func() metav1.Time { return metav1.Time{Time: fakeNow.Add(item.timeToPass)} }
				//item.fakeNodeHandler.Existing[0].Status = item.newNodeStatus
				//if err := nodeController.syncNodeStore(item.fakeNodeHandler); err != nil {
				if err := fakeNodeHandler.UpdateNodeStatuses(item.newNodeStatus); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				//if err := nodeController.syncLeaseStore(item.newLease); err != nil {
				if err := fakeNodeHandler.UpdateLease(item.newLease); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if err := nodeController.monitorNodeHealth(context.TODO()); err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
			if item.expectedRequestCount != fakeNodeHandler.RequestCount {
				t.Errorf("expected %v call, but got %v.", item.expectedRequestCount, fakeNodeHandler.RequestCount)
			}

			if len(fakeNodeHandler.UpdatedNodes) > 0 && !apiequality.Semantic.DeepEqual(item.expectedNodes, fakeNodeHandler.UpdatedNodes) {
				t.Errorf("case[%d] expected nodes: %#+v\n, got %#+v", i, item.expectedNodes[0], fakeNodeHandler.UpdatedNodes[0])
			}

			if len(fakeNodeHandler.UpdatedNodeStatuses) > 0 && !apiequality.Semantic.DeepEqual(item.expectedNodes, fakeNodeHandler.UpdatedNodeStatuses) {
				t.Errorf("case[%d]: expected nodes: %#+v\n, got %#+v", i, item.expectedNodes[0], fakeNodeHandler.UpdatedNodeStatuses[0])
			}

			podStatusUpdated := false
			for _, action := range fakeNodeHandler.Actions() {
				if action.GetVerb() == "update" && action.GetResource().Resource == "pods" && action.GetSubresource() == "status" {
					podStatusUpdated = true
				}
			}
			if podStatusUpdated != item.expectedPodStatusUpdate {
				t.Errorf("expect pod status updated to be %v, but got %v", item.expectedPodStatusUpdate, podStatusUpdated)
			}
		})
	}
}

func TestMonitorNodeHealthMarkPodsNotReady(t *testing.T) {
	fakeNow := metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC)
	table := []struct {
		//fakeNodeHandler         *testutil.FakeNodeHandler
		nodes                   []*v1.Node
		pods                    *v1.PodList
		timeToPass              time.Duration
		newNodeStatus           map[string]v1.NodeStatus
		expectedPodStatusUpdate bool
	}{
		// Node created recently, without status.
		// Expect no action from node controller (within startup grace period).
		{
			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: fakeNow,
					},
				},
			},
			pods:                    &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}},
			expectedPodStatusUpdate: false,
		},
		// Node created long time ago, with status updated recently.
		// Expect no action from node controller (within monitor grace period).
		{
			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:   v1.NodeReady,
								Status: v1.ConditionTrue,
								// Node status has just been updated.
								LastHeartbeatTime:  fakeNow,
								LastTransitionTime: fakeNow,
							},
						},
						Capacity: v1.ResourceList{
							v1.ResourceName(v1.ResourceCPU):    resource.MustParse("10"),
							v1.ResourceName(v1.ResourceMemory): resource.MustParse("10G"),
						},
					},
				},
			},
			pods:                    &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}},
			expectedPodStatusUpdate: false,
		},
		// Node created long time ago, with status updated by kubelet exceeds grace period.
		// Expect pods status updated and Unknown node status posted from node controller
		{
			nodes: []*v1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "node0",
						CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
					},
					Status: v1.NodeStatus{
						Conditions: []v1.NodeCondition{
							{
								Type:   v1.NodeReady,
								Status: v1.ConditionTrue,
								// Node status hasn't been updated for 1hr.
								LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
								LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							},
						},
						Capacity: v1.ResourceList{
							v1.ResourceName(v1.ResourceCPU):    resource.MustParse("10"),
							v1.ResourceName(v1.ResourceMemory): resource.MustParse("10G"),
						},
					},
				},
			},
			pods:       &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}},
			timeToPass: 1 * time.Minute,
			newNodeStatus: map[string]v1.NodeStatus{
				"node0": {
					Conditions: []v1.NodeCondition{
						{
							Type:   v1.NodeReady,
							Status: v1.ConditionTrue,
							// Node status hasn't been updated for 1hr.
							LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
						},
					},
					Capacity: v1.ResourceList{
						v1.ResourceName(v1.ResourceCPU):    resource.MustParse("10"),
						v1.ResourceName(v1.ResourceMemory): resource.MustParse("10G"),
					},
				},
			},
			expectedPodStatusUpdate: true,
		},
	}

	ctx := context.TODO()
	for i, item := range table {
		fakeNodeHandler := testutil.NewImprovedFakeNodeHandler(item.nodes, item.pods)
		nodeController, _ := newNodeLifecycleControllerFromClient(
			ctx,
			fakeNodeHandler,
			testRateLimiterQPS,
			testRateLimiterQPS,
			testLargeClusterThreshold,
			testUnhealthyThreshold,
			testNodeMonitorGracePeriod,
			testNodeStartupGracePeriod,
			testNodeMonitorPeriod,
		)
		nodeController.now = func() metav1.Time { return fakeNow }
		nodeController.recorder = testutil.NewFakeRecorder()
		//nodeController.getPodsAssignedToNode = fakeGetPodsAssignedToNode(item.fakeNodeHandler.Clientset)
		//if err := nodeController.syncNodeStore(item.fakeNodeHandler); err != nil {
		//	t.Errorf("unexpected error: %v", err)
		//}
		if err := nodeController.monitorNodeHealth(ctx); err != nil {
			t.Errorf("Case[%d] unexpected error: %v", i, err)
		}
		if item.timeToPass > 0 {
			nodeController.now = func() metav1.Time { return metav1.Time{Time: fakeNow.Add(item.timeToPass)} }
			//item.fakeNodeHandler.Existing[0].Status = item.newNodeStatus
			//if err := nodeController.syncNodeStore(item.fakeNodeHandler); err != nil {
			if err := fakeNodeHandler.UpdateNodeStatuses(item.newNodeStatus); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if err := nodeController.monitorNodeHealth(ctx); err != nil {
				t.Errorf("Case[%d] unexpected error: %v", i, err)
			}
		}

		podStatusUpdated := false
		for _, action := range fakeNodeHandler.Actions() {
			if action.GetVerb() == "update" && action.GetResource().Resource == "pods" && action.GetSubresource() == "status" {
				podStatusUpdated = true
			}
		}
		if podStatusUpdated != item.expectedPodStatusUpdate {
			t.Errorf("Case[%d] expect pod status updated to be %v, but got %v", i, item.expectedPodStatusUpdate, podStatusUpdated)
		}
	}
}

// TestMonitorNodeHealthMarkPodsNotReadyWithWorkerSize tests the happy path of
// TestMonitorNodeHealthMarkPodsNotReady with a large number of nodes/pods and
// varying numbers of workers.
func TestMonitorNodeHealthMarkPodsNotReadyWithWorkerSize(t *testing.T) {
	const numNodes = 50
	const podsPerNode = 100
	makeNodes := func() []*v1.Node {
		nodes := make([]*v1.Node, numNodes)
		// Node created long time ago, with status updated by kubelet exceeds grace period.
		// Expect pods status updated and Unknown node status posted from node controller
		for i := 0; i < numNodes; i++ {
			nodes[i] = &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              fmt.Sprintf("node%d", i),
					CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:   v1.NodeReady,
							Status: v1.ConditionTrue,
							// Node status hasn't been updated for 1hr.
							LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
						},
					},
					Capacity: v1.ResourceList{
						v1.ResourceName(v1.ResourceCPU):    resource.MustParse("10"),
						v1.ResourceName(v1.ResourceMemory): resource.MustParse("10G"),
					},
				},
			}
		}
		return nodes
	}
	makePods := func() []v1.Pod {
		pods := make([]v1.Pod, numNodes*podsPerNode)
		for i := 0; i < numNodes*podsPerNode; i++ {
			pods[i] = *testutil.NewPod(fmt.Sprintf("pod%d", i), fmt.Sprintf("node%d", i%numNodes))
		}
		return pods
	}

	table := []struct {
		workers int
	}{
		{workers: 0}, // will default to scheduler.UpdateWorkerSize
		{workers: 1},
	}

	ctx := context.TODO()
	for i, item := range table {
		fakeNow := metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC)

		nodes := makeNodes()
		pods := &v1.PodList{Items: makePods()}
		fakeNodeHandler := testutil.NewImprovedFakeNodeHandler(nodes, pods)
		nodeController, _ := newNodeLifecycleControllerFromClient(
			ctx,
			fakeNodeHandler,
			testRateLimiterQPS,
			testRateLimiterQPS,
			testLargeClusterThreshold,
			testUnhealthyThreshold,
			testNodeMonitorGracePeriod,
			testNodeStartupGracePeriod,
			testNodeMonitorPeriod)
		nodeController.now = func() metav1.Time { return fakeNow }
		nodeController.recorder = testutil.NewFakeRecorder()
		//nodeController.getPodsAssignedToNode = fakeGetPodsAssignedToNode(fakeNodeHandler.Clientset)
		if item.workers != 0 {
			nodeController.nodeUpdateWorkerSize = item.workers
		}

		if err := nodeController.monitorNodeHealth(ctx); err != nil {
			t.Errorf("Case[%d] unexpected error: %v", i, err)
		}

		nodeController.now = func() metav1.Time { return metav1.Time{Time: fakeNow.Add(1 * time.Minute)} }
		nodeStatuses := make(map[string]v1.NodeStatus, numNodes)
		for i := 0; i < numNodes; i++ {
			nodeStatuses[fmt.Sprintf("node%d", i)] = v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type:   v1.NodeReady,
						Status: v1.ConditionTrue,
						// Node status hasn't been updated for 1hr.
						LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
						LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
					},
				},
				Capacity: v1.ResourceList{
					v1.ResourceName(v1.ResourceCPU):    resource.MustParse("10"),
					v1.ResourceName(v1.ResourceMemory): resource.MustParse("10G"),
				},
			}
		}

		if err := fakeNodeHandler.UpdateNodeStatuses(nodeStatuses); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if err := nodeController.monitorNodeHealth(ctx); err != nil {
			t.Errorf("Case[%d] unexpected error: %v", i, err)
		}

		podStatusUpdates := 0
		for _, action := range fakeNodeHandler.Actions() {
			if action.GetVerb() == "update" && action.GetResource().Resource == "pods" && action.GetSubresource() == "status" {
				podStatusUpdates++
			}
		}
		const expectedPodStatusUpdates = numNodes * podsPerNode
		if podStatusUpdates != expectedPodStatusUpdates {
			t.Errorf("Case[%d] expect pod status updated to be %v, but got %v", i, expectedPodStatusUpdates, podStatusUpdates)
		}
	}
}

func TestMonitorNodeHealthMarkPodsNotReadyRetry(t *testing.T) {
	type nodeIteration struct {
		timeToPass time.Duration
		newNodes   []*v1.Node
	}
	timeNow := metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC)
	timePlusTwoMinutes := metav1.Date(2015, 1, 1, 12, 0, 2, 0, time.UTC)
	makeNodes := func(status v1.ConditionStatus, lastHeartbeatTime, lastTransitionTime metav1.Time) []*v1.Node {
		return []*v1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "node0",
					CreationTimestamp: timeNow,
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:               v1.NodeReady,
							Status:             status,
							LastHeartbeatTime:  lastHeartbeatTime,
							LastTransitionTime: lastTransitionTime,
						},
					},
				},
			},
		}
	}
	table := []struct {
		desc                      string
		fakeNodeHandler           *testutil.ImprovedFakeNodeHandler
		updateReactor             func() error
		fakeGetPodsAssignedToNode func(c client.Client) func(string) ([]*v1.Pod, error)
		nodeIterations            []nodeIteration
		expectedPodStatusUpdates  int
	}{
		// Node created long time ago, with status updated by kubelet exceeds grace period.
		// First monitorNodeHealth check will update pod status to NotReady.
		// Second monitorNodeHealth check will do no updates (no retry).
		{
			desc:                      "successful pod status update, no retry required",
			fakeNodeHandler:           testutil.NewImprovedFakeNodeHandler([]*v1.Node{}, &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}}),
			fakeGetPodsAssignedToNode: GenGetPodsAssignedToNode,
			nodeIterations: []nodeIteration{
				{
					timeToPass: 0,
					newNodes:   makeNodes(v1.ConditionTrue, timeNow, timeNow),
				},
				{
					timeToPass: 1 * time.Minute,
					newNodes:   makeNodes(v1.ConditionTrue, timeNow, timeNow),
				},
				{
					timeToPass: 1 * time.Minute,
					newNodes:   makeNodes(v1.ConditionFalse, timePlusTwoMinutes, timePlusTwoMinutes),
				},
			},
			expectedPodStatusUpdates: 1,
		},
		// Node created long time ago, with status updated by kubelet exceeds grace period.
		// First monitorNodeHealth check will fail to update pod status to NotReady.
		// Second monitorNodeHealth check will update pod status to NotReady (retry).
		{
			desc:            "unsuccessful pod status update, retry required",
			fakeNodeHandler: testutil.NewImprovedFakeNodeHandler([]*v1.Node{}, &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}}),
			updateReactor: func() func() error {
				i := 0
				return func() error {
					i++
					switch i {
					case 1:
						return fmt.Errorf("fake error")
					default:
						return nil
					}
				}
			}(),
			fakeGetPodsAssignedToNode: GenGetPodsAssignedToNode,
			nodeIterations: []nodeIteration{
				{
					timeToPass: 0,
					newNodes:   makeNodes(v1.ConditionTrue, timeNow, timeNow),
				},
				{
					timeToPass: 1 * time.Minute,
					newNodes:   makeNodes(v1.ConditionTrue, timeNow, timeNow),
				},
				{
					timeToPass: 1 * time.Minute,
					newNodes:   makeNodes(v1.ConditionFalse, timePlusTwoMinutes, timePlusTwoMinutes),
				},
			},
			expectedPodStatusUpdates: 2, // One failed and one retry.
		},
		// Node created long time ago, with status updated by kubelet exceeds grace period.
		// First monitorNodeHealth check will fail to list pods.
		// Second monitorNodeHealth check will update pod status to NotReady (retry).
		{
			desc:            "unsuccessful pod list, retry required",
			fakeNodeHandler: testutil.NewImprovedFakeNodeHandler([]*v1.Node{}, &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}}),
			fakeGetPodsAssignedToNode: func(c client.Client) func(string) ([]*v1.Pod, error) {
				i := 0
				f := GenGetPodsAssignedToNode(c)
				return func(nodeName string) ([]*v1.Pod, error) {
					i++
					if i == 1 {
						return nil, fmt.Errorf("fake error")
					}
					return f(nodeName)
				}
			},
			nodeIterations: []nodeIteration{
				{
					timeToPass: 0,
					newNodes:   makeNodes(v1.ConditionTrue, timeNow, timeNow),
				},
				{
					timeToPass: 1 * time.Minute,
					newNodes:   makeNodes(v1.ConditionTrue, timeNow, timeNow),
				},
				{
					timeToPass: 1 * time.Minute,
					newNodes:   makeNodes(v1.ConditionFalse, timePlusTwoMinutes, timePlusTwoMinutes),
				},
			},
			expectedPodStatusUpdates: 1,
		},
	}

	for i, item := range table {
		t.Run(item.desc, func(t *testing.T) {
			ctx := context.TODO()
			nodeController, _ := newNodeLifecycleControllerFromClient(
				ctx,
				item.fakeNodeHandler,
				testRateLimiterQPS,
				testRateLimiterQPS,
				testLargeClusterThreshold,
				testUnhealthyThreshold,
				testNodeMonitorGracePeriod,
				testNodeStartupGracePeriod,
				testNodeMonitorPeriod,
			)
			if item.updateReactor != nil {
				item.fakeNodeHandler.PodUpdateReactor = item.updateReactor
			}
			nodeController.now = func() metav1.Time { return timeNow }
			nodeController.recorder = testutil.NewFakeRecorder()
			nodeController.getPodsAssignedToNode = item.fakeGetPodsAssignedToNode(item.fakeNodeHandler.ClientWrapper)
			for _, iteration := range item.nodeIterations {
				nodeController.now = func() metav1.Time { return metav1.Time{Time: timeNow.Add(iteration.timeToPass)} }
				if err := item.fakeNodeHandler.UpdateNodes(iteration.newNodes); err != nil {
					t.Errorf("failed to update node, %v", err)
				}
				if err := nodeController.monitorNodeHealth(ctx); err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}

			podStatusUpdates := 0
			for _, action := range item.fakeNodeHandler.Actions() {
				if action.GetVerb() == "update" && action.GetResource().Resource == "pods" && action.GetSubresource() == "status" {
					podStatusUpdates++
				}
			}
			if podStatusUpdates != item.expectedPodStatusUpdates {
				t.Errorf("case[%d], expect pod status updated to happen %d times, but got %d", i, item.expectedPodStatusUpdates, podStatusUpdates)
			}
		})
	}
}

// TestApplyNoExecuteTaints, ensures we just have a NoExecute taint applied to node.
// NodeController is just responsible for enqueuing the node to tainting queue from which taint manager picks up
// and evicts the pods on the node.
func TestApplyNoExecuteTaints(t *testing.T) {
	// TODO: Remove skip once https://github.com/kubernetes/kubernetes/pull/114607 merges.
	if goruntime.GOOS == "windows" {
		t.Skip("Skipping test on Windows.")
	}
	fakeNow := metav1.Date(2017, 1, 1, 12, 0, 0, 0, time.UTC)
	//
	//fakeNodeHandler := &testutil.FakeNodeHandler{
	//	Existing: []*v1.Node{
	nodes := []*v1.Node{
		// Unreachable Taint with effect 'NoExecute' should be applied to this node.
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "node0",
				CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
				Labels: map[string]string{
					v1.LabelTopologyRegion:          "region1",
					v1.LabelTopologyZone:            "zone1",
					v1.LabelFailureDomainBetaRegion: "region1",
					v1.LabelFailureDomainBetaZone:   "zone1",
				},
			},
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type:               v1.NodeReady,
						Status:             v1.ConditionUnknown,
						LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
						LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
					},
				},
			},
		},
		// Because of the logic that prevents NC from evicting anything when all Nodes are NotReady
		// we need second healthy node in tests.
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "node1",
				CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
				Labels: map[string]string{
					v1.LabelTopologyRegion:          "region1",
					v1.LabelTopologyZone:            "zone1",
					v1.LabelFailureDomainBetaRegion: "region1",
					v1.LabelFailureDomainBetaZone:   "zone1",
				},
			},
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type:               v1.NodeReady,
						Status:             v1.ConditionTrue,
						LastHeartbeatTime:  metav1.Date(2017, 1, 1, 12, 0, 0, 0, time.UTC),
						LastTransitionTime: metav1.Date(2017, 1, 1, 12, 0, 0, 0, time.UTC),
					},
				},
			},
		},
		// NotReady Taint with NoExecute effect should be applied to this node.
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "node2",
				CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
				Labels: map[string]string{
					v1.LabelTopologyRegion:          "region1",
					v1.LabelTopologyZone:            "zone1",
					v1.LabelFailureDomainBetaRegion: "region1",
					v1.LabelFailureDomainBetaZone:   "zone1",
				},
			},
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type:               v1.NodeReady,
						Status:             v1.ConditionFalse,
						LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
						LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
					},
				},
			},
		},
	}
	//Clientset: fake.NewSimpleClientset(&v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}}),
	pods := &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}}
	//}
	healthyNodeNewStatus := v1.NodeStatus{
		Conditions: []v1.NodeCondition{
			{
				Type:               v1.NodeReady,
				Status:             v1.ConditionTrue,
				LastHeartbeatTime:  metav1.Date(2017, 1, 1, 12, 10, 0, 0, time.UTC),
				LastTransitionTime: metav1.Date(2017, 1, 1, 12, 0, 0, 0, time.UTC),
			},
		},
	}
	originalTaint := UnreachableTaintTemplate
	ctx := context.TODO()
	fakeNodeHandler := testutil.NewImprovedFakeNodeHandler(nodes, pods)
	nodeController, _ := newNodeLifecycleControllerFromClient(
		ctx,
		fakeNodeHandler,
		testRateLimiterQPS,
		testRateLimiterQPS,
		testLargeClusterThreshold,
		testUnhealthyThreshold,
		testNodeMonitorGracePeriod,
		testNodeStartupGracePeriod,
		testNodeMonitorPeriod,
	)
	nodeController.now = func() metav1.Time { return fakeNow }
	nodeController.recorder = testutil.NewFakeRecorder()
	//nodeController.getPodsAssignedToNode = fakeGetPodsAssignedToNode(fakeNodeHandler.Clientset)
	//if err := nodeController.syncNodeStore(fakeNodeHandler); err != nil {
	//	t.Errorf("unexpected error: %v", err)
	//}
	if err := nodeController.monitorNodeHealth(ctx); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	nodeController.doNoExecuteTaintingPass(ctx)
	//node0, err := fakeNodeHandler.Get(ctx, "node0", metav1.GetOptions{})
	node0, err := fakeNodeHandler.DelegateNodeHandler.Get(ctx, "node0", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Can't get current node0...")
		return
	}
	if !taintutils.TaintExists(node0.Spec.Taints, UnreachableTaintTemplate) {
		t.Errorf("Can't find taint %v in %v", originalTaint, node0.Spec.Taints)
	}
	klog.Infof("node0 test ended")
	//node2, err := fakeNodeHandler.Get(ctx, "node2", metav1.GetOptions{})
	node2, err := fakeNodeHandler.DelegateNodeHandler.Get(ctx, "node2", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Can't get current node2...")
		return
	}
	if !taintutils.TaintExists(node2.Spec.Taints, NotReadyTaintTemplate) {
		t.Errorf("Can't find taint %v in %v", NotReadyTaintTemplate, node2.Spec.Taints)
	}
	klog.Infof("node2 init test ended")

	// Make node3 healthy again.
	node2.Status = healthyNodeNewStatus
	//_, err = fakeNodeHandler.UpdateStatus(ctx, node2, metav1.UpdateOptions{})
	_, err = fakeNodeHandler.DelegateNodeHandler.UpdateStatus(ctx, node2, metav1.UpdateOptions{})
	if err != nil {
		t.Errorf(err.Error())
		return
	}
	//if err := nodeController.syncNodeStore(fakeNodeHandler); err != nil {
	//	t.Errorf("unexpected error: %v", err)
	//}
	if err := nodeController.monitorNodeHealth(ctx); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	nodeController.doNoExecuteTaintingPass(ctx)

	//node2, err = fakeNodeHandler.Get(ctx, "node2", metav1.GetOptions{})
	node2, err = fakeNodeHandler.DelegateNodeHandler.Get(ctx, "node2", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Can't get current node2...")
		return
	}
	// We should not see any taint on the node(especially the Not-Ready taint with NoExecute effect).
	if taintutils.TaintExists(node2.Spec.Taints, NotReadyTaintTemplate) || len(node2.Spec.Taints) > 0 {
		t.Errorf("Found taint %v in %v, which should not be present", NotReadyTaintTemplate, node2.Spec.Taints)
	}
}

// TestApplyNoExecuteTaintsToNodesEnqueueTwice ensures we taint every node with NoExecute even if enqueued twice
func TestApplyNoExecuteTaintsToNodesEnqueueTwice(t *testing.T) {
	// TODO: Remove skip once https://github.com/kubernetes/kubernetes/pull/114607 merges.
	if goruntime.GOOS == "windows" {
		t.Skip("Skipping test on Windows.")
	}
	fakeNow := metav1.Date(2017, 1, 1, 12, 0, 0, 0, time.UTC)

	//fakeNodeHandler := &testutil.FakeNodeHandler{
	//	Existing: []*v1.Node{
	nodes := []*v1.Node{
		// Unreachable Taint with effect 'NoExecute' should be applied to this node.
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "node0",
				CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
				Labels: map[string]string{
					v1.LabelTopologyRegion:          "region1",
					v1.LabelTopologyZone:            "zone1",
					v1.LabelFailureDomainBetaRegion: "region1",
					v1.LabelFailureDomainBetaZone:   "zone1",
				},
			},
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type:               v1.NodeReady,
						Status:             v1.ConditionUnknown,
						LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
						LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
					},
				},
			},
		},
		// Because of the logic that prevents NC from evicting anything when all Nodes are NotReady
		// we need second healthy node in tests.
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "node1",
				CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
				Labels: map[string]string{
					v1.LabelTopologyRegion:          "region1",
					v1.LabelTopologyZone:            "zone1",
					v1.LabelFailureDomainBetaRegion: "region1",
					v1.LabelFailureDomainBetaZone:   "zone1",
				},
			},
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type:               v1.NodeReady,
						Status:             v1.ConditionTrue,
						LastHeartbeatTime:  metav1.Date(2017, 1, 1, 12, 0, 0, 0, time.UTC),
						LastTransitionTime: metav1.Date(2017, 1, 1, 12, 0, 0, 0, time.UTC),
					},
				},
			},
		},
		// NotReady Taint with NoExecute effect should be applied to this node.
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "node2",
				CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
				Labels: map[string]string{
					v1.LabelTopologyRegion:          "region1",
					v1.LabelTopologyZone:            "zone1",
					v1.LabelFailureDomainBetaRegion: "region1",
					v1.LabelFailureDomainBetaZone:   "zone1",
				},
			},
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type:               v1.NodeReady,
						Status:             v1.ConditionFalse,
						LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
						LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
					},
				},
			},
		},
	}
	//Clientset: fake.NewSimpleClientset(&v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}}),
	pods := &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}}
	//}
	healthyNodeNewStatus := v1.NodeStatus{
		Conditions: []v1.NodeCondition{
			{
				Type:               v1.NodeReady,
				Status:             v1.ConditionTrue,
				LastHeartbeatTime:  metav1.Date(2017, 1, 1, 12, 10, 0, 0, time.UTC),
				LastTransitionTime: metav1.Date(2017, 1, 1, 12, 0, 0, 0, time.UTC),
			},
		},
	}
	ctx := context.TODO()
	fakeNodeHandler := testutil.NewImprovedFakeNodeHandler(nodes, pods)
	nodeController, _ := newNodeLifecycleControllerFromClient(
		ctx,
		fakeNodeHandler,
		testRateLimiterQPS,
		testRateLimiterQPS,
		testLargeClusterThreshold,
		testUnhealthyThreshold,
		testNodeMonitorGracePeriod,
		testNodeStartupGracePeriod,
		testNodeMonitorPeriod,
	)
	nodeController.now = func() metav1.Time { return fakeNow }
	nodeController.recorder = testutil.NewFakeRecorder()
	//nodeController.getPodsAssignedToNode = fakeGetPodsAssignedToNode(fakeNodeHandler.Clientset)
	//if err := nodeController.syncNodeStore(fakeNodeHandler); err != nil {
	//	t.Errorf("unexpected error: %v", err)
	//}
	// 1. monitor node health twice, add untainted node once
	if err := nodeController.monitorNodeHealth(ctx); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := nodeController.monitorNodeHealth(ctx); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// 2. mark node0 healthy
	node0, err := fakeNodeHandler.DelegateNodeHandler.Get(ctx, "node0", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Can't get current node0...")
		return
	}
	node0.Status = healthyNodeNewStatus
	_, err = fakeNodeHandler.DelegateNodeHandler.UpdateStatus(ctx, node0, metav1.UpdateOptions{})
	if err != nil {
		t.Errorf(err.Error())
		return
	}

	// add other notReady nodes
	//fakeNodeHandler.Existing = append(fakeNodeHandler.Existing, []*v1.Node{
	if err = fakeNodeHandler.UpdateNodes([]*v1.Node{
		// Unreachable Taint with effect 'NoExecute' should be applied to this node.
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "node3",
				CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
				Labels: map[string]string{
					v1.LabelTopologyRegion:          "region1",
					v1.LabelTopologyZone:            "zone1",
					v1.LabelFailureDomainBetaRegion: "region1",
					v1.LabelFailureDomainBetaZone:   "zone1",
				},
			},
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type:               v1.NodeReady,
						Status:             v1.ConditionUnknown,
						LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
						LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
					},
				},
			},
		},
		// Because of the logic that prevents NC from evicting anything when all Nodes are NotReady
		// we need second healthy node in tests.
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "node4",
				CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
				Labels: map[string]string{
					v1.LabelTopologyRegion:          "region1",
					v1.LabelTopologyZone:            "zone1",
					v1.LabelFailureDomainBetaRegion: "region1",
					v1.LabelFailureDomainBetaZone:   "zone1",
				},
			},
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type:               v1.NodeReady,
						Status:             v1.ConditionTrue,
						LastHeartbeatTime:  metav1.Date(2017, 1, 1, 12, 0, 0, 0, time.UTC),
						LastTransitionTime: metav1.Date(2017, 1, 1, 12, 0, 0, 0, time.UTC),
					},
				},
			},
		},
		// NotReady Taint with NoExecute effect should be applied to this node.
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "node5",
				CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
				Labels: map[string]string{
					v1.LabelTopologyRegion:          "region1",
					v1.LabelTopologyZone:            "zone1",
					v1.LabelFailureDomainBetaRegion: "region1",
					v1.LabelFailureDomainBetaZone:   "zone1",
				},
			},
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type:               v1.NodeReady,
						Status:             v1.ConditionFalse,
						LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
						LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
					},
				},
			},
		},
	}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	//if err := nodeController.syncNodeStore(fakeNodeHandler); err != nil {
	//	t.Errorf("unexpected error: %v", err)
	//}
	// 3. start monitor node health again, add untainted node twice, construct UniqueQueue with duplicated node cache
	if err := nodeController.monitorNodeHealth(ctx); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// 4. do NoExecute taint pass
	// when processing with node0, condition.Status is NodeReady, and return true with default case
	// then remove the set value and queue value both, the taint job never stuck
	nodeController.doNoExecuteTaintingPass(ctx)

	// 5. get node3 and node5, see if it has ready got NoExecute taint
	node3, err := fakeNodeHandler.DelegateNodeHandler.Get(ctx, "node3", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Can't get current node3...")
		return
	}
	if !taintutils.TaintExists(node3.Spec.Taints, UnreachableTaintTemplate) || len(node3.Spec.Taints) == 0 {
		t.Errorf("Not found taint %v in %v, which should be present in %s", UnreachableTaintTemplate, node3.Spec.Taints, node3.Name)
	}
	node5, err := fakeNodeHandler.DelegateNodeHandler.Get(ctx, "node5", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Can't get current node5...")
		return
	}
	if !taintutils.TaintExists(node5.Spec.Taints, NotReadyTaintTemplate) || len(node5.Spec.Taints) == 0 {
		t.Errorf("Not found taint %v in %v, which should be present in %s", NotReadyTaintTemplate, node5.Spec.Taints, node5.Name)
	}
}

func TestSwapUnreachableNotReadyTaints(t *testing.T) {
	fakeNow := metav1.Date(2017, 1, 1, 12, 0, 0, 0, time.UTC)

	//fakeNodeHandler := &testutil.FakeNodeHandler{
	//	Existing: []*v1.Node{
	nodes := []*v1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "node0",
				CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
				Labels: map[string]string{
					v1.LabelTopologyRegion:          "region1",
					v1.LabelTopologyZone:            "zone1",
					v1.LabelFailureDomainBetaRegion: "region1",
					v1.LabelFailureDomainBetaZone:   "zone1",
				},
			},
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type:               v1.NodeReady,
						Status:             v1.ConditionUnknown,
						LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
						LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
					},
				},
			},
		},
		// Because of the logic that prevents NC from evicting anything when all Nodes are NotReady
		// we need second healthy node in tests. Because of how the tests are written we need to update
		// the status of this Node.
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "node1",
				CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
				Labels: map[string]string{
					v1.LabelTopologyRegion:          "region1",
					v1.LabelTopologyZone:            "zone1",
					v1.LabelFailureDomainBetaRegion: "region1",
					v1.LabelFailureDomainBetaZone:   "zone1",
				},
			},
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type:               v1.NodeReady,
						Status:             v1.ConditionTrue,
						LastHeartbeatTime:  metav1.Date(2017, 1, 1, 12, 0, 0, 0, time.UTC),
						LastTransitionTime: metav1.Date(2017, 1, 1, 12, 0, 0, 0, time.UTC),
					},
				},
			},
		},
	}
	//Clientset: fake.NewSimpleClientset(&v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}}),
	pods := &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}}
	//}
	newNodeStatus := v1.NodeStatus{
		Conditions: []v1.NodeCondition{
			{
				Type:   v1.NodeReady,
				Status: v1.ConditionFalse,
				// Node status has just been updated, and is NotReady for 10min.
				LastHeartbeatTime:  metav1.Date(2017, 1, 1, 12, 9, 0, 0, time.UTC),
				LastTransitionTime: metav1.Date(2017, 1, 1, 12, 0, 0, 0, time.UTC),
			},
		},
	}
	healthyNodeNewStatus := v1.NodeStatus{
		Conditions: []v1.NodeCondition{
			{
				Type:               v1.NodeReady,
				Status:             v1.ConditionTrue,
				LastHeartbeatTime:  metav1.Date(2017, 1, 1, 12, 10, 0, 0, time.UTC),
				LastTransitionTime: metav1.Date(2017, 1, 1, 12, 0, 0, 0, time.UTC),
			},
		},
	}
	originalTaint := UnreachableTaintTemplate
	updatedTaint := NotReadyTaintTemplate

	ctx := context.TODO()
	fakeNodeHandler := testutil.NewImprovedFakeNodeHandler(nodes, pods)
	nodeController, _ := newNodeLifecycleControllerFromClient(
		ctx,
		fakeNodeHandler,
		testRateLimiterQPS,
		testRateLimiterQPS,
		testLargeClusterThreshold,
		testUnhealthyThreshold,
		testNodeMonitorGracePeriod,
		testNodeStartupGracePeriod,
		testNodeMonitorPeriod,
	)
	nodeController.now = func() metav1.Time { return fakeNow }
	nodeController.recorder = testutil.NewFakeRecorder()
	//nodeController.getPodsAssignedToNode = fakeGetPodsAssignedToNode(fakeNodeHandler.Clientset)
	//if err := nodeController.syncNodeStore(fakeNodeHandler); err != nil {
	//	t.Errorf("unexpected error: %v", err)
	//}
	if err := nodeController.monitorNodeHealth(ctx); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	nodeController.doNoExecuteTaintingPass(ctx)

	node0, err := fakeNodeHandler.DelegateNodeHandler.Get(ctx, "node0", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Can't get current node0...")
		return
	}
	node1, err := fakeNodeHandler.DelegateNodeHandler.Get(ctx, "node1", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Can't get current node1...")
		return
	}

	if originalTaint != nil && !taintutils.TaintExists(node0.Spec.Taints, originalTaint) {
		t.Errorf("Can't find taint %v in %v", originalTaint, node0.Spec.Taints)
	}

	nodeController.now = func() metav1.Time { return metav1.Time{Time: fakeNow.Time} }

	node0.Status = newNodeStatus
	node1.Status = healthyNodeNewStatus
	_, err = fakeNodeHandler.DelegateNodeHandler.UpdateStatus(ctx, node0, metav1.UpdateOptions{})
	if err != nil {
		t.Errorf(err.Error())
		return
	}
	_, err = fakeNodeHandler.DelegateNodeHandler.UpdateStatus(ctx, node1, metav1.UpdateOptions{})
	if err != nil {
		t.Errorf(err.Error())
		return
	}

	//if err := nodeController.syncNodeStore(fakeNodeHandler); err != nil {
	//	t.Errorf("unexpected error: %v", err)
	//}
	if err := nodeController.monitorNodeHealth(ctx); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	nodeController.doNoExecuteTaintingPass(ctx)

	node0, err = fakeNodeHandler.DelegateNodeHandler.Get(ctx, "node0", metav1.GetOptions{})
	if err != nil {
		t.Errorf("Can't get current node0...")
		return
	}
	if updatedTaint != nil {
		if !taintutils.TaintExists(node0.Spec.Taints, updatedTaint) {
			t.Errorf("Can't find taint %v in %v", updatedTaint, node0.Spec.Taints)
		}
	}
}

func TestTaintsNodeByCondition(t *testing.T) {
	fakeNow := metav1.Date(2017, 1, 1, 12, 0, 0, 0, time.UTC)

	//fakeNodeHandler := &testutil.FakeNodeHandler{
	//	Existing: []*v1.Node{
	nodes := []*v1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "node0",
				CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
				Labels: map[string]string{
					v1.LabelTopologyRegion:          "region1",
					v1.LabelTopologyZone:            "zone1",
					v1.LabelFailureDomainBetaRegion: "region1",
					v1.LabelFailureDomainBetaZone:   "zone1",
				},
			},
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type:               v1.NodeReady,
						Status:             v1.ConditionTrue,
						LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
						LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
					},
				},
			},
		},
	}
	//Clientset: fake.NewSimpleClientset(&v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}}),
	pods := &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}}
	//}

	ctx := context.TODO()
	fakeNodeHandler := testutil.NewImprovedFakeNodeHandler(nodes, pods)
	nodeController, _ := newNodeLifecycleControllerFromClient(
		ctx,
		fakeNodeHandler,
		testRateLimiterQPS,
		testRateLimiterQPS,
		testLargeClusterThreshold,
		testUnhealthyThreshold,
		testNodeMonitorGracePeriod,
		testNodeStartupGracePeriod,
		testNodeMonitorPeriod,
	)
	nodeController.now = func() metav1.Time { return fakeNow }
	nodeController.recorder = testutil.NewFakeRecorder()
	//nodeController.getPodsAssignedToNode = fakeGetPodsAssignedToNode(fakeNodeHandler.Clientset)

	networkUnavailableTaint := &v1.Taint{
		Key:    v1.TaintNodeNetworkUnavailable,
		Effect: v1.TaintEffectNoSchedule,
	}
	notReadyTaint := &v1.Taint{
		Key:    v1.TaintNodeNotReady,
		Effect: v1.TaintEffectNoSchedule,
	}
	unreachableTaint := &v1.Taint{
		Key:    v1.TaintNodeUnreachable,
		Effect: v1.TaintEffectNoSchedule,
	}

	tests := []struct {
		Name           string
		Node           *v1.Node
		ExpectedTaints []*v1.Taint
	}{
		{
			Name: "NetworkUnavailable is true",
			Node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "node0",
					CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
					Labels: map[string]string{
						v1.LabelTopologyRegion:          "region1",
						v1.LabelTopologyZone:            "zone1",
						v1.LabelFailureDomainBetaRegion: "region1",
						v1.LabelFailureDomainBetaZone:   "zone1",
					},
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:               v1.NodeReady,
							Status:             v1.ConditionTrue,
							LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
						},
						{
							Type:               v1.NodeNetworkUnavailable,
							Status:             v1.ConditionTrue,
							LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
						},
					},
				},
			},
			ExpectedTaints: []*v1.Taint{networkUnavailableTaint},
		},
		{
			Name: "NetworkUnavailable is true",
			Node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "node0",
					CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
					Labels: map[string]string{
						v1.LabelTopologyRegion:          "region1",
						v1.LabelTopologyZone:            "zone1",
						v1.LabelFailureDomainBetaRegion: "region1",
						v1.LabelFailureDomainBetaZone:   "zone1",
					},
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:               v1.NodeReady,
							Status:             v1.ConditionTrue,
							LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
						},
						{
							Type:               v1.NodeNetworkUnavailable,
							Status:             v1.ConditionTrue,
							LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
						},
					},
				},
			},
			ExpectedTaints: []*v1.Taint{networkUnavailableTaint},
		},
		{
			Name: "Ready is false",
			Node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "node0",
					CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
					Labels: map[string]string{
						v1.LabelTopologyRegion:          "region1",
						v1.LabelTopologyZone:            "zone1",
						v1.LabelFailureDomainBetaRegion: "region1",
						v1.LabelFailureDomainBetaZone:   "zone1",
					},
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:               v1.NodeReady,
							Status:             v1.ConditionFalse,
							LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
						},
					},
				},
			},
			ExpectedTaints: []*v1.Taint{notReadyTaint},
		},
		{
			Name: "Ready is unknown",
			Node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "node0",
					CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
					Labels: map[string]string{
						v1.LabelTopologyRegion:          "region1",
						v1.LabelTopologyZone:            "zone1",
						v1.LabelFailureDomainBetaRegion: "region1",
						v1.LabelFailureDomainBetaZone:   "zone1",
					},
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:               v1.NodeReady,
							Status:             v1.ConditionUnknown,
							LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
							LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
						},
					},
				},
			},
			ExpectedTaints: []*v1.Taint{unreachableTaint},
		},
	}

	for _, test := range tests {
		fakeNodeHandler.DelegateNodeHandler.Update(ctx, test.Node, metav1.UpdateOptions{})
		//if err := nodeController.syncNodeStore(fakeNodeHandler); err != nil {
		//	t.Errorf("unexpected error: %v", err)
		//}
		nodeController.doNoScheduleTaintingPass(ctx, test.Node.Name)
		//if err := nodeController.syncNodeStore(fakeNodeHandler); err != nil {
		//	t.Errorf("unexpected error: %v", err)
		//}
		node0 := new(v1.Node)
		err := nodeController.controllerRuntimeClient.Get(ctx, client.ObjectKey{Name: "node0"}, node0)
		if err != nil {
			t.Errorf("Can't get current node0...")
			return
		}
		if len(node0.Spec.Taints) != len(test.ExpectedTaints) {
			t.Errorf("%s: Unexpected number of taints: expected %d, got %d",
				test.Name, len(test.ExpectedTaints), len(node0.Spec.Taints))
		}
		for _, taint := range test.ExpectedTaints {
			if !taintutils.TaintExists(node0.Spec.Taints, taint) {
				t.Errorf("%s: Can't find taint %v in %v", test.Name, taint, node0.Spec.Taints)
			}
		}
	}
}

func TestNodeEventGeneration(t *testing.T) {
	fakeNow := metav1.Date(2016, 9, 10, 12, 0, 0, 0, time.UTC)
	//fakeNodeHandler := &testutil.FakeNodeHandler{
	//	Existing: []*v1.Node{
	nodes := []*v1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "node0",
				UID:               "1234567890",
				CreationTimestamp: metav1.Date(2015, 8, 10, 0, 0, 0, 0, time.UTC),
			},
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type:               v1.NodeReady,
						Status:             v1.ConditionUnknown,
						LastHeartbeatTime:  metav1.Date(2015, 8, 10, 0, 0, 0, 0, time.UTC),
						LastTransitionTime: metav1.Date(2015, 8, 10, 0, 0, 0, 0, time.UTC),
					},
				},
			},
		},
	}
	//Clientset: fake.NewSimpleClientset(&v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}}),
	pods := &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}}
	//}

	ctx := context.TODO()
	fakeNodeHandler := testutil.NewImprovedFakeNodeHandler(nodes, pods)
	nodeController, _ := newNodeLifecycleControllerFromClient(
		ctx,
		fakeNodeHandler,
		testRateLimiterQPS,
		testRateLimiterQPS,
		testLargeClusterThreshold,
		testUnhealthyThreshold,
		testNodeMonitorGracePeriod,
		testNodeStartupGracePeriod,
		testNodeMonitorPeriod,
	)
	nodeController.now = func() metav1.Time { return fakeNow }
	fakeRecorder := testutil.NewFakeRecorder()
	nodeController.recorder = fakeRecorder
	//nodeController.getPodsAssignedToNode = fakeGetPodsAssignedToNode(fakeNodeHandler.Clientset)

	//if err := nodeController.syncNodeStore(fakeNodeHandler); err != nil {
	//	t.Errorf("unexpected error: %v", err)
	//}
	if err := nodeController.monitorNodeHealth(ctx); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(fakeRecorder.Events) != 1 {
		t.Fatalf("unexpected events, got %v, expected %v: %+v", len(fakeRecorder.Events), 1, fakeRecorder.Events)
	}
	if fakeRecorder.Events[0].Reason != "RegisteredNode" {
		var reasons []string
		for _, event := range fakeRecorder.Events {
			reasons = append(reasons, event.Reason)
		}
		t.Fatalf("unexpected events generation: %v", strings.Join(reasons, ","))
	}
	for _, event := range fakeRecorder.Events {
		involvedObject := event.InvolvedObject
		actualUID := string(involvedObject.UID)
		if actualUID != "1234567890" {
			t.Fatalf("unexpected event uid: %v", actualUID)
		}
	}
}

func TestReconcileNodeLabels(t *testing.T) {
	fakeNow := metav1.Date(2017, 1, 1, 12, 0, 0, 0, time.UTC)

	//fakeNodeHandler := &testutil.FakeNodeHandler{
	//	Existing: []*v1.Node{
	nodes := []*v1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "node0",
				CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
				Labels: map[string]string{
					v1.LabelTopologyRegion:          "region1",
					v1.LabelTopologyZone:            "zone1",
					v1.LabelFailureDomainBetaRegion: "region1",
					v1.LabelFailureDomainBetaZone:   "zone1",
				},
			},
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type:               v1.NodeReady,
						Status:             v1.ConditionTrue,
						LastHeartbeatTime:  metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
						LastTransitionTime: metav1.Date(2015, 1, 1, 12, 0, 0, 0, time.UTC),
					},
				},
			},
		},
	}
	//Clientset: fake.NewSimpleClientset(&v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}}),
	pods := &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}}
	//}

	ctx := context.TODO()
	fakeNodeHandler := testutil.NewImprovedFakeNodeHandler(nodes, pods)
	nodeController, _ := newNodeLifecycleControllerFromClient(
		ctx,
		fakeNodeHandler,
		testRateLimiterQPS,
		testRateLimiterQPS,
		testLargeClusterThreshold,
		testUnhealthyThreshold,
		testNodeMonitorGracePeriod,
		testNodeStartupGracePeriod,
		testNodeMonitorPeriod,
	)
	nodeController.now = func() metav1.Time { return fakeNow }
	nodeController.recorder = testutil.NewFakeRecorder()
	//nodeController.getPodsAssignedToNode = fakeGetPodsAssignedToNode(fakeNodeHandler.Clientset)

	tests := []struct {
		Name           string
		Node           *v1.Node
		ExpectedLabels map[string]string
	}{
		{
			Name: "No-op if node has no labels",
			Node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "node0",
					CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
				},
			},
			ExpectedLabels: nil,
		},
		{
			Name: "No-op if no target labels present",
			Node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "node0",
					CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
					Labels: map[string]string{
						v1.LabelTopologyRegion: "region1",
					},
				},
			},
			ExpectedLabels: map[string]string{
				v1.LabelTopologyRegion: "region1",
			},
		},
		{
			Name: "Create OS/arch beta labels when they don't exist",
			Node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "node0",
					CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
					Labels: map[string]string{
						v1.LabelOSStable:   "linux",
						v1.LabelArchStable: "amd64",
					},
				},
			},
			ExpectedLabels: map[string]string{
				kubeletapis.LabelOS:   "linux",
				kubeletapis.LabelArch: "amd64",
				v1.LabelOSStable:      "linux",
				v1.LabelArchStable:    "amd64",
			},
		},
		{
			Name: "Reconcile OS/arch beta labels to match stable labels",
			Node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "node0",
					CreationTimestamp: metav1.Date(2012, 1, 1, 0, 0, 0, 0, time.UTC),
					Labels: map[string]string{
						kubeletapis.LabelOS:   "windows",
						kubeletapis.LabelArch: "arm",
						v1.LabelOSStable:      "linux",
						v1.LabelArchStable:    "amd64",
					},
				},
			},
			ExpectedLabels: map[string]string{
				kubeletapis.LabelOS:   "linux",
				kubeletapis.LabelArch: "amd64",
				v1.LabelOSStable:      "linux",
				v1.LabelArchStable:    "amd64",
			},
		},
	}

	for _, test := range tests {
		fakeNodeHandler.DelegateNodeHandler.Update(ctx, test.Node, metav1.UpdateOptions{})
		//if err := nodeController.syncNodeStore(fakeNodeHandler); err != nil {
		//	t.Fatalf("unexpected error: %v", err)
		//}
		nodeController.reconcileNodeLabels(ctx, test.Node.Name)
		//if err := nodeController.syncNodeStore(fakeNodeHandler); err != nil {
		//	t.Fatalf("unexpected error: %v", err)
		//}
		node0 := new(v1.Node)
		err := nodeController.controllerRuntimeClient.Get(ctx, client.ObjectKey{Name: "node0"}, node0)
		if err != nil {
			t.Fatalf("Can't get current node0...")
		}
		if len(node0.Labels) != len(test.ExpectedLabels) {
			t.Errorf("%s: Unexpected number of taints: expected %d, got %d",
				test.Name, len(test.ExpectedLabels), len(node0.Labels))
		}
		for key, expectedValue := range test.ExpectedLabels {
			actualValue, ok := node0.Labels[key]
			if !ok {
				t.Errorf("%s: Can't find label %v in %v", test.Name, key, node0.Labels)
			}
			if actualValue != expectedValue {
				t.Errorf("%s: label %q: expected value %q, got value %q", test.Name, key, expectedValue, actualValue)
			}
		}
	}
}

func TestTryUpdateNodeHealth(t *testing.T) {
	fakeNow := metav1.Date(2017, 1, 1, 12, 0, 0, 0, time.UTC)
	fakeOld := metav1.Date(2016, 1, 1, 12, 0, 0, 0, time.UTC)

	//fakeNodeHandler := &testutil.FakeNodeHandler{
	//	Existing: []*v1.Node{
	nodes := []*v1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "node0",
				CreationTimestamp: fakeNow,
			},
			Status: v1.NodeStatus{
				Conditions: []v1.NodeCondition{
					{
						Type:               v1.NodeReady,
						Status:             v1.ConditionTrue,
						LastHeartbeatTime:  fakeNow,
						LastTransitionTime: fakeNow,
					},
				},
			},
		},
	}
	//Clientset: fake.NewSimpleClientset(&v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}}),
	pods := &v1.PodList{Items: []v1.Pod{*testutil.NewPod("pod0", "node0")}}
	//}

	ctx := context.TODO()
	fakeNodeHandler := testutil.NewImprovedFakeNodeHandler(nodes, pods)
	nodeController, _ := newNodeLifecycleControllerFromClient(
		ctx,
		fakeNodeHandler,
		testRateLimiterQPS,
		testRateLimiterQPS,
		testLargeClusterThreshold,
		testUnhealthyThreshold,
		testNodeMonitorGracePeriod,
		testNodeStartupGracePeriod,
		testNodeMonitorPeriod,
	)
	nodeController.now = func() metav1.Time { return fakeNow }
	nodeController.recorder = testutil.NewFakeRecorder()
	//nodeController.getPodsAssignedToNode = fakeGetPodsAssignedToNode(fakeNodeHandler.Clientset)

	getStatus := func(cond *v1.NodeCondition) *v1.ConditionStatus {
		if cond == nil {
			return nil
		}
		return &cond.Status
	}

	tests := []struct {
		name string
		node *v1.Node
	}{
		{
			name: "Status true",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "node0",
					CreationTimestamp: fakeNow,
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:               v1.NodeReady,
							Status:             v1.ConditionTrue,
							LastHeartbeatTime:  fakeNow,
							LastTransitionTime: fakeNow,
						},
					},
				},
			},
		},
		{
			name: "Status false",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "node0",
					CreationTimestamp: fakeNow,
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:               v1.NodeReady,
							Status:             v1.ConditionFalse,
							LastHeartbeatTime:  fakeNow,
							LastTransitionTime: fakeNow,
						},
					},
				},
			},
		},
		{
			name: "Status unknown",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "node0",
					CreationTimestamp: fakeNow,
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:               v1.NodeReady,
							Status:             v1.ConditionUnknown,
							LastHeartbeatTime:  fakeNow,
							LastTransitionTime: fakeNow,
						},
					},
				},
			},
		},
		{
			name: "Status nil",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "node0",
					CreationTimestamp: fakeNow,
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{},
				},
			},
		},
		{
			name: "Status true - after grace period",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "node0",
					CreationTimestamp: fakeOld,
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:               v1.NodeReady,
							Status:             v1.ConditionTrue,
							LastHeartbeatTime:  fakeOld,
							LastTransitionTime: fakeOld,
						},
					},
				},
			},
		},
		{
			name: "Status false - after grace period",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "node0",
					CreationTimestamp: fakeOld,
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:               v1.NodeReady,
							Status:             v1.ConditionFalse,
							LastHeartbeatTime:  fakeOld,
							LastTransitionTime: fakeOld,
						},
					},
				},
			},
		},
		{
			name: "Status unknown - after grace period",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "node0",
					CreationTimestamp: fakeOld,
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{
						{
							Type:               v1.NodeReady,
							Status:             v1.ConditionUnknown,
							LastHeartbeatTime:  fakeOld,
							LastTransitionTime: fakeOld,
						},
					},
				},
			},
		},
		{
			name: "Status nil - after grace period",
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "node0",
					CreationTimestamp: fakeOld,
				},
				Status: v1.NodeStatus{
					Conditions: []v1.NodeCondition{},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nodeController.nodeHealthMap.set(test.node.Name, &nodeHealthData{
				status:                   &test.node.Status,
				probeTimestamp:           test.node.CreationTimestamp,
				readyTransitionTimestamp: test.node.CreationTimestamp,
			})
			_, _, currentReadyCondition, err := nodeController.tryUpdateNodeHealth(ctx, test.node)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			_, savedReadyCondition := nodeutil.GetNodeCondition(nodeController.nodeHealthMap.getDeepCopy(test.node.Name).status, v1.NodeReady)
			savedStatus := getStatus(savedReadyCondition)
			currentStatus := getStatus(currentReadyCondition)
			if !apiequality.Semantic.DeepEqual(currentStatus, savedStatus) {
				t.Errorf("expected %v, got %v", savedStatus, currentStatus)
			}
		})
	}
}

func Test_isNodeExcludedFromDisruptionChecks(t *testing.T) {
	validNodeStatus := v1.NodeStatus{Conditions: []v1.NodeCondition{{Type: "Test"}}}
	tests := []struct {
		name string

		input *v1.Node
		want  bool
	}{
		{want: false, input: &v1.Node{Status: validNodeStatus, ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{}}}},
		{want: false, input: &v1.Node{Status: validNodeStatus, ObjectMeta: metav1.ObjectMeta{Name: "master-abc"}}},
		{want: true, input: &v1.Node{Status: validNodeStatus, ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{labelNodeDisruptionExclusion: ""}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if result := isNodeExcludedFromDisruptionChecks(tt.input); result != tt.want {
				t.Errorf("isNodeExcludedFromDisruptionChecks() = %v, want %v", result, tt.want)
			}
		})
	}
}
