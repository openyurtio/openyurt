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

package v1alpha1

// SetDefaultsNodePool set default values for NodePool.
func SetDefaultsNodePool(obj *NodePool) {
	// example for set default value for NodePool
	if obj.Annotations == nil {
		obj.Annotations = make(map[string]string)
	}

}

// SetDefaultsYurtAppDaemon set default values for YurtAppDaemon.
func SetDefaultsYurtAppDaemon(obj *YurtAppDaemon) {
	// example for set default value for YurtAppDaemon 

	if len(obj.Spec.Default) == 0 {
		obj.Spec.Default = "set-default-value-0"
	}
}
