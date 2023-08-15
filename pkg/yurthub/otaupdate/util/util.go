/*
Copyright 2023 The OpenYurt Authors.

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

package util

import (
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	runtimescheme "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"

	yurtutil "github.com/openyurtio/openyurt/pkg/util"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonpodupdater"
)

// Derived from kubelet encodePods
func EncodePods(podList *corev1.PodList) (data []byte, err error) {
	codec := scheme.Codecs.LegacyCodec(runtimescheme.GroupVersion{Group: corev1.GroupName, Version: "v1"})
	return runtime.Encode(codec, podList)
}

// WriteErr writes the http status and the error string on the response
func WriteErr(w http.ResponseWriter, errReason string, httpStatus int) {
	w.WriteHeader(httpStatus)
	n := len([]byte(errReason))
	nw, e := w.Write([]byte(errReason))
	if e != nil || nw != n {
		klog.Errorf("Write resp for request, expect %d bytes but write %d bytes with error, %v", n, nw, e)
	}
}

// Derived from kubelet writeJSONResponse
func WriteJSONResponse(w http.ResponseWriter, data []byte) {
	if data == nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Header().Set(yurtutil.HttpHeaderContentType, yurtutil.HttpContentTypeJson)
	w.WriteHeader(http.StatusOK)
	n, err := w.Write(data)
	if err != nil || n != len(data) {
		klog.Errorf("Write resp for request, expect %d bytes but write %d bytes with error, %v", len(data), n, err)
	}
}

func NewPod(podName, kind string) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: metav1.NamespaceDefault,
			OwnerReferences: []metav1.OwnerReference{
				{Kind: kind},
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{},
		},
	}
	return pod
}

func NewPodWithCondition(podName, kind string, ready corev1.ConditionStatus) *corev1.Pod {
	pod := NewPod(podName, kind)
	SetPodUpgradeCondition(pod, ready)

	return pod
}

func SetPodUpgradeCondition(pod *corev1.Pod, ready corev1.ConditionStatus) {
	cond := corev1.PodCondition{
		Type:   daemonpodupdater.PodNeedUpgrade,
		Status: ready,
	}
	pod.Status.Conditions = append(pod.Status.Conditions, cond)
}

func RemoveNodeNameFromStaticPod(podname, nodename string) (bool, string) {
	if !strings.HasSuffix(podname, "-"+nodename) {
		return false, ""
	}
	return true, strings.Split(podname, "-"+nodename)[0]
}
