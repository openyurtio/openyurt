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

package yurthub

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/test/e2e/constants"
)

var (
	Edge2NginxPodIP  string
	NginxServiceIP   string
	CoreDNSServiceIP string

	flannelContainerID   string
	yurthubContainerID   string
	kubeProxyContainerID string
	coreDnsContainerID   string
	nginxContainerID     string
)

var _ = ginkgo.Describe("edge-autonomy"+constants.YurtE2ENamespaceName, ginkgo.Ordered, ginkgo.Label("edge-autonomy"), func() {
	defer ginkgo.GinkgoRecover()
	var _ = ginkgo.Describe("kubelet"+constants.YurtE2ENamespaceName, func() {
		ginkgo.It("kubelet edge-autonomy test", ginkgo.Label("edge-autonomy"), func() {
			// restart kubelet using systemctl restart kubelet in edge nodes；
			_, err := exec.Command("/bin/bash", "-c", "docker exec -t openyurt-e2e-test-worker /bin/bash -c 'systemctl restart kubelet'").CombinedOutput()
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to restart kubelet")
			// check periodically if kubelet restarted
			gomega.Eventually(func() bool {
				opBytes, err := exec.Command("/bin/bash", "-c", "docker exec -t openyurt-e2e-test-worker /bin/bash -c 'curl -m 2 http://127.0.0.1:10248/healthz'").CombinedOutput()
				if err != nil {
					return false
				}
				if string(opBytes) == "ok" {
					return true
				} else {
					return false
				}
			}).WithTimeout(10*time.Second).WithPolling(1*time.Second).Should(gomega.BeTrue(), "fail to check kubelet health")
			//check periodically if nginx restarted successfully
			gomega.Eventually(func() string {
				opBytes, err := exec.Command("/bin/bash", "-c", "docker exec -t openyurt-e2e-test-worker /bin/bash -c 'curl -m 2 http://127.0.0.1:80'").CombinedOutput()
				if err != nil {
					return ""
				}
				return string(opBytes)
			}).WithTimeout(10*time.Second).WithPolling(1*time.Second).Should(gomega.ContainSubstring("nginx"), "nginx pod not running")
		})
	})

	var _ = ginkgo.Describe("flannel"+constants.YurtE2ENamespaceName, func() {
		ginkgo.It("flannel edge-autonomy test", ginkgo.Label("edge-autonomy"), func() {
			// obtain flannel containerID with crictl
			cmd := `docker exec -t openyurt-e2e-test-worker /bin/bash -c "crictl ps | grep kube-flannel | awk '{print \$1}'"`
			opBytes, err := exec.Command("/bin/bash", "-c", cmd).CombinedOutput()
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to get flannel container ID")
			flannelContainerID = strings.TrimSpace(string(opBytes))

			// restart flannel
			_, err = exec.Command("/bin/bash", "-c", "docker exec -t openyurt-e2e-test-worker /bin/bash -c 'crictl stop "+flannelContainerID+"'").CombinedOutput()
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to stop flannel")

			// obtain nginx containerID with crictl
			cmd = `docker exec -t openyurt-e2e-test-worker /bin/bash -c "crictl ps | grep yurt-e2e-test-nginx | awk '{print \$1}'"`
			opBytes, err = exec.Command("/bin/bash", "-c", cmd).CombinedOutput()
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to get nginx container ID")
			nginxContainerID = strings.TrimSpace(string(opBytes))

			// curl pod on another edge node using podIP, periodically
			gomega.Eventually(func() string {
				curlCmd := "curl -m 2 " + Edge2NginxPodIP
				crictlCmd := "crictl exec -it " + nginxContainerID + " " + curlCmd
				dockerCmd := `docker exec -t openyurt-e2e-test-worker /bin/bash -c ` + "'" + crictlCmd + "'"
				opBytes, err := exec.Command("/bin/bash", "-c", dockerCmd).CombinedOutput()
				if err != nil {
					return ""
				}
				return string(opBytes)
			}).WithTimeout(10*time.Second).WithPolling(1*time.Second).Should(gomega.ContainSubstring("nginx"), "fail to curl worker2 nginx PodIP from nginx on worker1")
		})
	})

	var _ = ginkgo.Describe("yurthub"+constants.YurtE2ENamespaceName, func() {
		ginkgo.It("yurthub edge-autonomy test", ginkgo.Label("edge-autonomy"), func() {
			// obtain yurthub containerID with crictl
			cmd := `docker exec -t openyurt-e2e-test-worker /bin/bash -c "crictl ps | grep yurt-hub | awk '{print \$1}'"`
			opBytes, err := exec.Command("/bin/bash", "-c", cmd).CombinedOutput()
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to get yurthub container ID")
			yurthubContainerID = strings.TrimSpace(string(opBytes))

			// restart yurthub
			_, err = exec.Command("/bin/bash", "-c", "docker exec -t openyurt-e2e-test-worker /bin/bash -c 'crictl stop "+yurthubContainerID+"'").CombinedOutput()
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to stop yurthub")

			// check yurthub health
			gomega.Eventually(func() bool {
				opBytes, err := exec.Command("/bin/bash", "-c", "docker exec -t openyurt-e2e-test-worker /bin/bash -c 'curl -m 2 http://127.0.0.1:10267/v1/healthz'").CombinedOutput()
				if err != nil {
					return false
				}
				if strings.Contains(string(opBytes), "OK") {
					return true
				} else {
					return false
				}
			}).WithTimeout(120*time.Second).WithPolling(1*time.Second).Should(gomega.BeTrue(), "fail to check yurthub health")
		})
	})

	var _ = ginkgo.Describe("kube-proxy"+constants.YurtE2ENamespaceName, func() {
		ginkgo.It("kube-proxy edge-autonomy test", ginkgo.Label("edge-autonomy"), func() {
			// obtain kube-proxy containerID with crictl
			cmd := `docker exec -t openyurt-e2e-test-worker /bin/bash -c "crictl ps | grep kube-proxy | awk '{print \$1}'"`
			opBytes, err := exec.Command("/bin/bash", "-c", cmd).CombinedOutput()
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to get kube-proxy container ID")
			kubeProxyContainerID = strings.TrimSpace(string(opBytes))

			// delete created iptables related to NginxService, to see if kube-proxy will generate new ones and delegate services
			_, err = exec.Command("/bin/bash", "-c", "docker exec -t openyurt-e2e-test-worker /bin/bash -c \"iptables-save | sed '/"+constants.NginxServiceName+"/d' | iptables-restore\"").CombinedOutput()
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to remove iptables on node openyurt-e2e-test-worker")

			// restart kube-proxy
			_, err = exec.Command("/bin/bash", "-c", "docker exec -t openyurt-e2e-test-worker /bin/bash -c 'crictl stop "+kubeProxyContainerID+"'").CombinedOutput()
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to stop kube-proxy")

			// check periodically if kube-proxy guided the service request to actual pod
			gomega.Eventually(func() string {
				opBytes, err := exec.Command("/bin/bash", "-c", "docker exec -t openyurt-e2e-test-worker /bin/bash -c 'curl -m 2 "+NginxServiceIP+"'").CombinedOutput()
				if err != nil {
					return ""
				}
				return string(opBytes)
			}).WithTimeout(10*time.Second).WithPolling(1*time.Second).Should(gomega.ContainSubstring("nginx"), "fail to read curl response from service: "+constants.NginxServiceName)
		})
	})

	var _ = ginkgo.Describe("coredns"+constants.YurtE2ENamespaceName, func() {
		ginkgo.It("coredns edge-autonomy test", ginkgo.Label("edge-autonomy"), func() {
			// obtain coredns containerID with crictl on edge node1
			cmd := `docker exec -t openyurt-e2e-test-worker /bin/bash -c "crictl ps | grep coredns | awk '{print \$1}'"`
			opBytes, err := exec.Command("/bin/bash", "-c", cmd).CombinedOutput()
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to get coredns container ID")
			coreDnsContainerID = strings.TrimSpace(string(opBytes))

			// restart coredns
			_, err = exec.Command("/bin/bash", "-c", "docker exec -t openyurt-e2e-test-worker /bin/bash -c 'crictl stop "+coreDnsContainerID+"'").CombinedOutput()
			gomega.Expect(err).NotTo(gomega.HaveOccurred(), "fail to stop coredns")

			// check periodically if coredns is able of dns resolution
			gomega.Eventually(func() string {
				cmd := fmt.Sprintf("docker exec -t openyurt-e2e-test-worker /bin/bash -c 'dig @%s %s.%s.svc.cluster.local'", CoreDNSServiceIP, constants.NginxServiceName, constants.YurtDefaultNamespaceName)
				opBytes, err := exec.Command("/bin/bash", "-c", cmd).CombinedOutput()
				if err != nil {
					klog.Errorf("failed to execute dig command for coredns, %v", err)
					return ""
				}
				return string(opBytes)
			}).WithTimeout(60*time.Second).WithPolling(1*time.Second).Should(gomega.ContainSubstring("NOERROR"), "DNS resolution contains error, coreDNS dig failed")
		})
	})
})
