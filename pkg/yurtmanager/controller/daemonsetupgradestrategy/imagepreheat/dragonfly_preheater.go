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
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	klog "k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonsetupgradestrategy"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/daemonsetupgradestrategy/imagepreheat/dragonfly"
)

const (
	// DragonflyPreheatJobsAnnotation records the Dragonfly preheat jobs created for a pod's target revision.
	// Format: "<revision>;<unix-seconds-created>;<id>,<id>,...".
	DragonflyPreheatJobsAnnotation = "apps.openyurt.io/dragonfly-preheat-jobs"

	dragonflyTokenKey       = "token"
	dragonflyMaxIPsPerJob   = 100
	dragonflyPreheatTimeout = 60 * time.Minute
	dragonflyPollInterval   = 10 * time.Second
)

type preheatPhase int

const (
	phasePending preheatPhase = iota
	phaseSucceeded
	phaseFailed
)

// dragonflyPreheater preheats DaemonSet images through Dragonfly. Pods that share a DaemonSet,
// a target revision and a NodePool are preheated together with one Dragonfly job per image,
// aimed at the IPs of their nodes, so each pool pulls the image across the WAN about once.
type dragonflyPreheater struct {
	c client.Client
	// secrets reads the token Secret; nil falls back to c.
	secrets            client.Reader
	df                 dragonfly.Client
	tokenSecret        types.NamespacedName
	insecureRegistries []string
	now                func() time.Time
}

// Ensure drives preheating for pod. It returns phasePending while Dragonfly jobs run, phaseSucceeded or
// phaseFailed (with a message) once they finish, and an error for transient problems that should be retried.
func (d *dragonflyPreheater) Ensure(ctx context.Context, ds *appsv1.DaemonSet, pod *corev1.Pod) (preheatPhase, string, error) {
	rev := GetPodNextHashVersion(pod)
	if annRev, created, ids, ok := decodeJobsAnnotation(pod.Annotations[DragonflyPreheatJobsAnnotation]); ok && annRev == rev {
		phase, msg, err := d.poll(ctx, created, ids)
		if err == nil && phase == phaseFailed {
			// Forget the failed jobs so that a new preheat request for this revision starts over.
			if err := d.clearJobs(ctx, pod); err != nil {
				return phasePending, "", err
			}
		}
		return phase, msg, err
	}
	return d.start(ctx, ds, pod, rev)
}

func (d *dragonflyPreheater) clearJobs(ctx context.Context, pod *corev1.Pod) error {
	patch := client.MergeFrom(pod.DeepCopy())
	delete(pod.Annotations, DragonflyPreheatJobsAnnotation)
	if err := d.c.Patch(ctx, pod, patch); err != nil {
		return fmt.Errorf("clear dragonfly jobs annotation on pod %s/%s: %w", pod.Namespace, pod.Name, err)
	}
	return nil
}

func (d *dragonflyPreheater) poll(ctx context.Context, created time.Time, ids []int64) (preheatPhase, string, error) {
	if d.now().Sub(created) > dragonflyPreheatTimeout {
		return phaseFailed, fmt.Sprintf("dragonfly preheat timed out after %s", dragonflyPreheatTimeout), nil
	}

	token, err := d.token(ctx)
	if err != nil {
		return phasePending, "", err
	}

	pending := false
	for _, id := range ids {
		status, err := d.df.GetJob(ctx, token, id)
		if err != nil {
			return phasePending, "", err
		}
		switch status.State {
		case dragonfly.StateFailure:
			return phaseFailed, fmt.Sprintf("dragonfly job %d failed: %s", id, status.Message), nil
		case dragonfly.StateSuccess:
		default:
			pending = true
		}
	}
	if pending {
		return phasePending, "", nil
	}
	return phaseSucceeded, "", nil
}

func (d *dragonflyPreheater) start(ctx context.Context, ds *appsv1.DaemonSet, pod *corev1.Pod, rev string) (preheatPhase, string, error) {
	node, err := d.node(ctx, pod.Spec.NodeName)
	if err != nil {
		return phasePending, "", err
	}
	if nodeInternalIP(node) == "" {
		return phaseFailed, fmt.Sprintf("node %s has no InternalIP", pod.Spec.NodeName), nil
	}

	token, err := d.token(ctx)
	if err != nil {
		return phasePending, "", err
	}

	batch, ips, err := d.batch(ctx, ds, pod, node, rev)
	if err != nil {
		return phasePending, "", err
	}

	var ids []int64
	for start := 0; start < len(ips); start += dragonflyMaxIPsPerJob {
		chunk := ips[start:min(start+dragonflyMaxIPsPerJob, len(ips))]
		for _, image := range daemonSetImages(ds) {
			url, err := dragonfly.ManifestURL(image, d.insecureRegistries)
			if err != nil {
				return phaseFailed, err.Error(), nil
			}
			id, err := d.df.CreatePreheatJob(ctx, token, dragonfly.PreheatRequest{URL: url, IPs: chunk})
			if err != nil {
				return phasePending, "", err
			}
			ids = append(ids, id)
		}
	}

	value := encodeJobsAnnotation(rev, d.now(), ids)
	for _, p := range batch {
		patch := client.MergeFrom(p.DeepCopy())
		if p.Annotations == nil {
			p.Annotations = map[string]string{}
		}
		p.Annotations[DragonflyPreheatJobsAnnotation] = value
		if err := d.c.Patch(ctx, p, patch); err != nil {
			return phasePending, "", fmt.Errorf("annotate pod %s/%s with dragonfly jobs: %w", p.Namespace, p.Name, err)
		}
	}
	klog.Infof("created dragonfly preheat jobs %v for daemonset %s/%s revision %s on %d nodes", ids, ds.Namespace, ds.Name, rev, len(ips))
	return phasePending, "", nil
}

// batch returns the pods to preheat together with pod (pod itself included, as the same pointer),
// sorted by node name, and their node ips. Pods on nodes without an InternalIP are marked failed and left out.
func (d *dragonflyPreheater) batch(ctx context.Context, ds *appsv1.DaemonSet, pod *corev1.Pod, node *corev1.Node, rev string) ([]*corev1.Pod, []string, error) {
	type member struct {
		pod *corev1.Pod
		ip  string
	}
	members := []member{{pod: pod, ip: nodeInternalIP(node)}}

	pool := node.Labels[projectinfo.GetNodePoolLabel()]
	if pool != "" {
		podList := &corev1.PodList{}
		if err := d.c.List(ctx, podList, client.InNamespace(ds.Namespace)); err != nil {
			return nil, nil, err
		}
		for i := range podList.Items {
			p := &podList.Items[i]
			if p.Name == pod.Name || !ownedBy(p, ds) || !needsPreheat(p, rev) {
				continue
			}
			n, err := d.node(ctx, p.Spec.NodeName)
			if err != nil {
				return nil, nil, err
			}
			if n.Labels[projectinfo.GetNodePoolLabel()] != pool {
				continue
			}
			ip := nodeInternalIP(n)
			if ip == "" {
				r := &ReconcileImagePull{c: d.c}
				if err := r.patchPodImageStatus(p, corev1.ConditionFalse, daemonsetupgradestrategy.PullImageFail,
					fmt.Sprintf("node %s has no InternalIP", n.Name)); err != nil {
					return nil, nil, err
				}
				continue
			}
			members = append(members, member{pod: p, ip: ip})
		}
	}

	sort.Slice(members, func(i, j int) bool { return members[i].pod.Spec.NodeName < members[j].pod.Spec.NodeName })
	pods := make([]*corev1.Pod, 0, len(members))
	ips := make([]string, 0, len(members))
	for _, m := range members {
		pods = append(pods, m.pod)
		ips = append(ips, m.ip)
	}
	return pods, ips, nil
}

func (d *dragonflyPreheater) token(ctx context.Context) (string, error) {
	reader := d.secrets
	if reader == nil {
		reader = d.c
	}
	secret := &corev1.Secret{}
	if err := reader.Get(ctx, d.tokenSecret, secret); err != nil {
		return "", fmt.Errorf("get dragonfly token secret %s: %w", d.tokenSecret, err)
	}
	token := string(secret.Data[dragonflyTokenKey])
	if token == "" {
		return "", fmt.Errorf("dragonfly token secret %s has no %q key", d.tokenSecret, dragonflyTokenKey)
	}
	return token, nil
}

func (d *dragonflyPreheater) node(ctx context.Context, name string) (*corev1.Node, error) {
	node := &corev1.Node{}
	if err := d.c.Get(ctx, types.NamespacedName{Name: name}, node); err != nil {
		return nil, fmt.Errorf("get node %s: %w", name, err)
	}
	return node, nil
}

// needsPreheat reports whether a user requested preheat for p's target revision rev and no Dragonfly
// jobs were created for it yet.
func needsPreheat(p *corev1.Pod, rev string) bool {
	if p.DeletionTimestamp != nil || GetPodNextHashVersion(p) != rev || !ExpectedPodImageReadyStatus(p, corev1.ConditionFalse) {
		return false
	}
	annRev, _, _, ok := decodeJobsAnnotation(p.Annotations[DragonflyPreheatJobsAnnotation])
	return !ok || annRev != rev
}

func ownedBy(p *corev1.Pod, ds *appsv1.DaemonSet) bool {
	for _, ref := range p.OwnerReferences {
		if ref.Kind == "DaemonSet" && ref.UID == ds.UID && ref.Name == ds.Name {
			return true
		}
	}
	return false
}

func nodeInternalIP(node *corev1.Node) string {
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP {
			return addr.Address
		}
	}
	return ""
}

func daemonSetImages(ds *appsv1.DaemonSet) []string {
	seen := map[string]bool{}
	var images []string
	for _, c := range append(append([]corev1.Container{}, ds.Spec.Template.Spec.Containers...), ds.Spec.Template.Spec.InitContainers...) {
		if !seen[c.Image] {
			seen[c.Image] = true
			images = append(images, c.Image)
		}
	}
	return images
}

func encodeJobsAnnotation(rev string, created time.Time, ids []int64) string {
	strs := make([]string, 0, len(ids))
	for _, id := range ids {
		strs = append(strs, strconv.FormatInt(id, 10))
	}
	return fmt.Sprintf("%s;%d;%s", rev, created.Unix(), strings.Join(strs, ","))
}

func decodeJobsAnnotation(v string) (string, time.Time, []int64, bool) {
	parts := strings.Split(v, ";")
	if len(parts) != 3 || parts[0] == "" || parts[2] == "" {
		return "", time.Time{}, nil, false
	}
	secs, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", time.Time{}, nil, false
	}
	var ids []int64
	for _, s := range strings.Split(parts[2], ",") {
		id, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return "", time.Time{}, nil, false
		}
		ids = append(ids, id)
	}
	return parts[0], time.Unix(secs, 0).UTC(), ids, true
}
