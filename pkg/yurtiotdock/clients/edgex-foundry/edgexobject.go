/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package edgex_foundry

import (
	"fmt"

	"github.com/openyurtio/openyurt/pkg/yurtiotdock/clients"
	edgexcliv3 "github.com/openyurtio/openyurt/pkg/yurtiotdock/clients/edgex-foundry/v3"
)

type EdgeXObject interface {
	IsAddedToEdgeX() bool
}

type EdgexDock struct {
	Version          string
	CoreMetadataAddr string
	CoreCommandAddr  string
}

func NewEdgexDock(version string, coreMetadataAddr string, coreCommandAddr string) *EdgexDock {
	return &EdgexDock{
		Version:          version,
		CoreMetadataAddr: coreMetadataAddr,
		CoreCommandAddr:  coreCommandAddr,
	}
}

func (ep *EdgexDock) CreateDeviceClient() (clients.DeviceInterface, error) {
	switch ep.Version {
	case "napa", "minnesota":
		return edgexcliv3.NewEdgexDeviceClient(ep.CoreMetadataAddr, ep.CoreCommandAddr), nil
	default:
		return nil, fmt.Errorf("unsupported Edgex version: %v", ep.Version)
	}
}

func (ep *EdgexDock) CreateDeviceProfileClient() (clients.DeviceProfileInterface, error) {
	switch ep.Version {
	case "napa", "minnesota":
		return edgexcliv3.NewEdgexDeviceProfile(ep.CoreMetadataAddr), nil
	default:
		return nil, fmt.Errorf("unsupported Edgex version: %v", ep.Version)
	}
}

func (ep *EdgexDock) CreateDeviceServiceClient() (clients.DeviceServiceInterface, error) {
	switch ep.Version {
	case "napa", "minnesota":
		return edgexcliv3.NewEdgexDeviceServiceClient(ep.CoreMetadataAddr), nil
	default:
		return nil, fmt.Errorf("unsupported Edgex version: %v", ep.Version)
	}
}
