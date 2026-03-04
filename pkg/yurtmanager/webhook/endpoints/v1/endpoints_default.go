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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nodeutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/node"
	podutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/pod"
)

// Default satisfies the defaulting webhook interface.
func (webhook *EndpointsHandler) Default(ctx context.Context, obj runtime.Object) error {
	//nolint:staticcheck // SA1019: corev1.Endpoints is deprecated but still supported for backward compatibility
	endpoints, ok := obj.(*corev1.Endpoints)
	if !ok {
		apierrors.NewBadRequest(fmt.Sprintf("expected an Endpoints object but got %T", obj))
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
// for the subsets scheduled to nodes that have autonomy annotations.
// The function checks the pod status and if the pod is not in crashloopbackoff,
// it remaps the address to readyAddresses.
//
//nolint:staticcheck // SA1019: corev1.Endpoints is deprecated but still supported for backward compatibility
func remapAutonomyEndpoints(ctx context.Context, client client.Client, endpoints *corev1.Endpoints) error {
	// Track nodes with autonomy to avoid repeated checks
	nodesWithAutonomy := make(map[string]bool)

	// Get all the notReadyAddresses for subsets
	for i, s := range endpoints.Subsets {
		// Create a zero-length slice with the same underlying array
		newNotReadyAddresses := s.NotReadyAddresses[:0]

		for _, a := range s.NotReadyAddresses {
			if a.NodeName == nil || a.TargetRef == nil {
				newNotReadyAddresses = append(newNotReadyAddresses, a)
				continue
			}

			// Get the node and check autonomy annotations
			hasAutonomy, ok := nodesWithAutonomy[*a.NodeName]
			if !ok {
				isAutonomous, err := isNodeAutonomous(ctx, client, *a.NodeName)
				if err != nil {
					return err
				}
				// Store autonomy status for future checks
				nodesWithAutonomy[*a.NodeName] = isAutonomous
				hasAutonomy = isAutonomous
			}

			// If the node doesn't have autonomy, skip
			if !hasAutonomy {
				newNotReadyAddresses = append(newNotReadyAddresses, a)
				continue
			}

			// Get the pod
			isPodCrashLoopBackOff, err := isPodCrashLoopBackOff(ctx, client, a.TargetRef.Name, a.TargetRef.Namespace)
			if err != nil {
				return err
			}

			if isPodCrashLoopBackOff {
				newNotReadyAddresses = append(newNotReadyAddresses, a)
				continue
			}

			// Move the address to the ready addresses in the subset
			endpoints.Subsets[i].Addresses = append(endpoints.Subsets[i].Addresses, *a.DeepCopy())
		}

		// Update the subset with the new notReadyAddresses
		endpoints.Subsets[i].NotReadyAddresses = newNotReadyAddresses
	}

	return nil
}
