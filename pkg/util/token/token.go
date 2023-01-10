/*
Copyright 2022 The OpenYurt Authors.
Copyright 2017 The Kubernetes Authors.

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

package token

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	pkgerrors "github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	certutil "k8s.io/client-go/util/cert"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	bootstrap "k8s.io/cluster-bootstrap/token/jws"
	bootstraputil "k8s.io/cluster-bootstrap/token/util"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/util"
	kubeconfigutil "github.com/openyurtio/openyurt/pkg/util/kubeconfig"
	"github.com/openyurtio/openyurt/pkg/util/pubkeypin"
)

const (
	// BootstrapUser defines bootstrap user name
	BootstrapUser = "token-bootstrap-client"
	// DiscoveryRetryInterval specifies how long kubeadm should wait before retrying to connect to the control-plane when doing discovery
	DiscoveryRetryInterval = 5 * time.Second
	// PatchNodeTimeout specifies how long kubeadm should wait for applying the label and taint on the control-plane before timing out
	PatchNodeTimeout = 2 * time.Minute
)

// BootstrapData is used for retrieving Cluster-Info from remote server,
// and it includes remote server address, bootstrap token and CaCertHashes.
type BootstrapData struct {
	ServerAddr   string
	JoinToken    string
	CaCertHashes []string
}

// BootstrapTokenString is a token of the format abcdef.abcdef0123456789 that is used
// for both validation of the practically of the API server from a joining node's point
// of view and as an authentication method for the node in the bootstrap phase of
// "kubeadm join". This token is and should be short-lived
type BootstrapTokenString struct {
	ID     string `json:"-"`
	Secret string `json:"-" datapolicy:"token"`
}

// newBootstrapTokenString converts the given Bootstrap Token as a string
// to the BootstrapTokenString object used for serialization/deserialization
// and internal usage. It also automatically validates that the given token
// is of the right format
func newBootstrapTokenString(token string) (*BootstrapTokenString, error) {
	substrs := bootstraputil.BootstrapTokenRegexp.FindStringSubmatch(token)
	// TODO: Add a constant for the 3 value here, and explain better why it's needed (other than because how the regexp parsin works)
	if len(substrs) != 3 {
		return nil, pkgerrors.Errorf("the bootstrap token %q was not of the form %q", token, bootstrapapi.BootstrapTokenPattern)
	}

	return &BootstrapTokenString{ID: substrs[1], Secret: substrs[2]}, nil
}

// RetrieveValidatedConfigInfo is a private implementation of RetrieveValidatedConfigInfo.
// It accepts an optional clientset that can be used for testing purposes.
func RetrieveValidatedConfigInfo(client kubernetes.Interface, data *BootstrapData) (*clientcmdapi.Config, error) {
	token, err := newBootstrapTokenString(data.JoinToken)
	if err != nil {
		return nil, err
	}

	// Load the CACertHashes into a pubkeypin.Set
	pubKeyPins := pubkeypin.NewSet()
	if err = pubKeyPins.Allow(data.CaCertHashes...); err != nil {
		return nil, err
	}

	// If there are multiple master IP addresses, take the first one here
	endpoint := strings.Split(data.ServerAddr, ",")[0]
	insecureBootstrapConfig := buildInsecureBootstrapKubeConfig(endpoint, "kubernetes")
	clusterName := insecureBootstrapConfig.Contexts[insecureBootstrapConfig.CurrentContext].Cluster

	klog.V(1).Infof("[discovery] Created cluster-info discovery client, requesting info from %q", endpoint)
	insecureClusterInfo, err := getClusterInfo(client, insecureBootstrapConfig, token, DiscoveryRetryInterval, PatchNodeTimeout)
	if err != nil {
		return nil, err
	}

	// Validate the token in the cluster info
	insecureKubeconfigBytes, err := validateClusterInfoToken(insecureClusterInfo, token)
	if err != nil {
		return nil, err
	}

	// Load the insecure config
	insecureConfig, err := clientcmd.Load(insecureKubeconfigBytes)
	if err != nil {
		return nil, pkgerrors.Wrapf(err, "couldn't parse the kubeconfig file in the %s ConfigMap", bootstrapapi.ConfigMapClusterInfo)
	}

	// The ConfigMap should contain a single cluster
	if len(insecureConfig.Clusters) != 1 {
		return nil, pkgerrors.Errorf("expected the kubeconfig file in the %s ConfigMap to have a single cluster, but it had %d", bootstrapapi.ConfigMapClusterInfo, len(insecureConfig.Clusters))
	}

	// If no TLS root CA pinning was specified, we're done
	if pubKeyPins.Empty() {
		klog.V(1).Infof("[discovery] Cluster info signature and contents are valid and no TLS pinning was specified, will use API Server %q", endpoint)
		return insecureConfig, nil
	}

	// Load and validate the cluster CA from the insecure kubeconfig
	clusterCABytes, err := validateClusterCA(insecureConfig, pubKeyPins)
	if err != nil {
		return nil, err
	}

	// Now that we know the cluster CA, connect back a second time validating with that CA
	secureBootstrapConfig := buildSecureBootstrapKubeConfig(endpoint, clusterCABytes, clusterName)

	klog.V(1).Infof("[discovery] Requesting info from %q again to validate TLS against the pinned public key", endpoint)
	secureClusterInfo, err := getClusterInfo(client, secureBootstrapConfig, token, DiscoveryRetryInterval, PatchNodeTimeout)
	if err != nil {
		return nil, err
	}

	// Pull the kubeconfig from the securely-obtained ConfigMap and validate that it's the same as what we found the first time
	secureKubeconfigBytes := []byte(secureClusterInfo.Data[bootstrapapi.KubeConfigKey])
	if !bytes.Equal(secureKubeconfigBytes, insecureKubeconfigBytes) {
		return nil, pkgerrors.Errorf("the second kubeconfig from the %s ConfigMap (using validated TLS) was different from the first", bootstrapapi.ConfigMapClusterInfo)
	}

	secureKubeconfig, err := clientcmd.Load(secureKubeconfigBytes)
	if err != nil {
		return nil, pkgerrors.Wrapf(err, "couldn't parse the kubeconfig file in the %s ConfigMap", bootstrapapi.ConfigMapClusterInfo)
	}

	klog.V(1).Infof("[discovery] Cluster info signature and contents are valid and TLS certificate validates against pinned roots, will use API Server %q", endpoint)

	return secureKubeconfig, nil
}

// buildInsecureBootstrapKubeConfig makes a kubeconfig object that connects insecurely to the API Server for bootstrapping purposes
func buildInsecureBootstrapKubeConfig(endpoint, clustername string) *clientcmdapi.Config {
	controlPlaneEndpoint := fmt.Sprintf("https://%s", endpoint)
	bootstrapConfig := kubeconfigutil.CreateBasic(controlPlaneEndpoint, clustername, BootstrapUser, []byte{})
	bootstrapConfig.Clusters[clustername].InsecureSkipTLSVerify = true
	return bootstrapConfig
}

// buildSecureBootstrapKubeConfig makes a kubeconfig object that connects securely to the API Server for bootstrapping purposes (validating with the specified CA)
func buildSecureBootstrapKubeConfig(endpoint string, caCert []byte, clustername string) *clientcmdapi.Config {
	controlPlaneEndpoint := fmt.Sprintf("https://%s", endpoint)
	bootstrapConfig := kubeconfigutil.CreateBasic(controlPlaneEndpoint, clustername, BootstrapUser, caCert)
	return bootstrapConfig
}

// validateClusterInfoToken validates that the JWS token present in the cluster info ConfigMap is valid
func validateClusterInfoToken(insecureClusterInfo *v1.ConfigMap, token *BootstrapTokenString) ([]byte, error) {
	insecureKubeconfigString, ok := insecureClusterInfo.Data[bootstrapapi.KubeConfigKey]
	if !ok || len(insecureKubeconfigString) == 0 {
		return nil, pkgerrors.Errorf("there is no %s key in the %s ConfigMap. This API Server isn't set up for token bootstrapping, can't connect",
			bootstrapapi.KubeConfigKey, bootstrapapi.ConfigMapClusterInfo)
	}

	detachedJWSToken, ok := insecureClusterInfo.Data[bootstrapapi.JWSSignatureKeyPrefix+token.ID]
	if !ok || len(detachedJWSToken) == 0 {
		return nil, pkgerrors.Errorf("token id %q is invalid for this cluster or it has expired. Use \"kubeadm token create\" on the control-plane node to create a new valid token", token.ID)
	}

	if !bootstrap.DetachedTokenIsValid(detachedJWSToken, insecureKubeconfigString, token.ID, token.Secret) {
		return nil, errors.New("failed to verify JWS signature of received cluster info object, can't trust this API Server")
	}

	return []byte(insecureKubeconfigString), nil
}

// validateClusterCA validates the cluster CA found in the insecure kubeconfig
func validateClusterCA(insecureConfig *clientcmdapi.Config, pubKeyPins *pubkeypin.Set) ([]byte, error) {
	var clusterCABytes []byte
	for _, cluster := range insecureConfig.Clusters {
		clusterCABytes = cluster.CertificateAuthorityData
	}

	clusterCAs, err := certutil.ParseCertsPEM(clusterCABytes)
	if err != nil {
		return nil, pkgerrors.Wrapf(err, "failed to parse cluster CA from the %s ConfigMap", bootstrapapi.ConfigMapClusterInfo)
	}

	// Validate the cluster CA public key against the pinned set
	err = pubKeyPins.CheckAny(clusterCAs)
	if err != nil {
		return nil, pkgerrors.Wrapf(err, "cluster CA found in %s ConfigMap is invalid", bootstrapapi.ConfigMapClusterInfo)
	}

	return clusterCABytes, nil
}

// getClusterInfo creates a client from the given kubeconfig if the given client is nil,
// and requests the cluster info ConfigMap using PollImmediate.
// If a client is provided it will be used instead.
func getClusterInfo(client kubernetes.Interface, kubeconfig *clientcmdapi.Config, token *BootstrapTokenString, interval, duration time.Duration) (*v1.ConfigMap, error) {
	var cm *v1.ConfigMap
	var err error

	// Create client from kubeconfig
	if util.IsNil(client) {
		klog.V(2).Infof("[discovery] kube client is nil, use bootstrap config(%v) to create client", kubeconfig)
		client, err = kubeconfigutil.ToClientSet(kubeconfig)
		if err != nil {
			return nil, err
		}
	}

	ctx, cancel := context.WithTimeout(context.TODO(), duration)
	defer cancel()

	wait.JitterUntil(func() {
		cm, err = client.CoreV1().ConfigMaps(metav1.NamespacePublic).Get(context.TODO(), bootstrapapi.ConfigMapClusterInfo, metav1.GetOptions{})
		if err != nil {
			klog.V(1).Infof("[discovery] Failed to request cluster-info, will try again: %v", err)
			return
		}
		// Even if the ConfigMap is available the JWS signature is patched-in a bit later.
		// Make sure we retry util then.
		if _, ok := cm.Data[bootstrapapi.JWSSignatureKeyPrefix+token.ID]; !ok {
			klog.V(1).Infof("[discovery] The cluster-info ConfigMap does not yet contain a JWS signature for token ID %q, will try again", token.ID)
			err = pkgerrors.Errorf("could not find a JWS signature in the cluster-info ConfigMap for token ID %q", token.ID)
			return
		}
		// Cancel the context on success
		cancel()
	}, interval, 0.3, true, ctx.Done())

	if err != nil {
		return nil, err
	}

	return cm, nil
}
