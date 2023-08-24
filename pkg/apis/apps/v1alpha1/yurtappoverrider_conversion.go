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

/*
Implementing the hub method is pretty easy -- we just have to add an empty
method called Hub() to serve as a
[marker](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/conversion?tab=doc#Hub).
*/

// NOTE !!!!!! @kadisi
// If this version is storageversion, you only need to uncommand this method

// Hub marks this type as a conversion hub.
//func (*YurtAppOverrider) Hub() {}

// NOTE !!!!!!! @kadisi
// If this version is not storageversion, you need to implement the ConvertTo and ConvertFrom methods

// need import "sigs.k8s.io/controller-runtime/pkg/conversion"
//func (src *YurtAppOverrider) ConvertTo(dstRaw conversion.Hub) error {
//	return nil
//}

// NOTE !!!!!!! @kadisi
// If this version is not storageversion, you need to implement the ConvertTo and ConvertFrom methods

// need import "sigs.k8s.io/controller-runtime/pkg/conversion"
//func (dst *YurtAppOverrider) ConvertFrom(srcRaw conversion.Hub) error {
//	return nil
//}
