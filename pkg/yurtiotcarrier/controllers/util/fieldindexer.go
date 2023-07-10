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

package util

import (
	"context"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"

	iotv1alpha1 "github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha1"
)

const (
	IndexerPathForNodepool = "spec.nodePool"
)

var registerOnce sync.Once

func RegisterFieldIndexers(fi client.FieldIndexer) error {
	var err error
	registerOnce.Do(func() {
		// register the fieldIndexer for device
		if err = fi.IndexField(context.TODO(), &iotv1alpha1.Device{}, IndexerPathForNodepool, func(rawObj client.Object) []string {
			device := rawObj.(*iotv1alpha1.Device)
			return []string{device.Spec.NodePool}
		}); err != nil {
			return
		}

		// register the fieldIndexer for deviceService
		if err = fi.IndexField(context.TODO(), &iotv1alpha1.DeviceService{}, IndexerPathForNodepool, func(rawObj client.Object) []string {
			deviceService := rawObj.(*iotv1alpha1.DeviceService)
			return []string{deviceService.Spec.NodePool}
		}); err != nil {
			return
		}

		// register the fieldIndexer for deviceProfile
		if err = fi.IndexField(context.TODO(), &iotv1alpha1.DeviceProfile{}, IndexerPathForNodepool, func(rawObj client.Object) []string {
			profile := rawObj.(*iotv1alpha1.DeviceProfile)
			return []string{profile.Spec.NodePool}
		}); err != nil {
			return
		}
	})
	return err
}
