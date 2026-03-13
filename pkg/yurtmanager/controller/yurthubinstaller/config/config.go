/*
Copyright 2026 The OpenYurt Authors.

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

package config

// YurtHubInstallerControllerConfiguration contains configuration for the YurtHub installer controller
type YurtHubInstallerControllerConfiguration struct {
	// ConcurrentYurtHubInstallerWorkers is the number of workers for yurthub installer controller
	ConcurrentYurtHubInstallerWorkers int32 `json:"concurrentYurtHubInstallerWorkers,omitempty"`
	
	// NodeServantImage is the image used for installation/uninstallation jobs
	NodeServantImage string `json:"nodeServantImage,omitempty"`
	
	// YurtHubVersion is the default version of YurtHub to install
	YurtHubVersion string `json:"yurtHubVersion,omitempty"`
	
	// EnableYurtHubInstaller enables or disables the yurthub installer controller
	EnableYurtHubInstaller bool `json:"enableYurtHubInstaller,omitempty"`
}
