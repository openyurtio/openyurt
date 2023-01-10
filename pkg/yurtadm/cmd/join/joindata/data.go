/*
Copyright 2021 The OpenYurt Authors.

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

package joindata

import (
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type NodeRegistration struct {
	Name          string
	CRISocket     string
	WorkingMode   string
	Organizations string
}

type YurtJoinData interface {
	ServerAddr() string
	JoinToken() string
	PauseImage() string
	YurtHubImage() string
	YurtHubServer() string
	KubernetesVersion() string
	TLSBootstrapCfg() *clientcmdapi.Config
	BootstrapClient() *clientset.Clientset
	NodeRegistration() *NodeRegistration
	CaCertHashes() []string
	NodeLabels() map[string]string
	IgnorePreflightErrors() sets.String
	KubernetesResourceServer() string
}
