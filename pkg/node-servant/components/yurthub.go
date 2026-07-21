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

package components

import (
	"os"

	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
	yurthubutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/yurthub"
)

var (
	statYurthubBinaryFunc           = os.Stat
	removeYurthubBinaryFunc         = os.RemoveAll
	copyEmbeddedYurthubBinaryFunc   = edgenode.CopyFile
	checkAndInstallYurthubFunc      = installEmbeddedYurthub
	createYurthubSystemdServiceFunc = yurthubutil.CreateYurthubSystemdServiceWithConfig
	checkYurthubServiceHealthFunc   = yurthubutil.CheckYurthubServiceHealth
	checkYurthubReadyzFunc          = yurthubutil.CheckYurthubReadyz
	cleanYurthubHostArtifactsFunc   = yurthubutil.CleanYurthubHostArtifacts
)

type yurtHubOperator struct {
	cfg *yurthubutil.YurthubHostConfig
}

// NewYurthubOperator creates an operator that manages host-level yurthub
// lifecycle with the systemd + binary deployment path.
func NewYurthubOperator(cfg *yurthubutil.YurthubHostConfig) *yurtHubOperator {
	return &yurtHubOperator{cfg: cfg}
}

// Install installs yurthub on the host as a systemd service and waits for it
// to become healthy before kubelet traffic is redirected.
func (op *yurtHubOperator) Install() error {
	if err := checkAndInstallYurthubFunc(); err != nil {
		return err
	}

	if err := createYurthubSystemdServiceFunc(op.cfg); err != nil {
		return err
	}

	if err := checkYurthubServiceHealthFunc(op.bindAddress()); err != nil {
		return err
	}

	return checkYurthubReadyzFunc(op.bindAddress())
}

// UnInstall removes the host-level yurthub artifacts created during conversion.
func (op *yurtHubOperator) UnInstall() error {
	return cleanYurthubHostArtifactsFunc()
}

func (op *yurtHubOperator) bindAddress() string {
	if op.cfg != nil && len(op.cfg.BindAddress) != 0 {
		return op.cfg.BindAddress
	}

	return constants.DefaultYurtHubServerAddr
}

func installEmbeddedYurthub() error {
	if _, err := statYurthubBinaryFunc(constants.YurthubExecStart); err == nil {
		if err := removeYurthubBinaryFunc(constants.YurthubExecStart); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	return copyEmbeddedYurthubBinaryFunc(constants.YurthubEmbeddedPath, constants.YurthubExecStart, 0755)
}
