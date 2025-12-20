/*
Copyright 2021 The OpenYurt Authors.

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

package phases

import (
	"os"
	"path/filepath"

	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurtadm/cmd/join/joindata"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	yurtadmutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/kubernetes"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/system"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/yurthub"
)

// SystemManager abstracts all system-level operations and external dependencies
type SystemManager interface {
	// File system operations
	RemoveAll(path string) error

	// System configuration operations
	SetIpv4Forward() error
	SetBridgeSetting() error
	SetSELinux() error

	// Kubernetes component operations
	CheckAndInstallKubelet(server, version string) error
	CheckAndInstallKubeadm(server, version string) error
	CheckAndInstallKubernetesCni(reuseCNIBin bool) error
	SetKubeletService() error
	EnableKubeletService() error
	SetKubeletUnitConfig(data joindata.YurtJoinData) error
	SetKubeletConfigForNode() error
	SetDiscoveryConfig(data joindata.YurtJoinData) error
	SetKubeadmJoinConfig(data joindata.YurtJoinData) error

	// YurtHub operations
	SetHubBootstrapConfig(serverAddr, joinToken string, caCertHashes []string) error
	CheckAndInstallYurthub(version string) error
	CreateYurthubSystemdService(data joindata.YurtJoinData) error
}

// ProductionSystemManager is the production implementation of SystemManager
type ProductionSystemManager struct{}

// NewProductionSystemManager creates a new production system manager
func NewProductionSystemManager() SystemManager {
	return &ProductionSystemManager{}
}

// File system operations
func (p *ProductionSystemManager) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

// System configuration operations
func (p *ProductionSystemManager) SetIpv4Forward() error {
	return system.SetIpv4Forward()
}

func (p *ProductionSystemManager) SetBridgeSetting() error {
	return system.SetBridgeSetting()
}

func (p *ProductionSystemManager) SetSELinux() error {
	return system.SetSELinux()
}

// Kubernetes component operations
func (p *ProductionSystemManager) CheckAndInstallKubelet(server, version string) error {
	return yurtadmutil.CheckAndInstallKubelet(server, version)
}

func (p *ProductionSystemManager) CheckAndInstallKubeadm(server, version string) error {
	return yurtadmutil.CheckAndInstallKubeadm(server, version)
}

func (p *ProductionSystemManager) CheckAndInstallKubernetesCni(reuseCNIBin bool) error {
	return yurtadmutil.CheckAndInstallKubernetesCni(reuseCNIBin)
}

func (p *ProductionSystemManager) SetKubeletService() error {
	return yurtadmutil.SetKubeletService()
}

func (p *ProductionSystemManager) EnableKubeletService() error {
	return yurtadmutil.EnableKubeletService()
}

func (p *ProductionSystemManager) SetKubeletUnitConfig(data joindata.YurtJoinData) error {
	return yurtadmutil.SetKubeletUnitConfig(data)
}

func (p *ProductionSystemManager) SetKubeletConfigForNode() error {
	return yurtadmutil.SetKubeletConfigForNode()
}

func (p *ProductionSystemManager) SetDiscoveryConfig(data joindata.YurtJoinData) error {
	return yurtadmutil.SetDiscoveryConfig(data)
}

func (p *ProductionSystemManager) SetKubeadmJoinConfig(data joindata.YurtJoinData) error {
	return yurtadmutil.SetKubeadmJoinConfig(data)
}

// YurtHub operations
func (p *ProductionSystemManager) SetHubBootstrapConfig(serverAddr, joinToken string, caCertHashes []string) error {
	return yurthub.SetHubBootstrapConfig(serverAddr, joinToken, caCertHashes)
}

func (p *ProductionSystemManager) CheckAndInstallYurthub(version string) error {
	return yurthub.CheckAndInstallYurthub(version)
}

func (p *ProductionSystemManager) CreateYurthubSystemdService(data joindata.YurtJoinData) error {
	return yurthub.CreateYurthubSystemdService(data)
}

// RunPrepare executes the node initialization process.
// Maintains the original function signature, uses the new interface internally
func RunPrepare(data joindata.YurtJoinData) error {
	return RunPrepareWithManager(data, NewProductionSystemManager())
}

// RunPrepareWithManager executes the node initialization process using the specified SystemManager
func RunPrepareWithManager(data joindata.YurtJoinData, manager SystemManager) error {
	// cleanup at first
	if data.NodeRegistration().WorkingMode != constants.LocalNode {
		staticPodsPath := filepath.Join(constants.KubeletConfigureDir, constants.ManifestsSubDirName)
		if err := manager.RemoveAll(staticPodsPath); err != nil {
			klog.Warningf("remove %s: %v", staticPodsPath, err)
		}
	}

	if err := manager.SetIpv4Forward(); err != nil {
		return err
	}
	if err := manager.SetBridgeSetting(); err != nil {
		return err
	}
	if err := manager.SetSELinux(); err != nil {
		return err
	}
	if err := manager.CheckAndInstallKubelet(data.KubernetesResourceServer(), data.KubernetesVersion()); err != nil {
		return err
	}
	if err := manager.CheckAndInstallKubeadm(data.KubernetesResourceServer(), data.KubernetesVersion()); err != nil {
		return err
	}
	if data.NodeRegistration().WorkingMode != constants.LocalNode {
		if err := manager.CheckAndInstallKubernetesCni(data.ReuseCNIBin()); err != nil {
			return err
		}
	}
	if err := manager.SetKubeletService(); err != nil {
		return err
	}
	if err := manager.EnableKubeletService(); err != nil {
		return err
	}
	if err := manager.SetKubeletUnitConfig(data); err != nil {
		return err
	}

	if data.NodeRegistration().WorkingMode != constants.LocalNode {
		if err := manager.SetKubeletConfigForNode(); err != nil {
			return err
		}
		if err := manager.SetHubBootstrapConfig(data.ServerAddr(), data.JoinToken(), data.CaCertHashes()); err != nil {
			return err
		}
		if err := manager.CheckAndInstallYurthub(constants.YurthubVersion); err != nil {
			return err
		}
		if err := manager.CreateYurthubSystemdService(data); err != nil {
			return err
		}
	}
	if err := manager.SetDiscoveryConfig(data); err != nil {
		return err
	}
	if data.CfgPath() == "" {
		if err := manager.SetKubeadmJoinConfig(data); err != nil {
			return err
		}
	}
	return nil
}
