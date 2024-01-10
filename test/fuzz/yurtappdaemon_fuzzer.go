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

package yurtappdaemon

import (
	"context"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtappdaemon/workloadcontroller"
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

func FuzzAppDaemonReconcile(data []byte) int {
	f := fuzz.NewConsumer(data)

	appDaemon := &appsv1alpha1.YurtAppDaemon{}
	if err := f.GenerateStruct(appDaemon); err != nil {
		return 0
	}

	clientFake := fake.NewClientBuilder().WithScheme(fakeSchemeForFuzzing).WithObjects(
		appDaemon,
	).Build()

	r := &ReconcileYurtAppDaemon{
		Client:   clientFake,
		scheme:   fakeSchemeForFuzzing,
		recorder: record.NewFakeRecorder(10000),
		controls: map[appsv1alpha1.TemplateType]workloadcontroller.WorkloadController{
			appsv1alpha1.DeploymentTemplateType: &workloadcontroller.DeploymentControllor{Client: clientFake, Scheme: fakeSchemeForFuzzing},
		},
	}

	_, _ = r.Reconcile(fuzzCtx, reconcile.Request{NamespacedName: types.NamespacedName{Name: appDaemon.Name, Namespace: appDaemon.Namespace}})
	return 1
}
