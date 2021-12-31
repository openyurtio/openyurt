/*
Copyright 2019 The Kubernetes Authors.
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

package options

const (
	// APIServerAdvertiseAddress flag sets the IP address the API Server will advertise it's listening on. Specify '0.0.0.0' to use the address of the default network interface.
	APIServerAdvertiseAddress = "apiserver-advertise-address"

	// APIServerBindPort flag sets the port for the API Server to bind to.
	APIServerBindPort = "apiserver-bind-port"

	// CertificatesDir flag sets the path where to save and read the certificates.
	CertificatesDir = "cert-dir"

	// CfgPath flag sets the path to kubeadm config file.
	CfgPath = "config"

	// DryRun flag instruct kubeadm to don't apply any changes; just output what would be done.
	DryRun = "dry-run"

	// IgnorePreflightErrors sets the path a list of checks whose errors will be shown as warnings. Example: 'IsPrivilegedUser,Swap'. Value 'all' ignores errors from all checks.
	IgnorePreflightErrors = "ignore-preflight-errors"

	// KubeconfigPath flag sets the kubeconfig file to use when talking to the cluster. If the flag is not set, a set of standard locations are searched for an existing KubeConfig file.
	KubeconfigPath = "kubeconfig"

	// KubernetesVersion flag sets the Kubernetes version for the control plane.
	KubernetesVersion = "kubernetes-version"

	// NodeCRISocket flag sets the CRI socket to connect to.
	NodeCRISocket = "cri-socket"

	// NodeName flag sets the node name.
	NodeName = "node-name"

	// TokenStr flags sets both the discovery-token and the tls-bootstrap-token when those values are not provided
	TokenStr = "token"

	// TokenDiscoveryCAHash flag instruct kubeadm to validate that the root CA public key matches this hash (for token-based discovery)
	TokenDiscoveryCAHash = "discovery-token-ca-cert-hash"

	// TokenDiscoverySkipCAHash flag instruct kubeadm to skip CA hash verification (for token-based discovery)
	TokenDiscoverySkipCAHash = "discovery-token-unsafe-skip-ca-verification"

	// ForceReset flag instruct kubeadm to reset the node without prompting for confirmation
	ForceReset = "force"

	// NodeType flag sets the type of worker node to edge or cloud.
	NodeType = "node-type"

	// Organizations flag sets the extra organizations of hub agent client certificate.
	Organizations = "organizations"

	// NodeLabels flag sets the labels for worker node.
	NodeLabels = "node-labels"

	// PauseImage flag sets the pause image for worker node.
	PauseImage = "pause-image"

	// YurtHubImage flag sets the yurthub image for worker node.
	YurtHubImage = "yurthub-image"
)
