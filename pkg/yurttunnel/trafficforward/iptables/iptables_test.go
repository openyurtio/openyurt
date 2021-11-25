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
