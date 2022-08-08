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

package user

import (
	"fmt"
	"strings"

	"github.com/onsi/ginkgo/v2"

	"github.com/openyurtio/openyurt/test/e2e/common/node/types"
)

type UserController struct {
	RegionID string
}

func NewUserController(regionID, accessKeyID, accessKeySecret string) (*UserController, error) {
	return &UserController{
		RegionID: regionID,
	}, nil
}

func (u *UserController) CreateNode(instanceType, imageID, vswitchID, userData string) (string, error) {
	return "", nil
}

func (u *UserController) StartNode(instanceID string) error {
	var Start string
	for {
		ginkgo.By("You should start local machine. Yurt-e2e-test will wait for starting after your input. Please input y or Y to make sure you have started node.")
		fmt.Scan(&Start)
		if strings.ToLower(Start) == "y" {
			break
		}
	}
	return nil
}

func (u *UserController) DeleteNode(instanceID string) error {
	return nil
}

func (u *UserController) GetNodeInfo(instanceID string) (*types.NodeAttribute, error) {
	return nil, nil
}

func (u *UserController) RebootNode(instanceID string) error {
	return nil
}

func (u *UserController) StopNode(instanceID string) error {
	var Stop string
	for {
		ginkgo.By("You should stop local machine. Yurt-e2e-test will wait for stopping after your input. Please input y or Y to make sure you have stopped node.")
		fmt.Scan(&Stop)
		if strings.ToLower(Stop) == "y" {
			break
		}
	}
	return nil
}
