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
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openyurtio/openyurt/pkg/apis/apps/v1beta2"
	nodeutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/node"
	"github.com/openyurtio/openyurt/test/e2e/util"
	ycfg "github.com/openyurtio/openyurt/test/e2e/yurtconfig"
)

// Test hubleader elections must be run in Serial mode as they share the same node pool.
// The node pool spec is modified in each test spec, so running them in parallel will cause
// conflicts. This is intentional to avoid creating more nodes than necessary in Kind cluster.
var _ = Describe("Test hubleader elections", Serial, func() {
	ctx := context.Background()
	nodePoolName := "hubleadere2e"

	var k8sClient client.Client
	var pools util.TestNodePool

	// updateNodePoolSpec updates the nodepool spec with the provided spec
	updateNodePoolSpec := func(k8sClient client.Client, spec v1beta2.NodePoolSpec) func() error {
		return func() error {
			var pool = &v1beta2.NodePool{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: nodePoolName}, pool)
			if err != nil {
				return err
			}

			if spec.PoolScopeMetadata == nil {
				spec.PoolScopeMetadata = pool.Spec.PoolScopeMetadata
			}
			pool.Spec = spec
			return k8sClient.Update(ctx, pool)
		}
	}

	// getExpectedLeaders returns the expected leaders in the pool to nodes map provided
	// in the format of []v1beta2.Leader
	getExpectedLeaders := func(k8sClient client.Client, pool util.TestNodePool) []v1beta2.Leader {
		expectedLeaders := make([]v1beta2.Leader, 0, pool.Nodes.Len())
		for n := range pool.Nodes {
			node := &v1.Node{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: n}, node)
			Expect(err).ToNot(HaveOccurred())

			// Get node internal IP
			internalIP, ok := nodeutil.GetInternalIP(node)
			Expect(ok).To(BeTrue())

			expectedLeaders = append(expectedLeaders, v1beta2.Leader{
				Address:  internalIP,
				NodeName: n,
			})
		}

		// Sort for deterministic comparison
		slices.SortFunc(expectedLeaders, func(a, b v1beta2.Leader) int {
			return cmp.Compare(a.NodeName, b.NodeName)
		})

		return expectedLeaders
	}

	// getExpectedLeaderConfig returns the expected leader config map data
	getExpectedLeaderConfig := func(leaders []v1beta2.Leader) map[string]string {
		expectedLeaderConfig := make(map[string]string)

		leaderEndpoints := make([]string, 0, len(leaders))
		for _, leader := range leaders {
			leaderEndpoints = append(leaderEndpoints, leader.NodeName+"/"+leader.Address)
		}

		expectedLeaderConfig["leaders"] = strings.Join(leaderEndpoints, ",")
		expectedLeaderConfig["pool-scoped-metadata"] = "core/v1/services,discovery.k8s.io/v1/endpointslices"
		expectedLeaderConfig["interconnectivity"] = "true"
		expectedLeaderConfig["enable-leader-election"] = "true"

		return expectedLeaderConfig
	}

	getActualLeaders := func() []v1beta2.Leader {
		pool, err := util.GetNodepool(ctx, k8sClient, nodePoolName)
		if err != nil {
			return nil
		}

		// Sort for deterministic comparison
		slices.SortFunc(pool.Status.LeaderEndpoints, func(a, b v1beta2.Leader) int {
			return cmp.Compare(a.NodeName, b.NodeName)
		})
		return pool.Status.LeaderEndpoints
	}

	getActualLeaderConfig := func() map[string]string {
		configMap := v1.ConfigMap{}
		err := k8sClient.Get(
			ctx,
			client.ObjectKey{Name: "leader-hub-" + nodePoolName, Namespace: metav1.NamespaceSystem},
			&configMap,
		)
		if err != nil {
			return nil
		}
		return configMap.Data
	}

	resetNodePool := func() error {
		pool := &v1beta2.NodePool{}
		err := k8sClient.Get(
			ctx,
			client.ObjectKey{Name: nodePoolName},
			pool,
		)
		if err != nil {
			return err
		}
		pool.Spec.EnableLeaderElection = false

		return k8sClient.Update(ctx, pool)
	}

	BeforeEach(func() {
		By("Place workers 3 and 4 in the same node pool")
		k8sClient = ycfg.YurtE2eCfg.RuntimeClient

		pools = util.TestNodePool{
			NodePool: v1beta2.NodePool{
				ObjectMeta: metav1.ObjectMeta{
					Name: nodePoolName,
				},
				Spec: v1beta2.NodePoolSpec{
					Type:                 v1beta2.Edge,
					InterConnectivity:    true,
					EnableLeaderElection: false,
				},
			},
			Nodes: sets.New("openyurt-e2e-test-worker3", "openyurt-e2e-test-worker4"),
		}

		Eventually(
			func() error {
				return util.InitTestNodePool(ctx, k8sClient, pools)
			},
			time.Second*30, time.Millisecond*500).Should(BeNil())
	})

	AfterEach(func() {
		Eventually(
			resetNodePool,
			time.Second*30, time.Millisecond*500).Should(BeNil())
	})

	Context("Random strategy", func() {
		It("should elect 2 desired hub leaders correctly", Serial, func() {
			By("Update the node pool spec with random election strategy and 2 desired leader replicas")
			Eventually(
				retry.RetryOnConflict(
					retry.DefaultRetry,
					updateNodePoolSpec(
						k8sClient,
						v1beta2.NodePoolSpec{
							LeaderElectionStrategy: string(v1beta2.ElectionStrategyRandom),
							LeaderReplicas:         2,
							Type:                   v1beta2.Edge,
							InterConnectivity:      true,
							EnableLeaderElection:   true,
						},
					),
				),
				time.Second*30, time.Millisecond*500).Should(BeNil())

			expectedLeaders := getExpectedLeaders(k8sClient, pools)

			// Check leader endpoints
			By("Check leader endpoints have been set correctly in the nodepool")
			Eventually(
				getActualLeaders,
				time.Second*30, time.Millisecond*500).Should(Equal(expectedLeaders))

			// Check leader config map
			By("Check leader config map contains the correct leader information")
			Eventually(
				getActualLeaderConfig,
				time.Second*30, time.Millisecond*500).Should(Equal(getExpectedLeaderConfig(expectedLeaders)))
		})
	})

	Context("Mark strategy", func() {
		It("should elect the marked node correctly", Serial, func() {
			By("Update the node pool spec with mark election strategy and worker 3 as the marked leader")
			Eventually(
				retry.RetryOnConflict(
					retry.DefaultRetry,
					updateNodePoolSpec(
						k8sClient,
						v1beta2.NodePoolSpec{
							LeaderElectionStrategy: string(v1beta2.ElectionStrategyMark),
							LeaderReplicas:         2,
							LeaderNodeLabelSelector: map[string]string{
								"kubernetes.io/hostname": "openyurt-e2e-test-worker3", // Mark
							},
							Type:                 v1beta2.Edge,
							InterConnectivity:    true,
							EnableLeaderElection: true,
						},
					),
				),
				time.Second*30, time.Millisecond*500).Should(BeNil())

			// Remove worker 4 from the test pool and generate expected leaders
			pools.Nodes.Delete("openyurt-e2e-test-worker4")

			expectedLeaders := getExpectedLeaders(k8sClient, pools)

			By("Check leader endpoints have been set to worker 3")
			Eventually(
				getActualLeaders,
				time.Second*30, time.Millisecond*500).Should(Equal(expectedLeaders))

			By("Check leader config map contains worker 3 as the leader")
			Eventually(
				getActualLeaderConfig,
				time.Second*30, time.Millisecond*500).Should(Equal(getExpectedLeaderConfig(expectedLeaders)))
		})
	})
})
