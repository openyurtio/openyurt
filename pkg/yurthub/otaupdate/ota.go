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
	"strconv"

	"github.com/gorilla/mux"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	client "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

const (
	PodUpdatableAnnotation = "apps.openyurt.io/pod-updatable"
)

type PodStatus struct {
	Namespace string
	PodName   string
	// TODO: whether need to display
	// OldImgs    []string
	// NewImgs    []string
	Updatable bool
}

// GetPods return all daemonset pods' update information by PodStatus
func GetPods(clientset kubernetes.Interface) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		podsStatus, err := getPodUpdateStatus(clientset)
		if err != nil {
			klog.Errorf("Get pods updatable status failed, %v", err)
			returnErr(fmt.Errorf("Get daemonset's pods update status failed"), w, http.StatusInternalServerError)
			return
		}
		klog.V(5).Infof("Got pods status list: %v", podsStatus)

		// Successfully get daemonsets/pods update info
		w.Header().Set("content-type", "text/json")
		data, err := json.Marshal(podsStatus)
		if err != nil {
			klog.Errorf("Marshal pods status failed: %v", err.Error())
			returnErr(fmt.Errorf("Get daemonset's pods update status failed: data transfer to json format failed."), w, http.StatusInternalServerError)
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

		klog.Info("12", namespace, podName)
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

// getDaemonsetPodUpdateStatus check pods annotation "apps.openyurt.io/pod-updatable"
// to determine whether new version application is availabel
func getPodUpdateStatus(clientset client.Interface) ([]*PodStatus, error) {
	pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	podsStatus := make([]*PodStatus, 0)
	for _, pod := range pods.Items {
		var updatable bool
		v, ok := pod.Annotations[PodUpdatableAnnotation]

		if !ok {
			updatable = false
		} else {
			updatable, err = strconv.ParseBool(v)
			if err != nil {
				klog.Warningf("Pod %v with invalid update annotation %v", pod.Name, v)
				continue
			}
		}

		klog.V(5).Infof("Pod %v with update annotation %v", pod.Name, updatable)
		podStatus := &PodStatus{
			Namespace: pod.Namespace,
			PodName:   pod.Name,
			Updatable: updatable,
		}
		podsStatus = append(podsStatus, podStatus)

	}

	return podsStatus, nil
}

// applyUpdate execute pod update process by deleting pod under OnDelete update strategy
func applyUpdate(clientset client.Interface, namespace, podName string) (error, bool) {
	pod, err := clientset.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Get pod %v/%v failed, %v", namespace, podName, err)
		return err, false
	}

	// Pod will not be updated without annotation "apps.openyurt.io/pod-updatable"
	v, ok := pod.Annotations[PodUpdatableAnnotation]
	if !ok {
		klog.Infof("Daemonset pod: %v/%v is not updatable without PodUpdatable annotation", namespace, podName)
		return nil, false
	}

	// Pod will not be updated when annotation "apps.openyurt.io/pod-updatable" value cannot be parsed
	updatable, err := strconv.ParseBool(v)
	if err != nil {
		klog.Infof("Pod %v/%v is not updatable with invalid update annotation %v", namespace, podName, v)
		return nil, false
	}

	// Pod will not be updated when annotation "apps.openyurt.io/pod-updatable" value is false
	if !updatable {
		klog.Infof("Pod %v/%v is not updatable with PodUpdatable annotation is %v", namespace, podName, updatable)
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
