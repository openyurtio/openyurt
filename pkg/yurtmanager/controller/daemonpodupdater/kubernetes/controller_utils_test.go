/*
Copyright 2015 The Kubernetes Authors.

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

package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/apimachinery/pkg/util/uuid"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	clientscheme "k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	utiltesting "k8s.io/client-go/util/testing"
)

// NewFakeControllerExpectationsLookup creates a fake store for PodExpectations.
func NewFakeControllerExpectationsLookup(ttl time.Duration) (*ControllerExpectations, *clock.FakeClock) {
	fakeTime := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	fakeClock := clock.NewFakeClock(fakeTime)
	ttlPolicy := &cache.TTLPolicy{TTL: ttl, Clock: fakeClock}
	ttlStore := cache.NewFakeExpirationStore(
		ExpKeyFunc, nil, ttlPolicy, fakeClock)
	return &ControllerExpectations{ttlStore}, fakeClock
}

func newReplicationController(replicas int) *v1.ReplicationController {
	rc := &v1.ReplicationController{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1"},
		ObjectMeta: metav1.ObjectMeta{
			UID:             uuid.NewUUID(),
			Name:            "foobar",
			Namespace:       metav1.NamespaceDefault,
			ResourceVersion: "18",
		},
		Spec: v1.ReplicationControllerSpec{
			Replicas: func() *int32 { i := int32(replicas); return &i }(),
			Selector: map[string]string{"foo": "bar"},
			Template: &v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"name": "foo",
						"type": "production",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Image:                  "foo/bar",
							TerminationMessagePath: v1.TerminationMessagePathDefault,
							ImagePullPolicy:        v1.PullIfNotPresent,
						},
					},
					RestartPolicy: v1.RestartPolicyAlways,
					DNSPolicy:     v1.DNSDefault,
					NodeSelector: map[string]string{
						"baz": "blah",
					},
				},
			},
		},
	}
	return rc
}

func TestControllerExpectations(t *testing.T) {
	ttl := 30 * time.Second
	e, fakeClock := NewFakeControllerExpectationsLookup(ttl)
	// In practice we can't really have add and delete expectations since we only either create or
	// delete replicas in one rc pass, and the rc goes to sleep soon after until the expectations are
	// either fulfilled or timeout.
	adds, dels := 10, 30
	rc := newReplicationController(1)

	// RC fires off adds and deletes at apiserver, then sets expectations
	rcKey, err := KeyFunc(rc)
	assert.NoError(t, err, "Couldn't get key for object %#v: %v", rc, err)

	e.SetExpectations(rcKey, adds, dels)
	var wg sync.WaitGroup
	for i := 0; i < adds+1; i++ {
		wg.Add(1)
		go func() {
			// In prod this can happen either because of a failed create by the rc
			// or after having observed a create via informer
			e.CreationObserved(rcKey)
			wg.Done()
		}()
	}
	wg.Wait()

	// There are still delete expectations
	assert.False(t, e.SatisfiedExpectations(rcKey), "Rc will sync before expectations are met")

	for i := 0; i < dels+1; i++ {
		wg.Add(1)
		go func() {
			e.DeletionObserved(rcKey)
			wg.Done()
		}()
	}
	wg.Wait()

	// Expectations have been surpassed
	podExp, exists, err := e.GetExpectations(rcKey)
	assert.NoError(t, err, "Could not get expectations for rc, exists %v and err %v", exists, err)
	assert.True(t, exists, "Could not get expectations for rc, exists %v and err %v", exists, err)

	add, del := podExp.GetExpectations()
	assert.Equal(t, int64(-1), add, "Unexpected pod expectations %#v", podExp)
	assert.Equal(t, int64(-1), del, "Unexpected pod expectations %#v", podExp)
	assert.True(t, e.SatisfiedExpectations(rcKey), "Expectations are met but the rc will not sync")

	// Next round of rc sync, old expectations are cleared
	e.SetExpectations(rcKey, 1, 2)
	podExp, exists, err = e.GetExpectations(rcKey)
	assert.NoError(t, err, "Could not get expectations for rc, exists %v and err %v", exists, err)
	assert.True(t, exists, "Could not get expectations for rc, exists %v and err %v", exists, err)
	add, del = podExp.GetExpectations()

	assert.Equal(t, int64(1), add, "Unexpected pod expectations %#v", podExp)
	assert.Equal(t, int64(2), del, "Unexpected pod expectations %#v", podExp)

	// Expectations have expired because of ttl
	fakeClock.Step(ttl + 1)
	assert.True(t, e.SatisfiedExpectations(rcKey),
		"Expectations should have expired but didn't")
}

func TestCreatePods(t *testing.T) {
	ns := metav1.NamespaceDefault
	body := runtime.EncodeOrDie(clientscheme.Codecs.LegacyCodec(v1.SchemeGroupVersion), &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "empty_pod"}})
	fakeHandler := utiltesting.FakeHandler{
		StatusCode:   200,
		ResponseBody: string(body),
	}
	testServer := httptest.NewServer(&fakeHandler)
	defer testServer.Close()
	clientset := clientset.NewForConfigOrDie(&restclient.Config{Host: testServer.URL, ContentConfig: restclient.ContentConfig{GroupVersion: &schema.GroupVersion{Group: "", Version: "v1"}}})

	podControl := RealPodControl{
		KubeClient: clientset,
		Recorder:   &record.FakeRecorder{},
	}

	controllerSpec := newReplicationController(1)
	controllerRef := metav1.NewControllerRef(controllerSpec, v1.SchemeGroupVersion.WithKind("ReplicationController"))

	// Make sure createReplica sends a POST to the apiserver with a pod from the controllers pod template
	err := podControl.CreatePods(context.TODO(), ns, controllerSpec.Spec.Template, controllerSpec, controllerRef)
	assert.NoError(t, err, "unexpected error: %v", err)

	expectedPod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels:       controllerSpec.Spec.Template.Labels,
			GenerateName: fmt.Sprintf("%s-", controllerSpec.Name),
		},
		Spec: controllerSpec.Spec.Template.Spec,
	}
	fakeHandler.ValidateRequest(t, "/api/v1/namespaces/default/pods", "POST", nil)
	var actualPod = &v1.Pod{}
	err = json.Unmarshal([]byte(fakeHandler.RequestBody), actualPod)
	assert.NoError(t, err, "unexpected error: %v", err)
	assert.True(t, apiequality.Semantic.DeepDerivative(&expectedPod, actualPod),
		"Body: %s", fakeHandler.RequestBody)
}

func TestCreatePodsWithGenerateName(t *testing.T) {
	ns := metav1.NamespaceDefault
	body := runtime.EncodeOrDie(clientscheme.Codecs.LegacyCodec(v1.SchemeGroupVersion), &v1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "empty_pod"}})
	fakeHandler := utiltesting.FakeHandler{
		StatusCode:   200,
		ResponseBody: string(body),
	}
	testServer := httptest.NewServer(&fakeHandler)
	defer testServer.Close()
	clientset := clientset.NewForConfigOrDie(&restclient.Config{Host: testServer.URL, ContentConfig: restclient.ContentConfig{GroupVersion: &schema.GroupVersion{Group: "", Version: "v1"}}})

	podControl := RealPodControl{
		KubeClient: clientset,
		Recorder:   &record.FakeRecorder{},
	}

	controllerSpec := newReplicationController(1)
	controllerRef := metav1.NewControllerRef(controllerSpec, v1.SchemeGroupVersion.WithKind("ReplicationController"))

	// Make sure createReplica sends a POST to the apiserver with a pod from the controllers pod template
	generateName := "hello-"
	err := podControl.CreatePodsWithGenerateName(context.TODO(), ns, controllerSpec.Spec.Template, controllerSpec, controllerRef, generateName)
	assert.NoError(t, err, "unexpected error: %v", err)

	expectedPod := v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels:          controllerSpec.Spec.Template.Labels,
			GenerateName:    generateName,
			OwnerReferences: []metav1.OwnerReference{*controllerRef},
		},
		Spec: controllerSpec.Spec.Template.Spec,
	}

	fakeHandler.ValidateRequest(t, "/api/v1/namespaces/default/pods", "POST", nil)
	var actualPod = &v1.Pod{}
	err = json.Unmarshal([]byte(fakeHandler.RequestBody), actualPod)
	assert.NoError(t, err, "unexpected error: %v", err)
	assert.True(t, apiequality.Semantic.DeepDerivative(&expectedPod, actualPod),
		"Body: %s", fakeHandler.RequestBody)
}

func TestDeletePodsAllowsMissing(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	podControl := RealPodControl{
		KubeClient: fakeClient,
		Recorder:   &record.FakeRecorder{},
	}

	controllerSpec := newReplicationController(1)

	err := podControl.DeletePod(context.TODO(), "namespace-name", "podName", controllerSpec)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestComputeHash(t *testing.T) {
	collisionCount := int32(1)
	otherCollisionCount := int32(2)
	maxCollisionCount := int32(math.MaxInt32)
	tests := []struct {
		name                string
		template            *v1.PodTemplateSpec
		collisionCount      *int32
		otherCollisionCount *int32
	}{
		{
			name:                "simple",
			template:            &v1.PodTemplateSpec{},
			collisionCount:      &collisionCount,
			otherCollisionCount: &otherCollisionCount,
		},
		{
			name:                "using math.MaxInt64",
			template:            &v1.PodTemplateSpec{},
			collisionCount:      nil,
			otherCollisionCount: &maxCollisionCount,
		},
	}

	for _, test := range tests {
		hash := ComputeHash(test.template, test.collisionCount)
		otherHash := ComputeHash(test.template, test.otherCollisionCount)

		assert.NotEqual(t, hash, otherHash, "expected different hashes but got the same: %d", hash)
	}
}
