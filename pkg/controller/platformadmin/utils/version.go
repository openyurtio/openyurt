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
	iotv1alpha2 "github.com/openyurtio/openyurt/pkg/apis/iot/v1alpha2"
)

const IotDockName = "yurt-iot-dock"

func defaultVersion(platformAdmin *iotv1alpha2.PlatformAdmin) (string, string, error) {

	version := "v0.3"
	ns := platformAdmin.Namespace

	return version, ns, nil
}
