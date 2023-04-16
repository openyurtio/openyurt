/*
 * Copyright 2022 The OpenYurt Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package features

import (
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/component-base/featuregate"
)

var (
	// DefaultMutableFeatureGate is a mutable version of DefaultFeatureGate.
	DefaultMutableFeatureGate featuregate.MutableFeatureGate = featuregate.NewFeatureGate()

	// DefaultFeatureGate is a shared global FeatureGate.
	// Top-level commands/options setup that needs to modify this feature gate should use DefaultMutableFeatureGate.
	DefaultFeatureGate featuregate.FeatureGate = DefaultMutableFeatureGate
)

func init() {
	runtime.Must(DefaultMutableFeatureGate.Add(defaultRavenFeatureGates))
}

const (
	// RavenL7Proxy setups and serves a L7 proxy.
	//
	// owner: @luckymrwang
	// alpha: v0.3.1
	// Setting on openyurt and raven-agent side.
	RavenL7Proxy featuregate.Feature = "RavenL7Proxy"
)

// defaultRavenFeatureGates consists of all known Kubernetes-specific and raven feature keys.
// To add a new feature, define a key for it above and add it here. The features will be
// available throughout openyurt binaries.
var defaultRavenFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	RavenL7Proxy: {Default: false, PreRelease: featuregate.Alpha, LockToDefault: false},
}
