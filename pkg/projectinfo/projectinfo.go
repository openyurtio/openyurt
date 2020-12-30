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
	"fmt"
	"runtime"
)

var (
	projectPrefix = "yurt"
	labelPrefix   = "openyurt.io"
	gitVersion    = "v0.0.0"
	gitCommit     = "unknown"
	buildDate     = "1970-01-01T00:00:00Z"
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

// GetHubName returns name of yurthub agent
func GetHubName() string {
	return projectPrefix + "hub"
}

// GetEdgeEnableTunnelLabelKey returns the tunnel agent label, which is used
// to identify if tunnel agent is running on the node or not.
func GetEdgeEnableTunnelLabelKey() string {
	return labelPrefix + "/edge-enable-reverseTunnel-client"
}

// GetTunnelName returns name of tunnel
func GetTunnelName() string {
	return projectPrefix + "tunnel"
}

// GetYurtControllerManagerName returns name of openyurt controller-manager
func GetYurtControllerManagerName() string {
	return projectPrefix + "controller-manager"
}

// GetYurtAppManagerName returns name of tunnel
func GetYurtAppManagerName() string {
	return projectPrefix + "app-manager"
}

// normalizeGitCommit reserve 7 characters for gitCommit
func normalizeGitCommit(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}

	return commit
}

// Info contains version information.
type Info struct {
	GitVersion string `json:"gitVersion"`
	GitCommit  string `json:"gitCommit"`
	BuildDate  string `json:"buildDate"`
	GoVersion  string `json:"goVersion"`
	Compiler   string `json:"compiler"`
	Platform   string `json:"platform"`
}

// Get returns the overall codebase version.
func Get() Info {
	return Info{
		GitVersion: gitVersion,
		GitCommit:  normalizeGitCommit(gitCommit),
		BuildDate:  buildDate,
		GoVersion:  runtime.Version(),
		Compiler:   runtime.Compiler,
		Platform:   fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}
