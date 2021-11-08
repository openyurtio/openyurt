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
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurtctl/constants"
	"github.com/openyurtio/openyurt/pkg/yurtctl/lock"
	enutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/edgenode"
	kubeutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/kubernetes"
	strutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/strings"
	tunneldns "github.com/openyurtio/openyurt/pkg/yurttunnel/dns"
)

// RevertOptions has the information required by the revert operation
type RevertOptions struct {
	clientSet               *kubernetes.Clientset
	YurtctlServantImage     string
	PodMainfestPath         string
	KubeadmConfPath         string
	yurtAppManagerClientSet dynamic.Interface
}

// NewRevertOptions creates a new RevertOptions
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

	cmd.AddCommand(NewRevertEdgeNodeCmd())
	cmd.AddCommand(NewRevertCloudNodeCmd())

	cmd.Flags().String("yurtctl-servant-image",
		"openyurt/yurtctl-servant:latest",
		"The yurtctl-servant image.")
	cmd.Flags().String("kubeadm-conf-path",
		"/etc/systemd/system/kubelet.service.d/10-kubeadm.conf",
		"The path to kubelet service conf that is used by kubelet component to join the cluster on the edge node.")

	return cmd
}

// Complete completes all the required options
func (ro *RevertOptions) Complete(flags *pflag.FlagSet) error {
	ycsi, err := flags.GetString("yurtctl-servant-image")
	if err != nil {
		return err
	}
	ro.YurtctlServantImage = ycsi

	kcp, err := flags.GetString("kubeadm-conf-path")
	if err != nil {
		return err
	}
	ro.KubeadmConfPath = kcp

	ro.PodMainfestPath = enutil.GetPodManifestPath()

	ro.clientSet, err = kubeutil.GenClientSet(flags)
	if err != nil {
		return err
	}

	ro.yurtAppManagerClientSet, err = kubeutil.GenDynamicClientSet(flags)
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
		if deleteLockErr := lock.DeleteLock(ro.clientSet); deleteLockErr != nil {
			klog.Error(deleteLockErr)
		}
	}()
	klog.V(4).Info("successfully acquire the lock")

	// 1. check the server version
	if err = kubeutil.ValidateServerVersion(ro.clientSet); err != nil {
		return
	}
	klog.V(4).Info("the server version is valid")

	// 1.1. get kube-controller-manager HA nodes
	kcmNodeNames, err := kubeutil.GetKubeControllerManagerHANodes(ro.clientSet)
	if err != nil {
		return
	}

	// 1.2. check the state of worker nodes
	nodeLst, err := ro.clientSet.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return
	}

	for _, node := range nodeLst.Items {
		isEdgeNode, ok := node.Labels[projectinfo.GetEdgeWorkerLabelKey()]
		if ok && isEdgeNode == "true" || strutil.IsInStringLst(kcmNodeNames, node.GetName()) {
			if !isNodeReady(&node.Status) {
				klog.Errorf("Cannot do the revert, the status of worker or kube-controller-manager node: %s is not 'Ready'.", node.Name)
				return
			}
		}
	}
	klog.V(4).Info("the status of worker nodes and kube-controller-manager nodes are satisfied")

	var edgeNodeNames, cloudNodeNames []string
	for _, node := range nodeLst.Items {
		isEdgeNode := node.Labels[projectinfo.GetEdgeWorkerLabelKey()]
		// cache edge nodes and cloud nodes, we need to run servant job on each node later
		if isEdgeNode == "true" {
			edgeNodeNames = append(edgeNodeNames, node.GetName())
		}
		if isEdgeNode == "false" {
			cloudNodeNames = append(cloudNodeNames, node.GetName())
		}
	}

	// 2. remove the yurt controller manager
	if err = ro.clientSet.AppsV1().Deployments("kube-system").
		Delete(context.Background(), "yurt-controller-manager", metav1.DeleteOptions{
			PropagationPolicy: &kubeutil.PropagationPolicy,
		}); err != nil && !apierrors.IsNotFound(err) {
		klog.Errorf("fail to remove yurt controller manager: %s", err)
		return
	}
	klog.Info("yurt controller manager is removed")

	// 2.1 remove the serviceaccount for yurt-controller-manager
	if err = ro.clientSet.CoreV1().ServiceAccounts("kube-system").
		Delete(context.Background(), "yurt-controller-manager", metav1.DeleteOptions{
			PropagationPolicy: &kubeutil.PropagationPolicy,
		}); err != nil && !apierrors.IsNotFound(err) {
		klog.Errorf("fail to remove serviceaccount for yurt controller manager: %s", err)
		return
	}
	klog.Info("serviceaccount for yurt controller manager is removed")

	// 2.2 remove the clusterrole for yurt-controller-manager
	if err = ro.clientSet.RbacV1().ClusterRoles().
		Delete(context.Background(), "yurt-controller-manager", metav1.DeleteOptions{
			PropagationPolicy: &kubeutil.PropagationPolicy,
		}); err != nil && !apierrors.IsNotFound(err) {
		klog.Errorf("fail to remove clusterrole for yurt controller manager: %s", err)
		return
	}
	klog.Info("clusterrole for yurt controller manager is removed")

	// 2.3 remove the clusterrolebinding for yurt-controller-manager
	if err = ro.clientSet.RbacV1().ClusterRoleBindings().
		Delete(context.Background(), "yurt-controller-manager", metav1.DeleteOptions{
			PropagationPolicy: &kubeutil.PropagationPolicy,
		}); err != nil && !apierrors.IsNotFound(err) {
		klog.Errorf("fail to remove clusterrolebinding for yurt controller manager: %s", err)
		return
	}
	klog.Info("clusterrolebinding for yurt controller manager is removed")

	// 3. remove the yurt-tunnel agent
	if err = removeYurtTunnelAgent(ro.clientSet); err != nil {
		klog.Errorf("fail to remove the yurt tunnel agent: %s", err)
		return
	}

	// 4. remove the yurt-tunnel server
	if err = removeYurtTunnelServer(ro.clientSet); err != nil {
		klog.Errorf("fail to remove the yurt tunnel server: %s", err)
		return
	}

	// 5. remove the yurt app manager
	if err = removeYurtAppManager(ro.clientSet, ro.yurtAppManagerClientSet); err != nil {
		klog.Errorf("fail to remove the yurt app manager: %s", err)
		return
	}
	klog.Info("yurt app manager is removed")

	// 6. enable node-controller
	if err = kubeutil.RunServantJobs(ro.clientSet,
		map[string]string{
			"action":                "enable",
			"yurtctl_servant_image": ro.YurtctlServantImage,
			"pod_manifest_path":     ro.PodMainfestPath,
		},
		kcmNodeNames); err != nil {
		klog.Errorf("fail to run EnableNodeControllerJobs: %s", err)
		return
	}
	klog.Info("complete enabling node-controller")

	// 7. remove yurt-hub and revert kubelet service on edge nodes
	ctx := map[string]string{
		"action":                "revert",
		"yurtctl_servant_image": ro.YurtctlServantImage,
		"kubeadm_conf_path":     ro.KubeadmConfPath,
	}
	ctx["sub_command"] = "edgenode"
	if err = kubeutil.RunServantJobs(ro.clientSet, ctx, edgeNodeNames); err != nil {
		klog.Errorf("fail to revert edge node: %s", err)
		return
	}
	klog.Info("complete removing yurt-hub and resetting kubelet service on edge nodes")

	// 8. remove yurt-hub and revert kubelet service on cloud nodes
	ctx["sub_command"] = "cloudnode"
	if err = kubeutil.RunServantJobs(ro.clientSet, ctx, cloudNodeNames); err != nil {
		klog.Errorf("fail to revert edge node: %s", err)
		return
	}
	klog.Info("complete removing yurt-hub and resetting kubelet service on cloud nodes")

	// 9. remove yut-hub k8s config, roleBinding role
	err = kubeutil.DeleteYurthubSetting(ro.clientSet)
	if err != nil {
		klog.Error("DeleteYurthubSetting err: ", err)
		return err
	}
	klog.Info("delete yurthub clusterrole and clusterrolebinding")

	return
}

func removeYurtTunnelServer(client *kubernetes.Clientset) error {
	// 1. remove the DaemonSet
	if err := client.AppsV1().
		Deployments(constants.YurttunnelNamespace).
		Delete(context.Background(), constants.YurttunnelServerComponentName,
			metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fail to delete the daemonset/%s: %s",
			constants.YurttunnelServerComponentName, err)
	}
	klog.V(4).Infof("deployment/%s is deleted", constants.YurttunnelServerComponentName)

	// 2.1 remove the Service
	if err := client.CoreV1().Services(constants.YurttunnelNamespace).
		Delete(context.Background(), constants.YurttunnelServerSvcName,
			metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fail to delete the service/%s: %s",
			constants.YurttunnelServerSvcName, err)
	}
	klog.V(4).Infof("service/%s is deleted", constants.YurttunnelServerSvcName)

	// 2.2 remove the internal Service(type=ClusterIP)
	if err := client.CoreV1().Services(constants.YurttunnelNamespace).
		Delete(context.Background(), constants.YurttunnelServerInternalSvcName,
			metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fail to delete the service/%s: %s",
			constants.YurttunnelServerInternalSvcName, err)
	}
	klog.V(4).Infof("service/%s is deleted", constants.YurttunnelServerInternalSvcName)

	// 3. remove the ClusterRoleBinding
	if err := client.RbacV1().ClusterRoleBindings().
		Delete(context.Background(), constants.YurttunnelServerComponentName,
			metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fail to delete the clusterrolebinding/%s: %s",
			constants.YurttunnelServerComponentName, err)
	}
	klog.V(4).Infof("clusterrolebinding/%s is deleted", constants.YurttunnelServerComponentName)

	// 4. remove the SerivceAccount
	if err := client.CoreV1().ServiceAccounts(constants.YurttunnelNamespace).
		Delete(context.Background(), constants.YurttunnelServerComponentName,
			metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fail to delete the serviceaccount/%s: %s",
			constants.YurttunnelServerComponentName, err)
	}
	klog.V(4).Infof("serviceaccount/%s is deleted", constants.YurttunnelServerComponentName)

	// 5. remove the ClusterRole
	if err := client.RbacV1().ClusterRoles().
		Delete(context.Background(), constants.YurttunnelServerComponentName,
			metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fail to delete the clusterrole/%s: %s",
			constants.YurttunnelServerComponentName, err)
	}
	klog.V(4).Infof("clusterrole/%s is deleted", constants.YurttunnelServerComponentName)

	// 6. remove the yurt-tunnel-server-cfg
	if err := client.CoreV1().ConfigMaps(constants.YurttunnelNamespace).
		Delete(context.Background(), constants.YurttunnelServerCmName,
			metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fail to delete the configmap/%s: %s",
			constants.YurttunnelServerCmName, err)
	}

	// 7. remove the dns record configmap
	yurttunnelDnsRecordConfigMapName := tunneldns.GetYurtTunnelDNSRecordConfigMapName()
	if err := client.CoreV1().ConfigMaps(constants.YurttunnelNamespace).
		Delete(context.Background(), yurttunnelDnsRecordConfigMapName,
			metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fail to delete configmap/%s: %s",
			yurttunnelDnsRecordConfigMapName, err)
	}

	return nil
}

func removeYurtAppManager(client *kubernetes.Clientset, yurtAppManagerClientSet dynamic.Interface) error {
	// 1. remove the Deployment
	if err := client.AppsV1().
		Deployments("kube-system").
		Delete(context.Background(), constants.YurtAppManager,
			metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fail to delete the deployment/%s: %s",
			constants.YurtAppManager, err)
	}
	klog.V(4).Infof("deployment/%s is deleted", constants.YurtAppManager)

	// 2. remove the Role
	if err := client.RbacV1().Roles("kube-system").
		Delete(context.Background(), "yurt-app-leader-election-role",
			metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fail to delete the role/%s: %s",
			"yurt-app-leader-election-role", err)
	}

	// 3. remove the ClusterRole
	if err := client.RbacV1().ClusterRoles().
		Delete(context.Background(), "yurt-app-manager-role",
			metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fail to delete the clusterrole/%s: %s",
			"yurt-app-manager-role", err)
	}

	// 4. remove the ClusterRoleBinding
	if err := client.RbacV1().ClusterRoleBindings().
		Delete(context.Background(), "yurt-app-manager-rolebinding",
			metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fail to delete the clusterrolebinding/%s: %s",
			"yurt-app-manager-rolebinding", err)
	}
	klog.V(4).Infof("clusterrolebinding/%s is deleted", "yurt-app-manager-rolebinding")

	// 5. remove the RoleBinding
	if err := client.RbacV1().RoleBindings("kube-system").
		Delete(context.Background(), "yurt-app-leader-election-rolebinding",
			metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fail to delete the rolebinding/%s: %s",
			"yurt-app-leader-election-rolebinding", err)
	}
	klog.V(4).Infof("clusterrolebinding/%s is deleted", "yurt-app-leader-election-rolebinding")

	// 6 remove the Secret
	if err := client.CoreV1().Secrets("kube-system").
		Delete(context.Background(), "yurt-app-webhook-certs",
			metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fail to delete the secret/%s: %s",
			"yurt-app-webhook-certs", err)
	}
	klog.V(4).Infof("secret/%s is deleted", "yurt-app-webhook-certs")

	// 7 remove Service
	if err := client.CoreV1().Services("kube-system").
		Delete(context.Background(), "yurt-app-webhook-service",
			metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fail to delete the service/%s: %s",
			"yurt-app-webhook-service", err)
	}
	klog.V(4).Infof("service/%s is deleted", "yurt-app-webhook-service")

	// 8. remove the MutatingWebhookConfiguration
	if err := client.AdmissionregistrationV1beta1().MutatingWebhookConfigurations().
		Delete(context.Background(), "yurt-app-mutating-webhook-configuration",
			metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fail to delete the MutatingWebhookConfiguration/%s: %s",
			"yurt-app-mutating-webhook-configuration", err)
	}
	klog.V(4).Infof("MutatingWebhookConfiguration/%s is deleted", "yurt-app-mutating-webhook-configuration")

	// 9. remove the ValidatingWebhookConfiguration
	if err := client.AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().
		Delete(context.Background(), "yurt-app-validating-webhook-configuration",
			metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fail to delete the ValidatingWebhookConfiguration/%s: %s",
			"yurt-app-validating-webhook-configuration", err)
	}
	klog.V(4).Infof("ValidatingWebhookConfiguration/%s is deleted", "yurt-app-validating-webhook-configuration")

	// 10. remove nodepoolcrd
	if err := kubeutil.DeleteCRDResource(client, yurtAppManagerClientSet,
		"NodePool", "nodepools.apps.openyurt.io", []byte(constants.YurtAppManagerNodePool)); err != nil {
		return fmt.Errorf("fail to delete the NodePoolCRD/%s: %s",
			"nodepoolcrd", err)
	}
	klog.V(4).Infof("NodePoolCRD/%s is deleted", "NodePool")

	// 11. remove UnitedDeploymentcrd
	if err := kubeutil.DeleteCRDResource(client, yurtAppManagerClientSet,
		"UnitedDeployment", "uniteddeployments.apps.openyurt.io", []byte(constants.YurtAppManagerUnitedDeployment)); err != nil {
		return fmt.Errorf("fail to delete the UnitedDeploymentCRD/%s: %s",
			"UnitedDeployment", err)
	}
	klog.Info("removeYurtAppManager for yurt app manager is finished")
	klog.V(4).Infof("UnitedDeploymentCRD/%s is deleted", "UnitedDeployment")
	return nil
}

func removeYurtTunnelAgent(client *kubernetes.Clientset) error {
	// 1. remove the DaemonSet
	if err := client.AppsV1().
		DaemonSets(constants.YurttunnelNamespace).
		Delete(context.Background(), constants.YurttunnelAgentComponentName,
			metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fail to delete the daemonset/%s: %s",
			constants.YurttunnelAgentComponentName, err)
	}
	klog.V(4).Infof("daemonset/%s is deleted", constants.YurttunnelAgentComponentName)

	// 2. remove the ClusterRoleBinding
	if err := client.RbacV1().ClusterRoleBindings().
		Delete(context.Background(), constants.YurttunnelAgentComponentName,
			metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fail to delete the clusterrolebinding/%s: %s",
			constants.YurttunnelAgentComponentName, err)
	}
	klog.V(4).Infof("clusterrolebinding/%s is deleted", constants.YurttunnelAgentComponentName)

	// 3. remove the ClusterRole
	if err := client.RbacV1().ClusterRoles().
		Delete(context.Background(), constants.YurttunnelAgentComponentName,
			metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("fail to delete the clusterrole/%s: %s",
			constants.YurttunnelAgentComponentName, err)
	}
	klog.V(4).Infof("clusterrole/%s is deleted", constants.YurttunnelAgentComponentName)
	return nil
}
