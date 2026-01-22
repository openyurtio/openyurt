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

package yurthubinstaller

const (
	// EdgeWorkerLabel is the label that triggers YurtHub installation
	EdgeWorkerLabel = "openyurt.io/is-edge-worker"
	
	// YurtHubInstalledAnnotation indicates YurtHub installation status
	YurtHubInstalledAnnotation = "openyurt.io/yurthub-installed"
	
	// YurtHubInstallJobPrefix is the prefix for installation job names
	YurtHubInstallJobPrefix = "yurthub-install"
	
	// YurtHubUninstallJobPrefix is the prefix for uninstallation job names
	YurtHubUninstallJobPrefix = "yurthub-uninstall"
	
	// DefaultYurtHubVersion is the default version of YurtHub to install
	DefaultYurtHubVersion = "latest"
	
	// YurtHubVersionAnnotation allows specifying a custom YurtHub version
	YurtHubVersionAnnotation = "openyurt.io/yurthub-version"
	
	// YurtHubInstallationInProgressAnnotation marks a node as currently being processed
	YurtHubInstallationInProgressAnnotation = "openyurt.io/yurthub-installation-in-progress"
	
	// JobTTLSecondsAfterFinished defines how long to keep finished jobs
	JobTTLSecondsAfterFinished = 3600
)
