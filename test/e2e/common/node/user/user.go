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
    "github.com/alibaba/openyurt/test/e2e/common/node/types"
    "github.com/onsi/ginkgo"
    "strings"
)

type UserController struct {
    RegionId string
}

func NewUserController(regionId, accessKeyId, accessKeySecret string) (*UserController, error) {
    return &UserController{
        RegionId: regionId,
    }, nil
}

func (u *UserController) CreateNode(instanceType, imageId, vswitchId, userData string) (string, error) {
    return "", nil
}

func (u *UserController) StartNode(instanceId string) error {
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

func (u *UserController) DeleteNode(instanceId string) error {
    return nil
}

func (u *UserController) GetNodeInfo(instanceId string) (*types.NodeAttribute, error) {
    return nil, nil
}

func (u *UserController) RebootNode(instanceId string) error {
    return nil
}

func (u *UserController) StopNode(instanceId string) error {
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
