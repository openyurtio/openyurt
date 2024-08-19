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

package v1alpha1

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openyurtio/openyurt/pkg/apis"
	"github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

func TestYurtAppOverriderHandler_ValidateCreate(t *testing.T) {

	testCases := []struct {
		name        string
		obj         runtime.Object
		client      *FakeClient
		expectedErr string
	}{
		{
			name:        "should return error when object is not YurtAppOverrider",
			obj:         &runtime.Unknown{},
			expectedErr: "expected a YurtAppOverrider but got a *runtime.Unknown",
		},
		{
			name:        "should return error when list YurtAppOverrider fail",
			obj:         &v1alpha1.YurtAppOverrider{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			client:      NewFakeClient(buildClient(buildYurtAppOverrider(""))).WithErr(errors.New("could not list YurtAppOverrider")),
			expectedErr: "could not list YurtAppOverrider",
		},
		{
			name: "should return error when duplicate YurtAppOverrider exists",
			obj: &v1alpha1.YurtAppOverrider{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
				Subject: v1alpha1.Subject{
					Name: "app-set",
				},
			},
			client:      NewFakeClient(buildClient(buildYurtAppOverrider("test-not-exist"))),
			expectedErr: "unable to bind multiple yurtappoverriders to one subject resource",
		},
		{
			name:        "should succeed when YurtAppOverrider is valid",
			obj:         &v1alpha1.YurtAppOverrider{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			client:      NewFakeClient(buildClient(buildYurtAppOverrider("test"))),
			expectedErr: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			webhook := &YurtAppOverriderHandler{tc.client}
			_, err := webhook.ValidateCreate(context.Background(), tc.obj)
			if tc.expectedErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.expectedErr)
			}
		})
	}
}

func TestYurtAppOverriderHandler_ValidateUpdate(t *testing.T) {

	testCases := []struct {
		name        string
		oldObj      runtime.Object
		newObj      runtime.Object
		client      *FakeClient
		expectedErr string
	}{
		{
			name:        "should return error when old object is not YurtAppOverrider",
			oldObj:      &runtime.Unknown{},
			newObj:      &v1alpha1.YurtAppOverrider{},
			expectedErr: "expected a YurtAppOverrider but got a *runtime.Unknown",
		},
		{
			name:        "should return error when new object is not YurtAppOverrider",
			oldObj:      &v1alpha1.YurtAppOverrider{},
			newObj:      &runtime.Unknown{},
			expectedErr: "expected a YurtAppOverrider but got a *runtime.Unknown",
		},
		{
			name:        "should return error when Namespace or Name is modified",
			oldObj:      &v1alpha1.YurtAppOverrider{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}},
			newObj:      &v1alpha1.YurtAppOverrider{ObjectMeta: metav1.ObjectMeta{Name: "new-name", Namespace: "default"}},
			expectedErr: "unable to change metadata",
		},
		{
			name:        "should return error when Subject is modified",
			oldObj:      &v1alpha1.YurtAppOverrider{Subject: v1alpha1.Subject{Name: "app-set"}},
			newObj:      &v1alpha1.YurtAppOverrider{Subject: v1alpha1.Subject{Name: "app-daemon"}},
			expectedErr: "unable to modify subject",
		},
		{
			name:        "should return error when list YurtAppOverrider fail",
			oldObj:      &v1alpha1.YurtAppOverrider{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			newObj:      &v1alpha1.YurtAppOverrider{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			client:      NewFakeClient(buildClient(buildYurtAppOverrider(""))).WithErr(errors.New("could not list YurtAppOverrider")),
			expectedErr: "could not list YurtAppOverrider",
		},
		{
			name:        "should return nil when update success",
			oldObj:      &v1alpha1.YurtAppOverrider{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			newObj:      &v1alpha1.YurtAppOverrider{ObjectMeta: metav1.ObjectMeta{Name: "test"}},
			client:      NewFakeClient(buildClient(buildYurtAppOverrider("test"))),
			expectedErr: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			webhook := &YurtAppOverriderHandler{tc.client}
			_, err := webhook.ValidateUpdate(context.Background(), tc.oldObj, tc.newObj)
			if tc.expectedErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.expectedErr)
			}
		})
	}
}

func TestYurtAppOverriderHandler_ValidateDelete(t *testing.T) {
	testCases := []struct {
		name        string
		obj         runtime.Object
		client      *FakeClient
		expectedErr string
	}{
		{
			name:        "should return error when obj type is not YurtAppSet",
			obj:         &runtime.Unknown{},
			client:      NewFakeClient(buildClient(nil)),
			expectedErr: "expected a YurtAppOverrider but got a *runtime.Unknown",
		},
		{
			name: "should return error when YurtAppSet exists",
			obj: &v1alpha1.YurtAppOverrider{
				Subject: v1alpha1.Subject{
					Name: "app-set",
					TypeMeta: metav1.TypeMeta{
						Kind: "YurtAppSet",
					},
				},
			},
			client:      NewFakeClient(buildClient(nil)),
			expectedErr: "unable to delete YurtAppOverrider when subject resource exists",
		},
		{
			name: "should return error when YurtAppDaemon exists",
			obj: &v1alpha1.YurtAppOverrider{
				Subject: v1alpha1.Subject{
					Name: "app-set",
					TypeMeta: metav1.TypeMeta{
						Kind: "YurtAppDaemon",
					},
				},
			},
			client:      NewFakeClient(buildClient(nil)),
			expectedErr: "unable to delete YurtAppOverrider when subject resource exists",
		},
		{
			name: "should return nil when YurtAppSet not exists",
			obj: &v1alpha1.YurtAppOverrider{
				Subject: v1alpha1.Subject{
					Name: "app-set",
					TypeMeta: metav1.TypeMeta{
						Kind: "YurtAppSet",
					},
				},
			},
			client:      NewFakeClient(buildClient(nil)).WithErr(errors.New("YurtAppSet not exists")),
			expectedErr: "",
		},
		{
			name: "should return nil when YurtAppDaemon not exists",
			obj: &v1alpha1.YurtAppOverrider{
				Subject: v1alpha1.Subject{
					Name: "app-set",
					TypeMeta: metav1.TypeMeta{
						Kind: "YurtAppDaemon",
					},
				},
			},
			client:      NewFakeClient(buildClient(nil)).WithErr(errors.New("YurtAppDaemon not exists")),
			expectedErr: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			webhook := &YurtAppOverriderHandler{tc.client}
			_, err := webhook.ValidateDelete(context.Background(), tc.obj)
			if tc.expectedErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.expectedErr)
			}
		})
	}
}

func buildClient(objs []client.Object) client.WithWatch {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = apis.AddToScheme(scheme)
	if len(objs) > 0 {
		return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	}
	return fake.NewClientBuilder().WithScheme(scheme).Build()
}

type FakeClient struct {
	client.Client
	err error
}

func (f *FakeClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if f.err != nil {
		return f.err
	}
	return nil
}

func (f *FakeClient) List(ctx context.Context, obj client.ObjectList, opts ...client.ListOption) error {
	if f.err != nil {
		return f.err
	}
	return f.Client.List(ctx, obj, opts...)
}

func NewFakeClient(client client.Client) *FakeClient {
	return &FakeClient{Client: client}
}

func (f *FakeClient) WithErr(err error) *FakeClient {
	f.err = err
	return f
}

func buildYurtAppOverrider(name string) []client.Object {
	return []client.Object{
		&v1alpha1.YurtAppOverrider{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Subject: v1alpha1.Subject{
				Name: "app-set",
			},
		},
	}
}
