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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/keyutil"
)

func TestInitCA(t *testing.T) {

	caCert, caKey, _ := NewSelfSignedCA()
	caCertBytes, _ := EncodeCertPEM(caCert)
	caKeyBytes, _ := keyutil.MarshalPrivateKeyToPEM(caKey)

	tests := []struct {
		name   string
		client kubernetes.Interface
		reuse  bool
	}{
		{
			"CA already exist",
			fake.NewSimpleClientset(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      YurtCoordinatorCASecretName,
					Namespace: YurtCoordinatorNS,
				},
				Data: map[string][]byte{
					"ca.crt": []byte(caCertBytes),
					"ca.key": []byte(caKeyBytes),
				},
			}),
			true,
		},
		{
			"CA not exist",
			fake.NewSimpleClientset(&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      YurtCoordinatorCASecretName,
					Namespace: YurtCoordinatorNS,
				},
			}),
			false,
		},
		{
			"secret does not exist",
			fake.NewSimpleClientset(),
			false,
		},
	}

	for _, tt := range tests {
		st := tt
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", st.name)
			{
				_, _, reuse, err := initCA(st.client)
				assert.Equal(t, st.reuse, reuse)
				assert.Nil(t, err)
			}
		}
		t.Run(st.name, tf)
	}
}

func TestInitSAKeyPair(t *testing.T) {
	client := fake.NewSimpleClientset()

	for i := 1; i <= 3; i++ {
		err := initSAKeyPair(client, "test", "test")
		assert.Nil(t, err)
	}

}
