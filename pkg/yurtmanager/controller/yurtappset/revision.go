/*
Copyright 2024 The OpenYurt Authors.

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

package yurtappset

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	apps "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	appsbetav1 "github.com/openyurtio/openyurt/pkg/apis/apps/v1beta1"
	"github.com/openyurtio/openyurt/pkg/util/kubernetes/controller/history"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/util/refmanager"
	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/yurtappset/workloadmanager"
)

// get all histories controlled by the YurtAppSet
func controlledHistories(cli client.Client, scheme *runtime.Scheme, yas *appsbetav1.YurtAppSet) ([]*apps.ControllerRevision, error) {

	// List all histories to include those that don't match the selector anymore
	histories := &apps.ControllerRevisionList{}
	err := cli.List(context.TODO(), histories, &client.ListOptions{LabelSelector: labels.Everything(), Namespace: yas.GetNamespace()})
	if err != nil {
		return nil, err
	}

	// Use ControllerRefManager to adopt/orphan as needed.
	yasSelector, err := workloadmanager.NewLabelSelectorForYurtAppSet(yas)
	if err != nil {
		return nil, err
	}
	cm, err := refmanager.New(cli, yasSelector, yas, scheme)
	if err != nil {
		return nil, err
	}

	mts := make([]metav1.Object, len(histories.Items))
	for i := range histories.Items {
		mts[i] = &histories.Items[i]
	}
	claims, err := cm.ClaimOwnedObjects(mts)
	if err != nil {
		return nil, err
	}

	claimHistories := make([]*apps.ControllerRevision, len(claims))
	for i, mt := range claims {
		claimHistories[i] = mt.(*apps.ControllerRevision)
	}

	klog.V(4).Infof("List controller revision of YurtAppSet %s/%s: %d\n", yas.Namespace, yas.Name, len(claimHistories))

	return claimHistories, nil
}

func (r *ReconcileYurtAppSet) constructYurtAppSetRevisions(yas *appsbetav1.YurtAppSet) (allRevisions []*apps.ControllerRevision, updateRevision *apps.ControllerRevision, collisionCount int32, err error) {

	allRevisions, err = controlledHistories(r.Client, r.scheme, yas)
	if err != nil {
		if yas.Status.CollisionCount == nil {
			return nil, nil, 0, err
		}
		return nil, nil, *yas.Status.CollisionCount, err
	}

	// Use a local copy of set.Status.CollisionCount to avoid modifying set.Status directly.
	// This copy is returned so the value gets carried over to set.Status in updateStatefulSet.
	if yas.Status.CollisionCount != nil {
		collisionCount = *yas.Status.CollisionCount
	}

	// create a new revision from the current set
	history.SortControllerRevisions(allRevisions)
	updateRevision, err = newRevision(yas, nextRevision(allRevisions), &collisionCount, r.scheme)
	if err != nil {
		return nil, nil, collisionCount, err
	}

	// find any equivalent revisions
	equalRevisions := history.FindEqualRevisions(allRevisions, updateRevision)
	equalCount := len(equalRevisions)
	revisionCount := len(allRevisions)

	if equalCount > 0 && history.EqualRevision(allRevisions[revisionCount-1], equalRevisions[equalCount-1]) {
		// if the equivalent revision is immediately prior the update revision has not changed
		updateRevision = allRevisions[revisionCount-1]
	} else if equalCount > 0 {
		if isRevisionInvalid(equalRevisions[0]) {
			// if equal revision is invalid, just reuse it
			updateRevision = equalRevisions[0]
		} else {
			// if the equivalent revision is valid and not immediately prior, we will roll back by incrementing the
			// Revision of the equivalent revision
			equalRevisions[equalCount-1].Revision = updateRevision.Revision
			err := r.Client.Update(context.TODO(), equalRevisions[equalCount-1])
			if err != nil {
				return nil, nil, collisionCount, err
			}
			updateRevision = equalRevisions[equalCount-1]
		}
	} else {
		//if there is no equivalent revision we create a new one
		updateRevision, err = createControllerRevision(r.Client, yas, updateRevision, &collisionCount)
		if err != nil {
			return nil, nil, collisionCount, err
		}
		allRevisions = append(allRevisions, updateRevision)
	}

	klog.V(4).Infof("YurtAppSet [%s/%s] get expectRevision %s collisionCount %v", yas.GetNamespace(), yas.GetName(), updateRevision.Name, collisionCount)
	return allRevisions, updateRevision, collisionCount, nil
}

// clean expired and invalid revisions
func cleanRevisions(cli client.Client, yas *appsbetav1.YurtAppSet, revisions []*apps.ControllerRevision) error {

	// clean invalid revisions
	validRevisions := make([]*apps.ControllerRevision, 0)
	for _, revision := range revisions {
		if isRevisionInvalid(revision) {
			if err := cli.Delete(context.TODO(), revision); err != nil {
				klog.Errorf("YurtAppSet [%s/%s] delete invalid revision %s error: %v")
				return err
			}
			klog.Infof("YurtAppSet [%s/%s] delete invalid revision %s", yas.GetNamespace(), yas.GetName(), revision.Name)
		} else {
			validRevisions = append(validRevisions, revision)
		}
	}

	// clean expired revisions
	var revisionLimit int
	if yas.Spec.RevisionHistoryLimit != nil {
		revisionLimit = int(*(yas.Spec.RevisionHistoryLimit))
	} else {
		klog.Warningf("YurtAppSet [%s/%s] revisionHistoryLimit is nil, default to 10", yas.Namespace, yas.Name)
		revisionLimit = 10
	}

	if len(validRevisions) > revisionLimit {
		klog.V(4).Info("YurtAppSet [%s/%s] clean expired revisions", yas.GetNamespace(), yas.GetName())
		for i := 0; i < len(validRevisions)-revisionLimit; i++ {
			if validRevisions[i].GetName() == yas.Status.CurrentRevision {
				klog.Warningf("YurtAppSet [%s/%s] current revision %s is expired, skip")
				continue
			}
			if err := cli.Delete(context.TODO(), validRevisions[i]); err != nil {
				klog.Errorf("YurtAppSet [%s/%s] delete expired revision %s error: %v")
				return err
			}
			klog.Infof("YurtAppSet [%s/%s] delete expired revision %s", yas.GetNamespace(), yas.GetName(), validRevisions[i].Name)
		}
	}

	return nil
}

// createControllerRevision creates the revision owned by the parent and update the collisionCount
func createControllerRevision(cli client.Client, parent metav1.Object, revision *apps.ControllerRevision, collisionCount *int32) (*apps.ControllerRevision, error) {
	if collisionCount == nil {
		return nil, fmt.Errorf("collisionCount should not be nil")
	}

	// Clone the input
	clone := revision.DeepCopy()

	var err error
	// Continue to attempt to create the revision updating the name with a new hash on each iteration
	for {
		hash := history.HashControllerRevision(revision, collisionCount)
		// Update the revisions name
		clone.Name = history.ControllerRevisionName(parent.GetName(), hash)
		err = cli.Create(context.TODO(), clone)
		if errors.IsAlreadyExists(err) {
			klog.V(4).Infof("YurtAppSet [%s/%s] createControllerRevision %s error: name already exist", parent.GetNamespace(), parent.GetName(), clone.GetName())
			exists := &apps.ControllerRevision{}
			err := cli.Get(context.TODO(), client.ObjectKey{Namespace: parent.GetNamespace(), Name: clone.Name}, exists)
			if err != nil {
				klog.V(4).Infof("YurtAppSet [%s/%s] createControllerRevision %s: get failed: %v ", parent.GetNamespace(), parent.GetName(), clone.GetName(), err)
				return nil, err
			}
			if bytes.Equal(exists.Data.Raw, clone.Data.Raw) {
				klog.V(4).Infof("YurtAppSet [%s/%s] createControllerRevision %s: contents are the same with cr already exists", parent.GetNamespace(), parent.GetName(), clone.GetName())
				return exists, nil
			}
			klog.Info("YurtAppSet [%s/%s] createControllerRevision collision exists, collision count increased %d->%d", parent.GetNamespace(), parent.GetName(), *collisionCount, *collisionCount+1)
			*collisionCount++
			continue
		}
		klog.Infof("YurtAppSet [%s/%s] createControllerRevision %s success", parent.GetNamespace(), parent.GetName(), clone.GetName())
		return clone, err
	}
}

// newRevision creates a new ControllerRevision containing a patch that reapplies the target state of set.
// The Revision of the returned ControllerRevision is set to revision. If the returned error is nil, the returned
// ControllerRevision is valid. StatefulSet revisions are stored as patches that re-apply the current state of set
// to a new StatefulSet using a strategic merge patch to replace the saved state of the new StatefulSet.
func newRevision(yas *appsbetav1.YurtAppSet, revision int64, collisionCount *int32, scheme *runtime.Scheme) (*apps.ControllerRevision, error) {
	patch, err := getYurtAppSetPatch(yas)
	if err != nil {
		return nil, err
	}

	gvk, err := apiutil.GVKForObject(yas, scheme)
	if err != nil {
		return nil, err
	}

	selectedLabels := yas.GetLabels()
	labelSelector, err := workloadmanager.NewLabelSelectorForYurtAppSet(yas)
	if err == nil {
		selectedLabels = workloadmanager.CombineMaps(selectedLabels, labelSelector.MatchLabels)
	}

	cr, err := history.NewControllerRevision(yas,
		gvk,
		selectedLabels,
		runtime.RawExtension{Raw: patch},
		revision,
		collisionCount)
	if err != nil {
		return nil, err
	}
	cr.Namespace = yas.Namespace

	return cr, nil
}

// nextRevision finds the next valid revision number based on revisions. If the length of revisions
// is 0 this is 1. Otherwise, it is 1 greater than the largest revision's Revision. This method
// assumes that revisions has been sorted by Revision.
func nextRevision(revisions []*apps.ControllerRevision) int64 {
	count := len(revisions)
	if count <= 0 {
		return 1
	}
	return revisions[count-1].Revision + 1
}

// getYurtAppSetPatch creates a patch of the YurtAppSet that replaces spec.template
// it only contains spec.workload, which means only workload field change will be recorded in this patch
func getYurtAppSetPatch(yas *appsbetav1.YurtAppSet) ([]byte, error) {
	if yas == nil {
		return nil, fmt.Errorf("yurtAppSet is nil")
	}

	dsBytes, err := json.Marshal(yas)
	if err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	err = json.Unmarshal(dsBytes, &raw)
	if err != nil {
		return nil, err
	}
	objCopy := make(map[string]interface{})
	specCopy := make(map[string]interface{})

	// Create a patch of the YurtAppSet that replaces spec.template
	spec := raw["spec"].(map[string]interface{})

	if template, ok := spec["workload"].(map[string]interface{}); ok {
		template["$patch"] = "replace"
		specCopy["workload"] = template
	}

	objCopy["spec"] = specCopy
	patch, err := json.Marshal(objCopy)
	return patch, err
}

func isRevisionInvalid(revision *apps.ControllerRevision) bool {
	// set Revision to 0 to indicate invalid, because the revision number is increased from 1
	return revision.Revision == 0
}

func setRevisionInvalid(cli client.Client, revision *apps.ControllerRevision) {
	revision.Revision = 0
	if err := cli.Update(context.TODO(), revision); err != nil {
		klog.Warningf("YurtAppSet [%s/%s] set revision invalid error: %v", revision.Namespace, revision.Name, err)
	}
}
