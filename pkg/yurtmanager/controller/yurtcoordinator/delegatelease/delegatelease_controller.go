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

package delegatelease

import (
	"context"
	"time"

	coordv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	nodeutil "github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/node"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtcoordinator/constant"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtcoordinator/utils"
)

type ReconcileDelegateLease struct {
	client.Client
	dlClient kubernetes.Interface
	ldc      *utils.LeaseDelegatedCounter
	delLdc   *utils.LeaseDelegatedCounter
}

// Add creates a delegatelease controller and add it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(_ context.Context, cfg *appconfig.CompletedConfig, mgr manager.Manager) error {
	r := &ReconcileDelegateLease{
		ldc:    utils.NewLeaseDelegatedCounter(),
		delLdc: utils.NewLeaseDelegatedCounter(),
	}
	c, err := controller.New(names.DelegateLeaseController, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: int(cfg.ComponentConfig.DelegateLeaseController.ConcurrentDelegateLeaseWorkers),
	})
	if err != nil {
		return err
	}
	err = c.Watch(&source.Kind{Type: &coordv1.Lease{}}, &handler.EnqueueRequestForObject{})

	return err
}

// Reconcile reads that state of Node in cluster and makes changes if node autonomy state has been changed
func (r *ReconcileDelegateLease) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	lea := &coordv1.Lease{}
	if err := r.Get(ctx, req.NamespacedName, lea); err != nil {
		klog.V(4).Infof("lease not found for %q\n", req.NamespacedName)
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	if lea.Namespace != corev1.NamespaceNodeLease {
		return reconcile.Result{}, nil
	}

	klog.V(5).Infof("lease request: %s\n", lea.Name)

	nval, nok := lea.Annotations[constant.DelegateHeartBeat]

	if nok && nval == "true" {
		r.ldc.Inc(lea.Name)
		if r.ldc.Counter(lea.Name) >= constant.LeaseDelegationThreshold {
			r.taintNodeNotSchedulable(ctx, lea.Name)
			r.checkNodeReadyConditionAndSetIt(ctx, lea.Name)
			r.delLdc.Reset(lea.Name)
		}
	} else {
		if r.delLdc.Counter(lea.Name) == 0 {
			r.deTaintNodeNotSchedulable(ctx, lea.Name)
		}
		r.ldc.Reset(lea.Name)
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileDelegateLease) taintNodeNotSchedulable(ctx context.Context, name string) {
	node, err := r.dlClient.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		klog.Error(err)
		return
	}
	r.doTaintNodeNotSchedulable(node)
}

func (r *ReconcileDelegateLease) doTaintNodeNotSchedulable(node *corev1.Node) *corev1.Node {
	taints := node.Spec.Taints
	if utils.TaintKeyExists(taints, constant.NodeNotSchedulableTaint) {
		klog.V(4).Infof("taint %s: key %s already exists, nothing to do\n", node.Name, constant.NodeNotSchedulableTaint)
		return node
	}
	nn := node.DeepCopy()
	t := corev1.Taint{
		Key:    constant.NodeNotSchedulableTaint,
		Value:  "true",
		Effect: corev1.TaintEffectNoSchedule,
	}
	nn.Spec.Taints = append(nn.Spec.Taints, t)
	var err error
	if r.dlClient != nil {
		nn, err = r.dlClient.CoreV1().Nodes().Update(context.TODO(), nn, metav1.UpdateOptions{})
		if err != nil {
			klog.Error(err)
		}
	}
	return nn
}

func (r *ReconcileDelegateLease) deTaintNodeNotSchedulable(ctx context.Context, name string) {
	node, err := r.dlClient.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		klog.Error(err)
		return
	}
	r.doDeTaintNodeNotSchedulable(node)
}

func (r *ReconcileDelegateLease) doDeTaintNodeNotSchedulable(node *corev1.Node) *corev1.Node {
	taints := node.Spec.Taints
	taints, deleted := utils.DeleteTaintsByKey(taints, constant.NodeNotSchedulableTaint)
	if !deleted {
		r.delLdc.Inc(node.Name)
		klog.V(4).Infof("detaint %s: no key %s exists, nothing to do\n", node.Name, constant.NodeNotSchedulableTaint)
		return node
	}
	nn := node.DeepCopy()
	nn.Spec.Taints = taints
	var err error
	if r.dlClient != nil {
		nn, err = r.dlClient.CoreV1().Nodes().Update(context.TODO(), nn, metav1.UpdateOptions{})
		if err != nil {
			klog.Error(err)
		} else {
			r.delLdc.Inc(node.Name)
		}
	}
	return nn
}

// If node lease was delegate, check node ready condition.
// If ready condition is unknown, update to true.
// Because when node ready condition is unknown, the native kubernetes will set node.kubernetes.io/unreachable taints in node,
// and the pod will be evict after 300s, that's not what we're trying to do in delegate lease.
// Up to now, it's only happen when leader in nodePool is disconnected with cloud, and this node will be not-ready,
// because in an election cycle, the node lease will not delegate to cloud, after 40s, the kubernetes will set unknown.
func (r *ReconcileDelegateLease) checkNodeReadyConditionAndSetIt(ctx context.Context, name string) {
	node, err := r.dlClient.CoreV1().Nodes().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		klog.Error(err)
		return
	}

	// check node ready condition
	newNode := node.DeepCopy()
	_, currentCondition := nodeutil.GetNodeCondition(&newNode.Status, corev1.NodeReady)
	if currentCondition.Status != corev1.ConditionUnknown {
		// don't need to reset node ready condition
		return
	}

	// reset node ready condition as true
	currentCondition.Status = corev1.ConditionTrue
	currentCondition.Reason = "NodeDelegateLease"
	currentCondition.Message = "Node disconnect with ApiServer and lease delegate."
	currentCondition.LastTransitionTime = metav1.NewTime(time.Now())

	// update
	if _, err := r.dlClient.CoreV1().Nodes().UpdateStatus(ctx, newNode, metav1.UpdateOptions{}); err != nil {
		klog.Errorf("Error updating node %s: %v", newNode.Name, err)
		return
	}
	klog.Infof("successful set node %s ready condition with true", newNode.Name)
}

func (r *ReconcileDelegateLease) InjectClient(c client.Client) error {
	r.Client = c
	return nil
}

func (r *ReconcileDelegateLease) InjectConfig(cfg *rest.Config) error {
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Errorf("could not create kube client, %v", err)
		return err
	}
	r.dlClient = client
	return nil
}
