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
	"context"
	"fmt"
	"io/ioutil"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	iotv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha1"
)

const (
	PODHOSTNAME  = "/etc/hostname"
	PODNAMESPACE = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
)

// GetNodePool get nodepool where yurt-iot-dock run
func GetNodePool(cfg *rest.Config) (string, error) {
	var nodePool string
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nodePool, err
	}

	bn, err := ioutil.ReadFile(PODHOSTNAME)
	if err != nil {
		return nodePool, fmt.Errorf("Read file %s fail: %v", PODHOSTNAME, err)
	}
	bns, err := ioutil.ReadFile(PODNAMESPACE)
	if err != nil {
		return nodePool, fmt.Errorf("Read file %s fail: %v", PODNAMESPACE, err)
	}
	name := strings.Replace(string(bn), "\n", "", -1)
	namespace := string(bns)

	pod, err := client.CoreV1().Pods(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return nodePool, fmt.Errorf("not found pod %s/%s: %v", namespace, name, err)
	}
	node, err := client.CoreV1().Nodes().Get(context.Background(), pod.Spec.NodeName, metav1.GetOptions{})
	if err != nil {
		return nodePool, fmt.Errorf("not found node %s: %v", pod.Spec.NodeName, err)
	}
	nodePool, ok := node.Labels["apps.openyurt.io/nodepool"]
	if !ok {
		return nodePool, fmt.Errorf("node %s doesn't add to a nodepool", node.GetName())
	}
	return nodePool, err
}

func GetEdgeDeviceServiceName(ds *iotv1alpha1.DeviceService, label string) string {
	var actualDSName string
	if _, ok := ds.ObjectMeta.Labels[label]; ok {
		actualDSName = ds.ObjectMeta.Labels[label]
	} else {
		actualDSName = ds.GetName()
	}
	return actualDSName
}

func GetEdgeDeviceName(d *iotv1alpha1.Device, label string) string {
	var actualDeviceName string
	if _, ok := d.ObjectMeta.Labels[label]; ok {
		actualDeviceName = d.ObjectMeta.Labels[label]
	} else {
		actualDeviceName = d.GetName()
	}
	return actualDeviceName
}

func GetEdgeDeviceProfileName(dp *iotv1alpha1.DeviceProfile, label string) string {
	var actualDPName string
	if _, ok := dp.ObjectMeta.Labels[label]; ok {
		actualDPName = dp.ObjectMeta.Labels[label]
	} else {
		actualDPName = dp.GetName()
	}
	return actualDPName
}
