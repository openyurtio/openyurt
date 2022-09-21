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
package servicetopology

import (
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter/servicetopology"
	"github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/clientset/versioned/fake"
	yurtinformers "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions"
)

func TestGetServiceTopologyTypes(t *testing.T) {
	expectResult := map[string]string{
		"default/svc2": "openyurt.io/nodepool",
	}

	kubeClient := k8sfake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "svc1",
				Namespace: "default",
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "svc2",
				Namespace: "default",
				Annotations: map[string]string{
					servicetopology.AnnotationServiceTopologyKey: servicetopology.AnnotationServiceTopologyValueNodePool,
				},
			},
		},
	)
	yurtClient := fake.NewSimpleClientset()

	factory := informers.NewSharedInformerFactory(kubeClient, 24*time.Hour)
	yurtfactory := yurtinformers.NewSharedInformerFactory(yurtClient, 24*time.Hour)

	c, _ := NewServiceTopologyController(kubeClient, factory, yurtfactory)

	stopper := make(chan struct{})
	defer close(stopper)
	factory.Start(stopper)
	factory.WaitForCacheSync(stopper)
	yurtfactory.Start(stopper)
	yurtfactory.WaitForCacheSync(stopper)

	types := c.getSvcTopologyTypes()
	if !reflect.DeepEqual(types, expectResult) {
		t.Errorf("expect service topology types is %v, but got %v", expectResult, types)
	}
}
