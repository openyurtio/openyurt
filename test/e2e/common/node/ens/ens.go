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

package ens

/*
TODO
*/

import (
	"github.com/alibaba/openyurt/test/e2e/common/node/types"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ens"
)

type EnsController struct {
	RegionId string
	Client   *ens.Client
}

func NewEnsController(regionId, accessKeyId, accessKeySecret string) (*EnsController, error) {
	var e EnsController
	var err error
	e.RegionId = regionId
	e.Client, err = ens.NewClientWithAccessKey(regionId, accessKeyId, accessKeySecret)
	return &e, err
}

func (u *EnsController) RebootNode(nodeName string) error {
	return nil
}

func (u *EnsController) CreateNode(instanceType, imageId, vswitchId, userData string) (string, error) {
	return "", nil
}

func (u *EnsController) StartNode(instanceId string) error {
	return nil
}

func (u *EnsController) DeleteNode(instanceId string) error {
	return nil
}

func (u *EnsController) GetNodeInfo(instanceId string) (*types.NodeAttribute, error) {
	return nil, nil
}

func (u *EnsController) StopNode(instanceId string) error {
	return nil
}
