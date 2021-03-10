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
	"github.com/openyurtio/openyurt/test/e2e/common/node/ecs"
	"github.com/openyurtio/openyurt/test/e2e/common/node/ens"
	"github.com/openyurtio/openyurt/test/e2e/common/node/types"
	"github.com/openyurtio/openyurt/test/e2e/common/node/user"
)

const (
	NodeTypeAliyunECS = "aliyun_ecs"
	NodeTypeUserSelf  = "user_self"
	NodeTypeAliyunENS = "aliyun_ens"
)

type NodeController interface {
	CreateNode(string, string, string, string) (string, error)
	StartNode(string) error
	DeleteNode(string) error
	GetNodeInfo(string) (*types.NodeAttribute, error)
	RebootNode(string) error
	StopNode(string) error
}

func NewNodeController(nodeType, regionID, accessKeyID, accessKeySecret string) (NodeController, error) {
	var t NodeController
	var err error
	switch nodeType {
	case NodeTypeAliyunECS:
		t, err = ecs.NewEcsController(regionID, accessKeyID, accessKeySecret)
	case NodeTypeUserSelf:
		t, err = user.NewUserController(regionID, accessKeyID, accessKeySecret)
	case NodeTypeAliyunENS:
		t, err = ens.NewEnsController(regionID, accessKeyID, accessKeySecret)
	default:
		t, err = user.NewUserController(regionID, accessKeyID, accessKeySecret)
	}
	return t, err
}
