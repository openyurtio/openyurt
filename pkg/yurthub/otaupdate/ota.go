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
	"strings"

	"github.com/go-errors/errors"
	"github.com/gorilla/mux"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	yurtutil "github.com/openyurtio/openyurt/pkg/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/cachemanager"
	"github.com/openyurtio/openyurt/pkg/yurthub/healthchecker"
	upgrade "github.com/openyurtio/openyurt/pkg/yurthub/otaupdate/upgrader"
	"github.com/openyurtio/openyurt/pkg/yurthub/otaupdate/util"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage"
	"github.com/openyurtio/openyurt/pkg/yurthub/transport"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonsetupgradestrategy"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonsetupgradestrategy/daemonpodupdater"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonsetupgradestrategy/imagepreheat"
	podutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/pod"
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

// UpdatePod update a specific pod(namespace/podname) to the latest version
func UpdatePod(clientset kubernetes.Interface, nodeName string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := mux.Vars(r)
		namespace := params["ns"]
		podName := params["podname"]

		pod, err := getPod(clientset, namespace, podName)
		if err != nil {
			util.WriteErr(w, fmt.Sprintf("Get pod failed, %v", err), http.StatusInternalServerError)
			return
		}

		if err := preCheckUpdatePod(pod, nodeName); err != nil {
			util.WriteErr(w, fmt.Sprintf("Pre check update pod failed, %v", err), http.StatusForbidden)
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

func getPod(clientset kubernetes.Interface, namespace, podName string) (*corev1.Pod, error) {
	return clientset.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
}

func preCheckUpdatePod(pod *corev1.Pod, nodeName string) error {
	if err := checkPodStatus(pod, nodeName); err != nil {
		return errors.Errorf("Failed to check pod %v/%v status, error: %v", pod.Namespace, pod.Name, err)
	}

	if err := checkPodImageReady(pod); err != nil {
		return errors.Errorf("Failed to check pod %v/%v image ready, error: %v", pod.Namespace, pod.Name, err)
	}

	return nil
}

func checkPodStatus(pod *corev1.Pod, nodeName string) error {
	if pod.DeletionTimestamp != nil {
		return errors.Errorf("Pod %v/%v is deleting, can not be updated", pod.Namespace, pod.Name)
	}

	if pod.Spec.NodeName != nodeName {
		return errors.Errorf("Pod: %v/%v is running on %v, can not be updated", pod.Namespace, pod.Name, pod.Spec.NodeName)
	}

	if !daemonpodupdater.IsPodUpdatable(pod) {
		return errors.Errorf("Pod: %v/%v update status is False, can not be updated", pod.Namespace, pod.Name)
	}

	return nil
}

func checkPodImageReady(pod *corev1.Pod) error {
	cond := getPodImageReadyCondition(pod)
	if cond == nil {
		return nil
	}

	if cond.Status != corev1.ConditionTrue {
		return errors.Errorf("Pod: %v/%v image is not ready, reason: %s, message: %s", pod.Namespace, pod.Name, cond.Reason, cond.Message)
	}

	hashVersion := imagepreheat.GetPodNextHashVersion(pod)
	if strings.TrimPrefix(cond.Message, daemonsetupgradestrategy.VersionPrefix) != hashVersion {
		return errors.Errorf("Pod: %v/%v image is not ready, reason: %s, message: %s", pod.Namespace, pod.Name, cond.Reason, cond.Message)
	}

	return nil
}

func getPodImageReadyCondition(pod *corev1.Pod) *corev1.PodCondition {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == daemonsetupgradestrategy.PodImageReady {
			return &cond
		}
	}
	return nil
}

// HealthyCheck checks if cloud-edge is disconnected before ota update handle, ota update is not allowed when disconnected
func HealthyCheck(healthChecker healthchecker.Interface, clientManager transport.Interface, nodeName string, handler OTAHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var kubeClient kubernetes.Interface
		if yurtutil.IsNil(healthChecker) {
			// cloud mode: no health checker is prepared
			kubeClient = clientManager.GetDirectClientsetAtRandom()
		} else if u := healthChecker.PickOneHealthyBackend(); u != nil {
			// edge mode, get a kube client for healthy cloud kube-apiserver
			kubeClient = clientManager.GetDirectClientset(u)
		}

		if kubeClient != nil {
			handler(kubeClient, nodeName).ServeHTTP(w, r)
			return
		}

		klog.Infof("OTA upgrade is not allowed when node(%s) is disconnected to cloud", nodeName)
		util.WriteErr(w, "OTA upgrade is not allowed when node is disconnected to cloud", http.StatusServiceUnavailable)
	})
}

// PullPodImage handles image pre-pull requests for a specific pod
func PullPodImage(clientset kubernetes.Interface, nodeName string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		params := mux.Vars(r)
		namespace := params["ns"]
		podName := params["podname"]

		pod, err := getPod(clientset, namespace, podName)
		if err != nil {
			util.WriteErr(w, fmt.Sprintf("Get pod failed, %v", err), http.StatusInternalServerError)
			return
		}

		if err := checkPodStatus(pod, nodeName); err != nil {
			util.WriteErr(w, fmt.Sprintf("Failed to check pod status %v/%v, error: %v", namespace, podName, err), http.StatusForbidden)
			return
		}

		cond := corev1.PodCondition{
			Type:    daemonsetupgradestrategy.PodImageReady,
			Status:  corev1.ConditionFalse,
			Message: daemonsetupgradestrategy.VersionPrefix + imagepreheat.GetPodNextHashVersion(pod),
		}
		podutil.UpdatePodCondition(&pod.Status, &cond)

		patchBody := struct {
			Status struct {
				Conditions []corev1.PodCondition `json:"conditions"`
			} `json:"status"`
		}{}
		patchBody.Status.Conditions = pod.Status.Conditions

		patchBytes, err := json.Marshal(patchBody)
		if err != nil {
			klog.Errorf("Marshal patch body failed, %v", err)
			util.WriteErr(w, "Marshal patch body failed", http.StatusInternalServerError)
			return
		}

		_, err = clientset.CoreV1().Pods(namespace).Patch(
			context.TODO(),
			podName,
			types.MergePatchType,
			patchBytes,
			metav1.PatchOptions{
				FieldManager: "yurthub-ota",
			},
			"status",
		)
		if err != nil {
			klog.Errorf("Patch pod status for imagepull failed, %v", err)
			util.WriteErr(w, "Patch pod status for imagepull failed", http.StatusInternalServerError)
			return
		}

		util.WriteJSONResponse(w, []byte(fmt.Sprintf("Image pre-pull requested for pod %v/%v", namespace, podName)))
	})
}
