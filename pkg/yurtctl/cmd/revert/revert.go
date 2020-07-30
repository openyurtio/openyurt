/*
Copyright 2020 The OpenYurt Authors.

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

package revert

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"

	"github.com/alibaba/openyurt/pkg/yurtctl/constants"
	"github.com/alibaba/openyurt/pkg/yurtctl/lock"
	kubeutil "github.com/alibaba/openyurt/pkg/yurtctl/util/kubernetes"
)

// ConvertOptions has the information required by the revert operation
type RevertOptions struct {
	clientSet           *kubernetes.Clientset
	YurtctlServantImage string
}

// NewConvertOptions creates a new RevertOptions
func NewRevertOptions() *RevertOptions {
	return &RevertOptions{}
}

// NewRevertCmd generates a new revert command
func NewRevertCmd() *cobra.Command {
	ro := NewRevertOptions()
	cmd := &cobra.Command{
		Use:   "revert",
		Short: "Reverts the yurt cluster back to a Kubernetes cluster",
		Run: func(cmd *cobra.Command, _ []string) {
			if err := ro.Complete(cmd.Flags()); err != nil {
				klog.Fatalf("fail to complete the revert option: %s", err)
			}
			if err := ro.RunRevert(); err != nil {
				klog.Fatalf("fail to revert yurt to kubernetes: %s", err)
			}
		},
	}

	cmd.Flags().String("yurtctl-servant-image",
		"openyurt/yurtctl-servant:latest",
		"The yurtctl-servant image.")

	return cmd
}

// Complete completes all the required options
func (ro *RevertOptions) Complete(flags *pflag.FlagSet) error {
	ycsi, err := flags.GetString("yurtctl-servant-image")
	if err != nil {
		return err
	}
	ro.YurtctlServantImage = ycsi

	ro.clientSet, err = kubeutil.GenClientSet(flags)
	if err != nil {
		return err
	}
	return nil
}

// RunRevert reverts the target Yurt cluster back to a standard Kubernetes cluster
func (ro *RevertOptions) RunRevert() (err error) {
	if err = lock.AcquireLock(ro.clientSet); err != nil {
		return
	}
	defer func() {
		if releaseLockErr := lock.ReleaseLock(ro.clientSet); releaseLockErr != nil {
			klog.Error(releaseLockErr)
		}
	}()
	klog.V(4).Info("successfully acquire the lock")

	// 1. check the server version
	if err = kubeutil.ValidateServerVersion(ro.clientSet); err != nil {
		return
	}
	klog.V(4).Info("the server version is valid")

	// 2. remove labels from nodes
	nodeLst, err := ro.clientSet.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return
	}

	var edgeNodeNames []string
	for _, node := range nodeLst.Items {
		isEdgeNode, ok := node.Labels[constants.LabelEdgeWorker]
		if ok && isEdgeNode == "true" {
			edgeNodeNames = append(edgeNodeNames, node.GetName())

			_, found := node.Annotations[constants.AnnotationAutonomy]
			if found {
				delete(node.Annotations, constants.AnnotationAutonomy)
			}
		}
		if ok {
			delete(node.Labels, constants.LabelEdgeWorker)
			if _, err = ro.clientSet.CoreV1().Nodes().Update(&node); err != nil {
				return
			}
		}
	}
	klog.Info("label alibabacloud.com/is-edge-worker is removed")

	// 3. remove the yurt controller manager
	if err = ro.clientSet.AppsV1().Deployments("kube-system").
		Delete("yurt-controller-manager", &metav1.DeleteOptions{
			PropagationPolicy: &kubeutil.PropagationPolicy,
		}); err != nil && !apierrors.IsNotFound(err) {
		klog.Errorf("fail to remove yurt controller manager: %s", err)
		return
	}
	klog.Info("yurt controller manager is removed")

	// 4. recreate the node-controller service account
	ncSa := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "node-controller",
			Namespace: "kube-system",
		},
	}
	if _, err = ro.clientSet.CoreV1().
		ServiceAccounts(ncSa.GetNamespace()).Create(ncSa); err != nil && !apierrors.IsAlreadyExists(err) {
		klog.Errorf("fail to create node-controller service account: %s", err)
		return
	}
	klog.Info("ServiceAccount node-controller is created")

	// 5. remove yurt-hub and revert kubelet service
	if err = kubeutil.RunServantJobs(ro.clientSet,
		map[string]string{
			"action":                "revert",
			"yurtctl_servant_image": ro.YurtctlServantImage,
		},
		edgeNodeNames); err != nil {
		klog.Errorf("fail to revert edge node: %s", err)
		return
	}
	klog.Info("yurt-hub is removed, kubelet service is reset")
	return
}
