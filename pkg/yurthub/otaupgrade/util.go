package otaupgrade

import (
	"context"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"
)

func Pod(clientset *kubernetes.Clientset, namespace, podName string) (*corev1.Pod, error) {
	return clientset.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
}

func Pods(clientset *kubernetes.Clientset, namespace string) (*corev1.PodList, error) {
	return clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
}

func DeletePod(clientset *kubernetes.Clientset, namespace, podName string) error {
	return clientset.CoreV1().Pods(namespace).Delete(context.TODO(), podName, metav1.DeleteOptions{})
}

func Err(err error, w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusInternalServerError)
	n := len([]byte(err.Error()))
	nw, e := w.Write([]byte(err.Error()))
	if e != nil || nw != n {
		klog.Errorf("write resp for request, expect %d bytes but write %d bytes with error, %v", n, nw, e)
	}
}
