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

package util

import (
	"context"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha2"
)

const (
	IndexerPathForNodepool = "spec.poolName"
)

var registerOnce sync.Once

func RegisterFieldIndexers(fi client.FieldIndexer) error {
	var err error
	registerOnce.Do(func() {
		// register the fieldIndexer for device
		if err = fi.IndexField(context.TODO(), &v1alpha2.PlatformAdmin{}, IndexerPathForNodepool, func(rawObj client.Object) []string {
			platformAdmin, ok := rawObj.(*v1alpha2.PlatformAdmin)
			if ok {
				return []string{platformAdmin.Spec.PoolName}
			}
			return []string{}
		}); err != nil {
			return
		}
	})

	return err
}
