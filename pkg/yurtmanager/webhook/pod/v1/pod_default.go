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

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	nodeutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/node"
	podutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/pod"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtcoordinator/constant"
)

// Default satisfies the defaulting webhook interface.
func (webhook *PodHandler) Default(ctx context.Context, obj runtime.Object, req admission.Request) error {
	po, ok := obj.(*v1.Pod)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a Pod but got a %T", obj))
	}

	// if pod have ready condition and condition is False
	podReadyCondition := podutil.GetPodReadyCondition(po.Status)
	if podReadyCondition == nil {
		return nil
	}
	if podReadyCondition.Status != v1.ConditionFalse {
		return nil
	}

	// check the pod in which node and if node have pod_binding annotation
	nodeName := po.Spec.NodeName
	if nodeName == "" {
		return nil
	}
	node := v1.Node{}
	if err := webhook.Client.Get(context.TODO(), client.ObjectKey{Name: nodeName}, &node); err != nil {
		return nil
	}
	if node.Annotations != nil && node.Annotations[constant.PodBindingAnnotation] == "true" {
		// update pod ready condition to True
		podReadyCondition.Status = v1.ConditionTrue
		klog.V(2).Infof("PodWebhook: Updating ready condition of pod %v to true", po.Name)
		if !nodeutil.UpdatePodCondition(&po.Status, podReadyCondition) {
			return nil
		}
	}

	return nil
}
