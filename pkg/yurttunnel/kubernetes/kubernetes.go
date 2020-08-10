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

package kubernetes

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/klog"

	"github.com/alibaba/openyurt/pkg/yurttunnel/constants"
)

// CreateClientSet creates a clientset based on the given kubeConfig. If the
// kubeConfig is empty, it will creates the clientset based on the in-cluster
// config
func CreateClientSet(kubeConfig string) (*kubernetes.Clientset, error) {

	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}

// CreateClientSet creates a clientset based on the given kubeconfig
func CreateClientSetKubeConfig(kubeConfig string) (*kubernetes.Clientset, error) {
	var (
		cfg *rest.Config
		err error
	)
	if kubeConfig == "" {
		return nil, errors.New("kubeconfig is not set")
	}
	if _, err := os.Stat(kubeConfig); err != nil && os.IsNotExist(err) {
		return nil, err
	}
	cfg, err = clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("fail to create the clientset based on %s: %v",
			kubeConfig, err)
	}
	cliSet, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return cliSet, nil
}

// CreateClientSetApiserverAddr creates a clientset based on the given apiserverAddr.
// The clientset uses the serviceaccount's CA and Token for authentication and
// authorization.
func CreateClientSetApiserverAddr(apiserverAddr string) (*kubernetes.Clientset, error) {
	if apiserverAddr == "" {
		return nil, errors.New("apiserver addr can't be empty")
	}

	token, err := ioutil.ReadFile(constants.YurttunnelTokenFile)
	if err != nil {
		return nil, err
	}

	tlsClientConfig := rest.TLSClientConfig{}

	if _, err := certutil.NewPool(constants.YurttunnelCAFile); err != nil {
		klog.Errorf("Expected to load root CA config from %s, but got err: %v",
			constants.YurttunnelCAFile, err)
	} else {
		tlsClientConfig.CAFile = constants.YurttunnelCAFile
	}

	restConfig := rest.Config{
		Host:            "https://" + apiserverAddr,
		TLSClientConfig: tlsClientConfig,
		BearerToken:     string(token),
		BearerTokenFile: constants.YurttunnelTokenFile,
	}

	return kubernetes.NewForConfig(&restConfig)
}
