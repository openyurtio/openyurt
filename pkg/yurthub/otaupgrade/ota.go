package otaupgrade

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

type PodStatus struct {
	Namespace string
	PodName   string
	// TODO: whether need to display
	// OldImgs    []string
	// NewImgs    []string
	Upgradable bool
}

// GetPods return all daemonset pods' upgrade information by PodStatus
func GetPods(clientset *kubernetes.Clientset) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		psList, err := getDaemonsetPodUpgradeStatus(clientset)
		if err != nil {
			Err(fmt.Errorf("Get daemonset's pods upgrade status failed"), w, r)
			return
		}
		klog.V(4).Infof("Got pods status list: %v", psList)

		// Successfully get daemonsets/pods upgrade info
		w.Header().Set("content-type", "text/json")
		data, err := json.Marshal(psList)
		if err != nil {
			klog.Errorf("Marshal pods status failed: %v", err.Error())
			Err(fmt.Errorf("Get daemonset's pods upgrade status failed: data transfer to json format failed."), w, r)
			return
		}

		w.WriteHeader(http.StatusOK)
		n, err := w.Write(data)
		if err != nil || n != len(data) {
			klog.Errorf("Write resp for request, expect %d bytes but write %d bytes with error, %v", len(data), n, err)
		}
	})
}

// UpgradePod upgrade a specifc pod(namespace/podname) to the latest version
func UpgradePod(clientset *kubernetes.Clientset) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		params := mux.Vars(r)
		namespace := params["ns"]
		podName := params["podname"]

		err := applyUpgrade(clientset, namespace, podName)
		if err != nil {
			Err(fmt.Errorf("Apply upgrade failed"), w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}
