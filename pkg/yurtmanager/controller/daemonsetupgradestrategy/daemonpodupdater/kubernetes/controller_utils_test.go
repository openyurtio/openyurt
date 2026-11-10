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
	"errors"
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
	"k8s.io/apimachinery/pkg/util/uuid"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	clientscheme "k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	utiltesting "k8s.io/client-go/util/testing"
	testingclock "k8s.io/utils/clock/testing"
)

// NewFakeControllerExpectationsLookup creates a fake store for PodExpectations.
func NewFakeControllerExpectationsLookup(ttl time.Duration) (*ControllerExpectations, *testingclock.FakeClock) {
	fakeTime := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	fakeClock := testingclock.NewFakeClock(fakeTime)
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
	clientset := clientset.NewForConfigOrDie(&restclient.Config{Host: testServer.URL, ContentConfig: restclient.ContentConfig{ContentType: runtime.ContentTypeJSON, GroupVersion: &schema.GroupVersion{Group: "", Version: "v1"}}})

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
	clientset := clientset.NewForConfigOrDie(&restclient.Config{Host: testServer.URL, ContentConfig: restclient.ContentConfig{ContentType: runtime.ContentTypeJSON, GroupVersion: &schema.GroupVersion{Group: "", Version: "v1"}}})

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
func TestControllerExpectations_Methods(t *testing.T) {
	e := NewControllerExpectations() // Tests NewControllerExpectations
	key := "test-controller"

	// Test ExpectCreations
	err := e.ExpectCreations(key, 2)
	assert.NoError(t, err)
	exp, exists, err := e.GetExpectations(key)
	assert.NoError(t, err)
	assert.True(t, exists)
	add, del := exp.GetExpectations()
	assert.Equal(t, int64(2), add)
	assert.Equal(t, int64(0), del)
	assert.False(t, e.SatisfiedExpectations(key))

	// Test RaiseExpectations
	e.RaiseExpectations(key, 1, 1)
	exp, _, _ = e.GetExpectations(key)
	add, del = exp.GetExpectations()
	assert.Equal(t, int64(3), add)
	assert.Equal(t, int64(1), del)

	// Test ExpectDeletions
	err = e.ExpectDeletions(key, 2)
	assert.NoError(t, err)
	exp, _, _ = e.GetExpectations(key)
	add, del = exp.GetExpectations()
	assert.Equal(t, int64(0), add) // ExpectDeletions overrides add with 0
	assert.Equal(t, int64(2), del)
	assert.False(t, e.SatisfiedExpectations(key))

	// Fulfill expectations
	e.DeletionObserved(key)
	e.DeletionObserved(key)
	assert.True(t, e.SatisfiedExpectations(key))

	// Test DeleteExpectations
	e.DeleteExpectations(key)
	_, exists, _ = e.GetExpectations(key)
	assert.False(t, exists)
	assert.True(t, e.SatisfiedExpectations(key)) // Satisfied when no expectations
}

func TestFakePodControl(t *testing.T) {
	fakeControl := &FakePodControl{}

	// Test CreatePods
	spec := &v1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"foo": "bar"}}}
	controllerRef := &metav1.OwnerReference{Name: "test-controller"}

	err := fakeControl.CreatePods(context.TODO(), "default", spec, nil, controllerRef)
	assert.NoError(t, err)
	assert.Equal(t, 1, fakeControl.CreateCallCount)
	assert.Len(t, fakeControl.Templates, 1)
	assert.Len(t, fakeControl.ControllerRefs, 1)

	// Test PatchPod
	patchData := []byte("patch")
	err = fakeControl.PatchPod(context.TODO(), "default", "pod-name", patchData)
	assert.NoError(t, err)
	assert.Len(t, fakeControl.Patches, 1)
	assert.Equal(t, patchData, fakeControl.Patches[0])

	// Test DeletePod
	err = fakeControl.DeletePod(context.TODO(), "default", "pod-name", nil)
	assert.NoError(t, err)
	assert.Len(t, fakeControl.DeletePodName, 1)
	assert.Equal(t, "pod-name", fakeControl.DeletePodName[0])

	// Test error injection
	fakeControl.Err = errors.New("fake error")
	err = fakeControl.PatchPod(context.TODO(), "default", "pod-name", patchData)
	assert.EqualError(t, err, "fake error")
	err = fakeControl.CreatePods(context.TODO(), "default", spec, nil, controllerRef)
	assert.EqualError(t, err, "fake error")
	err = fakeControl.DeletePod(context.TODO(), "default", "pod-name", nil)
	assert.EqualError(t, err, "fake error")

	// Test limit
	fakeControl.Clear()
	fakeControl.Err = nil
	fakeControl.CreateLimit = 1
	err = fakeControl.CreatePods(context.TODO(), "default", spec, nil, controllerRef)
	assert.NoError(t, err)
	err = fakeControl.CreatePods(context.TODO(), "default", spec, nil, controllerRef)
	assert.ErrorContains(t, err, "limit 1 already reached")

	// Test Clear
	fakeControl.Clear()
	assert.Equal(t, 0, fakeControl.CreateCallCount)
	assert.Len(t, fakeControl.Templates, 0)
	assert.Len(t, fakeControl.ControllerRefs, 0)
	assert.Len(t, fakeControl.Patches, 0)
	assert.Len(t, fakeControl.DeletePodName, 0)
}

func TestRealPodControl_PatchPod(t *testing.T) {
	ns := metav1.NamespaceDefault
	fakeHandler := utiltesting.FakeHandler{
		StatusCode:   200,
		ResponseBody: `{}`,
	}
	testServer := httptest.NewServer(&fakeHandler)
	defer testServer.Close()
	clientset := clientset.NewForConfigOrDie(&restclient.Config{Host: testServer.URL, ContentConfig: restclient.ContentConfig{ContentType: runtime.ContentTypeJSON, GroupVersion: &schema.GroupVersion{Group: "", Version: "v1"}}})

	podControl := RealPodControl{
		KubeClient: clientset,
		Recorder:   &record.FakeRecorder{},
	}

	err := podControl.PatchPod(context.TODO(), ns, "podName", []byte("{}"))
	assert.NoError(t, err)
}

func TestValidateControllerRef(t *testing.T) {
	tests := []struct {
		name          string
		controllerRef *metav1.OwnerReference
		wantErr       bool
		errContains   string
	}{
		{
			name:          "nil ref",
			controllerRef: nil,
			wantErr:       true,
			errContains:   "controllerRef is nil",
		},
		{
			name:          "empty api version",
			controllerRef: &metav1.OwnerReference{Kind: "ReplicaSet"},
			wantErr:       true,
			errContains:   "empty APIVersion",
		},
		{
			name:          "empty kind",
			controllerRef: &metav1.OwnerReference{APIVersion: "v1"},
			wantErr:       true,
			errContains:   "empty Kind",
		},
		{
			name: "controller not set to true",
			controllerRef: &metav1.OwnerReference{
				APIVersion: "v1",
				Kind:       "ReplicaSet",
				Controller: func() *bool { b := false; return &b }(),
			},
			wantErr:     true,
			errContains: "Controller is not set to true",
		},
		{
			name: "blockOwnerDeletion not set",
			controllerRef: &metav1.OwnerReference{
				APIVersion: "v1",
				Kind:       "ReplicaSet",
				Controller: func() *bool { b := true; return &b }(),
			},
			wantErr:     true,
			errContains: "BlockOwnerDeletion is not set",
		},
		{
			name: "valid ref",
			controllerRef: &metav1.OwnerReference{
				APIVersion:         "v1",
				Kind:               "ReplicaSet",
				Controller:         func() *bool { b := true; return &b }(),
				BlockOwnerDeletion: func() *bool { b := true; return &b }(),
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateControllerRef(tt.controllerRef)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetPodFromTemplate(t *testing.T) {
	template := &v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      map[string]string{"foo": "bar"},
			Annotations: map[string]string{"ann": "val"},
			Finalizers:  []string{"fin1"},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{{Name: "c1"}},
		},
	}
	controllerRef := &metav1.OwnerReference{
		APIVersion:         "v1",
		Kind:               "ReplicaSet",
		Controller:         func() *bool { b := true; return &b }(),
		BlockOwnerDeletion: func() *bool { b := true; return &b }(),
	}

	// Test success
	parentObject := newReplicationController(1)
	pod, err := GetPodFromTemplate(template, parentObject, controllerRef)
	assert.NoError(t, err)
	assert.Equal(t, "foobar-", pod.GenerateName)
	assert.Equal(t, map[string]string{"foo": "bar"}, pod.Labels)
	assert.Equal(t, map[string]string{"ann": "val"}, pod.Annotations)
	assert.Equal(t, []string{"fin1"}, pod.Finalizers)
	assert.Len(t, pod.OwnerReferences, 1)

	// Test missing ObjectMeta
	type dummyObject struct{ runtime.Object }
	_, err = GetPodFromTemplate(template, &dummyObject{}, controllerRef)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not have ObjectMeta")
}

func TestRealPodControl_DeletePod_GenericError(t *testing.T) {
	ns := metav1.NamespaceDefault
	fakeHandler := utiltesting.FakeHandler{
		StatusCode:   500,
		ResponseBody: `{}`,
	}
	testServer := httptest.NewServer(&fakeHandler)
	defer testServer.Close()
	clientset := clientset.NewForConfigOrDie(&restclient.Config{Host: testServer.URL, ContentConfig: restclient.ContentConfig{ContentType: runtime.ContentTypeJSON, GroupVersion: &schema.GroupVersion{Group: "", Version: "v1"}}})

	podControl := RealPodControl{
		KubeClient: clientset,
		Recorder:   &record.FakeRecorder{},
	}

	controllerSpec := newReplicationController(1)
	err := podControl.DeletePod(context.TODO(), ns, "podName", controllerSpec)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unable to delete pods")

	// Test with parent object without ObjectMeta
	type dummyObject struct{ runtime.Object }
	err = podControl.DeletePod(context.TODO(), ns, "podName", &dummyObject{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not have ObjectMeta")
}

func TestRealPodControl_CreatePods_Failures(t *testing.T) {
	ns := metav1.NamespaceDefault
	fakeHandler := utiltesting.FakeHandler{
		StatusCode:   500,
		ResponseBody: `{}`,
	}
	testServer := httptest.NewServer(&fakeHandler)
	defer testServer.Close()
	clientset := clientset.NewForConfigOrDie(&restclient.Config{Host: testServer.URL, ContentConfig: restclient.ContentConfig{ContentType: runtime.ContentTypeJSON, GroupVersion: &schema.GroupVersion{Group: "", Version: "v1"}}})

	podControl := RealPodControl{
		KubeClient: clientset,
		Recorder:   &record.FakeRecorder{},
	}

	controllerSpec := newReplicationController(1)
	controllerRef := metav1.NewControllerRef(controllerSpec, v1.SchemeGroupVersion.WithKind("ReplicationController"))

	// Test CreatePods where create fails
	err := podControl.CreatePods(context.TODO(), ns, controllerSpec.Spec.Template, controllerSpec, controllerRef)
	assert.Error(t, err) // Server returns 500

	// Test CreatePods without labels
	badTemplate := controllerSpec.Spec.Template.DeepCopy()
	badTemplate.Labels = nil
	err = podControl.CreatePods(context.TODO(), ns, badTemplate, controllerSpec, controllerRef)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unable to create pods, no labels")
}

func TestGetPodsPrefix(t *testing.T) {
	// Name shorter than DNS subdomain limit (253)
	prefix := getPodsPrefix("short-name")
	assert.Equal(t, "short-name-", prefix)

	// Very long name
	longName := ""
	for i := 0; i < 300; i++ {
		longName += "a"
	}
	prefix = getPodsPrefix(longName)
	assert.Equal(t, longName, prefix) // the dash is omitted if name + dash is invalid
}
