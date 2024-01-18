/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nodebucket

import (
	"context"
	"flag"
	"fmt"
	"sort"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	appconfig "github.com/openyurtio/openyurt/cmd/yurt-manager/app/config"
	"github.com/openyurtio/openyurt/cmd/yurt-manager/names"
	appsv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1alpha1"
	appsv1beta1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/projectinfo"
)

func init() {
	flag.IntVar(&concurrentReconciles, "nodebucket-workers", concurrentReconciles, "Max concurrent workers for NodeBucket controller.")
}

var (
	concurrentReconciles = 3
	controllerResource   = appsv1alpha1.SchemeGroupVersion.WithResource("nodebuckets")
)

const (
	LabelNodePoolName = "openyurt.io/pool-name"
)

func Format(format string, args ...interface{}) string {
	s := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s: %s", names.NodeBucketController, s)
}

// Add creates a new NodeBucket Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(_ context.Context, cfg *appconfig.CompletedConfig, mgr manager.Manager) error {
	klog.Infof(Format("nodebucket-controller add controller %s", controllerResource.String()))
	r := &ReconcileNodeBucket{
		maxNodesPerBucket: int(cfg.ComponentConfig.NodeBucketController.MaxNodesPerBucket),
	}

	// Create a new controller
	c, err := controller.New(names.NodeBucketController, mgr, controller.Options{
		Reconciler: r, MaxConcurrentReconciles: concurrentReconciles,
	})
	if err != nil {
		return err
	}

	if _, err := r.mapper.KindFor(controllerResource); err != nil {
		klog.Infof("resource %s doesn't exist", controllerResource.String())
		return err
	}

	// Watch for changes to NodeBucket
	if err = c.Watch(&source.Kind{Type: &appsv1alpha1.NodeBucket{}}, &handler.EnqueueRequestForOwner{
		OwnerType:    &appsv1beta1.NodePool{},
		IsController: true,
	}); err != nil {
		return err
	}

	nodePredicate := predicate.Funcs{
		CreateFunc: func(evt event.CreateEvent) bool {
			return true
		},
		DeleteFunc: func(evt event.DeleteEvent) bool {
			return true
		},
		UpdateFunc: func(evt event.UpdateEvent) bool {
			oldNode, ok := evt.ObjectOld.(*v1.Node)
			if !ok {
				return false
			}
			newNode, ok := evt.ObjectNew.(*v1.Node)
			if !ok {
				return false
			}

			if oldNode.Labels[projectinfo.GetNodePoolLabel()] != newNode.Labels[projectinfo.GetNodePoolLabel()] {
				return true
			}
			return false
		},
		GenericFunc: func(evt event.GenericEvent) bool {
			return false
		},
	}

	reconcilePool := handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
		node, ok := obj.(*v1.Node)
		if !ok {
			return []reconcile.Request{}
		}
		if npName := node.Labels[projectinfo.GetNodePoolLabel()]; len(npName) != 0 {
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{Name: npName},
				},
			}
		}
		return []reconcile.Request{}
	})

	// Watch for changes to Node
	if err = c.Watch(&source.Kind{Type: &v1.Node{}}, reconcilePool, nodePredicate); err != nil {
		return err
	}
	return nil
}

var _ reconcile.Reconciler = &ReconcileNodeBucket{}

func (r *ReconcileNodeBucket) InjectClient(c client.Client) error {
	r.Client = c
	return nil
}

func (r *ReconcileNodeBucket) InjectMapper(mapper meta.RESTMapper) error {
	r.mapper = mapper
	return nil
}

// ReconcileNodeBucket reconciles a NodeBucket object
type ReconcileNodeBucket struct {
	client.Client
	mapper            meta.RESTMapper
	maxNodesPerBucket int
}

// +kubebuilder:rbac:groups=apps.openyurt.io,resources=nodebuckets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps.openyurt.io,resources=nodepools,verbs=list;watch
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=list;watch

// Reconcile reads that state of the cluster for a NodeBucket object and makes changes based on the state read
// and what is in the NodeBucket.Spec
func (r *ReconcileNodeBucket) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	klog.Infof(Format("Reconcile NodePool for NodeBuckets %s/%s", request.Namespace, request.Name))

	// 1. Fetch the NodePool instance
	ins := &appsv1beta1.NodePool{}
	err := r.Get(context.TODO(), request.NamespacedName, ins)
	if err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	if ins.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}

	// 2. list all nodes in the NodePool and prepare node set
	var currentNodeList v1.NodeList
	if err := r.List(ctx, &currentNodeList, client.MatchingLabels(map[string]string{
		projectinfo.GetNodePoolLabel(): ins.Name,
	})); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	desiredNodeSet := sets.String{}
	for i := range currentNodeList.Items {
		desiredNodeSet.Insert(currentNodeList.Items[i].Name)
	}

	// 3. list all exist NodeBuckets for the NodePool
	var existingNodeBucketList appsv1alpha1.NodeBucketList
	if err = r.List(ctx, &existingNodeBucketList, client.MatchingLabels(map[string]string{
		LabelNodePoolName: ins.Name,
	})); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}

	// 4. reconcile NodeBuckets based on nodes and existing NodeBuckets
	bucketsToCreate, bucketsToUpdate, bucketsToDelete, bucketsUnchanged := r.reconcileNodeBuckets(ins, desiredNodeSet, &existingNodeBucketList)
	klog.Infof("reconcile pool(%s): bucketsToCreate=%d, bucketsToUpdate=%d, bucketsToDelete=%d, bucketsUnchanged=%d", ins.Name, len(bucketsToCreate), len(bucketsToUpdate), len(bucketsToDelete), len(bucketsUnchanged))

	// 5.finalize creates, updates, and deletes buckets as specified
	if err = finalize(ctx, r.Client, bucketsToCreate, bucketsToUpdate, bucketsToDelete); err != nil {
		klog.Errorf("could not finalize buckets for pool(%s), %v", ins.Name, err)
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileNodeBucket) reconcileNodeBuckets(
	pool *appsv1beta1.NodePool,
	desiredNodeSet sets.String,
	buckets *appsv1alpha1.NodeBucketList,
) ([]*appsv1alpha1.NodeBucket, []*appsv1alpha1.NodeBucket, []*appsv1alpha1.NodeBucket, []*appsv1alpha1.NodeBucket) {
	bucketsUnchanged, bucketsToUpdate, bucketsToDelete, unFilledNodeSet := resolveExistingBuckets(buckets, desiredNodeSet)
	klog.V(4).Infof("reconcileNodeBuckets for pool(%s), len(bucketsUnchanged)=%d, len(bucketsToUpdate)=%d, len(bucketsToDelete)=%d, unFilledNodeSet=%v",
		pool.Name, len(bucketsUnchanged), len(bucketsToUpdate), len(bucketsToDelete), unFilledNodeSet.List())

	// If we still have unfilled nodes to add and buckets marked for update,
	// iterate through the buckets and fill them up with the unfilled nodes.
	if unFilledNodeSet.Len() > 0 && len(bucketsToUpdate) > 0 {
		sort.Sort(nodeBucketNodesLen(bucketsToUpdate))
		for _, bucket := range bucketsToUpdate {
			for unFilledNodeSet.Len() > 0 && len(bucket.Nodes) < r.maxNodesPerBucket {
				nodeName, _ := unFilledNodeSet.PopAny()
				bucket.Nodes = append(bucket.Nodes, appsv1alpha1.Node{Name: nodeName})
			}
		}
	}
	klog.V(4).Infof("reconcileNodeBuckets for pool(%s) after filling bucketsToUpdate, len(bucketsUnchanged)=%d, len(bucketsToUpdate)=%d, len(bucketsToDelete)=%d, unFilledNodeSet=%v",
		pool.Name, len(bucketsUnchanged), len(bucketsToUpdate), len(bucketsToDelete), unFilledNodeSet.List())

	// If there are still unfilled nodes left at this point, we try to fit the nodes in a single existing buckets.
	// If there are no buckets with that capacity, we create new buckets for the nodes.
	bucketsToCreate := []*appsv1alpha1.NodeBucket{}
	for unFilledNodeSet.Len() > 0 {
		var bucketToFill *appsv1alpha1.NodeBucket
		var index int

		if unFilledNodeSet.Len() < r.maxNodesPerBucket && len(bucketsUnchanged) > 0 {
			index, bucketToFill = getBucketToFill(bucketsUnchanged, unFilledNodeSet.Len(), r.maxNodesPerBucket)
		}

		// If we didn't find a bucketToFill, generate a new empty one.
		if bucketToFill == nil {
			bucketToFill = newNodeBucket(pool)
			bucketsToCreate = append(bucketsToCreate, bucketToFill)
		} else {
			bucketsToUpdate = append(bucketsToUpdate, bucketToFill)
			bucketsUnchanged = append(bucketsUnchanged[:index], bucketsUnchanged[index+1:]...)
		}

		// Fill the bucket up with remaining nodes.
		for unFilledNodeSet.Len() > 0 && len(bucketToFill.Nodes) < r.maxNodesPerBucket {
			nodeName, _ := unFilledNodeSet.PopAny()
			bucketToFill.Nodes = append(bucketToFill.Nodes, appsv1alpha1.Node{Name: nodeName})
		}
	}
	klog.V(4).Infof("reconcileNodeBuckets for pool(%s) after filling bucketsUnchanged, len(bucketsUnchanged)=%d, len(bucketsToCreate)=%d len(bucketsToUpdate)=%v, len(bucketsToDelete)=%d, unFilledNodeSet=%v",
		pool.Name, len(bucketsUnchanged), len(bucketsToCreate), len(bucketsToUpdate), len(bucketsToDelete), unFilledNodeSet.List())

	return bucketsToCreate, bucketsToUpdate, bucketsToDelete, bucketsUnchanged
}

// resolveExistingBuckets iterates through existing node buckets to delete nodes no longer desired and update node buckets that have changed
func resolveExistingBuckets(buckets *appsv1alpha1.NodeBucketList, desiredNodeSet sets.String) ([]*appsv1alpha1.NodeBucket, []*appsv1alpha1.NodeBucket, []*appsv1alpha1.NodeBucket, sets.String) {
	bucketsUnchanged := []*appsv1alpha1.NodeBucket{}
	bucketsToUpdate := []*appsv1alpha1.NodeBucket{}
	bucketsToDelete := []*appsv1alpha1.NodeBucket{}

	for _, bucket := range buckets.Items {
		copiedBucket := (&bucket).DeepCopy()
		newNodes := []appsv1alpha1.Node{}
		for _, node := range copiedBucket.Nodes {
			if desiredNodeSet.Has(node.Name) {
				newNodes = append(newNodes, node)
				desiredNodeSet.Delete(node.Name)
			}
		}

		if len(newNodes) != len(copiedBucket.Nodes) {
			if len(newNodes) == 0 {
				bucketsToDelete = append(bucketsToDelete, copiedBucket)
			} else {
				copiedBucket.Nodes = newNodes
				bucketsToUpdate = append(bucketsToUpdate, copiedBucket)
			}
		} else {
			bucketsUnchanged = append(bucketsUnchanged, copiedBucket)
		}
	}

	return bucketsUnchanged, bucketsToUpdate, bucketsToDelete, desiredNodeSet
}

// getBucketToFill will return the NodeBucket that will be closest to full
// when numNodes are added. If no NodeBucket can be found, a nil pointer
// will be returned.
func getBucketToFill(buckets []*appsv1alpha1.NodeBucket, numNodes, maxNodes int) (int, *appsv1alpha1.NodeBucket) {
	closestDiff := maxNodes
	index := 0
	var closestBucket *appsv1alpha1.NodeBucket
	for i, bucket := range buckets {
		currentDiff := maxNodes - (numNodes + len(bucket.Nodes))
		if currentDiff >= 0 && currentDiff < closestDiff {
			closestDiff = currentDiff
			closestBucket = bucket
			index = i
			if closestDiff == 0 {
				return index, closestBucket
			}
		}
	}
	return index, closestBucket
}

func newNodeBucket(pool *appsv1beta1.NodePool) *appsv1alpha1.NodeBucket {
	gvk := appsv1beta1.GroupVersion.WithKind("NodePool")
	ownerRef := metav1.NewControllerRef(pool, gvk)
	bucket := &appsv1alpha1.NodeBucket{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				LabelNodePoolName: pool.Name,
			},
			OwnerReferences: []metav1.OwnerReference{*ownerRef},
		},
		Nodes: make([]appsv1alpha1.Node, 0),
	}

	return bucket
}

func finalize(ctx context.Context, c client.Client, bucketsToCreate, bucketsToUpdate, bucketsToDelete []*appsv1alpha1.NodeBucket) error {
	// If there are buckets to create and delete, change the creates to updates of the buckets that would otherwise be deleted.
	for i := 0; i < len(bucketsToDelete); {
		if len(bucketsToCreate) == 0 {
			break
		}
		bucketToDelete := bucketsToDelete[i]
		bucket := bucketsToCreate[len(bucketsToCreate)-1]
		bucket.Name = bucketToDelete.Name
		bucket.ResourceVersion = bucketToDelete.ResourceVersion
		bucketsToCreate = bucketsToCreate[:len(bucketsToCreate)-1]
		bucketsToUpdate = append(bucketsToUpdate, bucket)
		bucketsToDelete = append(bucketsToDelete[:i], bucketsToDelete[i+1:]...)
	}

	for _, bucket := range bucketsToCreate {
		var collisionCount int
		for {
			collisionCount++
			bucket.Name = fmt.Sprintf("%s-%s", bucket.Labels[LabelNodePoolName], rand.String(6))
			bucket.NumNodes = int32(len(bucket.Nodes))
			if err := c.Create(ctx, bucket, &client.CreateOptions{}); err != nil {
				if errors.IsAlreadyExists(err) && collisionCount < 5 {
					continue
				}
				klog.Errorf("could not create bucket(%s), %v", bucket.Name, err)
				return err
			}
			break
		}
	}

	for _, bucket := range bucketsToUpdate {
		bucket.NumNodes = int32(len(bucket.Nodes))
		if err := c.Update(ctx, bucket, &client.UpdateOptions{}); err != nil {
			klog.Errorf("could not update bucket(%s), %v", bucket.Name, err)
			return err
		}
	}

	for _, bucket := range bucketsToDelete {
		if err := c.Delete(ctx, bucket, &client.DeleteOptions{}); err != nil {
			klog.Errorf("could not delete bucket(%s), %v", bucket.Name, err)
			return err
		}
	}

	return nil
}

// nodeBucketNodesLen helps sort node buckets by the number of nodes they contain.
type nodeBucketNodesLen []*appsv1alpha1.NodeBucket

func (sl nodeBucketNodesLen) Len() int      { return len(sl) }
func (sl nodeBucketNodesLen) Swap(i, j int) { sl[i], sl[j] = sl[j], sl[i] }
func (sl nodeBucketNodesLen) Less(i, j int) bool {
	return sl[i].NumNodes > sl[j].NumNodes
}
