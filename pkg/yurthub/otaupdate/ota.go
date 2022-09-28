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

package otaupdate

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gorilla/mux"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	runtimescheme "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/controller/daemonpodupdater"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
)

// GetPods return pod list
func GetPods(clientset kubernetes.Interface, nodeName string, checker healthchecker.HealthChecker,
	servers []*url.URL) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Pre-check if edge yurthub node is connected to the cloud
		if !isEdgeCloudConnected(checker, servers) {
			klog.Errorf("Get pod list is not allowed when edge is disconnected to cloud")
			WriteErr(w, "Get pod list is not allowed when edge is disconnected to cloud", http.StatusForbidden)
			return
		}

		podList, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
			FieldSelector: "spec.nodeName=" + nodeName,
		})
		if err != nil {
			klog.Errorf("Get pod list failed, %v", err)
			WriteErr(w, "Get pod list failed", http.StatusInternalServerError)
			return
		}
		klog.V(5).Infof("Got pod list: %v", podList)

		// Successfully get pod list, response 200
		data, err := encodePodList(podList)
		if err != nil {
			klog.Errorf("Encode pod list failed, %v", err)
			WriteErr(w, "Encode pod list failed", http.StatusInternalServerError)
		}
		WriteJSONResponse(w, data)
	})
}

// UpdatePod update a specifc pod(namespace/podname) to the latest version
func UpdatePod(clientset kubernetes.Interface, nodeName string, checker healthchecker.HealthChecker,
	servers []*url.URL) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Pre-check if edge yurthub node is connected to the cloud
		if !isEdgeCloudConnected(checker, servers) {
			klog.Errorf("Apply update is not allowed when edge is disconnected to cloud")
			WriteErr(w, "Apply update is not allowed when edge is disconnected to cloud", http.StatusForbidden)
			return
		}

		params := mux.Vars(r)
		namespace := params["ns"]
		podName := params["podname"]

		err, ok := applyUpdate(clientset, namespace, podName, nodeName)
		// Pod update failed with error
		if err != nil {
			WriteErr(w, "Apply update failed", http.StatusInternalServerError)
			return
		}
		// Pod update is not allowed
		if !ok {
			WriteErr(w, "Pod is not-updatable", http.StatusForbidden)
			return
		}

		// Successfully apply update, response 200
		WriteJSONResponse(w, []byte(fmt.Sprintf("Start updating pod %q/%q", namespace, podName)))
	})
}

// applyUpdate execute pod update process by deleting pod under OnDelete update strategy
func applyUpdate(clientset kubernetes.Interface, namespace, podName, nodeName string) (error, bool) {
	pod, err := clientset.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Get pod %v/%v failed, %v", namespace, podName, err)
		return err, false
	}

	// Pod will not be updated when it's being deleted
	if pod.DeletionTimestamp != nil {
		klog.Infof("Pod %v/%v is deleting, can not be updated", namespace, podName)
		return nil, false
	}

	// Pod will not be updated when it's not running on the current node
	if pod.Spec.NodeName != nodeName {
		klog.Infof("Pod: %v/%v is running on %v, can not be updated", namespace, podName, pod.Spec.NodeName)
		return nil, false
	}

	// Pod will not be updated without pod condition PodNeedUpgrade=true
	if !daemonpodupdater.IsPodUpdatable(pod) {
		klog.Infof("Pod: %v/%v is not updatable", namespace, podName)
		return nil, false
	}

	klog.V(5).Infof("Pod: %v/%v is updatable", namespace, podName)
	err = clientset.CoreV1().Pods(namespace).Delete(context.TODO(), podName, metav1.DeleteOptions{})
	if err != nil {
		klog.Errorf("Update pod %v/%v failed when delete, %v", namespace, podName, err)
		return err, false
	}

	klog.Infof("Start updating pod: %q/%q", namespace, podName)
	return nil, true
}

// encodePodList returns the encoded PodList
func encodePodList(podList *corev1.PodList) ([]byte, error) {
	codec := scheme.Codecs.LegacyCodec(runtimescheme.GroupVersion{Group: corev1.GroupName, Version: "v1"})
	return runtime.Encode(codec, podList)
}

// isEdgeCloudConnected will check if edge is disconnected to cloud. If there is any remote server is healthy, it is
// regarded as connected. Otherwise, it is regarded as disconnected and return false.
func isEdgeCloudConnected(checker healthchecker.HealthChecker, remoteServers []*url.URL) bool {
	for _, server := range remoteServers {
		if checker.IsHealthy(server) {
			return true
		}
	}
	return false
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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	n, err := w.Write(data)
	if err != nil || n != len(data) {
		klog.Errorf("Write resp for request, expect %d bytes but write %d bytes with error, %v", len(data), n, err)
	}
}
