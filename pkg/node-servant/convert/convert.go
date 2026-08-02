/*
Copyright 2026 The OpenYurt Authors.

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

package convert

import (
	"context"
	"fmt"
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	nodeservant "github.com/openyurtio/openyurt/pkg/node-servant"
	"github.com/openyurtio/openyurt/pkg/node-servant/components"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	yurthubutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/yurthub"
)

const (
	bootstrapModeKubeletCertificate   = "kubeletcertificate"
	multiplexerClusterRoleBindingName = "yurt-hub-multiplexer-binding"
)

var (
	getAPIServerAddressFunc = components.GetAPIServerAddress
	installYurthubFunc      = func(cfg *yurthubutil.YurthubHostConfig) error {
		return components.NewYurthubOperator(cfg).Install()
	}
	redirectKubeletFunc = func(openyurtDir string) error {
		return components.NewKubeletOperator(openyurtDir).RedirectTrafficToYurtHub()
	}
	restartContainersFunc = func(nodeName string) error {
		return components.RestartNonPauseContainers(nodeName, conversionJobPodPrefix(nodeName))
	}
	getKubeClientFunc = func(kubeadmConfPaths []string) (kubernetes.Interface, error) {
		cfg, err := rest.InClusterConfig()
		if err != nil {
			for _, path := range kubeadmConfPaths {
				if len(path) == 0 {
					continue
				}
				if c, err := clientcmd.BuildConfigFromFlags("", path); err == nil {
					cfg = c
					break
				}
			}
		}
		if cfg == nil {
			return nil, fmt.Errorf("failed to get kube client config")
		}
		return kubernetes.NewForConfig(cfg)
	}
)

// Config has the information that required by convert operation.
type Config struct {
	kubeadmConfPaths []string
	nodeName         string
	nodePoolName     string
	openyurtDir      string
}

// nodeConverter do the convert job.
type nodeConverter struct {
	Config
}

// NewConverterWithOptions create nodeConverter.
func NewConverterWithOptions(o *Options) *nodeConverter {
	return &nodeConverter{
		Config: Config{
			kubeadmConfPaths: strings.Split(o.kubeadmConfPaths, ","),
			nodeName:         o.nodeName,
			nodePoolName:     o.nodePoolName,
			openyurtDir:      o.openyurtDir,
		},
	}
}

// Do is used for the convert job.
// shall be implemented as idempotent, can execute multiple times with no side effect.
func (n *nodeConverter) Do() error {
	if err := n.ensureNodeInMultiplexerRBAC(); err != nil {
		return err
	}
	if err := n.installYurtHub(); err != nil {
		return err
	}
	if err := n.convertKubelet(); err != nil {
		return err
	}
	if err := n.restartContainers(); err != nil {
		return err
	}

	return nil
}

func (n *nodeConverter) ensureNodeInMultiplexerRBAC() error {
	client, err := getKubeClientFunc(n.kubeadmConfPaths)
	if err != nil {
		return fmt.Errorf("failed to get kube client for RBAC update: %w", err)
	}

	crb, err := client.RbacV1().ClusterRoleBindings().Get(context.TODO(), multiplexerClusterRoleBindingName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("ClusterRoleBinding %s not found: %w", multiplexerClusterRoleBindingName, err)
		}
		return fmt.Errorf("failed to get ClusterRoleBinding %s: %w", multiplexerClusterRoleBindingName, err)
	}

	targetSubject := rbacv1.Subject{
		Kind:     "User",
		Name:     "system:node:" + n.nodeName,
		APIGroup: "rbac.authorization.k8s.io",
	}

	for _, sub := range crb.Subjects {
		if sub.Kind == targetSubject.Kind && sub.Name == targetSubject.Name && sub.APIGroup == targetSubject.APIGroup {
			return nil
		}
	}

	crb.Subjects = append(crb.Subjects, targetSubject)
	_, err = client.RbacV1().ClusterRoleBindings().Update(context.TODO(), crb, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update ClusterRoleBinding %s: %w", multiplexerClusterRoleBindingName, err)
	}

	return nil
}

func (n *nodeConverter) installYurtHub() error {
	apiServerAddress, err := getAPIServerAddressFunc(n.kubeadmConfPaths)
	if err != nil {
		return err
	}
	if apiServerAddress == "" {
		return fmt.Errorf("get apiServerAddress empty")
	}

	return installYurthubFunc(n.yurthubHostConfig(apiServerAddress))
}

func (n *nodeConverter) convertKubelet() error {
	return redirectKubeletFunc(n.openyurtDir)
}

func (n *nodeConverter) restartContainers() error {
	return restartContainersFunc(n.nodeName)
}

func (n *nodeConverter) yurthubHostConfig(apiServerAddress string) *yurthubutil.YurthubHostConfig {
	return &yurthubutil.YurthubHostConfig{
		BootstrapMode: bootstrapModeKubeletCertificate,
		Namespace:     constants.YurthubNamespace,
		NodeName:      n.nodeName,
		NodePoolName:  n.nodePoolName,
		ServerAddr:    apiServerAddress,
		WorkingMode:   constants.EdgeNode,
	}
}

func conversionJobPodPrefix(nodeName string) string {
	return fmt.Sprintf("%s-%s", nodeservant.ConversionJobNameBase, nodeName)
}
