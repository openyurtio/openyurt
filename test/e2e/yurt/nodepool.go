/*
Copyright 2025 The OpenYurt Authors.

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

import (
	"context"
	"errors"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/sets"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openyurtio/openyurt/test/e2e/util"
	ycfg "github.com/openyurtio/openyurt/test/e2e/yurtconfig"
)

var _ = Describe("nodepool test", func() {
	ctx := context.Background()
	var k8sClient runtimeclient.Client

	// checkNodePoolStatus checks the status of the nodepool in poolToNodesMap
	// The nodepool is fetched from the k8sClient and the ready node number is checked
	// with the number of nodes expected in the pool
	checkNodePoolStatus := func(poolToNodesMap map[string]sets.Set[string]) error {
		for npName, nodes := range poolToNodesMap {
			// Get the node pool
			pool, err := util.GetNodepool(ctx, k8sClient, npName)
			if err != nil {
				return err
			}

			// Compare length with the number of nodes in map
			if int(pool.Status.ReadyNodeNum) != nodes.Len() {
				return errors.New("nodepool size not match")
			}
		}
		return nil
	}

	BeforeEach(func() {
		By("Start to run nodepool test, cleanup previous resources")
		k8sClient = ycfg.YurtE2eCfg.RuntimeClient
	})

	AfterEach(func() {})

	It("Test Nodepool lifecycle", func() {
		By("Run creating an empty nodepool and then deleting it")
		// We can delete an empty nodepool
		npName := fmt.Sprintf("test-%d", time.Now().Unix())
		poolToNodesMap := map[string]sets.Set[string]{
			npName: {},
		}

		Eventually(
			func() error {
				return util.InitNodeAndNodePool(ctx, k8sClient, poolToNodesMap)
			},
			time.Second*5, time.Millisecond*500).Should(BeNil())

		Eventually(
			func() error {
				return checkNodePoolStatus(poolToNodesMap)
			},
			time.Second*5, time.Millisecond*500).Should(BeNil())

		Eventually(
			func() error {
				return util.DeleteNodePool(ctx, k8sClient, npName)
			},
			time.Second*5, time.Millisecond*500).Should(BeNil())
	})

	It("Test NodePool create not empty", func() {
		By("Run nodepool create with worker 2") // worker 1 is already mapped to a pool
		npName := fmt.Sprintf("test-%d", time.Now().Unix())
		poolToNodesMap := map[string]sets.Set[string]{
			npName: sets.New("openyurt-e2e-test-worker2"), // we will use this worker in the nodepool
		}

		Eventually(
			func() error {
				return util.InitNodeAndNodePool(ctx, k8sClient, poolToNodesMap)
			},
			time.Second*5, time.Millisecond*500).Should(BeNil())

		Eventually(
			func() error {
				return checkNodePoolStatus(poolToNodesMap)
			},
			time.Second*5, time.Millisecond*500).Should(BeNil())
	})

})
