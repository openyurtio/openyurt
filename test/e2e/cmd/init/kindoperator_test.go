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
limitations under the License.z
*/

package init

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const (
	kindVersionOut = "kind v0.11.1 go1.17.7 linux/amd64"
	goVersionOut   = "go version go1.17.7 linux/amd64"
)

var operator = NewKindOperator("kind", "")

func execCommand(testFunc string, env map[string]string, name string, args ...string) *exec.Cmd {
	cs := []string{fmt.Sprintf("-test.run=%s", testFunc), "--", name}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)

	passenv := []string{}
	for k, v := range env {
		passenv = append(passenv, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = passenv
	return cmd
}

func TestKindVersion(t *testing.T) {
	fakeExecCommand := func(name string, args ...string) *exec.Cmd {
		return execCommand("TestKindVersionStub", map[string]string{
			"TEST_KIND_VERSION": "1",
		}, name, args...)
	}
	operator.SetExecCommand(fakeExecCommand)
	defer operator.SetExecCommand(fakeExecCommand)

	cases := map[string]struct {
		kindPath string
		want     string
	}{
		"normal case": {
			"/go/bin/kind",
			"v0.11.1",
		},
	}

	for name, c := range cases {
		kindVersion, _ := operator.KindVersion()
		if kindVersion != c.want {
			t.Errorf("failed parse kind version at case %s, want: %s, got: %s", name, c.want, kindVersion)
		}
	}
}

func TestKindVersionStub(t *testing.T) {
	if os.Getenv("TEST_KIND_VERSION") != "1" {
		return
	}

	fmt.Fprint(os.Stdout, kindVersionOut)
	os.Exit(0)
}

func TestGoMinorVersion(t *testing.T) {
	fakeExecCommand := func(name string, args ...string) *exec.Cmd {
		return execCommand("TestGoMinorVersionStub", map[string]string{
			"TEST_GO_MINOR_VERSION": "1",
		}, name, args...)
	}
	operator.SetExecCommand(fakeExecCommand)
	defer operator.SetExecCommand(exec.Command)

	cases := map[string]struct {
		want int
	}{
		"normal case": {
			17,
		},
	}

	for name, c := range cases {
		goMinorVer, _ := operator.goMinorVersion()
		if goMinorVer != c.want {
			t.Errorf("failed parse go version at case %s, want: %d, got: %d", name, c.want, goMinorVer)
		}
	}
}

func TestGoMinorVersionStub(t *testing.T) {
	if os.Getenv("TEST_GO_MINOR_VERSION") != "1" {
		return
	}
	fmt.Fprint(os.Stdout, goVersionOut)
	os.Exit(0)
}

func TestGetInstallCmd(t *testing.T) {
	kindVersion := "v0.11.1"
	cases := map[string]struct {
		goMinorVersion int
		want           string
	}{
		"gover > 1.17": {
			goMinorVersion: 17,
			want:           "go install sigs.k8s.io/kind@v0.11.1",
		},
		"1.13 <= gover <= 1.17": {
			goMinorVersion: 16,
			want:           "GO111MODULE=on go install sigs.k8s.io/kind@v0.11.1",
		},
	}

	for name, c := range cases {
		cmd := operator.getInstallCmd(c.goMinorVersion, kindVersion)
		if cmd != c.want {
			t.Errorf("unexpected kind install command at case %s, want: %s, got: %s", name, c.want, kindInstallCmd)
		}
	}
}

func TestKindOperator_KindLoadDockerImage(t *testing.T) {
	cases := map[string]struct {
		clusterName string
		image       string
		nodeNames   []string
		want        string
	}{
		"multiNodes": {
			clusterName: "openyurt",
			image:       "openyurt/yurthub:latest",
			nodeNames: []string{
				"control-plane",
				"worker",
			},
			want: "kind load docker-image openyurt/yurthub:latest --name openyurt --nodes control-plane,worker",
		},
	}
	for caseName, c := range cases {
		fakeExecCommand := func(name string, args ...string) *exec.Cmd {
			return execCommand("TestKindLoadDockerImageStub", map[string]string{
				"TEST_KIND_LOAD_DOCKER_IMAGE": "1",
				"CASE_NAME":                   caseName,
				"WANT":                        c.want,
			}, name, args...)
		}
		operator.SetExecCommand(fakeExecCommand)
		if err := operator.KindLoadDockerImage(os.Stdout, c.clusterName, c.image, c.nodeNames); err != nil {
			t.Errorf("unexpected cmd when loading docker images to kind at case %s, want: %s", caseName, c.want)
		}
	}
	operator.SetExecCommand(exec.Command)
}

func TestKindLoadDockerImageStub(t *testing.T) {
	if os.Getenv("TEST_KIND_LOAD_DOCKER_IMAGE") != "1" {
		return
	}
	_, want := os.Getenv("CASE_NAME"), os.Getenv("WANT")
	var cmd string
	for i, arg := range os.Args {
		if arg == "--" {
			cmd = strings.Join(os.Args[i+1:], " ")
			break
		}
	}

	if want != cmd {
		os.Exit(1)
	}
	os.Exit(0)
}

func fakeExeCommand(string, ...string) *exec.Cmd {
	cmd := exec.Command("echo", "ok")
	return cmd
}

func TestGetGoBinPath(t *testing.T) {
	gopath, err := getGoBinPath()
	if gopath == "" || err != nil {
		t.Errorf("failed to get GOPATH")
	}
}

func TestFindKindPath(t *testing.T) {
	home := os.Getenv("HOME")
	if home != "" {
		cases := home + "/go/bin/kind"
		kindPath, err := findKindPath()
		if err != nil && kindPath != cases {
			fmt.Println("failed to find kind")
		}
	}
}

func TestKindOperator_KindCreateClusterWithConfig(t *testing.T) {
	cases := []struct {
		configPath string
		err        interface{}
	}{
		{
			" ",
			nil,
		},
		{
			"/tmp/config",
			nil,
		},
	}
	var fakeOut io.Writer
	kindOperator := NewKindOperator("", "")
	kindOperator.execCommand = fakeExeCommand
	for _, v := range cases {
		err := kindOperator.KindCreateClusterWithConfig(fakeOut, v.configPath)
		if err != v.err {
			t.Errorf("falied create cluster with configure using kind")
		}
	}

}
