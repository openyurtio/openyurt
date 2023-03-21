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

package init

import (
	"io"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientsetfake "k8s.io/client-go/kubernetes/fake"

	yurtutil "github.com/openyurtio/openyurt/test/e2e/cmd/init/util/kubernetes"
)

func NewClusterConverter(ki *Initializer) *ClusterConverter {
	converter := &ClusterConverter{
		ClientSet:                 ki.kubeClient,
		CloudNodes:                ki.CloudNodes,
		EdgeNodes:                 ki.EdgeNodes,
		WaitServantJobTimeout:     yurtutil.DefaultWaitServantJobTimeout,
		YurthubHealthCheckTimeout: defaultYurthubHealthCheckTimeout,
		KubeConfigPath:            ki.KubeConfig,
		YurtManagerImage:          ki.YurtManagerImage,
		NodeServantImage:          ki.NodeServantImage,
		YurthubImage:              ki.YurtHubImage,
		EnableDummyIf:             ki.EnableDummyIf,
	}
	return converter
}

func TestClusterConverter_LabelEdgeNodes(t *testing.T) {
	case1 := struct {
		podObj *corev1.Node
		want   error
	}{
		podObj: &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "openyurt-worker",
				Namespace:   "",
				Labels:      map[string]string{},
				Annotations: map[string]string{},
			},
		},
		want: nil,
	}
	var fakeOut io.Writer
	initializer := newKindInitializer(fakeOut, newKindOptions().Config())
	initializer.kubeClient = clientsetfake.NewSimpleClientset(case1.podObj)
	converter := NewClusterConverter(initializer)
	if converter.labelEdgeNodes() != case1.want {
		t.Errorf("failed to label edge nodes")
	}
}

func TestPrepareClusterInfoConfigMap(t *testing.T) {
	case1 := struct {
		configObj *corev1.ConfigMap
		want      error
	}{
		configObj: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: "kube-public", Name: "cluster-info"},
		},
		want: nil,
	}
	fakeKubeClient := clientsetfake.NewSimpleClientset(case1.configObj)
	err := prepareClusterInfoConfigMap(fakeKubeClient, "")
	if err != case1.want {
		t.Errorf("failed to prepare the cluster information of ConfigMap ")
	}
}

func TestPrepareYurthubStart(t *testing.T) {
	case1 := struct {
		configObj *corev1.ConfigMap
		want      error
	}{
		configObj: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: "kube-public", Name: "cluster-info"},
		},
		want: nil,
	}
	fakeKubeClient := clientsetfake.NewSimpleClientset(case1.configObj)
	joinToken, err := prepareYurthubStart(fakeKubeClient, "")
	if err != case1.want && joinToken == "" {
		t.Errorf("failed to prepare yurthub start ")
	}
}

func TestNodePoolResourceExists(t *testing.T) {

	cases := []struct {
		apiResourceObj *metav1.APIResourceList
		want           error
		isExist        bool
	}{
		{
			apiResourceObj: &metav1.APIResourceList{
				GroupVersion: "apps.openyurt.io/v1alpha1",
				APIResources: []metav1.APIResource{
					{
						Name: "nodepools",
						Kind: "NodePool",
					},
				},
			},
			want:    nil,
			isExist: true,
		},
		{
			apiResourceObj: &metav1.APIResourceList{
				GroupVersion: "apps.openyurt.io/v1alpha1",
				APIResources: []metav1.APIResource{
					{
						Name: "nodepools",
						Kind: "pod",
					},
				},
			},
			want:    nil,
			isExist: false,
		},
	}
	for _, v := range cases {
		fakeKubeClient := clientsetfake.NewSimpleClientset()
		fakeKubeClient.Fake.Resources = append(fakeKubeClient.Fake.Resources, v.apiResourceObj)
		isExist, err := nodePoolResourceExists(fakeKubeClient)
		if err != v.want && isExist != v.isExist {
			t.Errorf("falied to verify nodes pool is exist")
		}
	}
}

func TestClusterConverter_DeployYurtHub(t *testing.T) {
	case1 := struct {
		configObj      *corev1.ConfigMap
		apiResourceObj *metav1.APIResourceList
		want           error
	}{
		configObj: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: "kube-public", Name: "cluster-info"},
		},
		apiResourceObj: &metav1.APIResourceList{
			GroupVersion: "apps.openyurt.io/v1alpha1",
			APIResources: []metav1.APIResource{
				{
					Name: "nodepools",
					Kind: "NodePool",
				},
			},
		},
		want: nil,
	}

	var fakeOut io.Writer
	initializer := newKindInitializer(fakeOut, newKindOptions().Config())
	fakeclient := clientsetfake.NewSimpleClientset(case1.configObj)
	fakeclient.Fake.Resources = append(fakeclient.Fake.Resources, case1.apiResourceObj)
	initializer.kubeClient = fakeclient
	converter := NewClusterConverter(initializer)
	converter.CloudNodes = []string{}
	converter.EdgeNodes = []string{}
	err := converter.deployYurthub()
	if err != case1.want {
		t.Errorf("failed to deploy yurthub")
	}

}
