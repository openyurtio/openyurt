/*
Copyright 2024 The OpenYurt Authors.

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

package autonomy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apirequest "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"

	appsv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	hubrest "github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/rest"
	proxyutil "github.com/openyurtio/openyurt/pkg/yurthub/proxy/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	nodeStatusUpdateRetry = 5
	maxCacheFailures      = 3
)

var (
	ErrRestConfigMgr = errors.New("failed to initialize restConfigMgr")
)

type AutonomyProxy struct {
	cacheMgr         cachemanager.CacheManager
	restConfigMgr    *hubrest.RestConfigManager
	cacheFailedCount *int32
}

func NewAutonomyProxy(
	restConfigMgr *hubrest.RestConfigManager,
	cacheMgr cachemanager.CacheManager,
) *AutonomyProxy {
	return &AutonomyProxy{
		restConfigMgr:    restConfigMgr,
		cacheMgr:         cacheMgr,
		cacheFailedCount: pointer.Int32(0),
	}
}

func (ap *AutonomyProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	obj, err := ap.updateNodeStatus(req)
	if err != nil {
		proxyutil.Err(err, rw, req)
	}
	util.WriteObject(http.StatusOK, obj, rw, req)
}

func (ap *AutonomyProxy) updateNodeStatus(req *http.Request) (runtime.Object, error) {
	_, ok := apirequest.RequestInfoFrom(req.Context())
	if !ok {
		return nil, fmt.Errorf("failed to resolve request")
	}

	var node, retNode runtime.Object
	var err error
	for i := 0; i < nodeStatusUpdateRetry; i++ {
		node, err = ap.tryUpdateNodeConditions(i, req)
		if node != nil {
			retNode = node
		}
		if errors.Is(err, ErrRestConfigMgr) {
			break
		} else if err != nil {
			klog.ErrorS(err, "Error getting or updating node status, will retry")
		} else {
			return retNode, nil
		}
	}
	if retNode == nil {
		return nil, fmt.Errorf("failed to get node")
	}
	klog.ErrorS(err, "failed to update node autonomy status")
	return retNode, nil
}

func (ap *AutonomyProxy) tryUpdateNodeConditions(tryNumber int, req *http.Request) (runtime.Object, error) {
	var originalNode, updatedNode *v1.Node
	var err error
	info, _ := apirequest.RequestInfoFrom(req.Context())

	if tryNumber == 0 {
		// get from local cache
		obj, err := ap.cacheMgr.QueryCache(req)
		if err != nil {
			return nil, err
		}
		ok := false
		originalNode, ok = obj.(*v1.Node)
		if !ok {
			return nil, fmt.Errorf("could not QueryCache, node is not found")
		}
	} else {
		// initialize client from restConfigMgr
		if ap.restConfigMgr == nil {
			return nil, ErrRestConfigMgr
		}
		config := ap.restConfigMgr.GetRestConfig(true)
		client, err := kubernetes.NewForConfig(config)
		if err != nil {
			return nil, err
		}

		// get node from cloud
		// when tryNumber equals to 1, get from apiServer cache
		// otherwise, get from etcd
		opts := metav1.GetOptions{}
		if tryNumber == 1 {
			util.FromApiserverCache(&opts)
		}
		originalNode, err = client.CoreV1().Nodes().Get(context.TODO(), info.Name, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to get node from cloud: %v", err)
		}
	}

	if originalNode == nil {
		return nil, fmt.Errorf("get nil node object: %s", info.Name)
	}

	changedNode, changed := ap.updateNodeConditions(originalNode)
	if !changed {
		return originalNode, nil
	}

	if ap.restConfigMgr == nil {
		return originalNode, ErrRestConfigMgr
	}
	config := ap.restConfigMgr.GetRestConfig(true)
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return originalNode, err
	}

	updatedNode, err = client.CoreV1().Nodes().UpdateStatus(context.TODO(), changedNode, metav1.UpdateOptions{})
	if err != nil {
		return originalNode, err
	}
	return updatedNode, nil
}

func (ap *AutonomyProxy) updateNodeConditions(originalNode *v1.Node) (*v1.Node, bool) {
	node := originalNode.DeepCopy()
	if _, ok := node.Annotations[projectinfo.GetAutonomyAnnotation()]; !ok {
		setNodeAutonomyCondition(node, v1.ConditionFalse, "autonomy disabled", "The autonomy is disabled or this node is not edge node")
	} else if node.Annotations[projectinfo.GetAutonomyAnnotation()] == "false" || node.Labels[projectinfo.GetEdgeWorkerLabelKey()] == "false" {
		setNodeAutonomyCondition(node, v1.ConditionFalse, "autonomy disabled", "The autonomy is disabled or this node is not edge node")
	} else {
		res := ap.cacheMgr.QueryCacheResult()
		if res.Length == 0 {
			setNodeAutonomyCondition(node, v1.ConditionTrue, "autonomy enabled successfully", "The autonomy is enabled and it works fine")
			atomic.StoreInt32(ap.cacheFailedCount, 0)
		} else {
			currentFailures := atomic.AddInt32(ap.cacheFailedCount, 1)
			if int(currentFailures) > maxCacheFailures {
				setNodeAutonomyCondition(node, v1.ConditionUnknown, "cache failed", res.Msg)
			}
		}
	}
	return node, util.NodeConditionsHaveChanged(originalNode.Status.Conditions, node.Status.Conditions)
}

func setNodeAutonomyCondition(node *v1.Node, expectedStatus v1.ConditionStatus, reason, message string) {
	for i := range node.Status.Conditions {
		if node.Status.Conditions[i].Type == appsv1beta1.NodeAutonomy {
			if node.Status.Conditions[i].Status == expectedStatus {
				return
			} else {
				node.Status.Conditions[i].Status = expectedStatus
				node.Status.Conditions[i].Reason = reason
				node.Status.Conditions[i].Message = message
				node.Status.Conditions[i].LastHeartbeatTime = metav1.Now()
				node.Status.Conditions[i].LastHeartbeatTime = metav1.Now()
				return
			}
		}
	}

	node.Status.Conditions = append(node.Status.Conditions, v1.NodeCondition{
		Type:               appsv1beta1.NodeAutonomy,
		Status:             expectedStatus,
		Reason:             reason,
		Message:            message,
		LastHeartbeatTime:  metav1.Now(),
		LastTransitionTime: metav1.Now(),
	})
}
