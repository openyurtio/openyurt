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
	gitVersion    = "v0.0.0"
	gitCommit     = "unknown"
	buildDate     = "1970-01-01T00:00:00Z"
)

type ProjectInfo struct {
	ProjectPrefix string
	GitVersion    string
	GitCommit     string
	BuildDate     string
	Compiler      string
	Platform      string
}

func Get() ProjectInfo {
	return ProjectInfo{
		ProjectPrefix: projectPrefix,
		GitVersion:    gitVersion,
		GitCommit:     gitCommit,
		BuildDate:     buildDate,
		Compiler:      runtime.Compiler,
		Platform:      runtime.GOOS + "/" + runtime.GOARCH,
	}
}

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

func GetServerName() string {
	return Get().ProjectPrefix + "tunnel-server"
}

func GetAgentName() string {
	return Get().ProjectPrefix + "tunnel-agent"
}
