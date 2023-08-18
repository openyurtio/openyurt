/*
Copyright 2020 The OpenYurt Authors.

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

package yurtappdaemon

import (
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/openyurtio/openyurt/pkg/apis/apps"
	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
)

func createQueue() workqueue.RateLimitingInterface {
	return workqueue.NewRateLimitingQueue(workqueue.NewItemExponentialFailureRateLimiter(1*time.Millisecond, 1*time.Second))
}

func TestAddYurtAppDaemonToWorkQueue(t *testing.T) {
	tests := []struct {
		namespace string
		name      string
		q         workqueue.RateLimitingInterface
		added     int // the items in queue
	}{
		{
			"default",
			"test",
			createQueue(),
			1,
		},
	}

	for _, st := range tests {
		st := st
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", st.name)
			{
				addYurtAppDaemonToWorkQueue(st.namespace, st.name, st.q)
				get := st.q.Len()
				if get != st.added {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, st.added, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, st.added, get)

			}
		}
		t.Run(st.name, tf)
	}
}

func TestCreate(t *testing.T) {
	scheme := runtime.NewScheme()
	appsv1alpha1.AddToScheme(scheme)

	ep := EnqueueYurtAppDaemonForNodePool{
		client: fake.NewClientBuilder().
			WithScheme(scheme).
			Build(),
	}
	tests := []struct {
		name              string
		event             event.CreateEvent
		limitingInterface workqueue.RateLimitingInterface
		expect            string
	}{
		{
			"default",
			event.CreateEvent{
				Object: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "kube-system",
						Name:      "kube-proxy",
						Annotations: map[string]string{
							apps.AnnotationRefNodePool: "a",
						},
					},
					Spec: v1.PodSpec{},
				},
			},
			createQueue(),
			"",
		},
	}

	for _, st := range tests {
		st := st
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", st.name)
			{
				ep.Create(st.event, st.limitingInterface)
				get := st.expect
				if get != st.expect {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, st.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, st.expect, get)

			}
		}
		t.Run(st.name, tf)
	}
}

func TestUpdate(t *testing.T) {
	scheme := runtime.NewScheme()
	appsv1alpha1.AddToScheme(scheme)

	ep := EnqueueYurtAppDaemonForNodePool{
		client: fake.NewClientBuilder().
			WithScheme(scheme).
			Build(),
	}
	tests := []struct {
		name              string
		event             event.UpdateEvent
		limitingInterface workqueue.RateLimitingInterface
		expect            string
	}{
		{
			name: "default",
			event: event.UpdateEvent{
				ObjectOld: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "kube-system",
						Name:      "kube-proxy",
						Annotations: map[string]string{
							apps.AnnotationRefNodePool: "a",
						},
					},
					Spec: v1.PodSpec{},
				},
				ObjectNew: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "kube-system",
						Name:      "kube-proxy",
						Annotations: map[string]string{
							apps.AnnotationRefNodePool: "a",
						},
					},
					Spec: v1.PodSpec{},
				},
			},
			limitingInterface: createQueue(),
		},
	}

	for _, st := range tests {
		st := st
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", st.name)
			{
				ep.Update(st.event, st.limitingInterface)
				get := st.expect
				if get != st.expect {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, st.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, st.expect, get)

			}
		}
		t.Run(st.name, tf)
	}
}

func TestDelete(t *testing.T) {
	scheme := runtime.NewScheme()
	appsv1alpha1.AddToScheme(scheme)

	ep := EnqueueYurtAppDaemonForNodePool{
		client: fake.NewClientBuilder().
			WithScheme(scheme).
			Build(),
	}
	tests := []struct {
		name              string
		event             event.DeleteEvent
		limitingInterface workqueue.RateLimitingInterface
		expect            string
	}{
		{
			"default",
			event.DeleteEvent{
				Object: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "kube-system",
						Name:      "kube-proxy",
						Annotations: map[string]string{
							apps.AnnotationRefNodePool: "a",
						},
					},
					Spec: v1.PodSpec{},
				},
				DeleteStateUnknown: true,
			},
			createQueue(),
			"",
		},
	}

	for _, st := range tests {
		st := st
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", st.name)
			{
				ep.Delete(st.event, st.limitingInterface)
				get := st.expect
				if get != st.expect {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, st.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, st.expect, get)

			}
		}
		t.Run(st.name, tf)
	}
}

func TestGeneric(t *testing.T) {
	scheme := runtime.NewScheme()
	appsv1alpha1.AddToScheme(scheme)

	ep := EnqueueYurtAppDaemonForNodePool{
		client: fake.NewClientBuilder().
			WithScheme(scheme).
			Build(),
	}
	tests := []struct {
		name              string
		event             event.GenericEvent
		limitingInterface workqueue.RateLimitingInterface
		expect            string
	}{
		{
			"default",
			event.GenericEvent{
				Object: &v1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "kube-system",
						Name:      "kube-proxy",
						Annotations: map[string]string{
							apps.AnnotationRefNodePool: "a",
						},
					},
					Spec: v1.PodSpec{},
				},
			},
			createQueue(),
			"",
		},
	}

	for _, st := range tests {
		st := st
		tf := func(t *testing.T) {
			t.Parallel()
			t.Logf("\tTestCase: %s", st.name)
			{
				ep.Generic(st.event, st.limitingInterface)
				get := st.expect
				if get != st.expect {
					t.Fatalf("\t%s\texpect %v, but get %v", failed, st.expect, get)
				}
				t.Logf("\t%s\texpect %v, get %v", succeed, st.expect, get)

			}
		}
		t.Run(st.name, tf)
	}
}
