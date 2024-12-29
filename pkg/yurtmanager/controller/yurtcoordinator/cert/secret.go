/*
Copyright 2022 The OpenYurt Authors.

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

package yurtcoordinatorcert

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	client "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// a simple client to handle secret operations
type SecretClient struct {
	Name      string
	Namespace string
	client    client.Interface
}

func NewSecretClient(clientSet client.Interface, ns, name string) (*SecretClient, error) {

	emptySecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Data:       make(map[string][]byte),
		StringData: make(map[string]string),
	}

	secret, err := clientSet.CoreV1().Secrets(ns).Create(context.TODO(), emptySecret, metav1.CreateOptions{})
	if err != nil {
		// since multiple SecretClient may share one secret
		// if this secret already exist, reuse it
		if kerrors.IsAlreadyExists(err) {
			secret, _ = clientSet.CoreV1().Secrets(ns).Get(context.TODO(), name, metav1.GetOptions{})
			klog.V(4).Info(Format("secret %s already exists", secret.Name))
		} else {
			return nil, fmt.Errorf("create secret client %s fail: %v", name, err)
		}
	} else {
		klog.V(4).Info(Format("secret %s does not exist, create one", secret.Name))
	}

	return &SecretClient{
		Name:      name,
		Namespace: ns,
		client:    clientSet,
	}, nil
}

func (c *SecretClient) AddData(key string, val []byte) error {

	patchBytes, _ := json.Marshal(map[string]interface{}{"data": map[string][]byte{key: val}})
	_, err := c.client.CoreV1().Secrets(c.Namespace).Patch(context.TODO(), c.Name, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{})

	if err != nil {
		return fmt.Errorf("update secret %v/%v %s fail: %v", c.Namespace, c.Name, key, err)
	}

	return nil
}

func (c *SecretClient) GetData(key string) ([]byte, error) {
	secret, err := c.client.CoreV1().Secrets(c.Namespace).Get(context.TODO(), c.Name, metav1.GetOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "could not get secret from secretClient")
	}

	val, ok := secret.Data[key]
	if !ok {
		return nil, fmt.Errorf("key %s don't exist in secretClient", key)
	}

	return val, nil
}
