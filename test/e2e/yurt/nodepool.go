/*
Copyright 2022 The OpenYurt Authors.

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

package yurt

//import (
//	"context"
//	"errors"
//	"fmt"
//	"time"
//
//	. "github.com/onsi/ginkgo/v2"
//	. "github.com/onsi/gomega"
//	"k8s.io/apimachinery/pkg/util/rand"
//	"k8s.io/apimachinery/pkg/util/sets"
//	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
//
//	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
//	"github.com/openyurtio/openyurt/test/e2e/util"
//	ycfg "github.com/openyurtio/openyurt/test/e2e/yurtconfig"
//)
//
//var _ = Describe("nodepool test", func() {
//	ctx := context.Background()
//	var k8sClient runtimeclient.Client
//	poolToNodesMap := make(map[string]sets.String)
//
//	checkNodePoolStatus := func(poolToNodesMap map[string]sets.String) error {
//		nps := &v1beta1.NodePoolList{}
//		if err := k8sClient.List(ctx, nps); err != nil {
//			return err
//		}
//		for _, tmp := range nps.Items {
//			if int(tmp.Status.ReadyNodeNum) != poolToNodesMap[tmp.Name].Len() {
//				return errors.New("nodepool size not match")
//			}
//		}
//		return nil
//	}
//
//	BeforeEach(func() {
//		By("Start to run nodepool test, cleanup previous resources")
//		k8sClient = ycfg.YurtE2eCfg.RuntimeClient
//		poolToNodesMap = map[string]sets.String{}
//
//		util.CleanupNodePoolLabel(ctx, k8sClient)
//		util.CleanupNodePool(ctx, k8sClient)
//	})
//
//	AfterEach(func() {
//		By("Cleanup resources after test")
//		util.CleanupNodePoolLabel(ctx, k8sClient)
//		util.CleanupNodePool(ctx, k8sClient)
//	})
//
//	It("Test NodePool empty", func() {
//		By("Run noolpool empty")
//		Eventually(
//			func() error {
//				return util.InitNodeAndNodePool(ctx, k8sClient, poolToNodesMap)
//			},
//			time.Second*5, time.Millisecond*500).Should(SatisfyAny(BeNil()))
//
//		Eventually(
//			func() error {
//				return checkNodePoolStatus(poolToNodesMap)
//			},
//			time.Second*5, time.Millisecond*500).Should(SatisfyAny(BeNil()))
//	})
//
//	It("Test NodePool create", func() {
//		By("Run nodepool create")
//
//		npName := fmt.Sprintf("test-%s", rand.String(4))
//		poolToNodesMap[npName] = sets.NewString("openyurt-e2e-test-worker", "openyurt-e2e-test-worker2")
//		Eventually(
//			func() error {
//				return util.InitNodeAndNodePool(ctx, k8sClient, poolToNodesMap)
//			},
//			time.Second*5, time.Millisecond*500).Should(SatisfyAny(BeNil()))
//
//		Eventually(
//			func() error {
//				return checkNodePoolStatus(poolToNodesMap)
//			},
//			time.Second*5, time.Millisecond*500).Should(SatisfyAny(BeNil()))
//	})
//
//	It(" Test Multiple NodePools With Nodes", func() {
//		poolToNodesMap["beijing"] = sets.NewString("openyurt-e2e-test-worker")
//		poolToNodesMap["hangzhou"] = sets.NewString("openyurt-e2e-test-worker2")
//
//		Eventually(
//			func() error {
//				return util.InitNodeAndNodePool(ctx, k8sClient, poolToNodesMap)
//			},
//			time.Second*5, time.Millisecond*500).Should(SatisfyAny(BeNil()))
//
//		Eventually(
//			func() error {
//				return checkNodePoolStatus(poolToNodesMap)
//			},
//			time.Second*5, time.Millisecond*500).Should(SatisfyAny(BeNil()))
//	})
//
//})
