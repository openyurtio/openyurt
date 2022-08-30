/*
Copyright 2021 The OpenYurt Authors.

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

package iptables

import (
	"net"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	coreinformer "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/exec"
	fakeexec "k8s.io/utils/exec/testing"

	"github.com/openyurtio/openyurt/pkg/util/iptables"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/constants"
	"github.com/openyurtio/openyurt/pkg/yurttunnel/util"
)

var (
	ListenAddrForMaster         = net.JoinHostPort("0.0.0.0", constants.YurttunnelServerMasterPort)
	ListenInsecureAddrForMaster = net.JoinHostPort("127.0.0.1", constants.YurttunnelServerMasterInsecurePort)
	IptablesSyncPeriod          = 60
)

func newFakeIptablesManager(client clientset.Interface,
	nodeInformer coreinformer.NodeInformer,
	listenAddr string,
	listenInsecureAddr string,
	syncPeriod int,
	execer exec.Interface) *iptablesManager {

	protocol := iptables.ProtocolIpv4
	iptInterface := iptables.New(execer, protocol)

	if syncPeriod < defaultSyncPeriod {
		syncPeriod = defaultSyncPeriod
	}

	im := &iptablesManager{
		kubeClient:       client,
		iptables:         iptInterface,
		execer:           execer,
		nodeInformer:     nodeInformer,
		secureDnatDest:   listenAddr,
		insecureDnatDest: listenInsecureAddr,
		lastNodesIP:      make([]string, 0),
		lastDnatPorts:    make([]string, 0),
		syncPeriod:       syncPeriod,
	}
	return im
}

func TestCleanupIptableSettingAllExists(t *testing.T) {
	//1. create iptabeleMgr
	fakeClient := &fake.Clientset{}
	fakeInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0*time.Second)
	fcmd := fakeexec.FakeCmd{
		CombinedOutputScript: []fakeexec.FakeAction{
			// iptables version check
			func() ([]byte, []byte, error) { return []byte("iptables v1.9.22"), nil, nil },
			// DeleteRule Success
			func() ([]byte, []byte, error) { return []byte{}, nil, nil }, // success on the first call
			func() ([]byte, []byte, error) { return []byte{}, nil, nil }, // success on the second call

			// FlushChain Success
			func() ([]byte, []byte, error) { return []byte{}, nil, nil },
			// DeleteChain Success
			func() ([]byte, []byte, error) { return []byte{}, nil, nil },

			// FlushChain Success
			func() ([]byte, []byte, error) { return []byte{}, nil, nil },
			// DeleteChain Success
			func() ([]byte, []byte, error) { return []byte{}, nil, nil },

			// FlushChain Success
			func() ([]byte, []byte, error) { return []byte{}, nil, nil },
			// DeleteChain Success
			func() ([]byte, []byte, error) { return []byte{}, nil, nil },
		},
	}
	fexec := fakeexec.FakeExec{
		CommandScript: []fakeexec.FakeCommandAction{
			func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(&fcmd, cmd, args...) },

			func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(&fcmd, cmd, args...) },
			func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(&fcmd, cmd, args...) },

			func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(&fcmd, cmd, args...) },
			func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(&fcmd, cmd, args...) },

			func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(&fcmd, cmd, args...) },
			func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(&fcmd, cmd, args...) },

			func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(&fcmd, cmd, args...) },
			func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(&fcmd, cmd, args...) },
		},
	}

	iptablesMgr := newFakeIptablesManager(fakeClient,
		fakeInformerFactory.Core().V1().Nodes(),
		ListenAddrForMaster,
		ListenInsecureAddrForMaster,
		IptablesSyncPeriod,
		&fexec)

	if iptablesMgr == nil {
		t.Errorf("fail to create a new IptableManager")
	}

	//2. create configmap
	configmap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util.YurttunnelServerDnatConfigMapName,
			Namespace: "kube-system",
		},
		Data: map[string]string{
			"dnat-ports-pair": "",
		},
	}
	fakeInformerFactory.Core().V1().ConfigMaps().Informer().GetStore().Add(configmap)

	//3. call cleanupIptableSetting
	iptablesMgr.cleanupIptableSetting()
}

func TestGetIPOfNodesWithoutAgent(t *testing.T) {
	// init iptables manager
	fakeClient := &fake.Clientset{}
	fakeInformerFactory := informers.NewSharedInformerFactory(fakeClient, 0*time.Second)
	fcmd := fakeexec.FakeCmd{
		CombinedOutputScript: []fakeexec.FakeAction{
			// iptables version check
			func() ([]byte, []byte, error) { return []byte("iptables v1.9.22"), nil, nil },
		},
	}
	fexec := fakeexec.FakeExec{
		CommandScript: []fakeexec.FakeCommandAction{
			func(cmd string, args ...string) exec.Cmd { return fakeexec.InitFakeCmd(&fcmd, cmd, args...) },
		},
	}
	iptablesMgr := newFakeIptablesManager(fakeClient,
		fakeInformerFactory.Core().V1().Nodes(),
		"",
		"",
		0,
		&fexec)

	// add two nodes into informer
	node1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"openyurt.io/is-edge-worker": "true",
			},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
			Addresses: []corev1.NodeAddress{
				{
					Type:    corev1.NodeInternalIP,
					Address: "192.168.2.1",
				},
			},
		},
	}

	node2 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"openyurt.io/is-edge-worker": "false",
			},
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
			Addresses: []corev1.NodeAddress{
				{
					Type:    corev1.NodeInternalIP,
					Address: "192.168.2.2",
				},
			},
		},
	}
	fakeInformerFactory.Core().V1().Nodes().Informer().GetStore().Add(node1)
	fakeInformerFactory.Core().V1().Nodes().Informer().GetStore().Add(node2)

	// call getIPOfNodesWithoutAgent and check the result
	nodesIp := iptablesMgr.getIPOfNodesWithoutAgent()
	if len(nodesIp) != 1 && nodesIp[0] != "192.168.2.1" {
		t.Errorf("expect nodes ip 192.168.2.1, but got %v", nodesIp)
	}
}

func TestWithoutAgent(t *testing.T) {
	testcases := map[string]struct {
		node   *corev1.Node
		result bool
	}{
		"node has edge-enable-reverseTunnel-client label": {
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"openyurt.io/edge-enable-reverseTunnel-client": "true",
					},
				},
			},
			result: false,
		},
		"node has is-edge-worker label": {
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"openyurt.io/is-edge-worker": "true",
					},
				},
			},
			result: false,
		},
		"node has no is-edge-worker and edge-enable-reverseTunnel-client label": {
			node:   &corev1.Node{},
			result: true,
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			hasNoAgent := withoutAgent(tt.node)
			if hasNoAgent != tt.result {
				t.Errorf("expect node has no agent: %v, but got %v", tt.result, hasNoAgent)
			}
		})
	}
}

func TestIsNodeReady(t *testing.T) {
	testcases := map[string]struct {
		node   *corev1.Node
		result bool
	}{
		"node has no ready condition": {
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeMemoryPressure,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			result: false,
		},
		"node has ready condition and status is false": {
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionFalse,
						},
					},
				},
			},
			result: false,
		},
		"node has ready condition and status is true": {
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			result: true,
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			isReady := isNodeReady(tt.node)
			if isReady != tt.result {
				t.Errorf("expect node ready condition is: %v, but got %v", tt.result, isReady)
			}
		})
	}
}

func TestGetNodeInternalIPs(t *testing.T) {
	testcases := map[string]struct {
		node   *corev1.Node
		result []string
	}{
		"node has no internal ip": {
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeHostName,
							Address: "test-node",
						},
					},
				},
			},
			result: []string{},
		},
		"node has normal internal ip": {
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeInternalIP,
							Address: "192.168.2.1",
						},
					},
				},
			},
			result: []string{"192.168.2.1"},
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			nodeIPs := getNodeInternalIPs(tt.node)
			if len(nodeIPs) != len(tt.result) {
				t.Errorf("expect node ips len: %d, but got %d", len(tt.result), len(nodeIPs))
			}

			if len(nodeIPs) != 0 && !reflect.DeepEqual(nodeIPs, tt.result) {
				t.Errorf("expect node ips: %v, but got %v", tt.result, nodeIPs)
			}
		})
	}
}

func TestDnatIptablesArgs(t *testing.T) {
	testcases := map[string]struct {
		msg       string
		destPort  string
		proxyDest string
		result    []string
	}{
		"dnat for port 10250": {
			msg:       "comment test",
			destPort:  "10250",
			proxyDest: "127.0.0.1:10263",
			result:    []string{"-p", "tcp", "-m", "comment", "--comment", "comment test", "--dport", "10250", "-j", "DNAT", "--to-destination", "127.0.0.1:10263"},
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			dnatArgs := dnatIptablesArgs(tt.msg, tt.destPort, tt.proxyDest)
			if len(dnatArgs) != len(tt.result) {
				t.Errorf("expect dnat args len: %d, but got %d", len(tt.result), len(dnatArgs))
			}

			for i := range tt.result {
				if tt.result[i] != dnatArgs[i] {
					t.Errorf("expect dnat args: %v, but got %v", tt.result, dnatArgs)
				}
			}
		})
	}
}

func TestReqReturnIptablesArgs(t *testing.T) {
	testcases := map[string]struct {
		msg      string
		destPort string
		ip       string
		result   []string
	}{
		"dnat for port 192.168.2.1:10250": {
			msg:      "comment test",
			destPort: "10250",
			ip:       "192.168.2.1",
			result:   []string{"-p", "tcp", "-m", "comment", "--comment", "comment test", "--dport", "10250", "-d", "192.168.2.1/32", "-j", "RETURN"},
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			reqReturnArgs := reqReturnIptablesArgs(tt.msg, tt.destPort, tt.ip)
			if len(reqReturnArgs) != len(tt.result) {
				t.Errorf("expect req return args len: %d, but got %d", len(tt.result), len(reqReturnArgs))
			}

			for i := range tt.result {
				if tt.result[i] != reqReturnArgs[i] {
					t.Errorf("expect req return args: %v, but got %v", tt.result, reqReturnArgs)
				}
			}
		})
	}
}

func TestIptablesCommonArgs(t *testing.T) {
	testcases := map[string]struct {
		msg      string
		destPort string
		destIP   string
		result   []string
	}{
		"common args for ipv4": {
			msg:      "comment test",
			destPort: "10001",
			destIP:   "192.168.2.1",
			result:   []string{"-p", "tcp", "-m", "comment", "--comment", "comment test", "--dport", "10001", "-d", "192.168.2.1/32"},
		},
		"common args for ipv6": {
			msg:      "comment test",
			destPort: "10001",
			destIP:   "2001:db8:85a3:8d3:1319:8a2e:370:7348",
			result:   []string{"-p", "tcp", "-m", "comment", "--comment", "comment test", "--dport", "10001", "-d", "2001:db8:85a3:8d3:1319:8a2e:370:7348/128"},
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			commonArgs := iptablesCommonArgs(tt.msg, tt.destPort, net.ParseIP(tt.destIP))
			if len(commonArgs) != len(tt.result) {
				t.Errorf("expect common args len: %d, but got %d", len(tt.result), len(commonArgs))
			}

			for i := range tt.result {
				if tt.result[i] != commonArgs[i] {
					t.Errorf("expect common args: %v, but got %v", tt.result, commonArgs)
				}
			}
		})
	}
}

func TestGetAddedAndDeletedNodes(t *testing.T) {
	testcases := map[string]struct {
		lastNodesIP    []string
		currentNodesIP []string
		changed        bool
		added          []string
		deleted        []string
	}{
		"no nodes ip changed": {
			lastNodesIP:    []string{"196.168.1.1", "196.168.1.2", "196.168.1.3"},
			currentNodesIP: []string{"196.168.1.1", "196.168.1.2", "196.168.1.3"},
			changed:        false,
			added:          []string{},
			deleted:        []string{},
		},
		"add nodes ip": {
			lastNodesIP:    []string{"196.168.1.1", "196.168.1.2", "196.168.1.3"},
			currentNodesIP: []string{"196.168.1.1", "196.168.1.2", "196.168.1.3", "196.168.1.4", "196.168.1.5"},
			changed:        true,
			added:          []string{"196.168.1.4", "196.168.1.5"},
			deleted:        []string{},
		},
		"delete nodes ip": {
			lastNodesIP:    []string{"196.168.1.1", "196.168.1.2", "196.168.1.3"},
			currentNodesIP: []string{"196.168.1.1", "196.168.1.2"},
			changed:        true,
			added:          []string{},
			deleted:        []string{"196.168.1.3"},
		},
		"both add and delete nodes ip": {
			lastNodesIP:    []string{"196.168.1.1", "196.168.1.2", "196.168.1.3"},
			currentNodesIP: []string{"196.168.1.1", "196.168.1.4", "196.168.1.5"},
			changed:        true,
			added:          []string{"196.168.1.4", "196.168.1.5"},
			deleted:        []string{"196.168.1.2", "196.168.1.3"},
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			changed, added, deleted := getAddedAndDeletedNodes(tc.lastNodesIP, tc.currentNodesIP)
			if changed != tc.changed {
				t.Errorf("expect changed: %v, but got %v", tc.changed, changed)
			}

			if !reflect.DeepEqual(tc.added, added) {
				t.Errorf("expect added nodes ip: %v, but got: %v", tc.added, added)
			}

			if !reflect.DeepEqual(tc.deleted, deleted) {
				t.Errorf("expect deleted nodes ip: %v, but got: %v", tc.deleted, deleted)
			}
		})
	}
}

func TestGetDeletedPorts(t *testing.T) {
	testcases := map[string]struct {
		lastDnatPorts []string
		currentPorts  []string
		changed       bool
		deleted       []string
	}{
		"no ports changed": {
			lastDnatPorts: []string{"10001", "10002", "10003"},
			currentPorts:  []string{"10001", "10002", "10003"},
			changed:       false,
			deleted:       []string{},
		},
		"ports added": {
			lastDnatPorts: []string{"10001", "10002", "10003"},
			currentPorts:  []string{"10001", "10002", "10003", "10004", "10005"},
			changed:       true,
			deleted:       []string{},
		},
		"ports deleted": {
			lastDnatPorts: []string{"10001", "10002", "10003"},
			currentPorts:  []string{"10001"},
			changed:       true,
			deleted:       []string{"10002", "10003"},
		},
		"ports added and deleted": {
			lastDnatPorts: []string{"10001", "10002", "10003"},
			currentPorts:  []string{"10001", "10004", "10005"},
			changed:       true,
			deleted:       []string{"10002", "10003"},
		},
	}

	for k, tt := range testcases {
		t.Run(k, func(t *testing.T) {
			changed, deleted := getDeletedPorts(tt.lastDnatPorts, tt.currentPorts)
			if changed != tt.changed {
				t.Errorf("expect changed: %v, but got %v", tt.changed, changed)
			}

			if !reflect.DeepEqual(deleted, tt.deleted) {
				t.Errorf("expect deleted ports: %v, but got %v", tt.deleted, deleted)
			}
		})
	}
}
