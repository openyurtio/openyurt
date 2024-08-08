/*
Copyright 2024 The OpenYurt Authors.
Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"

	"github.com/openyurtio/openyurt/pkg/apis"
	viplb "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/loadbalancerset/viploadbalancer"
)

func TestDefault(t *testing.T) {
	var (
		elbClass = "service.openyurt.io/elb"
		vipClass = "service.openyurt.io/viplb"
	)

	testcases := map[string]struct {
		errHappened   bool
		service       runtime.Object
		wantedService *corev1.Service
	}{
		"service with specified loadBalancerClass but not viplb": {
			service: &corev1.Service{
				Spec: corev1.ServiceSpec{
					LoadBalancerClass: &elbClass,
				},
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1alpha1",
					Kind:       "Service",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "nginx",
					ResourceVersion: "1",
				},
			},
			wantedService: &corev1.Service{
				Spec: corev1.ServiceSpec{
					LoadBalancerClass: &elbClass,
				},
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1alpha1",
					Kind:       "Service",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "nginx",
					ResourceVersion: "1",
				},
			},
		},
		"service with specified loadBalancerClass: viplb": {
			service: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1alpha1",
					Kind:       "Service",
				},
				Spec: corev1.ServiceSpec{
					LoadBalancerClass: &vipClass,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "nginx",
					ResourceVersion: "1",
				},
			},
			wantedService: &corev1.Service{
				Spec: corev1.ServiceSpec{
					LoadBalancerClass: &vipClass,
				},
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1alpha1",
					Kind:       "Service",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "nginx",
					ResourceVersion: "1",
					Annotations: map[string]string{
						viplb.AnnotationServiceTopologyKey: viplb.AnnotationServiceTopologyValueNodePool,
					},
				},
			},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			scheme := runtime.NewScheme()
			if err := clientgoscheme.AddToScheme(scheme); err != nil {
				t.Fatal("Fail to add kubernetes clint-go custom resource")
			}
			apis.AddToScheme(scheme)

			h := &ServiceHandler{}
			err := h.Default(context.TODO(), tc.service)
			if tc.errHappened {
				if err == nil {
					t.Errorf("expect error, got nil")
				}
			} else if err != nil {
				t.Errorf("expect no error, but got %v", err)
			} else {
				currentService := tc.service.(*corev1.Service)
				if !reflect.DeepEqual(currentService, tc.wantedService) {
					t.Errorf("expect %#+v ,\n got %#+v", tc.wantedService, currentService)
				}
			}
		})
	}
}
