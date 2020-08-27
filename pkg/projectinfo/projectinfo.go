/*
Copyright 2020 The OpenYurt Authors.

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

package projectinfo

import (
	"runtime"
)

var (
	projectPrefix = "yurt"
	labelPrefix   = "openyurt.io"
	gitVersion    = "v0.0.0"
	gitCommit     = "unknown"
	buildDate     = "1970-01-01T00:00:00Z"
	compiler      = runtime.Compiler
	platform      = runtime.GOOS + "/" + runtime.GOARCH
)

func ShortAgentVersion() string {
	commit := gitCommit
	if len(gitCommit) > 7 {
		commit = gitCommit[:7]
	}
	return GetAgentName() + "/" + gitVersion + "-" + commit
}

func ShortServerVersion() string {
	commit := gitCommit
	if len(gitCommit) > 7 {
		commit = gitCommit[:7]
	}
	return GetServerName() + "/" + gitVersion + "-" + commit
}

func GetProjectPrefix() string {
	return projectPrefix
}

func GetServerName() string {
	return projectPrefix + "tunnel-server"
}

func GetAgentName() string {
	return projectPrefix + "tunnel-agent"
}

// GetEdgeWorkerLabelKey returns the edge-worker label, which is used to
// identify if a node is a edge node ("true") or a cloud node ("false")
func GetEdgeWorkerLabelKey() string {
	return labelPrefix + "/is-edge-worker"
}
