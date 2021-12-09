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
	"github.com/aliyun/alibaba-cloud-sdk-go/services/ens"

	"github.com/openyurtio/openyurt/test/e2e/common/node/types"
)

type EnsController struct {
	RegionID string
	Client   *ens.Client
}

func NewEnsController(regionID, accessKeyID, accessKeySecret string) (*EnsController, error) {
	var e EnsController
	var err error
	e.RegionID = regionID
	e.Client, err = ens.NewClientWithAccessKey(regionID, accessKeyID, accessKeySecret)
	return &e, err
}

func (u *EnsController) RebootNode(nodeName string) error {
	return nil
}

func (u *EnsController) CreateNode(instanceType, imageID, vswitchID, userData string) (string, error) {
	return "", nil
}

func (u *EnsController) StartNode(instanceID string) error {
	return nil
}

func (u *EnsController) DeleteNode(instanceID string) error {
	return nil
}

func (u *EnsController) GetNodeInfo(instanceID string) (*types.NodeAttribute, error) {
	return nil, nil
}

func (u *EnsController) StopNode(instanceID string) error {
	return nil
}
