/*
Copyright 2016 The Kubernetes Authors.

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

package testutil

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	coordv1 "k8s.io/api/coordination/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
	v1apply "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/kubernetes/fake"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	clienttesting "k8s.io/client-go/testing"
	ref "k8s.io/client-go/tools/reference"
	utilnode "k8s.io/component-helpers/node/topology"
	"k8s.io/klog/v2"
	"k8s.io/utils/clock"
	testingclock "k8s.io/utils/clock/testing"
	ctlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type ImprovedFakeNodeHandler struct {
	sync.Mutex
	*ClientWrapper
	baseClient          ctlclient.Client
	DelegateNodeHandler *FakeNodeHandler
	*fake.Clientset
	UpdatedNodes        []*v1.Node
	UpdatedNodeStatuses []*v1.Node
}

func NewImprovedFakeNodeHandler(nodes []*v1.Node, pods *v1.PodList) *ImprovedFakeNodeHandler {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	clientBuilder := fakeclient.NewClientBuilder().WithScheme(scheme)
	for i := range nodes {
		clientBuilder.WithObjects(nodes[i])
	}
	if pods != nil {
		clientBuilder.WithLists(pods)
	}
	delegateClient := clientBuilder.Build()
	clientWrapper := NewClientWrapper(delegateClient, scheme)
	m := &ImprovedFakeNodeHandler{
		ClientWrapper: clientWrapper,
		baseClient:    delegateClient,
		DelegateNodeHandler: &FakeNodeHandler{
			runtimeClient: delegateClient,
		},
		Clientset:           fake.NewSimpleClientset(),
		UpdatedNodes:        make([]*v1.Node, 0),
		UpdatedNodeStatuses: make([]*v1.Node, 0),
	}
	m.ClientWrapper.NodeUpdateReactor = m.SyncNode
	m.DelegateNodeHandler.NodeUpdateReactor = m.SyncNode

	for i := range nodes {
		m.Clientset.Tracker().Add(nodes[i])
	}
	if pods != nil {
		m.Clientset.Tracker().Add(pods)
	}

	return m
}

func (m *ImprovedFakeNodeHandler) UpdateLease(lease *coordv1.Lease) error {
	if lease == nil {
		return nil
	}
	m.baseClient.Delete(context.TODO(), lease)
	if err := m.baseClient.Create(context.TODO(), lease); err != nil {
		return err
	}

	return nil
}

func (m *ImprovedFakeNodeHandler) UpdateNodeStatuses(updatedNodeStatuses map[string]v1.NodeStatus) error {
	nodeList := new(v1.NodeList)
	err := m.baseClient.List(context.TODO(), nodeList, &ctlclient.ListOptions{})
	if err != nil {
		return err
	}

	for _, node := range nodeList.Items {
		if status, ok := updatedNodeStatuses[node.Name]; ok {
			node.Status = status
			if err = m.baseClient.Status().Update(context.TODO(), &node, &ctlclient.UpdateOptions{}); err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *ImprovedFakeNodeHandler) UpdateNodes(updatedNodes []*v1.Node) error {
	for i := range updatedNodes {
		oldNode := new(v1.Node)
		if err := m.baseClient.Get(context.TODO(), ctlclient.ObjectKey{Name: updatedNodes[i].Name}, oldNode); err != nil {
			if err = m.baseClient.Create(context.TODO(), updatedNodes[i]); err != nil {
				return err
			}
		} else {
			if err = m.baseClient.Update(context.TODO(), updatedNodes[i], &ctlclient.UpdateOptions{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *ImprovedFakeNodeHandler) SyncNode(node *v1.Node, syncStatus bool) {
	m.Lock()
	defer m.Unlock()
	found := false
	node.ResourceVersion = ""
	for i := range m.UpdatedNodes {
		if m.UpdatedNodes[i].Name == node.Name {
			m.UpdatedNodes[i] = node
			found = true
			break
		}
	}

	if !found {
		m.UpdatedNodes = append(m.UpdatedNodes, node)
	}

	if syncStatus {
		m.UpdatedNodeStatuses = append(m.UpdatedNodeStatuses, node)
	}
}

// FakeLegacyHandler is a fake implementation of CoreV1Interface.
type FakeLegacyHandler struct {
	v1core.CoreV1Interface
	n *FakeNodeHandler
}

// Core returns fake CoreInterface.
func (m *ImprovedFakeNodeHandler) Core() v1core.CoreV1Interface {
	return &FakeLegacyHandler{m.Clientset.CoreV1(), m.DelegateNodeHandler}
}

// CoreV1 returns fake CoreV1Interface
func (m *ImprovedFakeNodeHandler) CoreV1() v1core.CoreV1Interface {
	return &FakeLegacyHandler{m.Clientset.CoreV1(), m.DelegateNodeHandler}
}

// Nodes return fake NodeInterfaces.
func (m *FakeLegacyHandler) Nodes() v1core.NodeInterface {
	return m.n
}

type ClientWrapper struct {
	sync.RWMutex
	RequestCount      int
	delegateClient    ctlclient.Client
	scheme            *runtime.Scheme
	actions           []clienttesting.Action
	PodUpdateReactor  func() error
	NodeUpdateReactor func(node *v1.Node, syncStatus bool)
}

func NewClientWrapper(client ctlclient.Client, scheme *runtime.Scheme) *ClientWrapper {
	return &ClientWrapper{
		delegateClient: client,
		scheme:         scheme,
		actions:        make([]clienttesting.Action, 0),
	}
}

func (m *ClientWrapper) Get(ctx context.Context, key ctlclient.ObjectKey, obj ctlclient.Object) error {
	m.Lock()
	defer m.Unlock()
	gvk, err := apiutil.GVKForObject(obj, m.scheme)
	if err != nil {
		klog.Infof("failed to get gvk for obj %v, %v", obj, err)
		return err
	}

	gvr, _ := meta.UnsafeGuessKindToResource(gvk)
	m.actions = append(m.actions, clienttesting.NewGetAction(gvr, key.Namespace, key.Name))
	defer func() {
		if strings.Contains(gvr.Resource, "node") {
			m.RequestCount++
		}
	}()
	return m.delegateClient.Get(ctx, key, obj)
}

func (m *ClientWrapper) List(ctx context.Context, list ctlclient.ObjectList, opts ...ctlclient.ListOption) error {
	m.Lock()
	defer m.Unlock()
	gvk, err := apiutil.GVKForObject(list, m.scheme)
	if err != nil {
		return err
	}
	gvk.Kind = strings.TrimSuffix(gvk.Kind, "list")

	gvr, _ := meta.UnsafeGuessKindToResource(gvk)
	m.actions = append(m.actions, clienttesting.NewListAction(gvr, gvk, "", metav1.ListOptions{}))
	defer func() {
		if strings.Contains(gvr.Resource, "node") {
			m.RequestCount++
		}
	}()
	return m.delegateClient.List(ctx, list, opts...)
}

func (m *ClientWrapper) Create(ctx context.Context, obj ctlclient.Object, opts ...ctlclient.CreateOption) error {
	m.Lock()
	defer m.Unlock()
	return m.delegateClient.Create(ctx, obj, opts...)
}

func (m *ClientWrapper) Update(ctx context.Context, obj ctlclient.Object, opts ...ctlclient.UpdateOption) error {
	m.Lock()
	defer m.Unlock()
	gvk, err := apiutil.GVKForObject(obj, m.scheme)
	if err != nil {
		return err
	}

	gvr, _ := meta.UnsafeGuessKindToResource(gvk)
	m.actions = append(m.actions, clienttesting.NewUpdateAction(gvr, obj.GetNamespace(), obj))
	defer func() {
		if strings.Contains(gvr.Resource, "node") {
			m.RequestCount++
		}
	}()

	if err := m.delegateClient.Update(ctx, obj, opts...); err != nil {
		return err
	}

	if strings.Contains(gvr.Resource, "node") && m.NodeUpdateReactor != nil {
		newNode := new(v1.Node)
		if err := m.delegateClient.Get(ctx, ctlclient.ObjectKey{Name: obj.GetName()}, newNode); err != nil {
			return err
		}
		m.NodeUpdateReactor(newNode, false)
	}
	return nil
}

// Delete deletes the given obj from Kubernetes cluster.
func (m *ClientWrapper) Delete(ctx context.Context, obj ctlclient.Object, opts ...ctlclient.DeleteOption) error {
	m.Lock()
	defer m.Unlock()
	gvk, err := apiutil.GVKForObject(obj, m.scheme)
	if err != nil {
		return err
	}

	gvr, _ := meta.UnsafeGuessKindToResource(gvk)
	m.actions = append(m.actions, clienttesting.NewDeleteAction(gvr, obj.GetNamespace(), obj.GetName()))
	defer func() {
		if strings.Contains(gvr.Resource, "node") {
			m.RequestCount++
		}
	}()
	return m.delegateClient.Delete(ctx, obj, opts...)
}

func (m *ClientWrapper) DeleteAllOf(ctx context.Context, obj ctlclient.Object, opts ...ctlclient.DeleteAllOfOption) error {
	return m.delegateClient.DeleteAllOf(ctx, obj, opts...)
}

// Patch patches the given obj in the Kubernetes cluster. obj must be a
// struct pointer so that obj can be updated with the content returned by the Server.
func (m *ClientWrapper) Patch(ctx context.Context, obj ctlclient.Object, patch ctlclient.Patch, opts ...ctlclient.PatchOption) error {
	m.Lock()
	defer m.Unlock()
	gvk, err := apiutil.GVKForObject(obj, m.scheme)
	if err != nil {
		return err
	}

	gvr, _ := meta.UnsafeGuessKindToResource(gvk)
	data, _ := patch.Data(obj)
	m.actions = append(m.actions, clienttesting.NewPatchAction(gvr, obj.GetNamespace(), obj.GetName(), patch.Type(), data))
	defer func() {
		if strings.Contains(gvr.Resource, "node") {
			m.RequestCount++
		}
	}()
	if err = m.delegateClient.Patch(ctx, obj, patch, opts...); err != nil {
		return err
	}

	if strings.Contains(gvr.Resource, "node") && m.NodeUpdateReactor != nil {
		newNode := new(v1.Node)
		if err := m.delegateClient.Get(ctx, ctlclient.ObjectKey{Name: obj.GetName()}, newNode); err != nil {
			return err
		}
		m.NodeUpdateReactor(newNode, false)
	}
	return nil
}

type fakeStatusWriter struct {
	client       *ClientWrapper
	statusWriter ctlclient.StatusWriter
}

func (sw *fakeStatusWriter) Update(ctx context.Context, obj ctlclient.Object, opts ...ctlclient.UpdateOption) error {
	// TODO(droot): This results in full update of the obj (spec + status). Need
	// a way to update status field only.
	gvk, err := apiutil.GVKForObject(obj, sw.client.scheme)
	if err != nil {
		return err
	}

	gvr, _ := meta.UnsafeGuessKindToResource(gvk)
	sw.client.Lock()
	sw.client.actions = append(sw.client.actions, clienttesting.NewUpdateSubresourceAction(gvr, "status", obj.GetNamespace(), obj))
	sw.client.Unlock()

	defer func() {
		if strings.Contains(gvr.Resource, "node") {
			sw.client.RequestCount++
		}
	}()

	if sw.client.PodUpdateReactor != nil {
		if gvr.Resource == "pods" {
			if err := sw.client.PodUpdateReactor(); err != nil {
				return err
			}
		}
	}
	if err = sw.statusWriter.Update(ctx, obj, opts...); err != nil {
		return err
	}

	if strings.Contains(gvr.Resource, "node") && sw.client.NodeUpdateReactor != nil {
		newNode := new(v1.Node)
		if err := sw.client.delegateClient.Get(ctx, ctlclient.ObjectKey{Name: obj.GetName()}, newNode); err != nil {
			return err
		}
		sw.client.NodeUpdateReactor(newNode, true)
	}
	return nil
}

func (sw *fakeStatusWriter) Patch(ctx context.Context, obj ctlclient.Object, patch ctlclient.Patch, opts ...ctlclient.PatchOption) error {
	// TODO(droot): This results in full update of the obj (spec + status). Need
	// a way to update status field only.
	patchData, err := patch.Data(obj)
	if err != nil {
		return err
	}

	gvk, err := apiutil.GVKForObject(obj, sw.client.scheme)
	if err != nil {
		return err
	}

	gvr, _ := meta.UnsafeGuessKindToResource(gvk)
	sw.client.Lock()
	sw.client.actions = append(sw.client.actions, clienttesting.NewPatchSubresourceAction(gvr, obj.GetNamespace(), obj.GetName(), patch.Type(), patchData, "status"))
	sw.client.Unlock()

	defer func() {
		if strings.Contains(gvr.Resource, "node") {
			sw.client.RequestCount++
		}
	}()

	if err = sw.statusWriter.Patch(ctx, obj, patch, opts...); err != nil {
		return err
	}

	if strings.Contains(gvr.Resource, "node") && sw.client.NodeUpdateReactor != nil {
		newNode := new(v1.Node)
		if err := sw.client.delegateClient.Get(ctx, ctlclient.ObjectKey{Name: obj.GetName()}, newNode); err != nil {
			return err
		}
		sw.client.NodeUpdateReactor(newNode, false)
	}
	return nil
}

func (m *ClientWrapper) Status() ctlclient.StatusWriter {
	return &fakeStatusWriter{
		client:       m,
		statusWriter: m.delegateClient.Status(),
	}
}

func (m *ClientWrapper) Scheme() *runtime.Scheme {
	return m.delegateClient.Scheme()
}

// RESTMapper returns the rest this client is using.
func (m *ClientWrapper) RESTMapper() meta.RESTMapper {
	return m.delegateClient.RESTMapper()
}

func (m *ClientWrapper) ClearActions() {
	m.Lock()
	defer m.Unlock()

	m.actions = make([]clienttesting.Action, 0)
}

func (m *ClientWrapper) Actions() []clienttesting.Action {
	m.RLock()
	defer m.RUnlock()
	fa := make([]clienttesting.Action, len(m.actions))
	copy(fa, m.actions)
	return fa
}

// FakeNodeHandler is a fake implementation of NodesInterface and NodeInterface. It
// allows test cases to have fine-grained control over mock behaviors. We also need
// PodsInterface and PodInterface to test list & delete pods, which is implemented in
// the embedded client.Fake field.
type FakeNodeHandler struct {
	RequestCount int

	// Synchronization
	lock           sync.Mutex
	DeleteWaitChan chan struct{}
	PatchWaitChan  chan struct{}

	runtimeClient     ctlclient.Client
	NodeUpdateReactor func(node *v1.Node, syncStatus bool)
}

// GetUpdatedNodesCopy returns a slice of Nodes with updates applied.
func (m *FakeNodeHandler) GetUpdatedNodesCopy() []*v1.Node {
	nodeList, err := m.List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return []*v1.Node{}
	}
	updatedNodesCopy := make([]*v1.Node, len(nodeList.Items), len(nodeList.Items))
	for i := range nodeList.Items {
		updatedNodesCopy[i] = &nodeList.Items[i]
	}
	return updatedNodesCopy
}

// GetUpdatedNodeStatusesCopy returns a slice of Nodes status with updates applied.
func (m *FakeNodeHandler) GetUpdatedNodeStatusesCopy() []*v1.NodeStatus {
	nodes := m.GetUpdatedNodesCopy()
	statuses := make([]*v1.NodeStatus, len(nodes), len(nodes))
	for i := range nodes {
		statuses[i] = &nodes[i].Status
	}

	return statuses
}

// Create adds a new Node to the fake store.
func (m *FakeNodeHandler) Create(ctx context.Context, node *v1.Node, _ metav1.CreateOptions) (*v1.Node, error) {
	m.lock.Lock()
	defer func() {
		m.RequestCount++
		m.lock.Unlock()
	}()

	if err := m.runtimeClient.Create(ctx, node, &ctlclient.CreateOptions{}); err != nil {
		return nil, err
	}

	newNode := new(v1.Node)
	if err := m.runtimeClient.Get(ctx, ctlclient.ObjectKey{Name: node.Name}, newNode); err != nil {
		return nil, err
	}

	return newNode, nil
}

// Get returns a Node from the fake store.
func (m *FakeNodeHandler) Get(ctx context.Context, name string, opts metav1.GetOptions) (*v1.Node, error) {
	m.lock.Lock()
	defer func() {
		m.RequestCount++
		m.lock.Unlock()
	}()

	newNode := new(v1.Node)
	if err := m.runtimeClient.Get(ctx, ctlclient.ObjectKey{Name: name}, newNode); err != nil {
		klog.Errorf("failed to get node(%s), %v", name, err)
		return nil, err
	}

	return newNode, nil
}

// List returns a list of Nodes from the fake store.
func (m *FakeNodeHandler) List(ctx context.Context, opts metav1.ListOptions) (*v1.NodeList, error) {
	m.lock.Lock()
	defer func() {
		m.RequestCount++
		m.lock.Unlock()
	}()

	clientOpts, err := convertListOptions(opts)
	if err != nil {
		return nil, err
	}

	nodeList := &v1.NodeList{}
	if err := m.runtimeClient.List(ctx, nodeList, &clientOpts); err != nil {
		return nil, err
	}

	return nodeList, nil
}

func convertListOptions(opts metav1.ListOptions) (ctlclient.ListOptions, error) {
	var clientOpts ctlclient.ListOptions
	if opts.LabelSelector != "" {
		if selector, err := labels.Parse(opts.LabelSelector); err != nil {
			return clientOpts, err
		} else {
			clientOpts.LabelSelector = selector
		}
	}

	if opts.FieldSelector != "" {
		if selector, err := fields.ParseSelector(opts.FieldSelector); err != nil {
			return clientOpts, err
		} else {
			clientOpts.FieldSelector = selector
		}
	}

	return clientOpts, nil
}

// Delete deletes a Node from the fake store.
func (m *FakeNodeHandler) Delete(ctx context.Context, id string, opt metav1.DeleteOptions) error {
	m.lock.Lock()
	defer func() {
		m.RequestCount++
		if m.DeleteWaitChan != nil {
			m.DeleteWaitChan <- struct{}{}
		}
		m.lock.Unlock()
	}()

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: id,
		},
	}
	return m.runtimeClient.Delete(ctx, node, &ctlclient.DeleteOptions{})
}

// DeleteCollection deletes a collection of Nodes from the fake store.
func (m *FakeNodeHandler) DeleteCollection(_ context.Context, opt metav1.DeleteOptions, listOpts metav1.ListOptions) error {
	return nil
}

// Update updates a Node in the fake store.
func (m *FakeNodeHandler) Update(ctx context.Context, node *v1.Node, _ metav1.UpdateOptions) (*v1.Node, error) {
	m.lock.Lock()
	defer func() {
		m.RequestCount++
		m.lock.Unlock()
	}()

	if err := m.runtimeClient.Update(ctx, node, &ctlclient.UpdateOptions{}); err != nil {
		return node, err
	}

	newNode := new(v1.Node)
	if err := m.runtimeClient.Get(ctx, ctlclient.ObjectKey{Name: node.Name}, newNode); err != nil {
		return newNode, err
	}

	if m.NodeUpdateReactor != nil {
		m.NodeUpdateReactor(newNode, false)
	}
	return newNode, nil
}

// UpdateStatus updates a status of a Node in the fake store.
func (m *FakeNodeHandler) UpdateStatus(ctx context.Context, node *v1.Node, _ metav1.UpdateOptions) (*v1.Node, error) {
	m.lock.Lock()
	defer func() {
		m.RequestCount++
		m.lock.Unlock()
	}()

	if err := m.runtimeClient.Status().Update(ctx, node, &ctlclient.UpdateOptions{}); err != nil {
		return node, err
	}

	newNode := new(v1.Node)
	if err := m.runtimeClient.Get(ctx, ctlclient.ObjectKey{Name: node.Name}, newNode); err != nil {
		return newNode, err
	}

	if m.NodeUpdateReactor != nil {
		m.NodeUpdateReactor(newNode, true)
	}

	return newNode, nil
}

// PatchStatus patches a status of a Node in the fake store.
func (m *FakeNodeHandler) PatchStatus(ctx context.Context, nodeName string, data []byte) (*v1.Node, error) {
	m.lock.Lock()
	defer func() {
		m.RequestCount++
		m.lock.Unlock()
	}()

	node := &v1.Node{}
	err := m.runtimeClient.Get(ctx, ctlclient.ObjectKey{Name: nodeName}, node)
	if err != nil {
		return nil, err
	}

	patch := ctlclient.RawPatch(types.StrategicMergePatchType, data)
	if err := m.runtimeClient.Status().Patch(ctx, node, patch); err != nil {
		return node, err
	}

	newNode := new(v1.Node)
	if err := m.runtimeClient.Get(ctx, ctlclient.ObjectKey{Name: node.Name}, newNode); err != nil {
		return newNode, err
	}

	if m.NodeUpdateReactor != nil {
		m.NodeUpdateReactor(newNode, false)
	}

	return newNode, nil
}

// Watch watches Nodes in a fake store.
func (m *FakeNodeHandler) Watch(_ context.Context, opts metav1.ListOptions) (watch.Interface, error) {
	return watch.NewFake(), nil
}

// Patch patches a Node in the fake store.
func (m *FakeNodeHandler) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, _ metav1.PatchOptions, subresources ...string) (*v1.Node, error) {
	m.lock.Lock()
	defer func() {
		m.RequestCount++
		if m.PatchWaitChan != nil {
			m.PatchWaitChan <- struct{}{}
		}
		m.lock.Unlock()
	}()

	node := new(v1.Node)
	err := m.runtimeClient.Get(ctx, ctlclient.ObjectKey{Name: name}, node)
	if err != nil {
		return nil, err
	}

	patch := ctlclient.RawPatch(pt, data)
	if err := m.runtimeClient.Patch(ctx, node, patch); err != nil {
		return node, err
	}

	newNode := new(v1.Node)
	if err := m.runtimeClient.Get(ctx, ctlclient.ObjectKey{Name: node.Name}, newNode); err != nil {
		return newNode, err
	}

	if m.NodeUpdateReactor != nil {
		m.NodeUpdateReactor(newNode, false)
	}

	return newNode, nil
}

// Apply applies a NodeApplyConfiguration to a Node in the fake store.
func (m *FakeNodeHandler) Apply(ctx context.Context, node *v1apply.NodeApplyConfiguration, opts metav1.ApplyOptions) (*v1.Node, error) {
	patchOpts := opts.ToPatchOptions()
	data, err := json.Marshal(node)
	if err != nil {
		return nil, err
	}
	name := node.Name
	if name == nil {
		return nil, fmt.Errorf("deployment.Name must be provided to Apply")
	}

	return m.Patch(ctx, *name, types.ApplyPatchType, data, patchOpts)
}

// ApplyStatus applies a status of a Node in the fake store.
func (m *FakeNodeHandler) ApplyStatus(ctx context.Context, node *v1apply.NodeApplyConfiguration, opts metav1.ApplyOptions) (*v1.Node, error) {
	patchOpts := opts.ToPatchOptions()
	data, err := json.Marshal(node)
	if err != nil {
		return nil, err
	}
	name := node.Name
	if name == nil {
		return nil, fmt.Errorf("deployment.Name must be provided to Apply")
	}

	return m.Patch(ctx, *name, types.ApplyPatchType, data, patchOpts, "status")
}

// FakeRecorder is used as a fake during testing.
type FakeRecorder struct {
	sync.Mutex
	source v1.EventSource
	Events []*v1.Event
	clock  clock.Clock
}

// Event emits a fake event to the fake recorder
func (f *FakeRecorder) Event(obj runtime.Object, eventtype, reason, message string) {
	f.generateEvent(obj, metav1.Now(), eventtype, reason, message)
}

// Eventf emits a fake formatted event to the fake recorder
func (f *FakeRecorder) Eventf(obj runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	f.Event(obj, eventtype, reason, fmt.Sprintf(messageFmt, args...))
}

// AnnotatedEventf emits a fake formatted event to the fake recorder
func (f *FakeRecorder) AnnotatedEventf(obj runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	f.Eventf(obj, eventtype, reason, messageFmt, args...)
}

func (f *FakeRecorder) generateEvent(obj runtime.Object, timestamp metav1.Time, eventtype, reason, message string) {
	scheme := runtime.NewScheme()
	clientgoscheme.AddToScheme(scheme)
	f.Lock()
	defer f.Unlock()
	ref, err := ref.GetReference(scheme, obj)
	if err != nil {
		klog.ErrorS(err, "Encountered error while getting reference")
		return
	}
	event := f.makeEvent(ref, eventtype, reason, message)
	event.Source = f.source
	if f.Events != nil {
		f.Events = append(f.Events, event)
	}
}

func (f *FakeRecorder) makeEvent(ref *v1.ObjectReference, eventtype, reason, message string) *v1.Event {
	t := metav1.Time{Time: f.clock.Now()}
	namespace := ref.Namespace
	if namespace == "" {
		namespace = metav1.NamespaceDefault
	}

	clientref := v1.ObjectReference{
		Kind:            ref.Kind,
		Namespace:       ref.Namespace,
		Name:            ref.Name,
		UID:             ref.UID,
		APIVersion:      ref.APIVersion,
		ResourceVersion: ref.ResourceVersion,
		FieldPath:       ref.FieldPath,
	}

	return &v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%v.%x", ref.Name, t.UnixNano()),
			Namespace: namespace,
		},
		InvolvedObject: clientref,
		Reason:         reason,
		Message:        message,
		FirstTimestamp: t,
		LastTimestamp:  t,
		Count:          1,
		Type:           eventtype,
	}
}

// NewFakeRecorder returns a pointer to a newly constructed FakeRecorder.
func NewFakeRecorder() *FakeRecorder {
	return &FakeRecorder{
		source: v1.EventSource{Component: "nodeControllerTest"},
		Events: []*v1.Event{},
		clock:  testingclock.NewFakeClock(time.Now()),
	}
}

// NewNode is a helper function for creating Nodes for testing.
func NewNode(name string) *v1.Node {
	return &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: v1.NodeStatus{
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceCPU):    resource.MustParse("10"),
				v1.ResourceName(v1.ResourceMemory): resource.MustParse("10G"),
			},
		},
	}
}

// NewPod is a helper function for creating Pods for testing.
func NewPod(name, host string) *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      name,
		},
		Spec: v1.PodSpec{
			NodeName: host,
		},
		Status: v1.PodStatus{
			Conditions: []v1.PodCondition{
				{
					Type:   v1.PodReady,
					Status: v1.ConditionTrue,
				},
			},
		},
	}

	return pod
}

// GetZones returns list of zones for all Nodes stored in FakeNodeHandler
func GetZones(nodeHandler *ImprovedFakeNodeHandler) []string {
	nodes, _ := nodeHandler.DelegateNodeHandler.List(context.TODO(), metav1.ListOptions{})
	zones := sets.NewString()
	for _, node := range nodes.Items {
		zones.Insert(utilnode.GetZoneKey(&node))
	}
	return zones.List()
}

// CreateZoneID returns a single zoneID for a given region and zone.
func CreateZoneID(region, zone string) string {
	return region + ":\x00:" + zone
}
