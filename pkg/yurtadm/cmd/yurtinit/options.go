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

package yurtinit

import (
	"fmt"
	"net"
	"strings"

	"github.com/pkg/errors"

	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
)

// InitOptions  defines all the init options exposed via flags by yurtadm init.
type InitOptions struct {
	AdvertiseAddress        string
	YurttunnelServerAddress string
	ServiceSubnet           string
	PodSubnet               string
	ClusterCIDR             string
	Password                string
	ImageRepository         string
	OpenYurtVersion         string
	K8sVersion              string
	KubeProxyBindAddress    string
}

func NewInitOptions() *InitOptions {
	return &InitOptions{
		ImageRepository:      constants.DefaultOpenYurtImageRegistry,
		OpenYurtVersion:      constants.DefaultOpenYurtVersion,
		K8sVersion:           constants.DefaultK8sVersion,
		PodSubnet:            constants.DefaultPodSubnet,
		ServiceSubnet:        constants.DefaultServiceSubnet,
		ClusterCIDR:          constants.DefaultClusterCIDR,
		KubeProxyBindAddress: constants.DefaultKubeProxyBindAddress,
	}
}

func (o *InitOptions) Validate() error {
	// There may be multiple ip addresses, separated by commas.
	if o.AdvertiseAddress != "" {
		ipArray := strings.Split(o.AdvertiseAddress, ",")
		for _, ip := range ipArray {
			if err := validateServerAddress(ip); err != nil {
				return err
			}
		}
	}

	if o.OpenYurtVersion == "" {
		return errors.New("OpenYurtVersion can not be empty!")
	}

	if o.K8sVersion == "" {
		return errors.New("K8sVersion can not be empty!")
	}

	if err := validateOpenYurtAndK8sVersions(o.OpenYurtVersion, o.K8sVersion); err != nil {
		return err
	}

	if o.YurttunnelServerAddress != "" {
		if err := validateServerAddress(o.YurttunnelServerAddress); err != nil {
			return err
		}
	}

	if o.Password == "" {
		return fmt.Errorf("password can't be empty.")
	}

	if o.PodSubnet != "" {
		if err := validateCidrString(o.PodSubnet); err != nil {
			return err
		}
	}

	if o.ServiceSubnet != "" {
		if err := validateCidrString(o.ServiceSubnet); err != nil {
			return err
		}
	}

	return nil
}

func validateOpenYurtAndK8sVersions(openyurtVersion string, k8sVersion string) error {
	for _, version := range ValidOpenYurtAndK8sVersions {
		if openyurtVersion == version.OpenYurtVersion && k8sVersion == openyurtVersion {
			return nil
		}
	}

	return errors.Errorf("cannot use openyurtVersion: %s and k8sVersion: %s !", openyurtVersion, k8sVersion)
}

func validateServerAddress(address string) error {
	ip := net.ParseIP(address)
	if ip == nil {
		return errors.Errorf("cannot parse IP address: %s", address)
	}
	if !ip.IsGlobalUnicast() {
		return errors.Errorf("cannot use %q as the bind address for the API Server", address)
	}
	return nil
}

func validateCidrString(cidr string) error {
	_, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil
	}
	return nil
}
