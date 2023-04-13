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

package v1

import (
	"context"
	"fmt"
	"strings"
	"time"

	leasev1 "k8s.io/api/coordination/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/controller/poolcoordinator/constant"
)

const (
	LabelCurrentNodePool = "apps.openyurt.io/nodepool"
	UserNodeController   = "system:serviceaccount:kube-system:node-controller"

	NodeLeaseDurationSeconds                 = 40
	DefaultPoolReadyNodeNumberRatioThreshold = 0.35
)

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *PodHandler) ValidateCreate(ctx context.Context, obj runtime.Object, req admission.Request) error {
	return nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *PodHandler) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object, req admission.Request) error {
	return nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type.
func (webhook *PodHandler) ValidateDelete(_ context.Context, obj runtime.Object, req admission.Request) error {
	po, ok := obj.(*v1.Pod)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Pod but got a %T", obj))
	}

	if allErrs := validatePodDeletion(webhook.Client, po, req); len(allErrs) > 0 {
		return apierrors.NewInvalid(v1.SchemeGroupVersion.WithKind("Pod").GroupKind(), po.Name, allErrs)
	}
	return nil
}

func validatePodDeletion(cli client.Client, pod *v1.Pod, req admission.Request) field.ErrorList {
	if !userIsNodeController(req) {
		return nil
	}

	nodeName := pod.Spec.NodeName
	if nodeName == "" {
		return nil
	}

	node := v1.Node{}
	if err := cli.Get(context.TODO(), client.ObjectKey{Name: nodeName}, &node); err != nil {
		return nil
	}

	// only validate pod which in nodePool
	var nodePoolName string
	if node.Labels != nil {
		if name, ok := node.Labels[LabelCurrentNodePool]; ok {
			nodePoolName = name
		}
	}
	if nodePoolName == "" {
		return nil
	}

	// check number of ready nodes in node pool
	nodePool := appsv1alpha1.NodePool{}
	if err := cli.Get(context.TODO(), client.ObjectKey{Name: nodePoolName}, &nodePool); err != nil {
		return nil
	}
	nodeNumber := len(nodePool.Status.Nodes)
	if nodeNumber == 0 {
		return nil
	}
	readyNumber := countAliveNode(cli, nodePool.Status.Nodes)

	// When number of ready nodes in node pool is below a configurable parameter,
	// we don't allow pods to move within the pool any more.
	// This threshold defaults to one third of the number of pool's nodes.
	if float64(readyNumber)/float64(nodeNumber) < DefaultPoolReadyNodeNumberRatioThreshold {
		return field.ErrorList([]*field.Error{
			field.Invalid(field.NewPath("metadata").Child("name"), pod.Name, "nodePool has too few ready nodes")})
	}
	return nil
}

func userIsNodeController(req admission.Request) bool {
	// only user is node-controller can validate pod delete/evict
	return strings.Contains(req.UserInfo.Username, UserNodeController)
}

// countAliveNode return number of node alive
func countAliveNode(cli client.Client, nodes []string) int {
	cnt := 0
	for _, n := range nodes {
		if nodeIsAlive(cli, n) {
			cnt++
		}
	}
	return cnt
}

// nodeIsAlive return true if node is alive, otherwise is false
func nodeIsAlive(cli client.Client, nodeName string) bool {
	lease := leasev1.Lease{}
	if err := cli.Get(context.TODO(), client.ObjectKey{Namespace: v1.NamespaceNodeLease, Name: nodeName}, &lease); err != nil {
		return false
	}

	// check lease update time
	diff := time.Now().Sub(lease.Spec.RenewTime.Time)
	if diff.Seconds() > NodeLeaseDurationSeconds {
		return false
	}

	// check lease if delegate or not
	if lease.Annotations != nil && lease.Annotations[constant.DelegateHeartBeat] == "true" {
		return false
	}
	return true
}
