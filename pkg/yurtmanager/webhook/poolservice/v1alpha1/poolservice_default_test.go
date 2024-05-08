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
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openyurtio/openyurt/pkg/apis"
	"github.com/openyurtio/openyurt/pkg/apis/network"
	"github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
	viplb "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/loadbalancerset/viploadbalancer"
)

func TestDefault(t *testing.T) {
	var (
		elbClass = "service.openyurt.io/elb"
		vipClass = "service.openyurt.io/viplb"
	)

	testcases := map[string]struct {
		poolservice   runtime.Object
		errHappened   bool
		service       *corev1.Service
		wantedService *corev1.Service
	}{
		"pod with specified loadBalancerClass but not viplb": {
			poolservice: &v1alpha1.PoolService{
				Spec: v1alpha1.PoolServiceSpec{
					LoadBalancerClass: &elbClass,
				},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						network.LabelServiceName: "nginx",
					},
				},
			},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "nginx",
					ResourceVersion: "1",
				},
			},
			wantedService: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Service",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "nginx",
					ResourceVersion: "1",
				},
			},
		},
		"pod with specified loadBalancerClass: viplb": {
			poolservice: &v1alpha1.PoolService{
				Spec: v1alpha1.PoolServiceSpec{
					LoadBalancerClass: &vipClass,
				},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						network.LabelServiceName: "nginx",
					},
				},
			},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "nginx",
					ResourceVersion: "1",
				},
			},
			wantedService: &corev1.Service{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "v1",
					Kind:       "Service",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:            "nginx",
					ResourceVersion: "2",
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

			var c client.Client
			if tc.service != nil {
				c = fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(tc.service).Build()
			} else {
				c = fakeclient.NewClientBuilder().WithScheme(scheme).Build()
			}

			h := &PoolServiceHandler{Client: c}
			err := h.Default(context.TODO(), tc.poolservice)
			if tc.errHappened {
				if err == nil {
					t.Errorf("expect error, got nil")
				}
			} else if err != nil {
				t.Errorf("expect no error, but got %v", err)
			} else {
				ps := tc.poolservice.(*v1alpha1.PoolService)
				serviceName := ps.Labels[network.LabelServiceName]
				currentServices := &corev1.Service{}
				if err := c.Get(context.TODO(), client.ObjectKey{Name: serviceName}, currentServices); err != nil {
					t.Errorf("failed to get service %s: %v", serviceName, err)
				}

				if !reflect.DeepEqual(currentServices, tc.wantedService) {
					t.Errorf("expect %#+v, got %#+v", tc.wantedService, currentServices)
				}
			}
		})
	}
}
