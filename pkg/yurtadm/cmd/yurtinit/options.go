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
	Password                string
	ImageRepository         string
	OpenYurtVersion         string
	K8sVersion              string
}

func NewInitOptions() *InitOptions {
	return &InitOptions{
		ImageRepository: constants.DefaultOpenYurtImageRegistry,
		OpenYurtVersion: constants.DefaultOpenYurtVersion,
		K8sVersion:      constants.DefaultK8sVersion,
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
