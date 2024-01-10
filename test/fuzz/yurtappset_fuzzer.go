//go:build gofuzz
// +build gofuzz

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

package yurtappset

import (
	"context"
	"fmt"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"

	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtappset/adapter"
)

var (
	fuzzCtx              = context.Background()
	fakeSchemeForFuzzing = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(fakeSchemeForFuzzing)
	_ = appsv1alpha1.AddToScheme(fakeSchemeForFuzzing)
	_ = corev1.AddToScheme(fakeSchemeForFuzzing)
}

// helper function to crate an unstructured object.
func GetUnstructured(f *fuzz.ConsumeFuzzer) (*unstructured.Unstructured, error) {
	yamlStr, err := f.GetString()
	if err != nil {
		return nil, err
	}
	obj := make(map[string]interface{})
	err = yaml.Unmarshal([]byte(yamlStr), &obj)
	if err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{Object: obj}, nil
}

func validateUnstructured(unstr *unstructured.Unstructured) error {
	if _, ok := unstr.Object["kind"]; !ok {
		return fmt.Errorf("invalid unstr")
	}
	if _, ok := unstr.Object["apiVersion"]; !ok {
		return fmt.Errorf("invalid unstr")
	}
	if _, ok := unstr.Object["spec"]; !ok {
		return fmt.Errorf("invalid unstr")
	}
	if _, ok := unstr.Object["status"]; !ok {
		return fmt.Errorf("invalid unstr")
	}
	return nil
}

func FuzzAppSetReconcile(data []byte) int {
	f := fuzz.NewConsumer(data)
	unstr, err := GetUnstructured(f)
	if err != nil {
		return 0
	}
	err = validateUnstructured(unstr)
	if err != nil {
		return 0
	}

	appset := &appsv1alpha1.YurtAppSet{}
	if err := f.GenerateStruct(appset); err != nil {
		return 0
	}

	clientFake := fake.NewClientBuilder().WithScheme(fakeSchemeForFuzzing).WithObjects(
		appset,
	).Build()

	r := &ReconcileYurtAppSet{
		Client:   clientFake,
		scheme:   fakeSchemeForFuzzing,
		recorder: record.NewFakeRecorder(10000),
		poolControls: map[appsv1alpha1.TemplateType]ControlInterface{
			appsv1alpha1.StatefulSetTemplateType: &PoolControl{Client: clientFake, scheme: fakeSchemeForFuzzing,
				adapter: &adapter.StatefulSetAdapter{Client: clientFake, Scheme: fakeSchemeForFuzzing}},
			appsv1alpha1.DeploymentTemplateType: &PoolControl{Client: clientFake, scheme: fakeSchemeForFuzzing,
				adapter: &adapter.DeploymentAdapter{Client: clientFake, Scheme: fakeSchemeForFuzzing}},
		},
	}

	_, _ = r.Reconcile(fuzzCtx, reconcile.Request{NamespacedName: types.NamespacedName{Name: appset.Name, Namespace: appset.Namespace}})
	return 1
}
