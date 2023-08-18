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
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/fake"
)

func TestSecretClient(t *testing.T) {

	testNS := "test-ns"
	testSecretName := "test-secret"
	clientset := fake.NewSimpleClientset()

	// 1. init from no secret
	emptyClient, err := NewSecretClient(clientset, testNS, testSecretName)
	assert.Equal(t, nil, err)

	// 1.1 add data into empty secret
	err = emptyClient.AddData("key_1", []byte("val_1"))
	assert.Equal(t, nil, err)

	val, err := emptyClient.GetData("key_1")
	assert.Equal(t, nil, err)
	assert.Equal(t, []byte("val_1"), val)

	// 2. init from existing secret
	existingClient, err := NewSecretClient(clientset, testNS, testSecretName)
	assert.Equal(t, nil, err)

	val, err = existingClient.GetData("key_1")
	assert.Equal(t, nil, err)
	assert.Equal(t, []byte("val_1"), val)

	// 2.1 add different data into existing secret
	err = existingClient.AddData("key_2", []byte("val_2"))
	assert.Equal(t, nil, err)

	val, err = existingClient.GetData("key_2")
	assert.Equal(t, nil, err)
	assert.Equal(t, []byte("val_2"), val)

	// 2.2 overwrite data in existing secret
	err = existingClient.AddData("key_1", []byte("val_2"))
	assert.Equal(t, nil, err)

	val, err = existingClient.GetData("key_1")
	assert.Equal(t, nil, err)
	assert.Equal(t, []byte("val_2"), val)
}
