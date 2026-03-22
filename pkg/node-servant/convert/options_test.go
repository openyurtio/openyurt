/*
Copyright 2026 The OpenYurt Authors.

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

package convert

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/spf13/pflag"

	enutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
)

func TestOptionsComplete(t *testing.T) {
	oldGetNodeNameFunc := getNodeNameFunc
	defer func() {
		getNodeNameFunc = oldGetNodeNameFunc
	}()

	var visitedPaths []string
	getNodeNameFunc = func(path string) (string, error) {
		visitedPaths = append(visitedPaths, path)
		if path == "/etc/kubernetes/kubelet.conf" {
			return "", fmt.Errorf("not found")
		}
		if path == "/usr/lib/systemd/system/kubelet.service.d/10-kubeadm.conf" {
			return "node-a", nil
		}
		return "", nil
	}

	t.Setenv(enutil.NODE_NAME, "")
	t.Setenv("OPENYURT_DIR", "/host/openyurt")

	o := NewConvertOptions()
	flags := pflag.NewFlagSet("convert", pflag.ContinueOnError)
	o.AddFlags(flags)
	if err := flags.Set("kubeadm-conf-path", "/etc/kubernetes/kubelet.conf, /usr/lib/systemd/system/kubelet.service.d/10-kubeadm.conf"); err != nil {
		t.Fatalf("set kubeadm-conf-path: %v", err)
	}

	if err := o.Complete(flags); err != nil {
		t.Fatalf("Complete() returned error: %v", err)
	}

	wantVisitedPaths := []string{
		"/etc/kubernetes/kubelet.conf",
		"/usr/lib/systemd/system/kubelet.service.d/10-kubeadm.conf",
	}
	if !reflect.DeepEqual(visitedPaths, wantVisitedPaths) {
		t.Fatalf("unexpected visited paths, got=%v, want=%v", visitedPaths, wantVisitedPaths)
	}
	if o.nodeName != "node-a" {
		t.Fatalf("unexpected node name %q", o.nodeName)
	}
	if o.openyurtDir != "/host/openyurt" {
		t.Fatalf("unexpected openyurt dir %q", o.openyurtDir)
	}
}

func TestOptionsValidate(t *testing.T) {
	o := &Options{
		namespace:    "kube-system",
		nodeName:     "node-a",
		nodePoolName: "pool-a",
		workingMode:  "edge",
	}

	if err := o.Validate(); err != nil {
		t.Fatalf("Validate() returned error: %v", err)
	}
}
