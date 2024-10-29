/*
Copyright 2024 The OpenYurt Authors.

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

package platformadmin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kjson "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/openyurtio/openyurt/pkg/apis"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha2"
	iotv1beta1 "github.com/openyurtio/openyurt/pkg/apis/iot/v1beta1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/platformadmin/config"
)

type fakeEventRecorder struct {
}

func (f *fakeEventRecorder) Event(object runtime.Object, eventtype, reason, message string) {
}

func (f *fakeEventRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
}

func (f *fakeEventRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
}

func getFakeScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	apis.AddToScheme(scheme)
	apps.AddToScheme(scheme)
	corev1.AddToScheme(scheme)
	v1alpha2.AddToScheme(scheme)
	return scheme
}

var fakeScheme = getFakeScheme()

func TestReconcilePlatformAdmin(t *testing.T) {
	tests := []struct {
		name           string
		request        reconcile.Request
		platformAdmin  *iotv1beta1.PlatformAdmin
		yasList        []*v1beta1.YurtAppSet
		svcList        []*corev1.Service
		expectedYasNum int
		expectedSvcNum int
		expectedErr    bool
		isUpdated      bool
	}{
		{
			name: "create PlatformAdmin with single NodePool",
			request: reconcile.Request{
				NamespacedName: client.ObjectKey{
					Name:      "test-platformadmin",
					Namespace: "default",
				},
			},
			platformAdmin: &iotv1beta1.PlatformAdmin{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-platformadmin",
					Namespace: "default",
				},
				Spec: iotv1beta1.PlatformAdminSpec{
					Version:   "minnesota",
					NodePools: []string{"pool1"},
				},
			},
			expectedYasNum: 5,
			expectedSvcNum: 4,
			expectedErr:    false,
		},
		{
			name: "create PlatformAdmin with multiple NodePools",
			request: reconcile.Request{
				NamespacedName: client.ObjectKey{
					Name:      "multi-pool-platformadmin",
					Namespace: "default",
				},
			},
			platformAdmin: &iotv1beta1.PlatformAdmin{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multi-pool-platformadmin",
					Namespace: "default",
				},
				Spec: iotv1beta1.PlatformAdminSpec{
					Version:   "minnesota",
					NodePools: []string{"pool1", "pool2", "pool3", "pool4"},
				},
			},
			expectedYasNum: 5,
			expectedSvcNum: 4,
			expectedErr:    false,
		},
		{
			name: "create PlatformAdmin with empty NodePools",
			request: reconcile.Request{
				NamespacedName: client.ObjectKey{
					Name:      "multi-pool-platformadmin",
					Namespace: "default",
				},
			},
			platformAdmin: &iotv1beta1.PlatformAdmin{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multi-pool-platformadmin",
					Namespace: "default",
				},
				Spec: iotv1beta1.PlatformAdminSpec{
					Version:   "minnesota",
					NodePools: []string{},
				},
			},
			expectedYasNum: 5,
			expectedSvcNum: 4,
			expectedErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objList := []client.Object{}
			if tt.platformAdmin != nil {
				objList = append(objList, tt.platformAdmin)
			}
			for _, yas := range tt.yasList {
				objList = append(objList, yas)
			}
			for _, svc := range tt.svcList {
				objList = append(objList, svc)
			}

			fakeClient := fake.NewClientBuilder().WithScheme(fakeScheme).WithObjects(objList...).Build()

			r := &ReconcilePlatformAdmin{
				Client:         fakeClient,
				scheme:         fakeScheme,
				recorder:       &fakeEventRecorder{},
				yamlSerializer: kjson.NewSerializerWithOptions(kjson.DefaultMetaFactory, scheme.Scheme, scheme.Scheme, kjson.SerializerOptions{Yaml: true, Pretty: true}),
				Configuration:  *config.NewPlatformAdminControllerConfiguration(),
			}
			_, err := r.Reconcile(context.TODO(), tt.request)
			if tt.expectedErr {
				assert.NotNil(t, err)
			}

			yasList := &v1beta1.YurtAppSetList{}
			if err := fakeClient.List(context.TODO(), yasList); err == nil {
				assert.Len(t, yasList.Items, tt.expectedYasNum)
			}

			svcList := &corev1.ServiceList{}
			if err := fakeClient.List(context.TODO(), svcList); err == nil {
				assert.Len(t, svcList.Items, tt.expectedSvcNum)
			}
		})
	}
}
