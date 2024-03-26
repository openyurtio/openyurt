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
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
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

	nodeName := "foo"
	nodePoolName := "foo-pool"
	masterServiceHost := "127.0.0.1"
	masterServicePort := "8080"

	obj := New(sharedFactory, fakeClient, nodeName, nodePoolName, masterServiceHost, masterServicePort)
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
		"init node err filter": {
			fn:     NewNodeErrFilter,
			result: nodeNameErr,
		},
		"init pool err filter": {
			fn:     NewPoolErrFilter,
			result: poolNameErr,
		},
		"init master svc host err filter": {
			fn:     NewMasterSvcHostErrFilter,
			result: masterSvcHostErr,
		},
		"init master svc port err filter": {
			fn:     NewMasterSvcPortErrFilter,
			result: masterSvcPortErr,
		},
		"init factory err filter": {
			fn:     NewFactoryErrFilter,
			result: factoryErr,
		},
		"init kube client err filter": {
			fn:     NewKubeClientErrFilter,
			result: kubeClientErr,
		},
	}
	fakeClient := &fake.Clientset{}
	sharedFactory := informers.NewSharedInformerFactory(fakeClient, 24*time.Hour)

	nodeName := "foo"
	nodePoolName := "foo-pool"
	masterServiceHost := "127.0.0.1"
	masterServicePort := "8080"

	obj := New(sharedFactory, fakeClient, nodeName, nodePoolName, masterServiceHost, masterServicePort)

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			objFilter, _ := tc.fn()
			err := obj.Initialize(objFilter)
			if !errors.Is(err, tc.result) {
				t.Errorf("expect result error: %v, but got %v", tc.result, err)
			}
		})
	}
}

type baseErrFilter struct {
}

func (bef *baseErrFilter) Name() string {
	return "nop"
}

func (bef *baseErrFilter) SupportedResourceAndVerbs() map[string]sets.String {
	return map[string]sets.String{}
}

func (bef *baseErrFilter) Filter(obj runtime.Object, _ <-chan struct{}) runtime.Object {
	return obj
}

var (
	nodeNameErr      = errors.New("node name error")
	poolNameErr      = errors.New("pool name error")
	masterSvcHostErr = errors.New("master svc host error")
	masterSvcPortErr = errors.New("master svc port error")
	factoryErr       = errors.New("factory error")
	kubeClientErr    = errors.New("kube client error")
)

type nodeErrFilter struct {
	baseErrFilter
	err error
}

func NewNodeErrFilter() (filter.ObjectFilter, error) {
	return &nodeErrFilter{
		err: nodeNameErr,
	}, nil
}

func (nef *nodeErrFilter) SetNodeName(nodeName string) error {
	return nef.err
}

type poolErrFilter struct {
	baseErrFilter
	err error
}

func NewPoolErrFilter() (filter.ObjectFilter, error) {
	return &poolErrFilter{
		err: poolNameErr,
	}, nil
}

func (pef *poolErrFilter) SetNodePoolName(poolName string) error {
	return pef.err
}

type masterSvcHostErrFilter struct {
	baseErrFilter
	err error
}

func NewMasterSvcHostErrFilter() (filter.ObjectFilter, error) {
	return &masterSvcHostErrFilter{
		err: masterSvcHostErr,
	}, nil
}

func (mshef *masterSvcHostErrFilter) SetMasterServiceHost(host string) error {
	return mshef.err
}

func (mshef *masterSvcHostErrFilter) SetMasterServicePort(port string) error {
	return nil
}

type masterSvcPortErrFilter struct {
	baseErrFilter
	err error
}

func NewMasterSvcPortErrFilter() (filter.ObjectFilter, error) {
	return &masterSvcPortErrFilter{
		err: masterSvcPortErr,
	}, nil
}

func (mvpef *masterSvcPortErrFilter) SetMasterServiceHost(host string) error {
	return nil
}

func (mvpef *masterSvcPortErrFilter) SetMasterServicePort(port string) error {
	return mvpef.err
}

type factoryErrFilter struct {
	baseErrFilter
	err error
}

func NewFactoryErrFilter() (filter.ObjectFilter, error) {
	return &factoryErrFilter{
		err: factoryErr,
	}, nil
}

func (fef *factoryErrFilter) SetSharedInformerFactory(factory informers.SharedInformerFactory) error {
	return fef.err
}

type kubeClientErrFilter struct {
	baseErrFilter
	err error
}

func NewKubeClientErrFilter() (filter.ObjectFilter, error) {
	return &kubeClientErrFilter{
		err: kubeClientErr,
	}, nil
}

func (kcef *kubeClientErrFilter) SetKubeClient(client kubernetes.Interface) error {
	return kcef.err
}
