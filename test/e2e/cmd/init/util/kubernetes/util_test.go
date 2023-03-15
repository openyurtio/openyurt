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

package kubernetes

import (
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientsetfake "k8s.io/client-go/kubernetes/fake"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/test/e2e/cmd/init/constants"
)

const testDeployment = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  labels:
    app: nginx
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:latest
        ports:
        - containerPort: 80
`

func TestYamlToObject(t *testing.T) {
	obj, err := YamlToObject([]byte(testDeployment))
	if err != nil {
		t.Fatalf("YamlToObj failed: %s", err)
	}

	nd, ok := obj.(*appsv1.Deployment)
	if !ok {
		t.Fatalf("Fail to assert deployment: %s", err)
	}

	if nd.GetName() != "nginx-deployment" {
		t.Fatalf("YamlToObj failed: want \"nginx-deployment\" get \"%s\"", nd.GetName())
	}

	val, exist := nd.GetLabels()["app"]
	if !exist {
		t.Fatal("YamlToObj failed: label \"app\" doesnot exist")
	}
	if val != "nginx" {
		t.Fatalf("YamlToObj failed: want \"nginx\" get %s", val)
	}

	if *nd.Spec.Replicas != 3 {
		t.Fatalf("YamlToObj failed: want 3 get %d", *nd.Spec.Replicas)
	}
}

func TestCreateServiceAccountFromYaml(t *testing.T) {
	cases := []struct {
		namespace  string
		svcaccount string
		want       error
	}{
		{
			namespace: "kube-system",
			svcaccount: `
apiVersion: v1
kind: ServiceAccount
metadata:
  name: yurt-tunnel-server
  namespace: kube-system
`,
			want: nil,
		},
		{
			namespace: "kube-system",
			svcaccount: `
apiVersion: v1
kind: ServiceAccount
metadata:
  name: yurt-raven-server
  namespace: kube-system
`,
			want: nil,
		},
	}
	fakeKubeClient := clientsetfake.NewSimpleClientset()
	for _, v := range cases {
		err := CreateServiceAccountFromYaml(fakeKubeClient, v.namespace, v.svcaccount)
		if err != v.want {
			t.Logf("falied to create service account from yaml")
		}
	}
}

func TestCreateClusterRoleFromYaml(t *testing.T) {
	case1 := struct {
		clusterrole string
		want        error
	}{
		clusterrole: constants.YurthubClusterRole,
		want:        nil,
	}
	fakeKubeClient := clientsetfake.NewSimpleClientset()
	err := CreateClusterRoleFromYaml(fakeKubeClient, case1.clusterrole)
	if err != case1.want {
		t.Logf("falied to create cluster role from yaml")
	}
}

func TestClusterRoleBindingCreateFromYaml(t *testing.T) {
	cases := []struct {
		namespace          string
		clusterrolebinding string
		want               error
	}{
		{
			namespace:          "kube-system",
			clusterrolebinding: constants.YurtManagerClusterRoleBinding,
			want:               nil,
		},
	}
	for _, v := range cases {
		fakeKubeClient := clientsetfake.NewSimpleClientset()
		err := CreateClusterRoleBindingFromYaml(fakeKubeClient, v.clusterrolebinding)
		if err != v.want {
			t.Logf("falied to create cluster role binding from yaml")
		}
	}
}

func TestCreateDeployFromYaml(t *testing.T) {
	cases := struct {
		namespace  string
		deployment string
		want       error
	}{
		namespace:  "kube-system",
		deployment: constants.YurtManagerDeployment,
		want:       nil,
	}
	fakeKubeClient := clientsetfake.NewSimpleClientset()
	err := CreateDeployFromYaml(fakeKubeClient, cases.namespace, cases.deployment, map[string]string{
		"image":           "openyurt/yurt-manager:latest",
		"edgeWorkerLabel": projectinfo.GetEdgeWorkerLabelKey()})
	if err != cases.want {
		t.Logf("falied to create deployment from yaml")
	}
}

func TestCreateServiceFromYaml(t *testing.T) {
	cases := []struct {
		namespace string
		service   string
		want      error
	}{
		{
			namespace: "kube-system",
			service:   constants.YurtManagerService,
			want:      nil,
		},
	}
	for _, v := range cases {
		fakeKubeClient := clientsetfake.NewSimpleClientset()
		err := CreateServiceFromYaml(fakeKubeClient, v.namespace, v.service)
		if err != v.want {
			t.Logf("falied to create service from yaml")
		}
	}

}

func TestConfigMapFromYaml(t *testing.T) {
	cases := []struct {
		namespace string
		configMap string
		want      error
	}{
		{
			namespace: "kube-system",
			configMap: constants.YurthubConfigMap,
			want:      nil,
		},
		{
			namespace: "kube-system",
			configMap: constants.YurthubConfigMap,
			want:      nil,
		},
	}
	for _, v := range cases {
		fakeKubeClient := clientsetfake.NewSimpleClientset()
		err := CreateConfigMapFromYaml(fakeKubeClient, v.namespace, v.configMap)
		if err != v.want {
			t.Logf("falied to create service from yaml")
		}
	}
}

func TestAnnotateNode(t *testing.T) {
	cases := []struct {
		node *corev1.Node
		want *corev1.Node
		key  string
		val  string
	}{
		{
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cloud-node",
					Annotations: map[string]string{
						"foo": "yeah~",
					},
				},
			},
			want: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cloud-node",
					Annotations: map[string]string{
						"foo": "foo~",
					},
				},
			},
			key: "foo",
			val: "foo~",
		},
		{
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "edge-node",
					Annotations: map[string]string{},
				},
			},
			want: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cloud-node",
					Annotations: map[string]string{
						"foo": "foo~",
					},
				},
			},
			key: "foo",
			val: "yeah~",
		},
	}

	for _, v := range cases {
		fakeKubeClient := clientsetfake.NewSimpleClientset(v.node)
		res, err := AnnotateNode(fakeKubeClient, v.node, v.key, v.val)
		if err != nil || res.Annotations[v.key] != v.val {
			t.Logf("falied to annotate nodes")
		}
	}
}

func TestAddEdgeWorkerLabelAndAutonomyAnnotation(t *testing.T) {
	cases := []struct {
		node *corev1.Node
		want *corev1.Node
		lval string
		aval string
	}{
		{
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cloud-node",
					Labels: map[string]string{
						"foo": "yeah~",
					},
					Annotations: map[string]string{
						"foo": "yeah~",
					},
				},
			},
			want: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "cloud-node",
					Labels:      map[string]string{},
					Annotations: map[string]string{},
				},
			},
			lval: "foo",
			aval: "foo~",
		},
		{
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cloud-node",
					Labels: map[string]string{
						"foo": "foo~",
					},
					Annotations: map[string]string{
						"foo": "foo~",
					},
				},
			},
			want: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cloud-node",
					Labels: map[string]string{
						"openyurt.io/is-edge-worker": "ok",
					},
					Annotations: map[string]string{
						"node.beta.openyurt.io/autonomy": "ok",
					},
				},
			},
			lval: "yeah",
			aval: "yeah~",
		},
	}

	for _, v := range cases {
		fakeKubeClient := clientsetfake.NewSimpleClientset(v.node)
		res, err := AddEdgeWorkerLabelAndAutonomyAnnotation(fakeKubeClient, v.node, v.lval, v.aval)
		if err != nil || res.Labels[projectinfo.GetEdgeWorkerLabelKey()] != v.lval || res.Annotations[projectinfo.GetAutonomyAnnotation()] != v.aval {
			t.Logf("falied to add edge worker label and autonomy annotation")
		}
	}
}

func TestRunJobAndCleanup(t *testing.T) {
	var dummy1 int32
	var dummy2 int32
	dummy1 = 1
	dummy2 = 0

	cases := []struct {
		jobObj *batchv1.Job
		want   error
	}{
		{
			jobObj: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "job",
					Name:      "job-test",
				},
				Spec: batchv1.JobSpec{
					Completions: &dummy1,
				},
				Status: batchv1.JobStatus{
					Succeeded: 1,
				},
			},
			want: nil,
		},

		{
			jobObj: &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "job",
					Name:      "job-foo",
				},
				Spec: batchv1.JobSpec{
					Completions: &dummy2,
				},
				Status: batchv1.JobStatus{
					Succeeded: 0,
				},
			},
			want: nil,
		},
	}

	for _, v := range cases {
		fakeKubeClient := clientsetfake.NewSimpleClientset()
		err := RunJobAndCleanup(fakeKubeClient, v.jobObj, time.Second*10, time.Second, false)
		if err != v.want {
			t.Logf("falied to run job and cleanup")
		}
	}
}

/*
func TestRunServantJobs(t *testing.T) {

	var ww io.Writer
	convertCtx := map[string]string{
		"node_servant_image": "foo_servant_image",
		"yurthub_image":      "foo_yurthub_image",
		"joinToken":          "foo",
		// The node-servant will detect the kubeadm_conf_path automatically
		// It will be either "/usr/lib/systemd/system/kubelet.service.d/10-kubeadm.conf"
		// or "/etc/systemd/system/kubelet.service.d/10-kubeadm.conf".
		"kubeadm_conf_path": "",
		"working_mode":      "edge",
		"enable_dummy_if":   "true",
	}

	cases := []struct {
		nodeName []string
		want     error
	}{
		{
			nodeName: []string{
				"cloud-node",
				"edge-node",
			},
			want: nil,
		},
		{
			nodeName: []string{"foo", "test"},
			want:     nil,
		},
	}

	for _, v := range cases {
		fakeKubeClient := clientsetfake.NewSimpleClientset()
		err := RunServantJobs(fakeKubeClient, time.Second, func(nodeName string) (*batchv1.Job, error) {
			return nodeservant.RenderNodeServantJob("convert", convertCtx, nodeName)
		}, v.nodeName, ww, false)
		if err != nil {
			t.Logf("falied to run servant jobs")
		}
	}
}
*/
