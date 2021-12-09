/*
Copyright 2020 The OpenYurt Authors.

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

package ecs

/*
TODO
*/

import (
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ecs"

	"github.com/openyurtio/openyurt/test/e2e/common/node/types"
)

type EcsController struct {
	RegionID string
	Client   *ecs.Client
}

func NewEcsController(regionID, accessKeyID, accessKeySecret string) (*EcsController, error) {
	var e EcsController
	var err error
	e.RegionID = regionID
	e.Client, err = ecs.NewClientWithAccessKey(regionID, accessKeyID, accessKeySecret)
	return &e, err
}

func (e *EcsController) RebootNode(instanceID string) error {
	return nil
}

func (e *EcsController) CreateNode(instanceType, imageID, vswitchID, userData string) (string, error) {
	return "", nil
}

func (e *EcsController) StopNode(instanceID string) error {
	return nil
}

func (e *EcsController) StartNode(instanceID string) error {
	return nil
}

func (e *EcsController) GetNodeInfo(instanceID string) (*types.NodeAttribute, error) {
	return nil, nil
}

func (e *EcsController) DeleteNode(instanceID string) error {
	return nil
}

func (e *EcsController) CheckEcsInstanceStatus(instanceID string, expectStatus string) (bool, error) {
	return false, nil
}
