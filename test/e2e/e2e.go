/*
Copyright 2014 The Kubernetes Authors.
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

package e2e

import (
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/config"
	"github.com/onsi/ginkgo/reporters"
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/test/e2e/util/ginkgowrapper"
	"github.com/openyurtio/openyurt/test/e2e/yurtconfig"
)

func RunE2ETests(t *testing.T) {
	klog.Infof("[edge] Start run e2e test")
	gomega.RegisterFailHandler(ginkgowrapper.Fail)
	var r []ginkgo.Reporter
	if yurtconfig.YurtE2eCfg.ReportDir != "" {
		if err := os.MkdirAll(yurtconfig.YurtE2eCfg.ReportDir, 0755); err != nil {
			klog.Errorf("Failed creating report directory: %v", err)
		} else {
			r = append(r, reporters.NewJUnitReporter(path.Join(yurtconfig.YurtE2eCfg.ReportDir, fmt.Sprintf("yurt-e2e-test-report_%02d.xml", config.GinkgoConfig.ParallelNode))))
		}
	}
	klog.Infof("Starting e2e run %q on Ginkgo node %d", uuid.NewUUID(), config.GinkgoConfig.ParallelNode)
	ginkgo.RunSpecsWithDefaultAndCustomReporters(t, "openyurt e2e suite", r)
}
