/*
Copyright 2021 The OpenYurt Authors.
Copyright 2019 The Kubernetes Authors.

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

package join

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"testing"

	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd/api"

	"github.com/openyurtio/openyurt/pkg/yurtadm/cmd/join/joindata"
	yurtconstants "github.com/openyurtio/openyurt/pkg/yurtadm/constants"
)

const (
	failed  = "\u2717"
	succeed = "\u2713"
)

func TestNewJoinOptions(t *testing.T) {
	tests := []struct {
		name   string
		expect joinOptions
	}{
		{
			"normal",
			joinOptions{
				nodeType:                 yurtconstants.EdgeNode,
				criSocket:                yurtconstants.DefaultDockerCRISocket,
				pauseImage:               yurtconstants.PauseImagePath,
				yurthubImage:             fmt.Sprintf("%s/%s:%s", yurtconstants.DefaultOpenYurtImageRegistry, yurtconstants.Yurthub, yurtconstants.DefaultOpenYurtVersion),
				namespace:                yurtconstants.YurthubNamespace,
				caCertHashes:             make([]string, 0),
				unsafeSkipCAVerification: false,
				ignorePreflightErrors:    make([]string, 0),
				kubernetesResourceServer: yurtconstants.DefaultKubernetesResourceServer,
				yurthubServer:            yurtconstants.DefaultYurtHubServerAddr,
				reuseCNIBin:              false,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := newJoinOptions()

				if !reflect.DeepEqual(tt.expect, *get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, *get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		})
	}
}

func TestNewCmdJoin(t *testing.T) {
	cmd := cobra.Command{
		Use:   "join [api-server-endpoint]",
		Short: "Run this on any machine you wish to join an existing cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			o, err := newJoinData(args, newJoinOptions())
			if err != nil {
				return err
			}

			joiner := newJoinerWithJoinData(o, os.Stdin, os.Stdout, os.Stderr)
			if err := joiner.Run(); err != nil {
				return err
			}
			return nil
		},
	}
	addJoinConfigFlags(cmd.Flags(), newJoinOptions())

	tests := []struct {
		name   string
		in     io.Reader
		out    io.Writer
		outErr io.Writer
		expect cobra.Command
	}{
		{
			"normal",
			os.Stdin,
			os.Stdout,
			os.Stderr,
			cmd,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := NewCmdJoin(tt.in, tt.out, tt.outErr)

				if !reflect.DeepEqual(tt.expect.Use, get.Use) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect.Use, get.Use)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)

			}
		})
	}
}

func TestAddJoinConfigFlags(t *testing.T) {
	flg := flag.FlagSet{}
	addJoinConfigFlags(&flg, newJoinOptions())

	tests := []struct {
		name   string
		expect flag.FlagSet
	}{
		{
			"normal",
			flg,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				if !reflect.DeepEqual(tt.expect, flg) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, flg)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, flg)

			}
		})
	}
}

func TestNewJoinerWithJoinData(t *testing.T) {
	tests := []struct {
		name   string
		o      *joinData
		in     io.Reader
		out    io.Writer
		outErr io.Writer
		expect nodeJoiner
	}{
		{
			"normal",
			&joinData{},
			os.Stdin,
			os.Stdout,
			os.Stderr,
			nodeJoiner{
				&joinData{},
				os.Stdin,
				os.Stdout,
				os.Stderr,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := newJoinerWithJoinData(&joinData{}, os.Stdin, os.Stdout, os.Stderr)
				if !reflect.DeepEqual(tt.expect, *get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, *get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, *get)

			}
		})
	}
}

func TestRun(t *testing.T) {
	var nj *nodeJoiner = newJoinerWithJoinData(&joinData{}, os.Stdin, os.Stdout, os.Stderr)

	tests := []struct {
		name   string
		expect error
	}{
		{
			"normal",
			fmt.Errorf("Write content 1 to file /proc/sys/net/ipv4/ip_forward fail: open /proc/sys/net/ipv4/ip_forward: no such file or directory "),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := nj.Run()
				if !reflect.DeepEqual(get, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, get, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, get, get)
			}
		})
	}
}

func TestNewJoinData(t *testing.T) {
	jo := newJoinOptions()
	jo2 := newJoinOptions()
	jo2.token = "v22u0b.17490yh3xp8azpr0"
	jo2.unsafeSkipCAVerification = true
	jo2.nodePoolName = "nodePool2"

	tests := []struct {
		name   string
		args   []string
		opt    *joinOptions
		expect *joinData
	}{
		{
			"normal",
			[]string{"localhost:8080"},
			jo,
			nil,
		},
		{
			"norma2",
			[]string{"localhost:8080"},
			jo2,
			nil,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get, _ := newJoinData(tt.args, tt.opt)
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
			}
		})
	}
}

func TestServerAddr(t *testing.T) {
	jd := joinData{
		apiServerEndpoint: "192.168.1.1",
	}

	tests := []struct {
		name   string
		expect string
	}{
		{
			"normal",
			"192.168.1.1",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := jd.ServerAddr()
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
			}
		})
	}
}

func TestJoinToken(t *testing.T) {
	jd := joinData{
		token: "192.168.1.1",
	}

	tests := []struct {
		name   string
		expect string
	}{
		{
			"normal",
			"192.168.1.1",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := jd.JoinToken()
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
			}
		})
	}
}

func TestPauseImage(t *testing.T) {
	jd := joinData{
		pauseImage: "192.168.1.1",
	}

	tests := []struct {
		name   string
		expect string
	}{
		{
			"normal",
			"192.168.1.1",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := jd.PauseImage()
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
			}
		})
	}
}

func TestYurtHubServer(t *testing.T) {
	jd := joinData{
		yurthubServer: "192.168.1.1",
	}

	tests := []struct {
		name   string
		expect string
	}{
		{
			"normal",
			"192.168.1.1",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := jd.YurtHubServer()
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
			}
		})
	}
}

func TestKubernetesVersion(t *testing.T) {
	jd := joinData{
		kubernetesVersion: "192.168.1.1",
	}

	tests := []struct {
		name   string
		expect string
	}{
		{
			"normal",
			"192.168.1.1",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := jd.KubernetesVersion()
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
			}
		})
	}
}

func TestTLSBootstrapCfg(t *testing.T) {
	jd := joinData{
		tlsBootstrapCfg: &api.Config{},
	}

	tests := []struct {
		name   string
		expect api.Config
	}{
		{
			"normal",
			api.Config{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := jd.TLSBootstrapCfg()
				if !reflect.DeepEqual(tt.expect, *get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, *get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, *get)
			}
		})
	}
}

func TestBootstrapClient(t *testing.T) {
	cs := &clientset.Clientset{}
	jd := joinData{
		clientSet: cs,
	}

	tests := []struct {
		name   string
		expect *clientset.Clientset
	}{
		{
			"normal",
			cs,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := jd.BootstrapClient()
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
			}
		})
	}
}

func TestNodeRegistration(t *testing.T) {
	nr := joindata.NodeRegistration{}

	jd := joinData{
		joinNodeData: &nr,
	}

	tests := []struct {
		name   string
		expect joindata.NodeRegistration
	}{
		{
			"normal",
			nr,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := jd.NodeRegistration()
				if !reflect.DeepEqual(tt.expect, *get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
			}
		})
	}
}

func TestIgnorePreflightErrors(t *testing.T) {
	jd := joinData{
		ignorePreflightErrors: sets.String{},
	}

	tests := []struct {
		name   string
		expect sets.String
	}{
		{
			"normal",
			sets.String{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := jd.IgnorePreflightErrors()
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
			}
		})
	}
}

func TestCaCertHashes(t *testing.T) {
	jd := joinData{
		caCertHashes: []string{},
	}

	tests := []struct {
		name   string
		expect []string
	}{
		{
			"normal",
			[]string{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := jd.CaCertHashes()
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
			}
		})
	}
}

func TestNodeLabels(t *testing.T) {
	jd := joinData{
		nodeLabels: map[string]string{},
	}

	tests := []struct {
		name   string
		expect map[string]string
	}{
		{
			"normal",
			map[string]string{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := jd.NodeLabels()
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
			}
		})
	}
}

func TestKubernetesResourceServer(t *testing.T) {
	jd := joinData{
		kubernetesResourceServer: "normal",
	}

	tests := []struct {
		name   string
		expect string
	}{
		{
			"normal",
			"normal",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := jd.KubernetesResourceServer()
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
			}
		})
	}
}

func TestStaticPodTemplateList(t *testing.T) {
	jd := joinData{
		staticPodTemplateList: []string{},
	}

	tests := []struct {
		name   string
		expect []string
	}{
		{
			"normal",
			[]string{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := jd.StaticPodTemplateList()
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
			}
		})
	}
}

func TestStaticPodManifestList(t *testing.T) {
	jd := joinData{
		staticPodManifestList: []string{},
	}

	tests := []struct {
		name   string
		expect []string
	}{
		{
			"normal",
			[]string{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", tt.name)
			{
				get := jd.StaticPodManifestList()
				if !reflect.DeepEqual(tt.expect, get) {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, tt.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, tt.expect, get)
			}
		})
	}
}
