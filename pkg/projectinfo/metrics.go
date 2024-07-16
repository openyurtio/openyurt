/*
Copyright 2024 The OpenYurt Authors.

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
	"github.com/prometheus/client_golang/prometheus"

	yurtutil "github.com/openyurtio/openyurt/pkg/util"
)

var (
	buildInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "openyurt",
			Name:      "version_info",
			Help:      "A metric with a constant '1' value labeled by git version, git commit, build date, Go version, compiler and nodepoollabelkey from which OpenYurt was built, and platform on which it is running.",
		},
		[]string{"component_name", "git_version", "git_commit", "build_date", "go_version", "compiler", "platform", "nodepoollabelkey"},
	)
)

func RegisterVersionInfo(reg prometheus.Registerer, componentName string) {
	info := Get()
	if yurtutil.IsNil(reg) {
		prometheus.MustRegister(buildInfo)
	} else {
		reg.MustRegister(buildInfo)
	}
	buildInfo.WithLabelValues(componentName, info.GitVersion, info.GitCommit, info.BuildDate, info.GoVersion, info.Compiler, info.Platform, info.NodePoolLabelKey).Set(1)
}
