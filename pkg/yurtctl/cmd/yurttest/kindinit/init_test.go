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

package kindinit

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"
)

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
			"unsupported kubernetes version: v1.23.122",
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
		want    string
	}{
		"valid": {
			"v0.6.0",
			false,
			"",
		},
		"unsupported": {
			"0.5.10",
			false,
			fmt.Sprintf("0.5.10 is not a valid openyurt version, all valid versions are %s. If you know what you're doing, you can set --ignore-error",
				strings.Join(validOpenYurtVersions, ",")),
		},
		"ignoreError": {
			"0.5.10",
			true,
			"",
		},
	}
	for name, c := range cases {
		err := validateOpenYurtVersion(c.version, c.ignore)
		if err == nil {
			if c.want != "" {
				t.Errorf("validateOpenYurtVersion failed at case %s, want: %s, got: nil", name, c.want)
			}
			continue
		}
		if err.Error() != c.want {
			t.Errorf("validateOpenYurtVersion failed at case %s, want: %s, got: %s", name, c.want, err.Error())
		}
	}
}

func TestPrepareConfigFile(t *testing.T) {
	var nodeImage = "kindest/node:v1.20.7@sha256:cbeaf907fc78ac97ce7b625e4bf0de16e3ea725daf6b04f930bd14c67c671ff9"
	cases := map[string]struct {
		clusterName    string
		nodesNum       int
		kindConfigPath string
		want           string
	}{
		"one node": {
			clusterName:    "case1",
			nodesNum:       1,
			kindConfigPath: "/tmp/prepareConfigFile.case1",
			want: `apiVersion: kind.x-k8s.io/v1alpha4
kind: Cluster
name: case1
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
nodes:
  - role: control-plane
    image: kindest/node:v1.20.7@sha256:cbeaf907fc78ac97ce7b625e4bf0de16e3ea725daf6b04f930bd14c67c671ff9
  - role: worker
    image: kindest/node:v1.20.7@sha256:cbeaf907fc78ac97ce7b625e4bf0de16e3ea725daf6b04f930bd14c67c671ff9`,
		},
	}
	for name, c := range cases {
		initializer := newKindInitializer(
			&initializerConfig{
				ClusterName:    c.clusterName,
				NodesNum:       c.nodesNum,
				KindConfigPath: c.kindConfigPath,
				NodeImage:      nodeImage,
			},
		)
		defer os.Remove(c.kindConfigPath)
		if err := initializer.prepareKindConfigFile(c.kindConfigPath); err != nil {
			t.Errorf("TestPrepareKindConfigFile failed at case %s for %s", name, err)
			continue
		}
		buf, err := ioutil.ReadFile(c.kindConfigPath)
		if err != nil {
			t.Errorf("TestPrepareKindConfigFile failed at case %s, when reading file %s, %s", name, c.kindConfigPath, err)
			continue
		}
		if string(buf) != c.want {
			t.Errorf("TestPrepareKindConfigFile failed at case %s, want: %s, got: %s", name, c.want, string(buf))
		}
	}
}
