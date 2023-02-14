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
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	leaselisterv1 "k8s.io/client-go/listers/coordination/v1"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/controller/poolcoordinator/constant"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	EnvCertDirName                       = "WEBHOOK_CERT_DIR"
	EnvServicePort                       = "WEBHOOK_SERVICE_PORT"
	EnvPoolReadyNodeNumberRatioThreshold = "WEBHOOK_POOL_READY_NODE_NUMBER_RATIO_THRESHOLD"
	EnvNamespace                         = "WEBHOOK_NAMESPACE"
	EnvServiceName                       = "WEBHOOK_SERVICE_NAME"
	EnvPodValidatingConfigurationName    = "WEBHOOK_POD_VALIDATING_CONFIGURATION_NAME"
	EnvPodValidatingName                 = "WEBHOOK_POD_VALIDATING_NAME"
	EnvPodValidatingPath                 = "WEBHOOK_POD_VALIDATING_PATH"

	DefaultCertDir                           = "/tmp/k8s-webhook-server/serving-certs"
	DefaultServicePort                       = "9443"
	DefaultContentType                       = "application/json"
	DefaultNamespace                         = "kube-system"
	DefaultServiceName                       = "yurt-controller-manager-webhook"
	DefaultPodValidatingConfigurationName    = "yurt-controller-manager"
	DefaultPodValidatingName                 = "vpoolcoordinator.openyurt.io"
	DefaultPodValidatingPath                 = "/pool-coordinator-webhook-validate"
	DefaultPoolReadyNodeNumberRatioThreshold = 0.35

	ValidatePath = "/pool-coordinator-webhook-validate"
	MutatePath   = "/pool-coordinator-webhook-mutate"

	NodeLeaseDurationSeconds = 40

	// LabelCurrentNodePool indicates which nodepool the node is currently belonging to
	LabelCurrentNodePool = "apps.openyurt.io/nodepool"
)

const (
	msgNodeNotInNodePool                 string = "node is not in nodePool"
	msgNodePoolStatusError               string = "get nodePool info error"
	msgPoolHasTooFewReadyNodes           string = "nodePool has too few ready nodes"
	msgPodAvailablePoolAndNodeIsNotAlive string = "node is not alive in a pool, eviction approved"
	msgPodNormalDelete                   string = "delete operation is not node controller, delete approved"
)

func GetEnv(key, defaultValue string) string {
	if p := os.Getenv(key); len(p) > 0 {
		return p
	}
	return defaultValue
}

func GetPoolReadyNodeNumberRatioThreshold() float64 {
	if p := os.Getenv(EnvPoolReadyNodeNumberRatioThreshold); len(p) > 0 {
		num, err := strconv.ParseFloat(p, 64)
		if err != nil {
			klog.Errorf("GetPoolReadyNodeNumberRatioThreshold ParseFloat num %s error: %+v", p, err)
			return 0
		}
		return num
	}
	return DefaultPoolReadyNodeNumberRatioThreshold
}

// ParseRequest extracts an AdmissionReview from an http.Request if possible
func ParseRequest(r http.Request) (*admissionv1.AdmissionReview, error) {
	contentType := r.Header.Get("Content-Type")
	if contentType != DefaultContentType {
		return nil, fmt.Errorf("Content-Type: %q should be %q", contentType, contentType)
	}

	var buf bytes.Buffer
	if n, err := buf.ReadFrom(r.Body); err != nil || n == 0 {
		return nil, fmt.Errorf("failed to read admission request %s, read %d bytes, %v", util.ReqString(&r), n, err)
	}

	var a admissionv1.AdmissionReview
	if err := json.Unmarshal(buf.Bytes(), &a); err != nil {
		return nil, fmt.Errorf("could not parse admission review request: %v", err)
	}

	if a.Request == nil {
		return nil, fmt.Errorf("admission review can't be used: Request field is nil")
	}

	return &a, nil
}

// ReviewResponse builds an admission review with given parameter
func ReviewResponse(uid types.UID, allowed bool, httpCode int32, reason string) *admissionv1.AdmissionReview {
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

// PatchReviewResponse builds an admission review with given json patch
func PatchReviewResponse(uid types.UID, patch []byte) (*admissionv1.AdmissionReview, error) {
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

// NodeIsAlive return true if node is alive, otherwise is false
func NodeIsAlive(leaseLister leaselisterv1.LeaseNamespaceLister, nodeName string) bool {
	lease, err := leaseLister.Get(nodeName)
	if err != nil {
		klog.Errorf("NodeIsAlive get node %s lease error: %+v", nodeName, err)
		return false
	}

	// check lease update time
	diff := time.Now().Sub(lease.Spec.RenewTime.Time)
	if diff.Seconds() > NodeLeaseDurationSeconds {
		return false
	}

	// check lease if delegate or not
	if lease.Annotations != nil && lease.Annotations[constant.DelegateHeartBeat] == "true" {
		return false
	}
	return true
}

// CountAliveNode return number of node alive
func CountAliveNode(leaseLister leaselisterv1.LeaseNamespaceLister, nodes []string) int {
	cnt := 0
	for _, n := range nodes {
		if NodeIsAlive(leaseLister, n) {
			cnt++
		}
	}
	return cnt
}
