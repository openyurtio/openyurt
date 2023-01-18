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

package webhook

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/openyurtio/openyurt/pkg/controller/poolcoordinator/constant"
)

func genPodCrteateRequest() *http.Request {
	body := []byte(`{"kind":"AdmissionReview","apiVersion":"admission.k8s.io/v1","request":{"uid":"4378f340-83a6-4c8c-88bc-9b8788db02d2","kind":{"group":"","version":"v1","kind":"Pod"},"resource":{"group":"","version":"v1","resource":"pods"},"requestKind":{"group":"","version":"v1","kind":"Pod"},"requestResource":{"group":"","version":"v1","resource":"pods"},"name":"nginx","namespace":"default","operation":"CREATE","userInfo":{"username":"kubernetes-admin","groups":["system:masters","system:authenticated"]},"object":{"kind":"Pod","apiVersion":"v1","metadata":{"name":"nginx","creationTimestamp":null,"labels":{"run":"nginx"},"managedFields":[{"manager":"kubectl","operation":"Update","apiVersion":"v1","time":"2023-01-13T07:21:09Z","fieldsType":"FieldsV1","fieldsV1":{"f:metadata":{"f:labels":{".":{},"f:run":{}}},"f:spec":{"f:containers":{"k:{\"name\":\"nginx\"}":{".":{},"f:image":{},"f:imagePullPolicy":{},"f:name":{},"f:resources":{},"f:terminationMessagePath":{},"f:terminationMessagePolicy":{}}},"f:dnsPolicy":{},"f:enableServiceLinks":{},"f:nodeSelector":{".":{},"f:kubernetes.io/hostname":{}},"f:restartPolicy":{},"f:schedulerName":{},"f:securityContext":{},"f:terminationGracePeriodSeconds":{}}}}]},"spec":{"volumes":[{"name":"default-token-b7xcv","secret":{"secretName":"default-token-b7xcv"}}],"containers":[{"name":"nginx","image":"nginx","resources":{},"volumeMounts":[{"name":"default-token-b7xcv","readOnly":true,"mountPath":"/var/run/secrets/kubernetes.io/serviceaccount"}],"terminationMessagePath":"/dev/termination-log","terminationMessagePolicy":"File","imagePullPolicy":"Always"}],"restartPolicy":"Always","terminationGracePeriodSeconds":30,"dnsPolicy":"ClusterFirst","nodeSelector":{"kubernetes.io/hostname":"ai-ice-vm05"},"serviceAccountName":"default","serviceAccount":"default","securityContext":{},"schedulerName":"default-scheduler","tolerations":[{"key":"node.kubernetes.io/not-ready","operator":"Exists","effect":"NoExecute","tolerationSeconds":300},{"key":"node.kubernetes.io/unreachable","operator":"Exists","effect":"NoExecute","tolerationSeconds":300}],"priority":0,"enableServiceLinks":true},"status":{}},"oldObject":null,"dryRun":false,"options":{"kind":"CreateOptions","apiVersion":"meta.k8s.io/v1"}}}`)
	req, _ := http.NewRequest(http.MethodPost,
		"yurt-controller-manager-webhook.kube-system.svc:443/pool-coordinator-webhook-mutate?timeout=10s",
		bytes.NewReader(body))
	req.Header = http.Header{
		"Accept":          []string{"application/json, */*"},
		"Accept-Encoding": []string{"gzip"},
		"Content-Type":    []string{"application/json"},
		"User-Agent":      []string{"kube-apiserver-admission"},
	}
	req.Host = "yurt-controller-manager-webhook.kube-system.svc:443"
	req.RemoteAddr = "192.168.122.247:57550"
	req.RequestURI = "/pool-coordinator-webhook-mutate?timeout=10s"

	return req
}

func genPodValidateRequest(body []byte) *http.Request {
	req, _ := http.NewRequest(http.MethodPost,
		"yurt-controller-manager-webhook.kube-system.svc:443/pool-coordinator-webhook-validate",
		bytes.NewReader(body))
	req.Header = http.Header{
		"Accept":          []string{"application/json, */*"},
		"Accept-Encoding": []string{"gzip"},
		"Content-Type":    []string{"application/json"},
		"User-Agent":      []string{"kube-apiserver-admission"},
	}
	req.Host = "yurt-controller-manager-webhook.kube-system.svc:443"
	req.RemoteAddr = "192.168.122.247:57550"
	req.RequestURI = "/pool-coordinator-webhook-validate?timeout=10s"

	return req
}

func genPodDeleteRequestNormal() *http.Request {
	body := []byte(`{"kind":"AdmissionReview","apiVersion":"admission.k8s.io/v1","request":{"uid":"dd7a36a4-229a-4491-b879-af5fe09eaf0d","kind":{"group":"","version":"v1","kind":"Pod"},"resource":{"group":"","version":"v1","resource":"pods"},"requestKind":{"group":"","version":"v1","kind":"Pod"},"requestResource":{"group":"","version":"v1","resource":"pods"},"name":"nginx","namespace":"default","operation":"DELETE","userInfo":{"username":"system:node:ai-ice-vm05","groups":["system:nodes","system:authenticated"]},"object":null,"oldObject":{"kind":"Pod","apiVersion":"v1","metadata":{"name":"nginx","namespace":"default","uid":"41bf977f-0c95-4f97-8c32-baf8412b8a79","resourceVersion":"77701767","creationTimestamp":"2023-01-13T07:21:09Z","deletionTimestamp":"2023-01-13T08:27:00Z","deletionGracePeriodSeconds":0,"labels":{"run":"nginx"},"annotations":{"cni.projectcalico.org/containerID":"c5ab9fac8e3227d0cd8d6013029d6e5e01ae4d57ad040b2995b18f50efac8a8e","cni.projectcalico.org/podIP":"","cni.projectcalico.org/podIPs":""},"managedFields":[{"manager":"kubectl","operation":"Update","apiVersion":"v1","time":"2023-01-13T07:21:09Z","fieldsType":"FieldsV1","fieldsV1":{"f:metadata":{"f:labels":{".":{},"f:run":{}}},"f:spec":{"f:containers":{"k:{\"name\":\"nginx\"}":{".":{},"f:image":{},"f:imagePullPolicy":{},"f:name":{},"f:resources":{},"f:terminationMessagePath":{},"f:terminationMessagePolicy":{}}},"f:dnsPolicy":{},"f:enableServiceLinks":{},"f:nodeSelector":{".":{},"f:kubernetes.io/hostname":{}},"f:restartPolicy":{},"f:schedulerName":{},"f:securityContext":{},"f:terminationGracePeriodSeconds":{}}}},{"manager":"calico","operation":"Update","apiVersion":"v1","time":"2023-01-13T07:21:11Z","fieldsType":"FieldsV1","fieldsV1":{"f:metadata":{"f:annotations":{".":{},"f:cni.projectcalico.org/containerID":{},"f:cni.projectcalico.org/podIP":{},"f:cni.projectcalico.org/podIPs":{}}}}},{"manager":"kubelet","operation":"Update","apiVersion":"v1","time":"2023-01-13T08:27:02Z","fieldsType":"FieldsV1","fieldsV1":{"f:status":{"f:conditions":{"k:{\"type\":\"ContainersReady\"}":{".":{},"f:lastProbeTime":{},"f:lastTransitionTime":{},"f:message":{},"f:reason":{},"f:status":{},"f:type":{}},"k:{\"type\":\"Initialized\"}":{".":{},"f:lastProbeTime":{},"f:lastTransitionTime":{},"f:status":{},"f:type":{}},"k:{\"type\":\"Ready\"}":{".":{},"f:lastProbeTime":{},"f:lastTransitionTime":{},"f:message":{},"f:reason":{},"f:status":{},"f:type":{}}},"f:containerStatuses":{},"f:hostIP":{},"f:phase":{},"f:startTime":{}}}}]},"spec":{"volumes":[{"name":"default-token-b7xcv","secret":{"secretName":"default-token-b7xcv","defaultMode":420}}],"containers":[{"name":"nginx","image":"nginx","resources":{},"volumeMounts":[{"name":"default-token-b7xcv","readOnly":true,"mountPath":"/var/run/secrets/kubernetes.io/serviceaccount"}],"terminationMessagePath":"/dev/termination-log","terminationMessagePolicy":"File","imagePullPolicy":"Always"}],"restartPolicy":"Always","terminationGracePeriodSeconds":30,"dnsPolicy":"ClusterFirst","nodeSelector":{"kubernetes.io/hostname":"ai-ice-vm05"},"serviceAccountName":"default","serviceAccount":"default","nodeName":"ai-ice-vm05","securityContext":{},"schedulerName":"default-scheduler","tolerations":[{"key":"node.kubernetes.io/unreachable","operator":"Exists","effect":"NoExecute"},{"key":"node.kubernetes.io/not-ready","operator":"Exists","effect":"NoExecute"}],"priority":0,"enableServiceLinks":true},"status":{"phase":"Pending","conditions":[{"type":"Initialized","status":"True","lastProbeTime":null,"lastTransitionTime":"2023-01-13T07:21:15Z"},{"type":"Ready","status":"False","lastProbeTime":null,"lastTransitionTime":"2023-01-13T08:27:08Z","reason":"ContainersNotReady","message":"containers with unready status: [nginx]"},{"type":"ContainersReady","status":"False","lastProbeTime":null,"lastTransitionTime":"2023-01-13T08:27:08Z","reason":"ContainersNotReady","message":"containers with unready status: [nginx]"},{"type":"PodScheduled","status":"True","lastProbeTime":null,"lastTransitionTime":"2023-01-13T07:21:09Z"}],"hostIP":"192.168.122.90","startTime":"2023-01-13T07:21:15Z","containerStatuses":[{"name":"nginx","state":{"waiting":{"reason":"ContainerCreating"}},"lastState":{},"ready":false,"restartCount":0,"image":"nginx","imageID":"","started":false}],"qosClass":"BestEffort"}},"dryRun":false,"options":{"kind":"DeleteOptions","apiVersion":"meta.k8s.io/v1","gracePeriodSeconds":0,"preconditions":{"uid":"41bf977f-0c95-4f97-8c32-baf8412b8a79"}}}}`)
	return genPodValidateRequest(body)
}

func genPodDeleteRequestEviction() *http.Request {
	body := []byte(`{"kind":"AdmissionReview","apiVersion":"admission.k8s.io/v1","request":{"uid":"dd7a36a4-229a-4491-b879-af5fe09eaf0d","kind":{"group":"","version":"v1","kind":"Pod"},"resource":{"group":"","version":"v1","resource":"pods"},"requestKind":{"group":"","version":"v1","kind":"Pod"},"requestResource":{"group":"","version":"v1","resource":"pods"},"name":"nginx","namespace":"default","operation":"DELETE","userInfo":{"username":"system:serviceaccount:kube-system:node-controller","groups":["system:nodes","system:authenticated"]},"object":null,"oldObject":{"kind":"Pod","apiVersion":"v1","metadata":{"name":"nginx","namespace":"default","uid":"41bf977f-0c95-4f97-8c32-baf8412b8a79","resourceVersion":"77701767","creationTimestamp":"2023-01-13T07:21:09Z","deletionTimestamp":"2023-01-13T08:27:00Z","deletionGracePeriodSeconds":0,"labels":{"run":"nginx"},"annotations":{"cni.projectcalico.org/containerID":"c5ab9fac8e3227d0cd8d6013029d6e5e01ae4d57ad040b2995b18f50efac8a8e","cni.projectcalico.org/podIP":"","cni.projectcalico.org/podIPs":""},"managedFields":[{"manager":"kubectl","operation":"Update","apiVersion":"v1","time":"2023-01-13T07:21:09Z","fieldsType":"FieldsV1","fieldsV1":{"f:metadata":{"f:labels":{".":{},"f:run":{}}},"f:spec":{"f:containers":{"k:{\"name\":\"nginx\"}":{".":{},"f:image":{},"f:imagePullPolicy":{},"f:name":{},"f:resources":{},"f:terminationMessagePath":{},"f:terminationMessagePolicy":{}}},"f:dnsPolicy":{},"f:enableServiceLinks":{},"f:nodeSelector":{".":{},"f:kubernetes.io/hostname":{}},"f:restartPolicy":{},"f:schedulerName":{},"f:securityContext":{},"f:terminationGracePeriodSeconds":{}}}},{"manager":"calico","operation":"Update","apiVersion":"v1","time":"2023-01-13T07:21:11Z","fieldsType":"FieldsV1","fieldsV1":{"f:metadata":{"f:annotations":{".":{},"f:cni.projectcalico.org/containerID":{},"f:cni.projectcalico.org/podIP":{},"f:cni.projectcalico.org/podIPs":{}}}}},{"manager":"kubelet","operation":"Update","apiVersion":"v1","time":"2023-01-13T08:27:02Z","fieldsType":"FieldsV1","fieldsV1":{"f:status":{"f:conditions":{"k:{\"type\":\"ContainersReady\"}":{".":{},"f:lastProbeTime":{},"f:lastTransitionTime":{},"f:message":{},"f:reason":{},"f:status":{},"f:type":{}},"k:{\"type\":\"Initialized\"}":{".":{},"f:lastProbeTime":{},"f:lastTransitionTime":{},"f:status":{},"f:type":{}},"k:{\"type\":\"Ready\"}":{".":{},"f:lastProbeTime":{},"f:lastTransitionTime":{},"f:message":{},"f:reason":{},"f:status":{},"f:type":{}}},"f:containerStatuses":{},"f:hostIP":{},"f:phase":{},"f:startTime":{}}}}]},"spec":{"volumes":[{"name":"default-token-b7xcv","secret":{"secretName":"default-token-b7xcv","defaultMode":420}}],"containers":[{"name":"nginx","image":"nginx","resources":{},"volumeMounts":[{"name":"default-token-b7xcv","readOnly":true,"mountPath":"/var/run/secrets/kubernetes.io/serviceaccount"}],"terminationMessagePath":"/dev/termination-log","terminationMessagePolicy":"File","imagePullPolicy":"Always"}],"restartPolicy":"Always","terminationGracePeriodSeconds":30,"dnsPolicy":"ClusterFirst","nodeSelector":{"kubernetes.io/hostname":"ai-ice-vm05"},"serviceAccountName":"default","serviceAccount":"default","nodeName":"ai-ice-vm05","securityContext":{},"schedulerName":"default-scheduler","tolerations":[{"key":"node.kubernetes.io/unreachable","operator":"Exists","effect":"NoExecute"},{"key":"node.kubernetes.io/not-ready","operator":"Exists","effect":"NoExecute"}],"priority":0,"enableServiceLinks":true},"status":{"phase":"Pending","conditions":[{"type":"Initialized","status":"True","lastProbeTime":null,"lastTransitionTime":"2023-01-13T07:21:15Z"},{"type":"Ready","status":"False","lastProbeTime":null,"lastTransitionTime":"2023-01-13T08:27:08Z","reason":"ContainersNotReady","message":"containers with unready status: [nginx]"},{"type":"ContainersReady","status":"False","lastProbeTime":null,"lastTransitionTime":"2023-01-13T08:27:08Z","reason":"ContainersNotReady","message":"containers with unready status: [nginx]"},{"type":"PodScheduled","status":"True","lastProbeTime":null,"lastTransitionTime":"2023-01-13T07:21:09Z"}],"hostIP":"192.168.122.90","startTime":"2023-01-13T07:21:15Z","containerStatuses":[{"name":"nginx","state":{"waiting":{"reason":"ContainerCreating"}},"lastState":{},"ready":false,"restartCount":0,"image":"nginx","imageID":"","started":false}],"qosClass":"BestEffort"}},"dryRun":false,"options":{"kind":"DeleteOptions","apiVersion":"meta.k8s.io/v1","gracePeriodSeconds":0,"preconditions":{"uid":"41bf977f-0c95-4f97-8c32-baf8412b8a79"}}}}`)
	return genPodValidateRequest(body)
}

func TestPodMutate(t *testing.T) {
	fmt.Println(">>>> Test pod create")
	req := genPodCrteateRequest()

	h := NewPoolcoordinatorWebhook(nil, nil)

	fmt.Println(">>>>>>>> Test when node is nil")
	res := httptest.NewRecorder()
	h.serveMutatePods(res, req)

	fmt.Println(">>>>>>>> Test when node is not nil")
	req = genPodCrteateRequest()
	pa, _ := h.NewPodAdmission(req)
	pa.node = &corev1.Node{}
	pa.pod.Annotations = map[string]string{}
	pa.pod.Annotations[PodAutonomyAnnotation] = PodAutonomyNode
	rev, _ := pa.mutateReview()
	fmt.Printf("%v", rev)

}

func TestPodValidate(t *testing.T) {
	fmt.Println("Test pod validate")
	h := NewPoolcoordinatorWebhook(nil, nil)

	fmt.Println(">>>> Test normal pod delete")
	req := genPodDeleteRequestNormal()

	pa, _ := h.NewPodAdmission(req)
	rev, _ := pa.validateReview()
	fmt.Printf("%v\n", rev)
	if rev.Response.Allowed != true {
		t.Fail()
	}

	res := httptest.NewRecorder()
	h.serveValidatePods(res, req)

	fmt.Println(">>>> Test pod eviction")

	fmt.Println(">>>>>>>> Test when node is nil")
	req = genPodDeleteRequestEviction()

	pa, _ = h.NewPodAdmission(req)
	pa.node = &corev1.Node{}

	fmt.Println(">>>>>>>> Test when node is in autonomy (leagcy)")
	pa.node.Annotations = map[string]string{}
	pa.node.Annotations[constant.AnnotationKeyNodeAutonomy] = "true"
	rev, _ = pa.validateReview()
	fmt.Printf("%v", rev)
	if rev.Response.Allowed != false {
		t.Fail()
	}

	pa.node.Annotations = map[string]string{}

	fmt.Println(">>>>>>>> Test when pod autonomy mode is node")
	pa.pod.Annotations[PodAutonomyAnnotation] = PodAutonomyNode
	rev, _ = pa.validateReview()
	fmt.Printf("%v\n", rev)
	if rev.Response.Allowed != false {
		t.Fail()
	}

	fmt.Println(">>>>>>>> Test when pod autonomy mode is pool")
	pa.pod.Annotations[PodAutonomyAnnotation] = PodAutonomyPool
	pa.node.Labels = map[string]string{}
	pa.node.Labels[constant.LabelKeyNodePool] = "ut"
	pa.nodepoolMap.Add("ut", "ai-ice-vm05")
	rev, _ = pa.validateReview()
	fmt.Printf("%v\n", rev)
	if rev.Response.Allowed != false {
		t.Fail()
	}
}

func TestEnsureMutatingConfiguration(t *testing.T) {
	h := NewPoolcoordinatorWebhook(nil, nil)
	h.ensureMutatingConfiguration(&Certs{})
}

func TestEnsureValidatingConfiguration(t *testing.T) {
	h := NewPoolcoordinatorWebhook(nil, nil)
	h.ensureValidatingConfiguration(&Certs{})
}

func TestHandler(t *testing.T) {
	h := NewPoolcoordinatorWebhook(nil, nil)
	hs := h.Handler()
	if len(hs) <= 0 {
		t.Fail()
	}
}
