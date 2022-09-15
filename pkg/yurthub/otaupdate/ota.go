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
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

// TODO(hxc): should use pkg/controller/daemonpodupdater.PodNeedUpgrade
const (
	PodNeedUpgrade corev1.PodConditionType = "PodNeedUpgrade"
)

// GetPods return pod list
func GetPods(clientset kubernetes.Interface, nodeName string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		podList, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
			FieldSelector: "spec.nodeName=" + nodeName,
		})
		if err != nil {
			klog.Errorf("Get pod list failed, %v", err)
			returnErr(fmt.Errorf("Get pods list failed"), w, http.StatusInternalServerError)
			return
		}
		klog.V(5).Infof("Got pod list: %v", podList)

		// Successfully get pod list
		w.Header().Set("content-type", "text/json")
		data, err := json.Marshal(podList)
		if err != nil {
			klog.Errorf("Marshal pod list failed: %v", err.Error())
			returnErr(fmt.Errorf("Get pod list failed: data transfer to json format failed."), w, http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		n, err := w.Write(data)
		if err != nil || n != len(data) {
			klog.Errorf("Write resp for request, expect %d bytes but write %d bytes with error, %v", len(data), n, err)
		}
	})
}

// UpdatePod update a specifc pod(namespace/podname) to the latest version
func UpdatePod(clientset kubernetes.Interface) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := mux.Vars(r)
		namespace := params["ns"]
		podName := params["podname"]

		err, ok := applyUpdate(clientset, namespace, podName)
		if err != nil {
			returnErr(fmt.Errorf("Apply update failed"), w, http.StatusInternalServerError)
			return
		}
		if !ok {
			returnErr(fmt.Errorf("Pod is not-updatable"), w, http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}

// applyUpdate execute pod update process by deleting pod under OnDelete update strategy
func applyUpdate(clientset kubernetes.Interface, namespace, podName string) (error, bool) {
	pod, err := clientset.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Get pod %v/%v failed, %v", namespace, podName, err)
		return err, false
	}

	// Pod will not be updated while it's being deleted
	if pod.DeletionTimestamp != nil {
		klog.Infof("Pod %v/%v is deleting, can not update", namespace, podName)
		return nil, false
	}

	// Pod will not be updated without pod condition PodNeedUpgrade=true
	if !IsPodUpdatable(pod) {
		klog.Infof("Pod: %v/%v is not updatable", namespace, podName)
		return nil, false
	}

	klog.V(5).Infof("Pod: %v/%v is updatable", namespace, podName)
	err = clientset.CoreV1().Pods(namespace).Delete(context.TODO(), podName, metav1.DeleteOptions{})
	if err != nil {
		klog.Errorf("Update pod %v/%v failed when delete, %v", namespace, podName, err)
		return err, false
	}

	klog.Infof("Update pod: %v/%v success", namespace, podName)
	return nil, true
}

// returnErr write the given error to response and set response header to the given error type
func returnErr(err error, w http.ResponseWriter, errType int) {
	w.WriteHeader(errType)
	n := len([]byte(err.Error()))
	nw, e := w.Write([]byte(err.Error()))
	if e != nil || nw != n {
		klog.Errorf("write resp for request, expect %d bytes but write %d bytes with error, %v", n, nw, e)
	}
}

// TODO(hxc): should use pkg/controller/daemonpodupdater.IsPodUpdatable()
// IsPodUpdatable returns true if a pod is updatable; false otherwise.
func IsPodUpdatable(pod *corev1.Pod) bool {
	if &pod.Status == nil || len(pod.Status.Conditions) == 0 {
		return false
	}

	for i := range pod.Status.Conditions {
		if pod.Status.Conditions[i].Type == PodNeedUpgrade && pod.Status.Conditions[i].Status == "true" {
			return true
		}
	}

	return false
}
