package otaupgrade

import (
	"fmt"
	"strconv"

	client "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
	upgradeUtil "github.com/openyurtio/openyurt/pkg/controller/podupgrade"
)

// getDaemonsetPodUpgradeStatus compares spec between all daemonsets and their pods
// to determine whether new version application is availabel
func getDaemonsetPodUpgradeStatus(clientset *client.Clientset) ([]*PodStatus, error) {
	pods, err := Pods(clientset, "")
	if err != nil {
		klog.Errorf("Get all pods in current node failed, %v", err)
		return nil, err
	}

	podStatusList := make([]*PodStatus, 0)

	for _, pod := range pods.Items {
		var upgradable bool
		v, ok := pod.Annotations[upgradeUtil.PodUpgradableAnnotation]

		if !ok {
			upgradable = false
		} else {
			upgradable, err = strconv.ParseBool(v)
			if err != nil {
				klog.Warningf("Pod %v with invalid upgrade annotation %v", pod.Name, v)
				continue
			}
		}

		klog.V(5).Infof("Pod %v with upgrade annotation %v", pod.Name, upgradable)

		if ok && upgradable {
			podStatus := &PodStatus{
				Namespace:  pod.Namespace,
				PodName:    pod.Name,
				Upgradable: upgradable,
			}

			podStatusList = append(podStatusList, podStatus)
		}
	}

	return podStatusList, nil
}

// applyUpgrade execute pod upgrade process by deleting pod under OnDelete update strategy
func applyUpgrade(clientset *client.Clientset, namespace, podName string) error {
	klog.Infof("Start to upgrade daemonset pod:%v/%v", namespace, podName)

	pod, err := Pod(clientset, namespace, podName)
	if err != nil {
		klog.Errorf("Get pod %v/%v failed, %v", namespace, podName, err)
		return err
	}

	// Pod is not upgradable without annotation "apps.openyurt.io/pod-upgradable"
	v, ok := pod.Annotations[upgradeUtil.PodUpgradableAnnotation]
	if !ok {
		klog.Infof("Daemonset pod: %v/%v is not upgradable", namespace, podName)
		return fmt.Errorf("Daemonset pod: %v/%v is not upgradable", namespace, podName)
	}

	// Pod is not upgradable when annotation "apps.openyurt.io/pod-upgradable" value cannot be parsed
	upgradable, err := strconv.ParseBool(v)
	if err != nil {
		klog.Errorf("Pod %v is not upgradable with invalid upgrade annotation %v", pod.Name, v)
		return err
	}

	// Pod is not upgradable when annotation "apps.openyurt.io/pod-upgradable" value is false
	if !upgradable {
		klog.Infof("Daemonset pod: %v/%v is not upgradable", namespace, podName)
		return fmt.Errorf("Current pod is not upgradable")
	}

	klog.Infof("Daemonset pod: %v/%v is upgradable", namespace, podName)
	err = DeletePod(clientset, namespace, podName)
	if err != nil {
		klog.Errorf("Upgrade pod %v/%v failed when delete pod %v", namespace, pod.Name, err)
		return err
	}

	klog.Infof("Daemonset pod: %v/%v upgrade success", namespace, podName)
	return nil
}
