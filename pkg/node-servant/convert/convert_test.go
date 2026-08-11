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
	"context"
	"errors"
	"reflect"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	yurthubutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/yurthub"
)

func newFakeClusterRoleBinding() *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: multiplexerClusterRoleBindingName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:     "Group",
				Name:     "openyurt:multiplexer",
				APIGroup: "rbac.authorization.k8s.io",
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     "yurt-hub-multiplexer",
			APIGroup: "rbac.authorization.k8s.io",
		},
	}
}

func TestNodeConverterDo(t *testing.T) {
	oldGetAPIServerAddressFunc := getAPIServerAddressFunc
	oldInstallYurthubFunc := installYurthubFunc
	oldRedirectKubeletFunc := redirectKubeletFunc
	oldRestartContainersFunc := restartContainersFunc
	oldGetKubeClientFunc := getKubeClientFunc
	defer func() {
		getAPIServerAddressFunc = oldGetAPIServerAddressFunc
		installYurthubFunc = oldInstallYurthubFunc
		redirectKubeletFunc = oldRedirectKubeletFunc
		restartContainersFunc = oldRestartContainersFunc
		getKubeClientFunc = oldGetKubeClientFunc
	}()

	fakeClient := fake.NewSimpleClientset(newFakeClusterRoleBinding())
	getKubeClientFunc = func(kubeadmConfPaths []string) (kubernetes.Interface, error) {
		return fakeClient, nil
	}

	converter := &nodeConverter{
		Config: Config{
			kubeadmConfPaths: []string{"/etc/systemd/system/kubelet.service.d/10-kubeadm.conf"},
			nodeName:         "node-a",
			nodePoolName:     "pool-a",
			openyurtDir:      "/var/lib/openyurt",
		},
	}

	var gotCfg *yurthubutil.YurthubHostConfig
	var calls []string
	getAPIServerAddressFunc = func(paths []string) (string, error) {
		if !reflect.DeepEqual(paths, converter.kubeadmConfPaths) {
			t.Fatalf("unexpected kubeadm conf paths: %v", paths)
		}
		calls = append(calls, "discover-apiserver")
		return "10.0.0.1:6443", nil
	}
	installYurthubFunc = func(cfg *yurthubutil.YurthubHostConfig) error {
		gotCfg = cfg
		calls = append(calls, "install-yurthub")
		return nil
	}
	redirectKubeletFunc = func(openyurtDir string) error {
		if openyurtDir != converter.openyurtDir {
			t.Fatalf("unexpected openyurt dir %q", openyurtDir)
		}
		calls = append(calls, "redirect-kubelet")
		return nil
	}
	restartContainersFunc = func(nodeName string) error {
		if nodeName != converter.nodeName {
			t.Fatalf("unexpected node name %q", nodeName)
		}
		calls = append(calls, "restart-containers")
		return nil
	}

	if err := converter.Do(); err != nil {
		t.Fatalf("Do() returned error: %v", err)
	}

	wantCalls := []string{"discover-apiserver", "install-yurthub", "redirect-kubelet", "restart-containers"}
	if !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("unexpected call order, got=%v, want=%v", calls, wantCalls)
	}

	wantCfg := &yurthubutil.YurthubHostConfig{
		BootstrapMode: bootstrapModeKubeletCertificate,
		Namespace:     "kube-system",
		NodeName:      "node-a",
		NodePoolName:  "pool-a",
		ServerAddr:    "10.0.0.1:6443",
		WorkingMode:   "edge",
	}
	if !reflect.DeepEqual(gotCfg, wantCfg) {
		t.Fatalf("unexpected yurthub host config, got=%#v, want=%#v", gotCfg, wantCfg)
	}

	// Verify RBAC update
	crb, err := fakeClient.RbacV1().ClusterRoleBindings().Get(context.TODO(), multiplexerClusterRoleBindingName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get CRB: %v", err)
	}
	found := false
	for _, sub := range crb.Subjects {
		if sub.Kind == "User" && sub.Name == "system:node:node-a" && sub.APIGroup == "rbac.authorization.k8s.io" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected subject User system:node:node-a in CRB subjects: %v", crb.Subjects)
	}
}

func TestEnsureNodeInMultiplexerRBACIdempotency(t *testing.T) {
	oldGetKubeClientFunc := getKubeClientFunc
	defer func() {
		getKubeClientFunc = oldGetKubeClientFunc
	}()

	fakeClient := fake.NewSimpleClientset(newFakeClusterRoleBinding())
	getKubeClientFunc = func(kubeadmConfPaths []string) (kubernetes.Interface, error) {
		return fakeClient, nil
	}

	converter := &nodeConverter{
		Config: Config{
			nodeName: "node-a",
		},
	}

	// Run twice to verify idempotency
	if err := converter.ensureNodeInMultiplexerRBAC(); err != nil {
		t.Fatalf("first ensureNodeInMultiplexerRBAC() failed: %v", err)
	}
	if err := converter.ensureNodeInMultiplexerRBAC(); err != nil {
		t.Fatalf("second ensureNodeInMultiplexerRBAC() failed: %v", err)
	}

	crb, err := fakeClient.RbacV1().ClusterRoleBindings().Get(context.TODO(), multiplexerClusterRoleBindingName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get CRB: %v", err)
	}

	userCount := 0
	for _, sub := range crb.Subjects {
		if sub.Kind == "User" && sub.Name == "system:node:node-a" {
			userCount++
		}
	}
	if userCount != 1 {
		t.Fatalf("expected exactly 1 User system:node:node-a subject, got %d", userCount)
	}
}

func TestEnsureNodeInMultiplexerRBACMissingCRB(t *testing.T) {
	oldGetKubeClientFunc := getKubeClientFunc
	defer func() {
		getKubeClientFunc = oldGetKubeClientFunc
	}()

	fakeClient := fake.NewSimpleClientset()
	getKubeClientFunc = func(kubeadmConfPaths []string) (kubernetes.Interface, error) {
		return fakeClient, nil
	}

	converter := &nodeConverter{
		Config: Config{
			nodeName: "node-a",
		},
	}

	err := converter.ensureNodeInMultiplexerRBAC()
	if err == nil {
		t.Fatal("expected error when ClusterRoleBinding is missing, got nil")
	}
}

func TestNodeConverterStopsWhenInstallFails(t *testing.T) {
	oldGetAPIServerAddressFunc := getAPIServerAddressFunc
	oldInstallYurthubFunc := installYurthubFunc
	oldRedirectKubeletFunc := redirectKubeletFunc
	oldRestartContainersFunc := restartContainersFunc
	oldGetKubeClientFunc := getKubeClientFunc
	defer func() {
		getAPIServerAddressFunc = oldGetAPIServerAddressFunc
		installYurthubFunc = oldInstallYurthubFunc
		redirectKubeletFunc = oldRedirectKubeletFunc
		restartContainersFunc = oldRestartContainersFunc
		getKubeClientFunc = oldGetKubeClientFunc
	}()

	fakeClient := fake.NewSimpleClientset(newFakeClusterRoleBinding())
	getKubeClientFunc = func(kubeadmConfPaths []string) (kubernetes.Interface, error) {
		return fakeClient, nil
	}

	converter := &nodeConverter{
		Config: Config{
			kubeadmConfPaths: []string{"/etc/systemd/system/kubelet.service.d/10-kubeadm.conf"},
			openyurtDir:      "/var/lib/openyurt",
		},
	}

	getAPIServerAddressFunc = func(paths []string) (string, error) {
		return "10.0.0.1:6443", nil
	}

	expectedErr := errors.New("install failed")
	redirectCalled := false
	restartCalled := false
	installYurthubFunc = func(cfg *yurthubutil.YurthubHostConfig) error {
		return expectedErr
	}
	redirectKubeletFunc = func(openyurtDir string) error {
		redirectCalled = true
		return nil
	}
	restartContainersFunc = func(nodeName string) error {
		restartCalled = true
		return nil
	}

	err := converter.Do()
	if !errors.Is(err, expectedErr) {
		t.Fatalf("unexpected error, got=%v, want=%v", err, expectedErr)
	}
	if redirectCalled {
		t.Fatal("expected kubelet redirection to be skipped after install failure")
	}
	if restartCalled {
		t.Fatal("expected container restart to be skipped after install failure")
	}
}
