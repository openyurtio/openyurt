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

package node

import (
	"github.com/alibaba/openyurt/test/e2e/common/node/ecs"
	"github.com/alibaba/openyurt/test/e2e/common/node/ens"
	"github.com/alibaba/openyurt/test/e2e/common/node/types"
	"github.com/alibaba/openyurt/test/e2e/common/node/user"
)

const (
	NODE_TYPE_ALIYUN_ECS = "aliyun_ecs"
	NODE_TYPE_USER_SELF  = "user_self"
	NODE_TYPE_ALIYUN_ENS = "aliyun_ens"
	NODE_TYPE_LOCAL      = "minikube"
)

type NodeController interface {
	CreateNode(string, string, string, string) (string, error)
	StartNode(string) error
	DeleteNode(string) error
	GetNodeInfo(string) (*types.NodeAttribute, error)
	RebootNode(string) error
	StopNode(string) error
}

func NewNodeController(nodeType, regionId, accessKeyId, accessKeySecret string) (NodeController, error) {
	var t NodeController
	var err error
	switch nodeType {
	case NODE_TYPE_ALIYUN_ECS:
		t, err = ecs.NewEcsController(regionId, accessKeyId, accessKeySecret)
	case NODE_TYPE_USER_SELF:
		t, err = user.NewUserController(regionId, accessKeyId, accessKeySecret)
	case NODE_TYPE_ALIYUN_ENS:
		t, err = ens.NewEnsController(regionId, accessKeyId, accessKeySecret)
	default:
		t, err = user.NewUserController(regionId, accessKeyId, accessKeySecret)
	}
	return t, err
}
