/*
Copyright 2024 The OpenYurt Authors.
Copyright 2018 The Kubernetes Authors.

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
package app

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/oauth2"
	v1authenticationapi "k8s.io/api/authentication/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	utilpointer "k8s.io/utils/ptr"
)

var (
	// defaultExpirationSeconds defines the duration of a TokenRequest in seconds.
	defaultExpirationSeconds = int64(3600)
	// defaultLeewayPercent defines the percentage of expiration left before the client trigger a token rotation.
	// range[0, 100]
	defaultLeewayPercent = 20
)

// migrate from kubernetes/staging/src/k8s.io/controller-manager/pkg/clientbuilder/client_builder_dynamic.go
type tokenSourceImpl struct {
	namespace          string
	serviceAccountName string
	cli                kubernetes.Clientset
	expirationSeconds  int64
	leewayPercent      int
}

func (ts *tokenSourceImpl) Token() (*oauth2.Token, error) {
	klog.V(5).Info("start get token")
	var retTokenRequest *v1authenticationapi.TokenRequest

	backoff := wait.Backoff{
		Duration: 500 * time.Millisecond,
		Factor:   2, // double the timeout for every failure
		Steps:    4,
	}
	if err := wait.ExponentialBackoff(backoff, func() (bool, error) {
		_, inErr := getOrCreateServiceAccount(ts.cli, ts.namespace, ts.serviceAccountName)
		if inErr != nil {
			klog.Warningf("get or create service account failed: %v", inErr)
			return false, nil
		}
		klog.V(5).Infof("get serviceaccount %s successfully", ts.serviceAccountName)

		tr, inErr := ts.cli.CoreV1().ServiceAccounts(ts.namespace).CreateToken(context.TODO(), ts.serviceAccountName, &v1authenticationapi.TokenRequest{
			Spec: v1authenticationapi.TokenRequestSpec{
				ExpirationSeconds: utilpointer.To(ts.expirationSeconds),
			},
		}, metav1.CreateOptions{})
		if inErr != nil {
			klog.Warningf("get token failed: %v", inErr)
			return false, nil
		}
		retTokenRequest = tr
		klog.V(5).Infof("create token successfully for serviceaccount %s", ts.serviceAccountName)

		return true, nil
	}); err != nil {
		return nil, fmt.Errorf("failed to get token for %s/%s: %v", ts.namespace, ts.serviceAccountName, err)
	}

	if retTokenRequest.Spec.ExpirationSeconds == nil {
		return nil, fmt.Errorf("nil pointer of expiration in token request")
	}

	lifetime := retTokenRequest.Status.ExpirationTimestamp.Time.Sub(time.Now())
	if lifetime < time.Minute*10 {
		// possible clock skew issue, pin to minimum token lifetime
		lifetime = time.Minute * 10
	}

	leeway := time.Duration(int64(lifetime) * int64(ts.leewayPercent) / 100)
	expiry := time.Now().Add(lifetime).Add(-1 * leeway)

	return &oauth2.Token{
		AccessToken: retTokenRequest.Status.Token,
		TokenType:   "Bearer",
		Expiry:      expiry,
	}, nil
}

func getOrCreateServiceAccount(cli kubernetes.Clientset, namespace, name string) (*v1.ServiceAccount, error) {
	sa, err := cli.CoreV1().ServiceAccounts(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	if err == nil {
		return sa, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, err
	}

	// Create the namespace if we can't verify it exists.
	// Tolerate errors, since we don't know whether this component has namespace creation permissions.
	if _, err := cli.CoreV1().Namespaces().Get(context.TODO(), namespace, metav1.GetOptions{}); apierrors.IsNotFound(err) {
		if _, err = cli.CoreV1().Namespaces().Create(context.TODO(), &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
			klog.Warningf("create non-exist namespace %s failed:%v", namespace, err)
		}
	}

	// Create the service account
	sa, err = cli.CoreV1().ServiceAccounts(namespace).Create(context.TODO(), &v1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name}}, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		// If we're racing to init and someone else already created it, re-fetch
		return cli.CoreV1().ServiceAccounts(namespace).Get(context.TODO(), name, metav1.GetOptions{})
	}
	return sa, err
}
