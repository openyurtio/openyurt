/*
Copyright 2023 The OpenYurt Authors.
Copyright 2017 The Kubernetes Authors.

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

package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	yurtinformers "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/informers/externalversions"
	yurtlisters "github.com/openyurtio/yurt-app-manager-api/pkg/yurtappmanager/client/listers/apps/v1alpha1"
	admissionv1 "k8s.io/api/admission/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/informers/core/v1"
	client "k8s.io/client-go/kubernetes"
	leaselisterv1 "k8s.io/client-go/listers/coordination/v1"
	listerv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
)

type PoolCoordinatorWebhook struct {
	client         client.Interface
	nodeInformer   v1.NodeInformer
	nodeSynced     cache.InformerSynced
	nodeLister     listerv1.NodeLister
	nodePoolSynced cache.InformerSynced
	nodePoolLister yurtlisters.NodePoolLister
	leaseSynced    cache.InformerSynced
	leaseLister    leaselisterv1.LeaseNamespaceLister
}

func NewPoolcoordinatorWebhook(kc client.Interface,
	yurtInformers yurtinformers.SharedInformerFactory,
	informerFactory informers.SharedInformerFactory) *PoolCoordinatorWebhook {

	nodePoolInformer := yurtInformers.Apps().V1alpha1().NodePools()
	leaseInformer := informerFactory.Coordination().V1().Leases()
	h := &PoolCoordinatorWebhook{
		client:         kc,
		nodeInformer:   informerFactory.Core().V1().Nodes(),
		nodePoolSynced: nodePoolInformer.Informer().HasSynced,
		nodePoolLister: nodePoolInformer.Lister(),
		leaseSynced:    leaseInformer.Informer().HasSynced,
		leaseLister:    leaseInformer.Lister().Leases(corev1.NamespaceNodeLease),
	}
	h.nodeSynced = h.nodeInformer.Informer().HasSynced
	h.nodeLister = h.nodeInformer.Lister()

	return h
}

func (h *PoolCoordinatorWebhook) Init(certs *Certs, stopCH <-chan struct{}) {
	if !cache.WaitForCacheSync(stopCH, h.nodeSynced, h.nodePoolSynced, h.leaseSynced) {
		klog.Error("sync poolcoordinator webhook timeout")
	}

	h.ensureValidatingConfiguration(certs)
}

func (h *PoolCoordinatorWebhook) Handler() []Handler {
	return []Handler{
		{ValidatePath, h.serveValidatePods},
	}
}

func (h *PoolCoordinatorWebhook) ensureValidatingConfiguration(certs *Certs) {
	fail := admissionregistrationv1.Fail
	sideEffects := admissionregistrationv1.SideEffectClassNone
	validatingConfigurationName := GetEnv(EnvPodValidatingConfigurationName, DefaultPodValidatingConfigurationName)
	validatingPath := GetEnv(EnvPodValidatingPath, DefaultPodValidatingPath)
	config := &admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: validatingConfigurationName,
		},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{{
			Name: GetEnv(EnvPodValidatingName, DefaultPodValidatingName),
			ClientConfig: admissionregistrationv1.WebhookClientConfig{
				CABundle: certs.CACert,
				Service: &admissionregistrationv1.ServiceReference{
					Name:      GetEnv(EnvServiceName, DefaultServiceName),
					Namespace: GetEnv(EnvNamespace, DefaultNamespace),
					Path:      &validatingPath,
				},
			},
			Rules: []admissionregistrationv1.RuleWithOperations{
				{Operations: []admissionregistrationv1.OperationType{
					admissionregistrationv1.Delete},
					Rule: admissionregistrationv1.Rule{
						APIGroups:   []string{""},
						APIVersions: []string{"v1"},
						Resources:   []string{"pods"},
					},
				}},
			FailurePolicy:           &fail,
			SideEffects:             &sideEffects,
			AdmissionReviewVersions: []string{"v1"},
		}},
	}

	oldConfig, err := h.client.AdmissionregistrationV1().ValidatingWebhookConfigurations().
		Get(context.TODO(), validatingConfigurationName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("validateWebhookConfiguration %s not found, create it.", validatingConfigurationName)
			if _, err = h.client.AdmissionregistrationV1().ValidatingWebhookConfigurations().Create(
				context.TODO(), config, metav1.CreateOptions{}); err != nil {
				klog.Fatal(err)
			}
		}
	} else {
		klog.Infof("validateWebhookConfiguration %s already exists, update it.", validatingConfigurationName)
		oldConfig.Webhooks = config.Webhooks
		if _, err = h.client.AdmissionregistrationV1().ValidatingWebhookConfigurations().Update(
			context.TODO(), oldConfig, metav1.UpdateOptions{}); err != nil {
			klog.Fatal(err)
		}
	}
}

// ServeValidatePods validates an admission request and then writes an admission
func (h *PoolCoordinatorWebhook) serveValidatePods(w http.ResponseWriter, r *http.Request) {
	klog.Infof("poolcoordinator webhook uri: %s", r.RequestURI)

	pa, err := h.NewPodAdmission(r)
	if err != nil {
		klog.Error(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	out, err := pa.validateReview()
	if err != nil {
		e := fmt.Sprintf("could not generate admission response: %v", err)
		klog.Error(e)
		http.Error(w, e, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", DefaultContentType)

	jout, err := json.Marshal(out)
	if err != nil {
		e := fmt.Sprintf("could not parse admission response: %v", err)
		klog.Error(e)
		http.Error(w, e, http.StatusInternalServerError)
		return
	}

	klog.Infof("sending response: %s", jout)
	fmt.Fprintf(w, "%s", jout)
}

func (h *PoolCoordinatorWebhook) NewPodAdmission(r *http.Request) (*PodAdmission, error) {
	admissionReview, err := ParseRequest(*r)
	if err != nil {
		return nil, err
	}

	req := admissionReview.Request
	if req.Kind.Kind != "Pod" {
		return nil, fmt.Errorf("only pods are supported")
	}

	pod := &corev1.Pod{}
	if req.Operation == admissionv1.Delete || req.Operation == admissionv1.Update {
		err = json.Unmarshal(req.OldObject.Raw, pod)
	} else {
		err = json.Unmarshal(req.Object.Raw, pod)
	}
	if err != nil {
		klog.Errorf("json unmarshal to pod error: %+v", err)
		return nil, err
	}

	nodeName := pod.Spec.NodeName
	klog.Infof("pod %s is on node: %s\n", pod.Name, nodeName)
	node, err := h.nodeLister.Get(nodeName)
	if err != nil {
		klog.Errorf("nodeLister get node %s error: %+v", nodeName, err)
		return nil, err
	}

	pa := &PodAdmission{
		request:        req,
		pod:            pod,
		node:           node,
		leaseLister:    h.leaseLister,
		nodePoolLister: h.nodePoolLister,
	}
	klog.Infof("name: %s, namespace: %s, operation: %s, from: %v", req.Name, req.Namespace, req.Operation, &req.UserInfo)
	return pa, nil
}

type PodAdmission struct {
	request        *admissionv1.AdmissionRequest
	pod            *corev1.Pod
	node           *corev1.Node
	leaseLister    leaselisterv1.LeaseNamespaceLister
	nodePoolLister yurtlisters.NodePoolLister
}

func (pa *PodAdmission) validateReview() (*admissionv1.AdmissionReview, error) {
	if pa.request.Kind.Kind != "Pod" {
		err := fmt.Errorf("only pods are supported here")
		return ReviewResponse(pa.request.UID, false, http.StatusBadRequest, ""), err
	}

	if pa.request.Operation != admissionv1.Delete {
		reason := fmt.Sprintf("Operation %v is accepted always", pa.request.Operation)
		return ReviewResponse(pa.request.UID, true, http.StatusAccepted, reason), nil
	}

	isValid, msg := pa.validateDelete()
	klog.Infof("validateDelete result: %+v, msg: %+v", isValid, msg)
	if !isValid {
		return ReviewResponse(pa.request.UID, false, http.StatusForbidden, msg), nil
	}
	return ReviewResponse(pa.request.UID, true, http.StatusAccepted, msg), nil
}

// validateDelete returns true if a pod is valid to delete/evict
func (pa *PodAdmission) validateDelete() (bool, string) {
	if !pa.userIsNodeController() {
		return true, msgPodNormalDelete
	}

	// get number of nodes in node pool
	var nodePoolName string
	if pa.node.Labels != nil {
		if name, ok := pa.node.Labels[LabelCurrentNodePool]; ok {
			nodePoolName = name
		}
	}
	if nodePoolName == "" {
		return true, msgNodeNotInNodePool
	}

	nodePool, err := pa.nodePoolLister.Get(nodePoolName)
	if err != nil {
		klog.Errorf("validateDelete get nodePool %s error: %+v", nodePoolName, err)
		return false, msgNodePoolStatusError
	}
	nodeNumber := len(nodePool.Status.Nodes)

	// check number of ready nodes in node pool
	readyNumber := CountAliveNode(pa.leaseLister, nodePool.Status.Nodes)

	// When number of ready nodes in node pool is below a configurable parameter,
	// we don't allow pods to move within the pool any more.
	// This threshold defaults to one third of the number of pool's nodes.
	threshold := GetPoolReadyNodeNumberRatioThreshold()
	if float64(readyNumber)/float64(nodeNumber) < threshold {
		return false, msgPoolHasTooFewReadyNodes
	}
	return true, msgPodAvailablePoolAndNodeIsNotAlive
}

func (pa *PodAdmission) userIsNodeController() bool {
	return strings.Contains(pa.request.UserInfo.Username, "system:serviceaccount:kube-system:node-controller")
}
