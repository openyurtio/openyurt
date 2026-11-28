/*
Copyright 2026 The OpenYurt Authors.

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

package imagepreheat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonsetupgradestrategy"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonsetupgradestrategy/imagepreheat/dragonfly"
)

const (
	dfAddr      = "http://dfm:8080"
	dfJobsURL   = dfAddr + "/oapi/v1/jobs"
	bigImage    = "kind-registry:5000/demo/big:v2"
	initImage   = "busybox:1.36"
	testDSName  = "ds1"
	testDSUID   = types.UID("u1")
	testRev     = "r2"
	tokenNS     = "dragonfly-system"
	tokenName   = "df-token"
	tokenSecret = "tok"
)

var dfFixedNow = time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

// dfRecorder records preheat job creations made against the mocked Dragonfly manager.
type dfRecorder struct {
	mu      sync.Mutex
	nextID  int64
	creates []map[string]any
}

func (r *dfRecorder) createdIPs() [][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out [][]string
	for _, body := range r.creates {
		var ips []string
		for _, ip := range body["args"].(map[string]any)["ips"].([]any) {
			ips = append(ips, ip.(string))
		}
		out = append(out, ips)
	}
	return out
}

func (r *dfRecorder) createdURLs() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, body := range r.creates {
		out = append(out, body["args"].(map[string]any)["url"].(string))
	}
	return out
}

func newDFServer(t *testing.T) (dragonfly.Client, *dfRecorder) {
	t.Helper()
	hc := &http.Client{}
	httpmock.ActivateNonDefault(hc)
	t.Cleanup(httpmock.DeactivateAndReset)
	rec := &dfRecorder{nextID: 100}
	httpmock.RegisterResponder(http.MethodPost, dfJobsURL, func(req *http.Request) (*http.Response, error) {
		assert.Equal(t, "Bearer "+tokenSecret, req.Header.Get("Authorization"))
		b, _ := io.ReadAll(req.Body)
		var body map[string]any
		require.NoError(t, json.Unmarshal(b, &body))
		rec.mu.Lock()
		rec.nextID++
		id := rec.nextID
		rec.creates = append(rec.creates, body)
		rec.mu.Unlock()
		return httpmock.NewStringResponse(http.StatusOK, fmt.Sprintf(`{"id":%d,"state":"PENDING"}`, id)), nil
	})
	return dragonfly.NewClient(dfAddr, hc), rec
}

func setJobState(id int64, state string) {
	httpmock.RegisterResponder(http.MethodGet, fmt.Sprintf("%s/%d", dfJobsURL, id),
		httpmock.NewStringResponder(http.StatusOK, fmt.Sprintf(`{"id":%d,"state":%q,"result":{"error":"boom"}}`, id, state)))
}

func dfTestDaemonSet(images ...string) *appsv1.DaemonSet {
	if len(images) == 0 {
		images = []string{bigImage, initImage}
	}
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: testDSName, Namespace: "default", UID: testDSUID},
		Spec: appsv1.DaemonSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "main", Image: images[0]}},
		}}},
	}
	if len(images) > 1 {
		ds.Spec.Template.Spec.InitContainers = []corev1.Container{{Name: "init", Image: images[1]}}
	}
	return ds
}

func dfTestNode(name, pool, ip string) *corev1.Node {
	n := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{}}}
	if pool != "" {
		n.Labels[projectinfo.GetNodePoolLabel()] = pool
	}
	if ip != "" {
		n.Status.Addresses = []corev1.NodeAddress{{Type: corev1.NodeHostName, Address: name}, {Type: corev1.NodeInternalIP, Address: ip}}
	}
	return n
}

func dfTestPod(node string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-" + node,
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1", Kind: "DaemonSet", Name: testDSName, UID: testDSUID,
			}},
		},
		Spec: corev1.PodSpec{NodeName: node},
		Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{
			Type: daemonsetupgradestrategy.PodNeedUpgrade, Status: corev1.ConditionTrue,
			Message: daemonsetupgradestrategy.VersionPrefix + testRev,
		}, {
			Type: daemonsetupgradestrategy.PodImageReady, Status: corev1.ConditionFalse,
			Message: daemonsetupgradestrategy.VersionPrefix + testRev,
		}}},
	}
}

func dfTokenSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: tokenName, Namespace: tokenNS},
		Data:       map[string][]byte{"token": []byte(tokenSecret)},
	}
}

// dfStandardObjects builds nodes a1,a2 (pool-a), b1 (pool-b), x1 (no pool), noip (pool-a, no ip)
// with one requested pod each, plus the DaemonSet and the token secret.
func dfStandardObjects() []client.Object {
	objs := []client.Object{
		dfTestDaemonSet(), dfTokenSecret(),
		dfTestNode("a1", "pool-a", "10.0.0.1"), dfTestNode("a2", "pool-a", "10.0.0.2"),
		dfTestNode("b1", "pool-b", "10.0.1.1"), dfTestNode("x1", "", "10.0.2.1"),
		dfTestNode("noip", "pool-a", ""),
	}
	for _, n := range []string{"a1", "a2", "b1", "x1", "noip"} {
		objs = append(objs, dfTestPod(n))
	}
	return objs
}

func newTestPreheater(t *testing.T, objs ...client.Object) (*dragonflyPreheater, client.Client, *dfRecorder) {
	t.Helper()
	c := fake.NewClientBuilder().WithObjects(objs...).WithStatusSubresource(&corev1.Pod{}).Build()
	df, rec := newDFServer(t)
	return &dragonflyPreheater{
		c:                  c,
		df:                 df,
		tokenSecret:        types.NamespacedName{Namespace: tokenNS, Name: tokenName},
		insecureRegistries: []string{"kind-registry:5000"},
		now:                func() time.Time { return dfFixedNow },
	}, c, rec
}

func getTestPod(t *testing.T, c client.Client, node string) *corev1.Pod {
	t.Helper()
	pod := &corev1.Pod{}
	require.NoError(t, c.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: "pod-" + node}, pod))
	return pod
}

func getTestDS(t *testing.T, c client.Client) *appsv1.DaemonSet {
	t.Helper()
	ds := &appsv1.DaemonSet{}
	require.NoError(t, c.Get(context.TODO(), types.NamespacedName{Namespace: "default", Name: testDSName}, ds))
	return ds
}

func ensureFor(t *testing.T, d *dragonflyPreheater, c client.Client, node string) (preheatPhase, string, error) {
	t.Helper()
	return d.Ensure(context.TODO(), getTestDS(t, c), getTestPod(t, c, node))
}

func postCount() int { return httpmock.GetCallCountInfo()["POST "+dfJobsURL] }

func getCount() int {
	n := 0
	for k, v := range httpmock.GetCallCountInfo() {
		if strings.HasPrefix(k, "GET ") {
			n += v
		}
	}
	return n
}

func TestJobsAnnotation_RoundTrip(t *testing.T) {
	v := encodeJobsAnnotation("r2", dfFixedNow, []int64{7, 8})
	rev, created, ids, ok := decodeJobsAnnotation(v)
	assert.True(t, ok)
	assert.Equal(t, "r2", rev)
	assert.True(t, created.Equal(dfFixedNow))
	assert.Equal(t, []int64{7, 8}, ids)

	for _, bad := range []string{"", "r2", "r2;abc;1", "r2;1;", "r2;1;x", ";1;1"} {
		_, _, _, ok := decodeJobsAnnotation(bad)
		assert.False(t, ok, bad)
	}
}

func TestDragonfly_BatchesSamePoolOnly(t *testing.T) {
	d, c, rec := newTestPreheater(t, dfStandardObjects()...)

	phase, _, err := ensureFor(t, d, c, "a1")
	require.NoError(t, err)
	assert.Equal(t, phasePending, phase)

	assert.Equal(t, 2, postCount(), "one job per image")
	assert.Equal(t, [][]string{{"10.0.0.1", "10.0.0.2"}, {"10.0.0.1", "10.0.0.2"}}, rec.createdIPs())
	assert.ElementsMatch(t, []string{
		"http://kind-registry:5000/v2/demo/big/manifests/v2",
		"https://registry-1.docker.io/v2/library/busybox/manifests/1.36",
	}, rec.createdURLs())

	for _, n := range []string{"a1", "a2"} {
		ann := getTestPod(t, c, n).Annotations[DragonflyPreheatJobsAnnotation]
		assert.Equal(t, encodeJobsAnnotation(testRev, dfFixedNow, []int64{101, 102}), ann, n)
	}
	assert.Empty(t, getTestPod(t, c, "b1").Annotations[DragonflyPreheatJobsAnnotation])
	assert.Empty(t, getTestPod(t, c, "x1").Annotations[DragonflyPreheatJobsAnnotation])
}

func TestDragonfly_SecondPodReusesBatchJobs(t *testing.T) {
	d, c, _ := newTestPreheater(t, dfStandardObjects()...)
	_, _, err := ensureFor(t, d, c, "a1")
	require.NoError(t, err)
	setJobState(101, dragonfly.StatePending)
	setJobState(102, dragonfly.StatePending)

	phase, _, err := ensureFor(t, d, c, "a2")
	require.NoError(t, err)
	assert.Equal(t, phasePending, phase)
	assert.Equal(t, 2, postCount(), "no new jobs for the second pod")
	assert.Equal(t, 2, getCount())
}

func TestDragonfly_PollSuccess(t *testing.T) {
	d, c, _ := newTestPreheater(t, dfStandardObjects()...)
	_, _, err := ensureFor(t, d, c, "a1")
	require.NoError(t, err)
	setJobState(101, dragonfly.StateSuccess)
	setJobState(102, dragonfly.StateSuccess)

	phase, _, err := ensureFor(t, d, c, "a1")
	require.NoError(t, err)
	assert.Equal(t, phaseSucceeded, phase)
}

func TestDragonfly_PollFailure(t *testing.T) {
	d, c, _ := newTestPreheater(t, dfStandardObjects()...)
	_, _, err := ensureFor(t, d, c, "a1")
	require.NoError(t, err)
	setJobState(101, dragonfly.StateSuccess)
	setJobState(102, dragonfly.StateFailure)

	phase, msg, err := ensureFor(t, d, c, "a1")
	require.NoError(t, err)
	assert.Equal(t, phaseFailed, phase)
	assert.Contains(t, msg, "102")
	assert.Contains(t, msg, "boom")
}

func TestDragonfly_PollPending(t *testing.T) {
	d, c, _ := newTestPreheater(t, dfStandardObjects()...)
	_, _, err := ensureFor(t, d, c, "a1")
	require.NoError(t, err)
	setJobState(101, dragonfly.StateSuccess)
	setJobState(102, dragonfly.StatePending)

	phase, _, err := ensureFor(t, d, c, "a1")
	require.NoError(t, err)
	assert.Equal(t, phasePending, phase)
}

func TestDragonfly_StaleRevisionAnnotationIgnored(t *testing.T) {
	objs := dfStandardObjects()
	for _, o := range objs {
		if p, ok := o.(*corev1.Pod); ok && p.Name == "pod-a1" {
			p.Annotations = map[string]string{DragonflyPreheatJobsAnnotation: encodeJobsAnnotation("r1", dfFixedNow, []int64{7})}
		}
	}
	d, c, _ := newTestPreheater(t, objs...)

	phase, _, err := ensureFor(t, d, c, "a1")
	require.NoError(t, err)
	assert.Equal(t, phasePending, phase)
	assert.Equal(t, 2, postCount())
	assert.True(t, strings.HasPrefix(getTestPod(t, c, "a1").Annotations[DragonflyPreheatJobsAnnotation], testRev+";"))
}

func TestDragonfly_Timeout(t *testing.T) {
	objs := dfStandardObjects()
	for _, o := range objs {
		if p, ok := o.(*corev1.Pod); ok && p.Name == "pod-a1" {
			p.Annotations = map[string]string{DragonflyPreheatJobsAnnotation: encodeJobsAnnotation(testRev, dfFixedNow.Add(-61*time.Minute), []int64{7})}
		}
	}
	d, c, _ := newTestPreheater(t, objs...)

	phase, msg, err := ensureFor(t, d, c, "a1")
	require.NoError(t, err)
	assert.Equal(t, phaseFailed, phase)
	assert.Contains(t, msg, "timed out")
	assert.Equal(t, 0, postCount())
	assert.Equal(t, 0, getCount())
}

func TestDragonfly_CreateJobHTTPError_Requeues(t *testing.T) {
	d, c, _ := newTestPreheater(t, dfStandardObjects()...)
	httpmock.RegisterResponder(http.MethodPost, dfJobsURL, httpmock.NewStringResponder(http.StatusInternalServerError, "down"))

	before := getTestPod(t, c, "a1").Status.Conditions
	_, _, err := ensureFor(t, d, c, "a1")
	assert.Error(t, err)
	for _, n := range []string{"a1", "a2"} {
		assert.Empty(t, getTestPod(t, c, n).Annotations[DragonflyPreheatJobsAnnotation], n)
	}
	assert.Equal(t, before, getTestPod(t, c, "a1").Status.Conditions)
}

func TestDragonfly_MissingTokenSecret(t *testing.T) {
	var objs []client.Object
	for _, o := range dfStandardObjects() {
		if _, ok := o.(*corev1.Secret); !ok {
			objs = append(objs, o)
		}
	}
	d, c, _ := newTestPreheater(t, objs...)
	_, _, err := ensureFor(t, d, c, "a1")
	assert.Error(t, err)
	assert.Equal(t, 0, postCount())
}

func TestDragonfly_NodeWithoutInternalIP(t *testing.T) {
	d, c, rec := newTestPreheater(t, dfStandardObjects()...)
	_, _, err := ensureFor(t, d, c, "a1")
	require.NoError(t, err)

	assert.Equal(t, [][]string{{"10.0.0.1", "10.0.0.2"}, {"10.0.0.1", "10.0.0.2"}}, rec.createdIPs())
	noip := getTestPod(t, c, "noip")
	assert.Empty(t, noip.Annotations[DragonflyPreheatJobsAnnotation])
	var cond *corev1.PodCondition
	for i := range noip.Status.Conditions {
		if noip.Status.Conditions[i].Type == daemonsetupgradestrategy.PodImageReady {
			cond = &noip.Status.Conditions[i]
		}
	}
	require.NotNil(t, cond)
	assert.Equal(t, corev1.ConditionFalse, cond.Status)
	assert.Equal(t, daemonsetupgradestrategy.PullImageFail, cond.Reason)
	assert.Contains(t, cond.Message, "noip")
}

func TestDragonfly_CurrentPodWithoutInternalIP(t *testing.T) {
	d, c, _ := newTestPreheater(t, dfStandardObjects()...)
	phase, msg, err := ensureFor(t, d, c, "noip")
	require.NoError(t, err)
	assert.Equal(t, phaseFailed, phase)
	assert.Contains(t, msg, "noip")
}

func TestDragonfly_NodeWithoutPoolBatchedAlone(t *testing.T) {
	d, c, rec := newTestPreheater(t, dfStandardObjects()...)
	_, _, err := ensureFor(t, d, c, "x1")
	require.NoError(t, err)
	assert.Equal(t, [][]string{{"10.0.2.1"}, {"10.0.2.1"}}, rec.createdIPs())
}

func TestDragonfly_SkipsPodsNotRequested(t *testing.T) {
	objs := dfStandardObjects()
	for _, o := range objs {
		if p, ok := o.(*corev1.Pod); ok && p.Name == "pod-a2" {
			p.Status.Conditions = p.Status.Conditions[:1] // upgrade available, preheat not requested
		}
	}
	d, c, rec := newTestPreheater(t, objs...)
	_, _, err := ensureFor(t, d, c, "a1")
	require.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.1"}, rec.createdIPs()[0])
}

func TestDragonfly_ChunksOver100(t *testing.T) {
	objs := []client.Object{dfTestDaemonSet(), dfTokenSecret()}
	for i := 0; i < 150; i++ {
		n := fmt.Sprintf("n%03d", i)
		objs = append(objs, dfTestNode(n, "pool-a", fmt.Sprintf("10.1.%d.%d", i/250, i%250+1)), dfTestPod(n))
	}
	d, c, rec := newTestPreheater(t, objs...)
	_, _, err := ensureFor(t, d, c, "n000")
	require.NoError(t, err)

	assert.Equal(t, 4, postCount())
	var sizes []int
	for _, ips := range rec.createdIPs() {
		sizes = append(sizes, len(ips))
	}
	assert.ElementsMatch(t, []int{100, 100, 50, 50}, sizes)
}

func TestDragonfly_DedupImages(t *testing.T) {
	objs := dfStandardObjects()
	objs[0] = dfTestDaemonSet(bigImage, bigImage)
	d, c, _ := newTestPreheater(t, objs...)
	_, _, err := ensureFor(t, d, c, "a1")
	require.NoError(t, err)
	assert.Equal(t, 1, postCount())
}

func TestDragonfly_UpdatesCurrentPodInPlace(t *testing.T) {
	d, c, _ := newTestPreheater(t, dfStandardObjects()...)
	pod := getTestPod(t, c, "a1")
	_, _, err := d.Ensure(context.TODO(), getTestDS(t, c), pod)
	require.NoError(t, err)
	assert.NotEmpty(t, pod.Annotations[DragonflyPreheatJobsAnnotation], "caller's pod object must carry the new annotation and resourceVersion")
	assert.Equal(t, getTestPod(t, c, "a1").ResourceVersion, pod.ResourceVersion)
}

func TestDragonfly_FailureClearsAnnotationForRetry(t *testing.T) {
	d, c, _ := newTestPreheater(t, dfStandardObjects()...)
	_, _, err := ensureFor(t, d, c, "a1")
	require.NoError(t, err)
	setJobState(101, dragonfly.StateFailure)
	setJobState(102, dragonfly.StateSuccess)

	pod := getTestPod(t, c, "a1")
	phase, _, err := d.Ensure(context.TODO(), getTestDS(t, c), pod)
	require.NoError(t, err)
	assert.Equal(t, phaseFailed, phase)
	assert.Empty(t, pod.Annotations[DragonflyPreheatJobsAnnotation], "caller's pod must drop the failed jobs")
	assert.Empty(t, getTestPod(t, c, "a1").Annotations[DragonflyPreheatJobsAnnotation])

	// a new preheat request for the same revision creates new jobs instead of re-reading the failed ones
	_, _, err = ensureFor(t, d, c, "a1")
	require.NoError(t, err)
	assert.Equal(t, 4, postCount())
}

func TestDragonfly_TimeoutClearsAnnotationForRetry(t *testing.T) {
	objs := dfStandardObjects()
	for _, o := range objs {
		if p, ok := o.(*corev1.Pod); ok && p.Name == "pod-a1" {
			p.Annotations = map[string]string{DragonflyPreheatJobsAnnotation: encodeJobsAnnotation(testRev, dfFixedNow.Add(-61*time.Minute), []int64{7})}
		}
	}
	d, c, _ := newTestPreheater(t, objs...)
	phase, _, err := ensureFor(t, d, c, "a1")
	require.NoError(t, err)
	assert.Equal(t, phaseFailed, phase)
	assert.Empty(t, getTestPod(t, c, "a1").Annotations[DragonflyPreheatJobsAnnotation])
}
