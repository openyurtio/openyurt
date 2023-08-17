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

	"github.com/gorilla/mux"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/kubernetes/rest"
	upgrade "github.com/openyurtio/openyurt/pkg/yurthub/otaupdate/upgrader"
	"github.com/openyurtio/openyurt/pkg/yurthub/otaupdate/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonpodupdater"
)

const (
	StaticPod = "Node"
	DaemonPod = "DaemonSet"
)

type OTAHandler func(kubernetes.Interface, string) http.Handler

type OTAUpgrader interface {
	Apply() error
}

// GetPods return pod list
func GetPods(store cachemanager.StorageWrapper) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		podsKey, err := store.KeyFunc(storage.KeyBuildInfo{
			Component: "kubelet",
			Resources: "pods",
			Version:   "v1",
			Group:     "",
		})
		if err != nil {
			klog.Errorf("get pods key failed, %v", err)
			util.WriteErr(w, "Get pods key failed", http.StatusInternalServerError)
			return
		}
		objs, err := store.List(podsKey)
		if err != nil {
			klog.Errorf("Get pod list failed, %v", err)
			util.WriteErr(w, "Get pod list failed", http.StatusInternalServerError)
			return
		}

		podList := new(corev1.PodList)
		for _, obj := range objs {
			pod, ok := obj.(*corev1.Pod)
			if !ok {
				klog.Errorf("Get pod list failed, %v", err)
				util.WriteErr(w, "Get pod list failed", http.StatusInternalServerError)
				return
			}
			podList.Items = append(podList.Items, *pod)
		}

		// Successfully get pod list, response 200
		data, err := util.EncodePods(podList)
		if err != nil {
			klog.Errorf("Encode pod list failed, %v", err)
			util.WriteErr(w, "Encode pod list failed", http.StatusInternalServerError)
		}
		util.WriteJSONResponse(w, data)
	})
}

// UpdatePod update a specifc pod(namespace/podname) to the latest version
func UpdatePod(clientset kubernetes.Interface, nodeName string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := mux.Vars(r)
		namespace := params["ns"]
		podName := params["podname"]

		pod, ok := preCheck(clientset, namespace, podName, nodeName)
		// Pod update is not allowed
		if !ok {
			util.WriteErr(w, "Pod is not-updatable", http.StatusForbidden)
			return
		}

		var upgrader OTAUpgrader
		kind := pod.GetOwnerReferences()[0].Kind
		switch kind {
		case StaticPod:
			ok, staticName, err := upgrade.PreCheck(podName, nodeName, namespace, clientset)
			if err != nil {
				klog.Errorf("Static pod pre-check failed, %v", err)
				util.WriteErr(w, "Static pod pre-check failed", http.StatusInternalServerError)
				return
			}
			if !ok {
				util.WriteErr(w, "Configmap for static pod does not exist", http.StatusForbidden)
				return
			}
			upgrader = &upgrade.StaticPodUpgrader{Interface: clientset,
				NamespacedName: types.NamespacedName{Namespace: namespace, Name: podName}, StaticName: staticName}

		case DaemonPod:
			upgrader = &upgrade.DaemonPodUpgrader{Interface: clientset,
				NamespacedName: types.NamespacedName{Namespace: namespace, Name: podName}}
		default:
			util.WriteErr(w, fmt.Sprintf("Not support ota upgrade pod type %v", kind), http.StatusBadRequest)
			return
		}

		if err := upgrader.Apply(); err != nil {
			klog.Errorf("Apply update failed, %v", err)
			// Pod update failed with error
			util.WriteErr(w, "Apply update failed", http.StatusInternalServerError)
			return
		}

		// Successfully apply update, response 200
		util.WriteJSONResponse(w, []byte(fmt.Sprintf("Start updating pod %v/%v", namespace, podName)))
	})
}

// preCheck will check the necessary requirements before apply upgrade
// 1. target pod has not been deleted yet
// 2. target pod belongs to current node
// 3. check whether target pod is updatable
// At last, return the target pod to do further operation
func preCheck(clientset kubernetes.Interface, namespace, podName, nodeName string) (*corev1.Pod, bool) {
	pod, err := clientset.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("Get pod %v/%v failed, %v", namespace, podName, err)
		return nil, false
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
	return pod, true
}

// HealthyCheck checks if cloud-edge is disconnected before ota update handle, ota update is not allowed when disconnected
func HealthyCheck(rest *rest.RestConfigManager, nodeName string, handler OTAHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		restCfg := rest.GetRestConfig(true)
		if restCfg == nil {
			klog.Infof("Get pod list is not allowed when edge is disconnected to cloud")
			util.WriteErr(w, "OTA update is not allowed when edge is disconnected to cloud", http.StatusForbidden)
			return
		}

		clientSet, err := kubernetes.NewForConfig(restCfg)
		if err != nil {
			klog.Errorf("Get client set failed: %v", err)
			util.WriteErr(w, "Get client set failed", http.StatusInternalServerError)
			return
		}

		handler(clientSet, nodeName).ServeHTTP(w, r)
	})
}
