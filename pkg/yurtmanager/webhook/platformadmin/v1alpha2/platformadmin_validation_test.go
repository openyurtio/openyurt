/*
Copyright 2023 The OpenYurt Authors.

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

package v1alpha2

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openyurtio/openyurt/pkg/apis"
	ut "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha2"
	version "github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha2"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/platformadmin/config"
)

// TestValidateCreate tests the ValidateCreate method of PlatformAdminHandler.
func TestValidateCreate(t *testing.T) {

	type testCase struct {
		name    string
		client  *FakeClient
		obj     runtime.Object
		errCode int
	}

	tests := []testCase{
		{
			name:    "should get StatusBadRequestError when invalid PlatformAdmin type",
			obj:     &unstructured.Unstructured{},
			errCode: http.StatusBadRequest,
		},
		{
			name:   "should get StatusUnprocessableEntityError when Platform is invalid",
			client: NewFakeClient(buildClient(nil, nil)).Build(),
			obj: &v1alpha2.PlatformAdmin{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: v1alpha2.PlatformAdminSpec{
					Platform: "invalid",
				},
			},
			errCode: http.StatusUnprocessableEntity,
		},
		{
			name:   "should get StatusUnprocessableEntityError when version is invalid",
			client: NewFakeClient(buildClient(nil, nil)).Build(),
			obj: &v1alpha2.PlatformAdmin{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: v1alpha2.PlatformAdminSpec{
					Platform: v1alpha2.PlatformAdminPlatformEdgeX,
					Version:  "invalid version",
				},
			},
			errCode: http.StatusUnprocessableEntity,
		},
		{
			name:   "should get StatusUnprocessableEntityError when list NodePoolList failed",
			client: NewFakeClient(buildClient(nil, nil)).WithErr(&ut.NodePoolList{}, errors.New("list failed")).Build(),
			obj: &v1alpha2.PlatformAdmin{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: v1alpha2.PlatformAdminSpec{
					Platform: v1alpha2.PlatformAdminPlatformEdgeX,
					Version:  "v2",
				},
			},
			errCode: http.StatusUnprocessableEntity,
		},
		{
			name:   "should get StatusUnprocessableEntityError when list NodePoolList is empty",
			client: NewFakeClient(buildClient(nil, nil)).Build(),
			obj: &v1alpha2.PlatformAdmin{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: v1alpha2.PlatformAdminSpec{
					Platform: v1alpha2.PlatformAdminPlatformEdgeX,
					Version:  "v2",
				},
			},
			errCode: http.StatusUnprocessableEntity,
		},
		{
			name:   "should get StatusUnprocessableEntityError when find NodePoolList is empty by PlatformAdmin PoolName",
			client: NewFakeClient(buildClient(buildNodePool(), nil)).Build(),
			obj: &v1alpha2.PlatformAdmin{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: v1alpha2.PlatformAdminSpec{
					PoolName: "not-exit-poll",
					Platform: v1alpha2.PlatformAdminPlatformEdgeX,
					Version:  "v2",
				},
			},
			errCode: http.StatusUnprocessableEntity,
		},
		{
			name:   "should get StatusUnprocessableEntityError when list PlatformAdmin failed",
			client: NewFakeClient(buildClient(buildNodePool(), buildPlatformAdmin())).WithErr(&v1alpha2.PlatformAdminList{}, errors.New("list failed")).Build(),
			obj: &v1alpha2.PlatformAdmin{
				ObjectMeta: metav1.ObjectMeta{},
				Spec: v1alpha2.PlatformAdminSpec{
					PoolName: "beijing",
					Platform: v1alpha2.PlatformAdminPlatformEdgeX,
					Version:  "v2",
				},
			},
			errCode: http.StatusUnprocessableEntity,
		},
		{
			name:   "should get StatusUnprocessableEntityError when get other PlatformAdmin in same node pool",
			client: NewFakeClient(buildClient(buildNodePool(), buildPlatformAdmin())).Build(),
			obj: &v1alpha2.PlatformAdmin{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hangzhou-PlatformAdmin",
				},
				Spec: v1alpha2.PlatformAdminSpec{
					PoolName: "beijing",
					Platform: v1alpha2.PlatformAdminPlatformEdgeX,
					Version:  "v2",
				},
			},
			errCode: http.StatusUnprocessableEntity,
		},
		{
			name:   "should get no err",
			client: NewFakeClient(buildClient(buildNodePool(), buildPlatformAdmin())).Build(),
			obj: &v1alpha2.PlatformAdmin{
				ObjectMeta: metav1.ObjectMeta{
					Name: "beijing-PlatformAdmin",
				},
				Spec: v1alpha2.PlatformAdminSpec{
					PoolName: "beijing",
					Platform: v1alpha2.PlatformAdminPlatformEdgeX,
					Version:  "v2",
				},
			},
			errCode: 0,
		},
	}

	manifest := &config.Manifest{
		Versions: []config.ManifestVersion{
			{
				Name:               "v2",
				RequiredComponents: []string{"edgex-core-data", "edgex-core-metadata"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := &PlatformAdminHandler{
				Client:    tc.client,
				Manifests: manifest,
			}
			_, err := handler.ValidateCreate(context.TODO(), tc.obj)
			if tc.errCode == 0 {
				assert.NoError(t, err, "success case result err must be nil")
			} else {
				assert.Equal(t, tc.errCode, int(err.(*apierrors.StatusError).Status().Code))
			}
		})
	}
}

// TestValidateUpdate tests the ValidateUpdate method of PlatformAdminHandler.
func TestValidateUpdate(t *testing.T) {
	type testCase struct {
		name    string
		client  *FakeClient
		oldObj  runtime.Object
		newObj  runtime.Object
		errCode int
	}

	tests := []testCase{
		{
			name:    "should get StatusBadRequestError when invalid new PlatformAdmin type",
			client:  NewFakeClient(buildClient(buildNodePool(), buildPlatformAdmin())).Build(),
			oldObj:  &v1alpha2.PlatformAdmin{},
			newObj:  &unstructured.Unstructured{},
			errCode: http.StatusBadRequest,
		},
		{
			name:    "should get StatusBadRequestError when invalid old PlatformAdmin type",
			client:  NewFakeClient(buildClient(buildNodePool(), buildPlatformAdmin())).Build(),
			oldObj:  &unstructured.Unstructured{},
			newObj:  &v1alpha2.PlatformAdmin{},
			errCode: http.StatusBadRequest,
		},
		{
			name:   "should get StatusUnprocessableEntityError when old PlatformAdmin is valid and new PlatformAdmin is invalid",
			client: NewFakeClient(buildClient(buildNodePool(), buildPlatformAdmin())).Build(),
			oldObj: &v1alpha2.PlatformAdmin{
				ObjectMeta: metav1.ObjectMeta{
					Name: "beijing-PlatformAdmin",
				},
				Spec: v1alpha2.PlatformAdminSpec{
					PoolName: "beijing",
					Platform: v1alpha2.PlatformAdminPlatformEdgeX,
					Version:  "v2",
				},
			},
			newObj:  &v1alpha2.PlatformAdmin{},
			errCode: http.StatusUnprocessableEntity,
		},
		{
			name:   "should get StatusUnprocessableEntityError when old PlatformAdmin is invalid and old PlatformAdmin is valid",
			client: NewFakeClient(buildClient(buildNodePool(), buildPlatformAdmin())).Build(),
			oldObj: &v1alpha2.PlatformAdmin{},
			newObj: &v1alpha2.PlatformAdmin{
				ObjectMeta: metav1.ObjectMeta{
					Name: "beijing-PlatformAdmin",
				},
				Spec: v1alpha2.PlatformAdminSpec{
					PoolName: "beijing",
					Platform: v1alpha2.PlatformAdminPlatformEdgeX,
					Version:  "v2",
				},
			},
			errCode: http.StatusUnprocessableEntity,
		},
		{
			name:   "should no err when new PlatformAdmin and old PlatformAdmin both valid",
			client: NewFakeClient(buildClient(buildNodePool(), buildPlatformAdmin())).Build(),
			oldObj: &v1alpha2.PlatformAdmin{
				ObjectMeta: metav1.ObjectMeta{
					Name: "beijing-PlatformAdmin",
				},
				Spec: v1alpha2.PlatformAdminSpec{
					PoolName: "beijing",
					Platform: v1alpha2.PlatformAdminPlatformEdgeX,
					Version:  "v2",
				},
			},
			newObj: &v1alpha2.PlatformAdmin{
				ObjectMeta: metav1.ObjectMeta{
					Name: "beijing-PlatformAdmin",
				},
				Spec: v1alpha2.PlatformAdminSpec{
					PoolName: "beijing",
					Platform: v1alpha2.PlatformAdminPlatformEdgeX,
					Version:  "v2",
				},
			},
			errCode: 0,
		},
	}

	manifest := &config.Manifest{
		Versions: []config.ManifestVersion{
			{
				Name:               "v2",
				RequiredComponents: []string{"edgex-core-data", "edgex-core-metadata"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := &PlatformAdminHandler{Client: tc.client, Manifests: manifest}
			_, err := handler.ValidateUpdate(context.TODO(), tc.oldObj, tc.newObj)
			if tc.errCode == 0 {
				assert.NoError(t, err, "success case result err must be nil")
			} else {
				assert.Equal(t, tc.errCode, int(err.(*apierrors.StatusError).Status().Code))
			}
		})
	}
}

// TestValidateDelete tests the ValidateDelete method of PlatformAdminHandler.
func TestValidateDelete(t *testing.T) {
	handler := &PlatformAdminHandler{}

	_, err := handler.ValidateDelete(context.TODO(), nil)

	assert.Nil(t, err)
}

func buildClient(nodePools []client.Object, platformAdmin []client.Object) client.WithWatch {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = apis.AddToScheme(scheme)
	_ = version.SchemeBuilder.AddToScheme(scheme)
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodePools...).WithObjects(platformAdmin...).WithIndex(&v1alpha2.PlatformAdmin{}, "spec.poolName", Indexer).Build()
}

func buildPlatformAdmin() []client.Object {
	nodes := []client.Object{
		&v1alpha2.PlatformAdmin{
			ObjectMeta: metav1.ObjectMeta{
				Name: "beijing-PlatformAdmin",
			},
			Spec: v1alpha2.PlatformAdminSpec{
				PoolName: "beijing",
			},
		},
	}
	return nodes
}

func buildNodePool() []client.Object {
	pools := []client.Object{
		&ut.NodePool{
			ObjectMeta: metav1.ObjectMeta{
				Name: "hangzhou",
			},
			Spec: ut.NodePoolSpec{
				Type: ut.Edge,
				Labels: map[string]string{
					"region": "hangzhou",
				},
			},
		},
		&ut.NodePool{
			ObjectMeta: metav1.ObjectMeta{
				Name: "beijing",
			},
			Spec: ut.NodePoolSpec{
				Type: ut.Edge,
				Labels: map[string]string{
					"region": "beijing",
				},
			},
		},
	}
	return pools
}

type FakeClient struct {
	client.Client
	obj interface{}
	err error
}

func (f *FakeClient) List(ctx context.Context, obj client.ObjectList, opts ...client.ListOption) error {
	if f.err != nil && reflect.TypeOf(f.obj) == reflect.TypeOf(obj) {
		return f.err
	}
	return f.Client.List(ctx, obj, opts...)
}

func NewFakeClient(client client.Client) *FakeClient {
	return &FakeClient{Client: client}
}

func (f *FakeClient) WithErr(obj interface{}, err error) *FakeClient {
	f.obj = obj
	f.err = err
	return f
}
func (f *FakeClient) Build() *FakeClient {
	return f
}

func Indexer(rawObj client.Object) []string {
	platformAdmin, ok := rawObj.(*v1alpha2.PlatformAdmin)
	if !ok {
		return []string{}
	}
	if len(platformAdmin.Spec.PoolName) == 0 {
		return []string{}
	}
	return []string{platformAdmin.Spec.PoolName}
}
