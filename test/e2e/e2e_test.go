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

package e2e

import (
	"flag"
	nd "github.com/alibaba/openyurt/test/e2e/common/node"
	"github.com/alibaba/openyurt/test/e2e/yurt"
	"github.com/alibaba/openyurt/test/e2e/yurtconfig"
	"github.com/alibaba/openyurt/test/e2e/yurthub"
	"github.com/alibaba/openyurt/test/e2e/yurttunnel"
	"github.com/onsi/ginkgo"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/framework/config"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"
)

func IsEmptyString(s string) bool {
	return s == ""
}

var EnableYurtAutonomy = flag.Bool("enable-yurt-autonomy", false, "switch of yurt node autonomy. If set to true, yurt node autonomy test can be run normally")
var RegionId = flag.String("region-id", "", "aliyun region id for ailunyun:ecs/ens")
var NodeType = flag.String("node-type", "minikube", "node type such as ailunyun:ecs/ens, minikube and user_self")
var AccessKeyId = flag.String("access-key-id", "", "aliyun AccessKeyId  for ailunyun:ecs/ens")
var AccessKeySecret = flag.String("access-key-secret", "", "aliyun AccessKeySecret  for ailunyun:ecs/ens")

func handleFlags() {
	config.CopyFlags(config.Flags, flag.CommandLine)
	framework.RegisterCommonFlags(flag.CommandLine)
	framework.RegisterClusterFlags(flag.CommandLine)
	flag.Parse()
}

func IsvalidYurtArg() bool {
	//enable-yurt-autonomy and node-type arg will decide whether enable node autonomy test or not.
	//because one of node autonomy feature is depend on node restart.
	//After node restarts, it can get data from localdisk and ensure business can run normally.
	if !*EnableYurtAutonomy {
		return true
	}

	//if node type is not aliyun related, then node autonomy test will depend on userself to operate node
	nodeType := strings.ToLower(*NodeType)
	if nodeType != nd.NODE_TYPE_ALIYUN_ECS && nodeType != nd.NODE_TYPE_ALIYUN_ENS {
		klog.Infof("now,your node type is not aliyun_ecs and aliyun_ens, so yurt-autonomy test,will depend on you operationg your node")
		return true
	}

	//if aliyun ecs or ens is used, then must provide ak/sk and regionid
	//so yurt-e2e-test can operate node through aliyun sdk
	if IsEmptyString(*RegionId) || IsEmptyString(*AccessKeyId) || IsEmptyString(*AccessKeySecret) {
		klog.Errorf("if enable-yurt-autonomy is set true and node type is aliyun related, region-id && access-key-id && access-key-secret must not be empty")
		return false
	}
	return true
}

func PreCheckOk() bool {
	c, err := framework.LoadClientset()
	if err != nil {
		klog.Errorf("pre_check_load_client_set failed errmsg:%v", err)
		return false
	}

	nodes, err := c.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		klog.Errorf("pre_check_get_nodes failed errmsg:%v", err)
		return false
	}

	for _, node := range nodes.Items {
		status := node.Status.Conditions[len(node.Status.Conditions)-1].Type
		if status != apiv1.NodeReady {
			klog.Errorf("pre_check_get_node_status: not_ready, so exit")
			return false
		}
	}
	return true
}

func SetYurtE2eCfg() {
	yurtconfig.YurtE2eCfg.NodeType = strings.ToLower(*NodeType)
	yurtconfig.YurtE2eCfg.RegionId = *RegionId
	yurtconfig.YurtE2eCfg.EnableYurtAutonomy = *EnableYurtAutonomy
	yurtconfig.YurtE2eCfg.AccessKeyId = *AccessKeyId
	yurtconfig.YurtE2eCfg.AccessKeySecret = *AccessKeySecret
}

func TestMain(m *testing.M) {
	defer ginkgo.GinkgoRecover()

	handleFlags()

	if !IsvalidYurtArg() {
		os.Exit(-1)
	}

	if !PreCheckOk() {
		os.Exit(-1)
	}

	SetYurtE2eCfg()

	framework.AfterReadingAllFlags(&framework.TestContext)
	rand.Seed(time.Now().UnixNano())

	yurt.Register()
	yurthub.Register()
	yurttunnel.Register()

	os.Exit(m.Run())
}

func TestE2E(t *testing.T) {
	RunE2ETests(t)
}
