/*
Copyright 2023 The OpenYurt Authors.

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

package initializer

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic/dynamicinformer"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/discardcloudservice"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/inclusterconfig"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/masterservice"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/nodeportisolation"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/servicetopology"
)

func TestNew(t *testing.T) {
	fakeClient := &fake.Clientset{}
	sharedFactory := informers.NewSharedInformerFactory(fakeClient, 24*time.Hour)

	scheme := runtime.NewScheme()
	fakeDynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	nodePoolFactory := dynamicinformer.NewDynamicSharedInformerFactory(fakeDynamicClient, 24*time.Hour)

	nodeName := "foo"
	nodePoolName := "foo-pool"
	masterServiceHost := "127.0.0.1"
	masterServicePort := "8080"

	obj := New(sharedFactory, nodePoolFactory, fakeClient, nodeName, nodePoolName, masterServiceHost, masterServicePort)
	_, ok := obj.(filter.Initializer)
	if !ok {
		t.Errorf("expect a filter Initializer object, but got %v", reflect.TypeOf(obj))
	}
}

func TestInitialize(t *testing.T) {
	testcases := map[string]struct {
		fn     func() (filter.ObjectFilter, error)
		result error
	}{
		"init discardcloudservice filter": {
			fn:     discardcloudservice.NewDiscardCloudServiceFilter,
			result: nil,
		},
		"init inclusterconfig filter": {
			fn:     inclusterconfig.NewInClusterConfigFilter,
			result: nil,
		},
		"init masterservice filter": {
			fn:     masterservice.NewMasterServiceFilter,
			result: nil,
		},
		"init nodeportisolation filter": {
			fn:     nodeportisolation.NewNodePortIsolationFilter,
			result: nil,
		},
		"init servicetopology filter": {
			fn:     servicetopology.NewServiceTopologyFilter,
			result: nil,
		},
		"init errfilter filter": {
			fn:     NewErrFilter,
			result: nodeNameErr,
		},
	}
	fakeClient := &fake.Clientset{}
	sharedFactory := informers.NewSharedInformerFactory(fakeClient, 24*time.Hour)

	scheme := runtime.NewScheme()
	fakeDynamicClient := dynamicfake.NewSimpleDynamicClient(scheme)
	nodePoolFactory := dynamicinformer.NewDynamicSharedInformerFactory(fakeDynamicClient, 24*time.Hour)
	nodeName := "foo"
	nodePoolName := "foo-pool"
	masterServiceHost := "127.0.0.1"
	masterServicePort := "8080"

	obj := New(sharedFactory, nodePoolFactory, fakeClient, nodeName, nodePoolName, masterServiceHost, masterServicePort)

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			objFilter, _ := tc.fn()
			err := obj.Initialize(objFilter)
			if tc.result != err {
				t.Errorf("expect result error: %v, but got %v", tc.result, err)
			}
		})
	}
}

type errFilter struct {
	err error
}

var (
	nodeNameErr = errors.New("node name error")
)

func NewErrFilter() (filter.ObjectFilter, error) {
	return &errFilter{
		err: nodeNameErr,
	}, nil
}

func (ef *errFilter) Name() string {
	return "nop"
}

func (ef *errFilter) SupportedResourceAndVerbs() map[string]sets.String {
	return map[string]sets.String{}
}

func (ef *errFilter) Filter(obj runtime.Object, _ <-chan struct{}) runtime.Object {
	return obj
}

func (ef *errFilter) SetNodeName(nodeName string) error {
	return ef.err
}
