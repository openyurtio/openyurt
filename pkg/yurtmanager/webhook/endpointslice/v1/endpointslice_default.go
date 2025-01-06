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

package v1

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	discovery "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nodeutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/node"
	podutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/pod"
)

// Default satisfies the defaulting webhook interface.
func (webhook *EndpointSliceHandler) Default(ctx context.Context, obj runtime.Object) error {
	endpoints, ok := obj.(*discovery.EndpointSlice)
	if !ok {
		apierrors.NewBadRequest(fmt.Sprintf("expected an EndpointSlice object but got %T", obj))
	}

	return remapAutonomyEndpoints(ctx, webhook.Client, endpoints)
}

// isNodeAutonomous checks if the node has autonomy annotations
// and returns true if it does, false otherwise.
func isNodeAutonomous(ctx context.Context, c client.Client, nodeName string) (bool, error) {
	node := &corev1.Node{}
	err := c.Get(ctx, client.ObjectKey{Name: nodeName}, node)
	if err != nil {
		// If node doesn't exist, it doesn't have autonomy
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	return nodeutil.IsPodBoundenToNode(node), nil
}

// isPodCrashLoopBackOff checks if the pod is crashloopbackoff
// and returns true if it is, false otherwise.
func isPodCrashLoopBackOff(ctx context.Context, c client.Client, podName, namespace string) (bool, error) {
	pod := &corev1.Pod{}
	err := c.Get(ctx, client.ObjectKey{Name: podName, Namespace: namespace}, pod)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	return podutil.IsPodCrashLoopBackOff(pod.Status), nil
}

// remapAutonomyEndpoints remaps the notReadyAddresses to the readyAddresses
func remapAutonomyEndpoints(ctx context.Context, client client.Client, slice *discovery.EndpointSlice) error {
	if slice == nil || len(slice.Endpoints) == 0 {
		return nil
	}

	readyServing := true
	for i, e := range slice.Endpoints {
		// If the endpoint is ready, skip
		if e.Conditions.Ready != nil && *e.Conditions.Ready {
			continue
		}

		// If the endpoint is terminating, skip
		if e.Conditions.Terminating != nil && *e.Conditions.Terminating {
			continue
		}

		// If the endpoint doesn't have a node name or target ref, skip
		if e.NodeName == nil || *e.NodeName == "" || e.TargetRef == nil {
			continue
		}

		isAutonomous, err := isNodeAutonomous(ctx, client, *e.NodeName)
		if err != nil {
			return err
		}

		// If the node doesn't have autonomy, skip
		if !isAutonomous {
			continue
		}

		isPodCrashLoopBackOff, err := isPodCrashLoopBackOff(ctx, client, e.TargetRef.Name, e.TargetRef.Namespace)
		if err != nil {
			return err
		}

		// If the pod is in crashloopbackoff, skip
		if isPodCrashLoopBackOff {
			continue
		}

		// Set not ready addresses to ready & serving
		if e.Conditions.Ready != nil {
			slice.Endpoints[i].Conditions.Ready = &readyServing
		}
		if e.Conditions.Serving != nil {
			slice.Endpoints[i].Conditions.Serving = &readyServing
		}
	}

	return nil
}
