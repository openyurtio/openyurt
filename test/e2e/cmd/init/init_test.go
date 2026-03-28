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

package init

import (
	"io"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientsetfake "k8s.io/client-go/kubernetes/fake"
)

func TestAddFlags(t *testing.T) {
	args := []string{
		"--kind-config-path=/home/root/.kube/config.yaml",
		"--node-num=100",
		"--cluster-name=test-openyurt",
		"--cloud-nodes=worker3",
		"--openyurt-version=v1.0.1",
		"--kubernetes-version=v1.22.7",
		"--use-local-images=true",
		"--kube-config=/home/root/.kube/config",
		"--ignore-error=true",
		"--disable-default-cni=true",
	}
	o := newKindOptions()
	cmd := &cobra.Command{}
	fs := cmd.Flags()
	addFlags(fs, o)
	fs.Parse(args)

	expectedOpts := &kindOptions{
		KindConfigPath:    "/home/root/.kube/config.yaml",
		NodeNum:           100,
		ClusterName:       "test-openyurt",
		CloudNodes:        "worker3",
		OpenYurtVersion:   "v1.0.1",
		KubernetesVersion: "v1.22.7",
		UseLocalImages:    true,
		KubeConfig:        "/home/root/.kube/config",
		IgnoreError:       true,
		DisableDefaultCNI: true,
	}

	if !reflect.DeepEqual(expectedOpts, o) {
		t.Errorf("expect options: %v, but got %v", expectedOpts, o)
	}
}

func TestValidateKubernetesVersion(t *testing.T) {
	cases := map[string]struct {
		version string
		want    string
	}{
		"invalid format": {
			"invalid",
			"invalid format of kubernetes version: invalid",
		},
		"unsupported version": {
			"v1.1",
			"unsupported kubernetes version: v1.1",
		},
		"1-dot format": {
			"v1.20",
			"",
		},
		"unsupported 2-dot format": {
			"v1.23.122",
			"",
		},
		"unsupported 1-dot format": {
			"v1.0",
			"unsupported kubernetes version: v1.0",
		},
	}

	for name, c := range cases {
		err := validateKubernetesVersion(c.version)
		if err == nil {
			if c.want != "" {
				t.Errorf("validateKubernetesVersion failed at case %s, want: nil, got: %s", name, c.want)
			}
			continue
		}
		if err.Error() != c.want {
			t.Errorf("validateKubernetesVersion failed at case %s, want: %s, got: %s", name, c.want, err.Error())
		}
	}
}

func TestValidateOpenYurtVersion(t *testing.T) {
	cases := map[string]struct {
		version string
		ignore  bool
		wantErr bool
	}{
		"valid": {
			"v0.6.0",
			false,
			false,
		},
		"unsupported": {
			"0.5.10",
			false,
			true,
		},
		"ignoreError": {
			"0.5.10",
			true,
			false,
		},
	}
	for name, c := range cases {
		err := validateOpenYurtVersion(c.version, c.ignore)
		if err == nil {
			if c.wantErr {
				t.Errorf("validateOpenYurtVersion failed at case %s, wantErr: %v, got: nil", name, c.wantErr)
			}
		}
	}
}

func TestPrepareConfigFile(t *testing.T) {
	var nodeImage = "kindest/node:v1.20.7@sha256:cbeaf907fc78ac97ce7b625e4bf0de16e3ea725daf6b04f930bd14c67c671ff9"
	cases := map[string]struct {
		clusterName       string
		nodesNum          int
		kindConfigPath    string
		disableDefaultCNI bool
		want              string
	}{
		"one node": {
			clusterName:    "case1",
			nodesNum:       1,
			kindConfigPath: "/tmp/prepareConfigFile.case1",
			want: `apiVersion: kind.x-k8s.io/v1alpha4
kind: Cluster
name: case1
networking:
  disableDefaultCNI: false
nodes:
  - role: control-plane
    image: kindest/node:v1.20.7@sha256:cbeaf907fc78ac97ce7b625e4bf0de16e3ea725daf6b04f930bd14c67c671ff9`,
		},
		"two nodes": {
			clusterName:    "case2",
			nodesNum:       2,
			kindConfigPath: "/tmp/prepareConfigFile.case2",
			want: `apiVersion: kind.x-k8s.io/v1alpha4
kind: Cluster
name: case2
networking:
  disableDefaultCNI: false
nodes:
  - role: control-plane
    image: kindest/node:v1.20.7@sha256:cbeaf907fc78ac97ce7b625e4bf0de16e3ea725daf6b04f930bd14c67c671ff9
  - role: worker
    image: kindest/node:v1.20.7@sha256:cbeaf907fc78ac97ce7b625e4bf0de16e3ea725daf6b04f930bd14c67c671ff9`,
		},
		"disable default cni": {
			clusterName:       "case3",
			nodesNum:          2,
			kindConfigPath:    "/tmp/prepareConfigFile.case3",
			disableDefaultCNI: true,
			want: `apiVersion: kind.x-k8s.io/v1alpha4
kind: Cluster
name: case3
networking:
  disableDefaultCNI: true
nodes:
  - role: control-plane
    image: kindest/node:v1.20.7@sha256:cbeaf907fc78ac97ce7b625e4bf0de16e3ea725daf6b04f930bd14c67c671ff9
  - role: worker
    image: kindest/node:v1.20.7@sha256:cbeaf907fc78ac97ce7b625e4bf0de16e3ea725daf6b04f930bd14c67c671ff9`,
		},
	}
	for name, c := range cases {
		initializer := newKindInitializer(
			os.Stdout,
			&initializerConfig{
				ClusterName:       c.clusterName,
				NodesNum:          c.nodesNum,
				KindConfigPath:    c.kindConfigPath,
				DisableDefaultCNI: c.disableDefaultCNI,
				NodeImage:         nodeImage,
			},
		)
		defer os.Remove(c.kindConfigPath)
		if err := initializer.prepareKindConfigFile(c.kindConfigPath); err != nil {
			t.Errorf("TestPrepareKindConfigFile failed at case %s for %s", name, err)
			continue
		}
		buf, err := os.ReadFile(c.kindConfigPath)
		if err != nil {
			t.Errorf("TestPrepareKindConfigFile failed at case %s, when reading file %s, %s", name, c.kindConfigPath, err)
			continue
		}
		if string(buf) != c.want {
			t.Errorf("TestPrepareKindConfigFile failed at case %s, want: %s, got: %s", name, c.want, string(buf))
		}
	}
}

func TestKindOptions_Validate(t *testing.T) {
	AllValidOpenYurtVersions = []string{"v0.6.0", "v0.7.0"}
	cases1 := []struct {
		nodeNum           int
		kubernetesVersion string
		openyurtVersion   string
		ignoreErr         bool
		wantErr           bool
		description       string
	}{
		{
			0,
			"v1.22",
			"v0.6.0",
			false,
			true,
			"the number of nodes must be greater than 0",
		},
		{
			1,
			"v1.10.1",
			"v0.6.0",
			false,
			true,
			"unsupported kubernetes version: v1.10.1",
		},
		{
			3,
			"v1.22",
			"v0.0.0",
			false,
			true,
			"v0.0.0 is not a valid openyurt version",
		},
	}

	cases2 := []struct {
		nodeNum           int
		kubernetesVersion string
		openyurtVersion   string
		ignoreErr         bool
		wantErr           bool
	}{
		{
			2,
			"v1.22",
			"v0.6.0",
			false,
			false,
		},
		{
			2,
			"v1.22",
			"v0.6.0",
			true,
			false,
		},
		{
			1,
			"v1.22",
			"v0.100.0",
			true,
			false,
		},
		{
			1,
			"v1.22",
			"v0.100.0",
			false,
			true,
		},
	}

	o := newKindOptions()
	for _, v := range cases1 {
		o.NodeNum = v.nodeNum
		o.KubernetesVersion = v.kubernetesVersion
		o.OpenYurtVersion = v.openyurtVersion
		o.IgnoreError = v.ignoreErr
		err := o.Validate()
		if (v.wantErr && err == nil) || (!v.wantErr && err != nil) {
			t.Errorf("failed validate")
		}
	}

	for _, v := range cases2 {
		o.NodeNum = v.nodeNum
		o.KubernetesVersion = v.kubernetesVersion
		o.OpenYurtVersion = v.openyurtVersion
		o.IgnoreError = v.ignoreErr
		err := o.Validate()
		if (v.wantErr && err == nil) || (!v.wantErr && err != nil) {
			t.Errorf("failed validate")
		}
	}
}

func TestGetNodeNamesOfKindCluster(t *testing.T) {
	cases := []struct {
		nodeNum              int
		clusterName          string
		wantControlPlaneNode string
		wantWorkerNodes      []string
	}{
		{
			nodeNum:              1,
			clusterName:          "openyurt",
			wantControlPlaneNode: "openyurt-control-plane",
			wantWorkerNodes: []string{
				"openyurt-worker",
			},
		},
		{
			nodeNum:              2,
			clusterName:          "openyurt",
			wantControlPlaneNode: "openyurt-control-plane",
			wantWorkerNodes: []string{
				"openyurt-worker",
			},
		},
		{
			nodeNum:              4,
			clusterName:          "kubernetes",
			wantControlPlaneNode: "kubernetes-control-plane",
			wantWorkerNodes: []string{
				"kubernetes-worker",
				"kubernetes-worker2",
				"kubernetes-worker3",
			},
		},
	}

	for _, v := range cases {
		controlPlaneNode, workerNodes := getNodeNamesOfKindCluster(v.clusterName, v.nodeNum)
		if controlPlaneNode != v.wantControlPlaneNode {
			t.Errorf("kind cluster nodes naming failed")
		}
		if len(workerNodes) != len(v.wantWorkerNodes) {
			t.Errorf("inconsistent number of worker nodes")
		}
		for i := 0; i < len(workerNodes); i++ {
			if v.wantWorkerNodes[i] != workerNodes[i] {
				t.Errorf("work node mismatch")
			}
		}
	}

}

func IsConsistent(initPoint1, initPoint2 *initializerConfig) bool {
	return reflect.DeepEqual(initPoint1, initPoint2)
}

func TestKindOptions_Config(t *testing.T) {
	case1 := newKindOptions()
	home := os.Getenv("HOME")
	wants := initializerConfig{
		CloudNodes:        []string{"openyurt-control-plane"},
		EdgeNodes:         []string{"openyurt-worker"},
		KindConfigPath:    "/tmp/kindconfig.yaml",
		KubeConfig:        home + "/.kube/config",
		NodesNum:          2,
		ClusterName:       "openyurt",
		KubernetesVersion: "v1.28",
		UseLocalImage:     false,
		YurtManagerImage:  "openyurt/yurt-manager:latest",
		NodeServantImage:  "openyurt/node-servant:latest",
		yurtIotDockImage:  "openyurt/yurt-iot-dock:latest",
		DisableDefaultCNI: false,
	}
	if !IsConsistent(&wants, case1.Config()) {
		t.Errorf("Failed to configure initializer")
	}
}

func TestInitializer_PrepareKindNodeImage(t *testing.T) {
	var fakeOut io.Writer
	cfg := newKindOptions().Config()

	initlzer := newKindInitializer(fakeOut, cfg)

	cases := []struct {
		command string
		want    interface{}
	}{
		{
			command: "kind v0.25.0 go1.17.7 darwin/arm64",
			want:    nil,
		},
		{
			command: "kind v0.26.0 go1.17.7 darwin/arm64",
			want:    "failed to get node image by kind version= v0.26.0 and kubernetes version= v1.28",
		},
	}

	for _, v := range cases {
		initlzer.operator.execCommand = func(string, ...string) *exec.Cmd {
			cmd := exec.Command("echo", v.command)
			return cmd
		}
		tmp := initlzer.prepareKindNodeImage()
		switch want := v.want.(type) {
		case nil:
			if tmp != nil {
				t.Errorf("failed prepare node image for kind pattern, want nil, got %v", tmp)
			}
		case string:
			if tmp == nil || tmp.Error() != want {
				t.Errorf("failed prepare node image for kind pattern, want %q, got %v", want, tmp)
			}
		default:
			t.Errorf("failed prepare node image for kind pattern")
		}
	}
}

func TestInitializer_PrepareImages(t *testing.T) {
	cases := map[string]struct {
		config       *initializerConfig
		wantCommands []string
	}{
		"load manager servant and iot images only": {
			config: &initializerConfig{
				ClusterName:      "openyurt",
				CloudNodes:       []string{"openyurt-control-plane"},
				EdgeNodes:        []string{"openyurt-worker"},
				UseLocalImage:    true,
				YurtManagerImage: "openyurt/yurt-manager:latest",
				NodeServantImage: "openyurt/node-servant:latest",
				yurtIotDockImage: "openyurt/yurt-iot-dock:latest",
			},
			wantCommands: []string{
				"kind load docker-image openyurt/yurt-manager:latest --name openyurt --nodes openyurt-control-plane",
				"kind load docker-image openyurt/node-servant:latest --name openyurt --nodes openyurt-control-plane",
				"kind load docker-image openyurt/yurt-iot-dock:latest --name openyurt --nodes openyurt-control-plane",
				"kind load docker-image openyurt/node-servant:latest --name openyurt --nodes openyurt-worker",
				"kind load docker-image openyurt/yurt-iot-dock:latest --name openyurt --nodes openyurt-worker",
			},
		},
		"skip loads when local images are disabled": {
			config: &initializerConfig{
				ClusterName:      "openyurt",
				CloudNodes:       []string{"openyurt-control-plane"},
				EdgeNodes:        []string{"openyurt-worker"},
				UseLocalImage:    false,
				YurtManagerImage: "openyurt/yurt-manager:latest",
				NodeServantImage: "openyurt/node-servant:latest",
				yurtIotDockImage: "openyurt/yurt-iot-dock:latest",
			},
		},
		"group multiple edge nodes into one load command": {
			config: &initializerConfig{
				ClusterName:      "openyurt",
				CloudNodes:       []string{"openyurt-control-plane"},
				EdgeNodes:        []string{"openyurt-worker", "openyurt-worker2", "openyurt-worker3"},
				UseLocalImage:    true,
				YurtManagerImage: "openyurt/yurt-manager:v0.1.0",
				NodeServantImage: "openyurt/node-servant:v0.6.0",
				yurtIotDockImage: "openyurt/yurt-iot-dock:v0.6.0",
			},
			wantCommands: []string{
				"kind load docker-image openyurt/yurt-manager:v0.1.0 --name openyurt --nodes openyurt-control-plane",
				"kind load docker-image openyurt/node-servant:v0.6.0 --name openyurt --nodes openyurt-control-plane",
				"kind load docker-image openyurt/yurt-iot-dock:v0.6.0 --name openyurt --nodes openyurt-control-plane",
				"kind load docker-image openyurt/node-servant:v0.6.0 --name openyurt --nodes openyurt-worker,openyurt-worker2,openyurt-worker3",
				"kind load docker-image openyurt/yurt-iot-dock:v0.6.0 --name openyurt --nodes openyurt-worker,openyurt-worker2,openyurt-worker3",
			},
		},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			initializer := newKindInitializer(io.Discard, tt.config)
			initializer.operator.kindCMDPath = "kind"

			var gotCommands []string
			initializer.operator.execCommand = func(name string, args ...string) *exec.Cmd {
				gotCommands = append(gotCommands, strings.Join(append([]string{name}, args...), " "))
				return exec.Command("true")
			}

			if err := initializer.prepareImages(); err != nil {
				t.Fatalf("prepareImages returned error: %v", err)
			}
			if !reflect.DeepEqual(gotCommands, tt.wantCommands) {
				t.Fatalf("unexpected image load commands:\nwant: %#v\ngot:  %#v", tt.wantCommands, gotCommands)
			}
		})
	}
}

func TestInitializer_ConfigureCoreDnsAddon(t *testing.T) {
	var fakeOut io.Writer
	initializer := newKindInitializer(fakeOut, newKindOptions().Config())

	case1 := struct {
		configObj     *corev1.ConfigMap
		serviceObj    *corev1.Service
		deploymentObj *v1.Deployment
		nodeObj       *corev1.Node
		want          interface{}
	}{
		configObj: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", Name: "coredns"},
			Data: map[string]string{
				"Corefile": "{ cd .. \n hosts /etc/edge/tunnels-nodes \n  kubernetes cluster.local",
			},
		},
		serviceObj: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   "kube-system",
				Name:        "kube-dns",
				Annotations: map[string]string{},
			},
		},
		deploymentObj: &v1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "kube-system",
				Name:      "coredns",
			},
			Spec: v1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{},
						Containers: []corev1.Container{
							{
								VolumeMounts: []corev1.VolumeMount{},
							},
						},
					},
				},
			},
		},
		nodeObj: &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "foo",
			},
		},
		want: nil,
	}

	initializer.kubeClient = clientsetfake.NewSimpleClientset(case1.configObj, case1.serviceObj, case1.deploymentObj, case1.nodeObj)
	err := initializer.configureCoreDnsAddon()
	if err != case1.want {
		t.Errorf("failed to configure core dns addon")
	}
}

func TestInitializer_ConfigureAddons(t *testing.T) {

	var replicasNum int32
	replicasNum = 3

	case1 := struct {
		coreDnsConfigObj *corev1.ConfigMap
		serviceObj       *corev1.Service
		podObj           *corev1.Pod
		deploymentObj    *v1.Deployment
		nodeObjs         []*corev1.Node
		want             interface{}
	}{
		coreDnsConfigObj: &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Namespace: "kube-system", Name: "coredns"},
			Data: map[string]string{
				"Corefile": "{ cd .. \n hosts /etc/edge/tunnels-nodes \n  kubernetes cluster.local {",
			},
		},
		serviceObj: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:   "kube-system",
				Name:        "kube-dns",
				Annotations: map[string]string{},
			},
		},

		podObj: &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "kube-system",
				Name:      "kube-proxy",
			},
		},

		deploymentObj: &v1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace:  "kube-system",
				Name:       "coredns",
				Generation: 3,
			},
			Spec: v1.DeploymentSpec{
				Replicas: &replicasNum,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{},
						Containers: []corev1.Container{
							{
								VolumeMounts: []corev1.VolumeMount{},
							},
						},
					},
				},
			},
			Status: v1.DeploymentStatus{
				ObservedGeneration: 3,
				AvailableReplicas:  3,
			},
		},
		nodeObjs: []*corev1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo1",
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo2",
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo3",
				},
			},
		},
		want: nil,
	}

	var fakeOut io.Writer
	initializer := newKindInitializer(fakeOut, newKindOptions().Config())
	client := clientsetfake.NewSimpleClientset(case1.coreDnsConfigObj, case1.serviceObj, case1.podObj, case1.deploymentObj)
	for i := range case1.nodeObjs {
		client.Tracker().Add(case1.nodeObjs[i])
	}
	initializer.kubeClient = client
	err := initializer.configureAddons()
	if err != case1.want {
		t.Errorf("failed to configure addons")
	}
}
