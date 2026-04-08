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

package yurthub

import (
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/openyurtio/openyurt/pkg/yurtadm/cmd/join/joindata"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
)

var (
	yurthubBootstrapConfigPath = constants.YurtHubBootstrapConfig
	yurthubExecStartPath       = constants.YurthubExecStart
	yurthubServiceFilePath     = constants.YurthubServicePath
	yurthubServiceConfFilePath = constants.YurthubServiceConfPath
	yurthubWorkDirPath         = constants.YurtHubWorkdir
	yurthubCacheDirPath        = disk.CacheBaseDir
)

// YurthubHostConfig describes the host-side installation and systemd
// configuration needed to run yurthub outside of static pod mode.
type YurthubHostConfig struct {
	BindAddress   string
	BootstrapFile string
	BootstrapMode string
	Namespace     string
	NodeName      string
	NodePoolName  string
	ServerAddr    string
	WorkingMode   string
}

// NewYurthubHostConfigFromJoinData converts join-scoped data into the smaller
// host-side config used by the shared lifecycle helpers.
func NewYurthubHostConfigFromJoinData(data joindata.YurtJoinData) *YurthubHostConfig {
	return &YurthubHostConfig{
		BindAddress:   constants.DefaultYurtHubServerAddr,
		BootstrapFile: yurthubBootstrapConfigPath,
		Namespace:     data.Namespace(),
		NodeName:      data.NodeRegistration().Name,
		NodePoolName:  data.NodeRegistration().NodePoolName,
		ServerAddr:    data.ServerAddr(),
		WorkingMode:   data.NodeRegistration().WorkingMode,
	}
}

func (cfg *YurthubHostConfig) validateForSystemdService() error {
	if cfg == nil {
		return errors.New("yurthub host config is nil")
	}

	if len(cfg.NodeName) == 0 {
		return errors.New("node name is empty")
	}

	if len(cfg.ServerAddr) == 0 {
		return errors.New("server address is empty")
	}

	if len(cfg.WorkingMode) == 0 {
		return errors.New("working mode is empty")
	}

	if len(cfg.BootstrapMode) == 0 && len(cfg.BootstrapFile) == 0 {
		return errors.New("bootstrap mode and bootstrap file are both empty")
	}

	return nil
}

func (cfg *YurthubHostConfig) bindAddress() string {
	if len(cfg.BindAddress) != 0 {
		return cfg.BindAddress
	}
	return constants.DefaultYurtHubServerAddr
}

func (cfg *YurthubHostConfig) namespace() string {
	if len(cfg.Namespace) != 0 {
		return cfg.Namespace
	}
	return constants.YurthubNamespace
}

func (cfg *YurthubHostConfig) normalizedServerAddr() string {
	addrs := strings.Split(cfg.ServerAddr, ",")
	for i := range addrs {
		addr := strings.TrimSpace(addrs[i])
		if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
			addrs[i] = addr
			continue
		}
		addrs[i] = fmt.Sprintf("https://%s", addr)
	}
	return strings.Join(addrs, ",")
}

func (cfg *YurthubHostConfig) bootstrapArgs() string {
	args := make([]string, 0, 2)
	if len(cfg.BootstrapMode) != 0 {
		args = append(args, fmt.Sprintf("--bootstrap-mode=%s", cfg.BootstrapMode))
	}
	if len(cfg.BootstrapFile) != 0 {
		args = append(args, fmt.Sprintf("--bootstrap-file=%s", cfg.BootstrapFile))
	}
	return strings.Join(args, " ")
}
