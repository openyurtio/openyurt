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

	"github.com/openyurtio/openyurt/pkg/apis/network/v1alpha1"
	viplb "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/loadbalancerset/viploadbalancer"
)

func TestDefault(t *testing.T) {
	var (
		elbClass = "service.openyurt.io/elb"
		vipClass = "service.openyurt.io/viplb"
	)

	testcases := map[string]struct {
		obj               runtime.Object
		errHappened       bool
		wantedPoolService *v1alpha1.PoolService
	}{
		"it is not a v1alpha1": {
			obj:         &corev1.Node{},
			errHappened: true,
		},
		"pod with specified loadBalancerClass but not viplb": {
			obj: &v1alpha1.PoolService{
				Spec: v1alpha1.PoolServiceSpec{
					LoadBalancerClass: &elbClass,
				},
			},
			wantedPoolService: &v1alpha1.PoolService{
				Spec: v1alpha1.PoolServiceSpec{
					LoadBalancerClass: &elbClass,
				},
			},
		},
		"pod with specified loadBalancerClass: vrip": {
			obj: &v1alpha1.PoolService{
				Spec: v1alpha1.PoolServiceSpec{
					LoadBalancerClass: &vipClass,
				},
			},
			wantedPoolService: &v1alpha1.PoolService{
				Spec: v1alpha1.PoolServiceSpec{
					LoadBalancerClass: &vipClass,
				},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						viplb.AnnotationServiceTopologyKey: viplb.AnnotationServiceTopologyValueNodePool,
					},
				},
			},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			h := PoolServiceHandler{}
			err := h.Default(context.TODO(), tc.obj)
			if tc.errHappened {
				if err == nil {
					t.Errorf("expect error, got nil")
				}
			} else if err != nil {
				t.Errorf("expect no error, but got %v", err)
			} else {
				currentPoolServices := tc.obj.(*v1alpha1.PoolService)
				if !reflect.DeepEqual(currentPoolServices, tc.wantedPoolService) {
					t.Errorf("expect %#+v, got %#+v", tc.wantedPoolService, currentPoolServices)
				}
			}
		})
	}
}
