/*
Copyright 2022 The OpenYurt Authors.
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
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/wI2L/jsondiff"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/informers"
	informercoordv1 "k8s.io/client-go/informers/coordination/v1"
	v1 "k8s.io/client-go/informers/core/v1"
	client "k8s.io/client-go/kubernetes"
	leaselisterv1 "k8s.io/client-go/listers/coordination/v1"
	listerv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/controller/poolcoordinator/utils"
)

const (
	msgNodeAutonomy                      string = "node autonomy annotated, eviction aborted"
	msgPodAvailableNode                  string = "pod should exist on the specific node, eviction aborted"
	msgPodAvailablePoolAndNodeIsAlive    string = "node is actually alive in a pool, eviction aborted"
	msgPodAvailablePoolAndNodeIsNotAlive string = "node is not alive in a pool, eviction approved"
	msgPodDeleteValidated                string = "pod deletion validated"
	msgPoolHasTooFewReadyNodes           string = "nodepool has too few ready nodes"

	// pod can have two autonomy modes: node scope autonomy, or nodepool scope autonomy
	PodAutonomyAnnotation = "apps.openyurt.io/autonomy"
	PodAutonomyNode       = "node"
	PodAutonomyPool       = "pool"

	// when ready nodes in a pool is below this value, we don't allow pod transition any more
	PoolReadyNodeNumberRatioThresholdDefault = 0.35

	ValidatePath string = "/pool-coordinator-webhook-validate"
	MutatePath   string = "/pool-coordinator-webhook-mutate"
	HealthPath   string = "/pool-coordinator-webhook-health"
	MaxRetries          = 30
)

type PoolCoordinatorWebhook struct {
	client        client.Interface
	nodeInformer  v1.NodeInformer
	nodeSynced    cache.InformerSynced
	nodeLister    listerv1.NodeLister
	leaseInformer informercoordv1.LeaseInformer
	leaseSynced   cache.InformerSynced
	leaseLister   leaselisterv1.LeaseNamespaceLister

	nodepoolMap *utils.NodepoolMap

	nodePoolUpdateQueue workqueue.RateLimitingInterface
}

type validation struct {
	Valid  bool
	Reason string
}

type PodAdmission struct {
	request     *admissionv1.AdmissionRequest
	pod         *corev1.Pod
	node        *corev1.Node
	leaseLister leaselisterv1.LeaseNamespaceLister
	nodepoolMap *utils.NodepoolMap
}

func getPoolReadyNodeNumberRatioThreshold() float64 {
	return PoolReadyNodeNumberRatioThresholdDefault
}

func (pa *PodAdmission) userIsNodeController() bool {
	return strings.Contains(pa.request.UserInfo.Username, "system:serviceaccount:kube-system:node-controller")
}

func (pa *PodAdmission) validateReview() (*admissionv1.AdmissionReview, error) {
	if pa.request.Kind.Kind != "Pod" {
		err := fmt.Errorf("only pods are supported here")
		return pa.reviewResponse(pa.request.UID, false, http.StatusBadRequest, ""), err
	}

	if pa.request.Operation != admissionv1.Delete {
		reason := fmt.Sprintf("Operation %v is accepted always", pa.request.Operation)
		return pa.reviewResponse(pa.request.UID, true, http.StatusAccepted, reason), nil
	}

	val, err := pa.validateDel()
	if err != nil {
		e := fmt.Sprintf("could not validate pod: %v", err)
		return pa.reviewResponse(pa.request.UID, false, http.StatusBadRequest, e), err
	}
	if !val.Valid {
		return pa.reviewResponse(pa.request.UID, false, http.StatusForbidden, val.Reason), nil
	}

	return pa.reviewResponse(pa.request.UID, true, http.StatusAccepted, val.Reason), nil
}

// ValidateDel returns true if a pod is valid to delete/evict
func (pa *PodAdmission) validateDel() (validation, error) {
	if pa.request.Operation == admissionv1.Delete {
		if pa.userIsNodeController() {

			// node is autonomy annotated
			// although pod would be added tolerations to avoid eviction after pool-coordinator introduction,
			// for poosible pods created before, we keep the logic for the time being.
			if utils.NodeIsInAutonomy(pa.node) {
				return validation{Valid: false, Reason: msgNodeAutonomy}, nil
			}

			if pa.pod.Annotations != nil {
				// pod has annotation of node available
				if pa.pod.Annotations[PodAutonomyAnnotation] == PodAutonomyNode {
					return validation{Valid: false, Reason: msgPodAvailableNode}, nil
				}

				if pa.pod.Annotations[PodAutonomyAnnotation] == PodAutonomyPool {
					if utils.NodeIsAlive(pa.leaseLister, pa.node.Name) {
						return validation{Valid: false, Reason: msgPodAvailablePoolAndNodeIsAlive}, nil
					} else {
						pool, ok := utils.NodeNodepool(pa.node)
						if ok {
							// When number of ready nodes in node pool is below a configurable parameter,
							// we don't alllow pods to move within the pool any more.
							// This threshold defaluts to one third of the number of pool's nodes.
							threshold := getPoolReadyNodeNumberRatioThreshold()
							if float64(utils.CountAliveNode(pa.leaseLister, pa.nodepoolMap.Nodes(pool)))/float64(pa.nodepoolMap.Count(pool)) < threshold {
								return validation{Valid: false, Reason: msgPoolHasTooFewReadyNodes}, nil
							}
						}
						return validation{Valid: true, Reason: msgPodAvailablePoolAndNodeIsNotAlive}, nil
					}
				}
			}
		}
	}
	return validation{Valid: true, Reason: msgPodDeleteValidated}, nil
}

func (pa *PodAdmission) mutateAddToleration() ([]byte, error) {
	toadd := []corev1.Toleration{
		{Key: "node.kubernetes.io/unreachable",
			Operator: "Exists",
			Effect:   "NoExecute"},
		{Key: "node.kubernetes.io/not-ready",
			Operator: "Exists",
			Effect:   "NoExecute"},
	}
	tols := pa.pod.Spec.Tolerations
	merged, changed := utils.MergeTolerations(tols, toadd)
	if !changed {
		return nil, nil
	}

	mpod := pa.pod.DeepCopy()
	mpod.Spec.Tolerations = merged

	// generate json patch
	patch, err := jsondiff.Compare(pa.pod, mpod)
	if err != nil {
		return nil, err
	}

	patchb, err := json.Marshal(patch)
	if err != nil {
		return nil, err
	}

	return patchb, nil
}

func (pa *PodAdmission) mutateReview() (*admissionv1.AdmissionReview, error) {
	if pa.request.Kind.Kind != "Pod" {
		err := fmt.Errorf("only pods are supported here")
		return pa.reviewResponse(pa.request.UID, false, http.StatusBadRequest, ""), err
	}

	if pa.request.Operation != admissionv1.Create && pa.request.Operation != admissionv1.Update {
		reason := fmt.Sprintf("Operation %v is accepted always", pa.request.Operation)
		return pa.reviewResponse(pa.request.UID, true, http.StatusAccepted, reason), nil
	}

	if !utils.NodeIsInAutonomy(pa.node) &&
		(pa.pod.Annotations == nil || pa.pod.Annotations[PodAutonomyAnnotation] != PodAutonomyNode) {
		return pa.reviewResponse(pa.request.UID, true, http.StatusAccepted, "no need of mutation"), nil
	}

	// add tolerations if not yet
	val, err := pa.mutateAddToleration()
	if err != nil {
		return pa.reviewResponse(pa.request.UID, true, http.StatusAccepted, "could not merge tolerations"), err
	}
	if val == nil {
		return pa.reviewResponse(pa.request.UID, true, http.StatusAccepted, "tolerations already existed"), nil
	}

	return pa.patchReviewResponse(pa.request.UID, val)
}

func (pa *PodAdmission) reviewResponse(uid types.UID, allowed bool, httpCode int32, reason string) *admissionv1.AdmissionReview {
	return &admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AdmissionReview",
			APIVersion: "admission.k8s.io/v1",
		},
		Response: &admissionv1.AdmissionResponse{
			UID:     uid,
			Allowed: allowed,
			Result: &metav1.Status{
				Code:    httpCode,
				Message: reason,
			},
		},
	}
}

// patchReviewResponse builds an admission review with given json patch
func (pa *PodAdmission) patchReviewResponse(uid types.UID, patch []byte) (*admissionv1.AdmissionReview, error) {
	patchType := admissionv1.PatchTypeJSONPatch

	return &admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			Kind:       "AdmissionReview",
			APIVersion: "admission.k8s.io/v1",
		},
		Response: &admissionv1.AdmissionResponse{
			UID:       uid,
			Allowed:   true,
			PatchType: &patchType,
			Patch:     patch,
		},
	}, nil
}

// ServeHealth returns 200 when things are good
func (h *PoolCoordinatorWebhook) serveHealth(w http.ResponseWriter, r *http.Request) {
	klog.Info("uri", r.RequestURI)
	fmt.Fprint(w, "OK")
}

// ServeValidatePods validates an admission request and then writes an admission
func (h *PoolCoordinatorWebhook) serveValidatePods(w http.ResponseWriter, r *http.Request) {
	klog.Info("uri", r.RequestURI)
	klog.Info("received validation request")

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

	w.Header().Set("Content-Type", "application/json")

	jout, err := json.Marshal(out)
	if err != nil {
		e := fmt.Sprintf("could not parse admission response: %v", err)
		klog.Error(e)
		http.Error(w, e, http.StatusInternalServerError)
		return
	}

	klog.Info("sending response")
	klog.Infof("%s", jout)
	fmt.Fprintf(w, "%s", jout)
}

// ServeMutatePods mutates an admission request and then writes an admission
func (h *PoolCoordinatorWebhook) serveMutatePods(w http.ResponseWriter, r *http.Request) {
	klog.Info("uri", r.RequestURI)
	klog.Info("received validation request")

	pa, err := h.NewPodAdmission(r)
	if err != nil {
		klog.Error(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	out, err := pa.mutateReview()
	if err != nil {
		e := fmt.Sprintf("could not generate admission response: %v", err)
		klog.Error(e)
		http.Error(w, e, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	jout, err := json.Marshal(out)
	if err != nil {
		e := fmt.Sprintf("could not parse admission response: %v", err)
		klog.Error(e)
		http.Error(w, e, http.StatusInternalServerError)
		return
	}

	klog.Info("sending response")
	klog.Infof("%s", jout)
	fmt.Fprintf(w, "%s", jout)
}

// parseRequest extracts an AdmissionReview from an http.Request if possible
func (h *PoolCoordinatorWebhook) parseRequest(r http.Request) (*admissionv1.AdmissionReview, error) {
	if r.Header.Get("Content-Type") != "application/json" {
		return nil, fmt.Errorf("Content-Type: %q should be %q",
			r.Header.Get("Content-Type"), "application/json")
	}

	bodybuf := new(bytes.Buffer)
	bodybuf.ReadFrom(r.Body)
	body := bodybuf.Bytes()
	if len(body) == 0 {
		return nil, fmt.Errorf("admission request body is empty")
	}

	var a admissionv1.AdmissionReview

	if err := json.Unmarshal(body, &a); err != nil {
		return nil, fmt.Errorf("could not parse admission review request: %v", err)
	}

	if a.Request == nil {
		return nil, fmt.Errorf("admission review can't be used: Request field is nil")
	}

	return &a, nil
}

func (h *PoolCoordinatorWebhook) NewPodAdmission(r *http.Request) (*PodAdmission, error) {
	in, err := h.parseRequest(*r)
	if err != nil {
		return nil, err
	}

	req := in.Request

	pod := &corev1.Pod{}

	if err := json.Unmarshal(req.OldObject.Raw, pod); err != nil {
		klog.Error(err)
		return nil, err
	}

	nodeName := pod.Spec.NodeName
	node, err := h.nodeLister.Get(nodeName)
	if err != nil {
		return nil, err
	}

	pa := &PodAdmission{
		request:     req,
		pod:         pod,
		node:        node,
		leaseLister: h.leaseLister,
		nodepoolMap: h.nodepoolMap,
	}

	klog.Infof("name: %s, namespace: %s, operation: %s, from: %v",
		req.Name, req.Namespace, req.Operation, &req.UserInfo)

	return pa, nil
}

func NewPoolcoordinatorWebhook(kc client.Interface, informerFactory informers.SharedInformerFactory) *PoolCoordinatorWebhook {
	h := &PoolCoordinatorWebhook{
		client:              kc,
		nodePoolUpdateQueue: workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter()),
	}
	h.nodeInformer = informerFactory.Core().V1().Nodes()
	h.nodeSynced = h.nodeInformer.Informer().HasSynced
	h.nodeLister = h.nodeInformer.Lister()
	h.leaseInformer = informerFactory.Coordination().V1().Leases()
	h.leaseSynced = h.leaseInformer.Informer().HasSynced
	h.leaseLister = h.leaseInformer.Lister().Leases(corev1.NamespaceNodeLease)

	h.nodepoolMap = utils.NewNodepoolMap()

	h.nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    h.onNodeCreate,
		UpdateFunc: h.onNodeUpdate,
		DeleteFunc: h.onNodeDelete,
	})
	return h
}

func (h *PoolCoordinatorWebhook) onNodeCreate(n interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(n)
	if err == nil {
		h.nodePoolUpdateQueue.Add(key)
	}
}

func (h *PoolCoordinatorWebhook) onNodeDelete(n interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(n)
	if err == nil {
		h.nodePoolUpdateQueue.Add(key)
	}
}

func (h *PoolCoordinatorWebhook) onNodeUpdate(o interface{}, n interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(n)
	if err == nil {
		h.nodePoolUpdateQueue.Add(key)
	}
}

func (h *PoolCoordinatorWebhook) syncHandler(key string) error {
	_, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid resource key: %s", key)
	}

	node, err := h.nodeLister.Get(name)
	if node == nil && err != nil {
		// the node has been deleted
		h.nodepoolMap.DelNode(name)
		return nil
	}

	pool, ok := utils.NodeNodepool(node)
	if ok {
		if opool, ok := h.nodepoolMap.GetPool(name); ok {
			if opool == pool {
				return nil
			} else {
				h.nodepoolMap.Del(opool, name)
			}
		}
		h.nodepoolMap.Add(pool, name)
	} else {
		h.nodepoolMap.DelNode(name)
	}

	h.nodePoolUpdateQueue.Done(key)
	return nil
}

func (h *PoolCoordinatorWebhook) nodePoolWorker() {
	for {
		key, shutdown := h.nodePoolUpdateQueue.Get()
		if shutdown {
			klog.Info("nodepool work queue shutdown")
			return
		}

		if err := h.syncHandler(key.(string)); err != nil {
			if h.nodePoolUpdateQueue.NumRequeues(key) < MaxRetries {
				klog.Infof("error syncing event %v: %v", key, err)
				h.nodePoolUpdateQueue.AddRateLimited(key)
				h.nodePoolUpdateQueue.Done(key)
				continue
			}
			runtime.HandleError(err)
		}

		h.nodePoolUpdateQueue.Forget(key)
		h.nodePoolUpdateQueue.Done(key)
	}
}

func (h *PoolCoordinatorWebhook) Handler() []Handler {
	return []Handler{
		{MutatePath, h.serveMutatePods},
		{ValidatePath, h.serveMutatePods},
	}
}

func (h *PoolCoordinatorWebhook) Init(stopCH <-chan struct{}) {
	if !cache.WaitForCacheSync(stopCH, h.nodeSynced, h.leaseSynced) {
		klog.Error("sync poolcoordinator webhook timeout")
	}

	klog.Info("populate nodepool map")
	nl, err := h.nodeLister.List(labels.Everything())
	if err != nil {
		klog.Error(err)
	}
	h.nodepoolMap.Sync(nl)

	klog.Info("start nodepool maintenance worker")
	go h.nodePoolWorker()

	go func() {
		defer h.nodePoolUpdateQueue.ShutDown()
		<-stopCH
	}()
}
