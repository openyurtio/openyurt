/*
Copyright 2021 The OpenYurt Authors.

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

package dns

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurttunnel/constants"
)

func (dnsctl *coreDNSRecordController) addNode(obj interface{}) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		return
	}
	if node.DeletionTimestamp != nil {
		dnsctl.deleteNode(node)
		return
	}
	klog.V(2).Infof("enqueue node add event for %v", node.Name)
	dnsctl.enqueue(node, NodeAdd)
}

func (dnsctl *coreDNSRecordController) updateNode(oldObj, newObj interface{}) {
	oldNode, ok := oldObj.(*corev1.Node)
	if !ok {
		return
	}
	newNode, ok := newObj.(*corev1.Node)
	if !ok {
		return
	}

	oldIsEdgeNode, newIsEdgeNode := isEdgeNode(oldNode), isEdgeNode(newNode)
	if oldIsEdgeNode == newIsEdgeNode {
		return
	}

	klog.V(2).Infof("enqueue node update event for %v, will update dns record", newNode.Name)
	dnsctl.enqueue(newNode, NodeUpdate)
}

func (dnsctl *coreDNSRecordController) deleteNode(obj interface{}) {
	node, ok := obj.(*corev1.Node)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("can not get object from tombstone %#v", obj))
			return
		}
		node, ok = tombstone.Obj.(*corev1.Node)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object is not a node %#v", obj))
			return
		}
	}
	klog.V(2).Infof("enqueue node delete event for %v", node.Name)
	dnsctl.enqueue(node, NodeDelete)
}

func (dnsctl *coreDNSRecordController) addConfigMap(obj interface{}) {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return
	}
	if cm.DeletionTimestamp != nil {
		dnsctl.deleteConfigMap(cm)
		return
	}
	klog.V(2).Infof("enqueue configmap add event for %v/%v", cm.Namespace, cm.Name)
	dnsctl.enqueue(cm, ConfigMapAdd)
}

func (dnsctl *coreDNSRecordController) updateConfigMap(oldObj, newObj interface{}) {
	oldConfigMap, ok := oldObj.(*corev1.ConfigMap)
	if !ok {
		return
	}
	newConfigMap, ok := newObj.(*corev1.ConfigMap)
	if !ok {
		return
	}

	if reflect.DeepEqual(oldConfigMap.Data, newConfigMap.Data) {
		return
	}

	klog.V(2).Infof("enqueue configmap update event for %v/%v, will sync tunnel server svc", newConfigMap.Namespace, newConfigMap.Name)
	dnsctl.enqueue(newConfigMap, ConfigMapUpdate)
}

func (dnsctl *coreDNSRecordController) deleteConfigMap(obj interface{}) {
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("can not get object from tombstone %#v", obj))
			return
		}
		cm, ok = tombstone.Obj.(*corev1.ConfigMap)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("tombstone contained object is not a node %#v", obj))
			return
		}
	}
	klog.V(2).Infof("enqueue configmap delete event for %v/%v", cm.Namespace, cm.Name)
	dnsctl.enqueue(cm, ConfigMapDelete)
}

func (dnsctl *coreDNSRecordController) addService(obj interface{}) {
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return
	}
	if svc.Namespace != constants.YurttunnelServerServiceNs || svc.Name != constants.YurttunnelServerInternalServiceName {
		return
	}
	klog.V(2).Infof("enqueue service add event for %v/%v", svc.Namespace, svc.Name)
	dnsctl.enqueue(svc, ServiceAdd)
}

func (dnsctl *coreDNSRecordController) updateService(oldObj, newObj interface{}) {
	// do nothing
}

func (dnsctl *coreDNSRecordController) deleteService(obj interface{}) {
	// do nothing
}

func (dnsctl *coreDNSRecordController) onConfigMapAdd(cm *corev1.ConfigMap) error {
	return dnsctl.syncTunnelServerServiceAsWhole()
}

func (dnsctl *coreDNSRecordController) onConfigMapUpdate(cm *corev1.ConfigMap) error {
	return dnsctl.syncTunnelServerServiceAsWhole()
}

func (dnsctl *coreDNSRecordController) onConfigMapDelete(cm *corev1.ConfigMap) error {
	return dnsctl.syncTunnelServerServiceAsWhole()
}

func (dnsctl *coreDNSRecordController) onNodeAdd(node *corev1.Node) error {
	klog.V(2).Infof("adding node dns record for %v", node.Name)
	return dnsctl.addOrUpdateNode(node)
}

func (dnsctl *coreDNSRecordController) onNodeUpdate(node *corev1.Node) error {
	klog.V(2).Infof("updating node dns record for %v", node.Name)
	return dnsctl.addOrUpdateNode(node)
}

func (dnsctl *coreDNSRecordController) addOrUpdateNode(node *corev1.Node) error {
	ip, err := getNodeHostIP(node)
	if err != nil {
		return err
	}
	if isEdgeNode(node) {
		ip, err = dnsctl.getTunnelServerIP(true)
		if err != nil {
			return err
		}
	}

	records, err := dnsctl.getCurrentDNSRecords()
	if err != nil {
		return err
	}

	updatedRecords, changed, err := addOrUpdateRecord(records, formatDNSRecord(ip, node.Name))
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}

	return dnsctl.updateDNSRecords(updatedRecords)
}

func (dnsctl *coreDNSRecordController) onNodeDelete(node *corev1.Node) error {
	klog.V(2).Infof("deleting node dns record for %v", node.Name)

	dnsctl.lock.Lock()
	defer dnsctl.lock.Unlock()

	records, err := dnsctl.getCurrentDNSRecords()
	if err != nil {
		return err
	}
	mergedRecords, changed := removeRecordByHostname(records, node.Name)
	if !changed {
		return nil
	}

	return dnsctl.updateDNSRecords(mergedRecords)
}

func (dnsctl *coreDNSRecordController) getCurrentDNSRecords() ([]string, error) {
	cm, err := dnsctl.kubeClient.CoreV1().ConfigMaps(constants.YurttunnelServerServiceNs).
		Get(context.Background(), yurttunnelDNSRecordConfigMapName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	data, ok := cm.Data[constants.YurttunnelDNSRecordNodeDataKey]
	if !ok || len(data) == 0 {
		return []string{}, nil
	}

	return strings.Split(data, "\n"), nil
}

func (dnsctl *coreDNSRecordController) onServiceAdd(svc *corev1.Service) error {
	return dnsctl.syncDNSRecordAsWhole()
}

func (dnsctl *coreDNSRecordController) onServiceUpdate(svc *corev1.Service) error {
	return nil
}

func (dnsctl *coreDNSRecordController) onServiceDelete(svc *corev1.Service) error {
	return nil
}
