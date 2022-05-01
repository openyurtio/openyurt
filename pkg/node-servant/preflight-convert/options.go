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

package preflight_convert

import (
	"os"
	"strings"

	"github.com/spf13/pflag"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openyurtio/openyurt/pkg/node-servant/components"
)

const (
	kubeAdmFlagsEnvFile = "/var/lib/kubelet/kubeadm-flags.env"
)

// Options has the information that required by preflight-convert operation
type Options struct {
	KubeadmConfPaths           []string
	YurthubImage               string
	YurttunnelAgentImage       string
	YurttunnelServerImage      string
	YurtAppManagerImage        string
	YurtControllerManagerImage string
	DeployTunnel               bool
	DeployAppManager           bool
	IgnorePreflightErrors      sets.String

	KubeAdmFlagsEnvFile string
	ImagePullPolicy     v1.PullPolicy
	CRISocket           string
}

func (o *Options) GetCRISocket() string {
	return o.CRISocket
}

func (o *Options) GetImageList() []string {
	imgs := []string{}

	// should we consider images for worker node and master node seperately?
	isCloudNode := false
	if os.Getenv("IS_CLOUD_NODE") == "TRUE" {
		isCloudNode = true
	}
	imgs = append(imgs, o.YurthubImage)
	if o.DeployTunnel {
		if isCloudNode {
			imgs = append(imgs, o.YurttunnelServerImage)
		} else {
			imgs = append(imgs, o.YurttunnelAgentImage)
		}
	}
	if isCloudNode {
		imgs = append(imgs, o.YurtControllerManagerImage)
		if o.DeployAppManager {
			imgs = append(imgs, o.YurtAppManagerImage)
		}
	}
	return imgs
}

func (o *Options) GetImagePullPolicy() v1.PullPolicy {
	return o.ImagePullPolicy
}

func (o *Options) GetKubeadmConfPaths() []string {
	return o.KubeadmConfPaths
}

func (o *Options) GetKubeAdmFlagsEnvFile() string {
	return o.KubeAdmFlagsEnvFile
}

// NewPreflightConvertOptions creates a new Options
func NewPreflightConvertOptions() *Options {
	return &Options{
		KubeadmConfPaths:      components.GetDefaultKubeadmConfPath(),
		IgnorePreflightErrors: sets.NewString(),
		KubeAdmFlagsEnvFile:   kubeAdmFlagsEnvFile,
		ImagePullPolicy:       v1.PullIfNotPresent,
	}
}

// Complete completes all the required options.
func (o *Options) Complete(flags *pflag.FlagSet) error {

	kubeadmConfPaths, err := flags.GetString("kubeadm-conf-path")
	if err != nil {
		return err
	}
	if kubeadmConfPaths != "" {
		o.KubeadmConfPaths = strings.Split(kubeadmConfPaths, ",")
	}

	yurthubImage, err := flags.GetString("yurthub-image")
	if err != nil {
		return err
	}
	o.YurthubImage = yurthubImage

	yurtControllerManagerImage, err := flags.GetString("yurt-controller-manager-image")
	if err != nil {
		return err
	}
	o.YurtControllerManagerImage = yurtControllerManagerImage

	yurtAppManagerImage, err := flags.GetString("yurt-app-manager-image")
	if err != nil {
		return err
	}
	o.YurtAppManagerImage = yurtAppManagerImage

	yurttunnelAgentImage, err := flags.GetString("yurt-tunnel-agent-image")
	if err != nil {
		return err
	}
	o.YurttunnelAgentImage = yurttunnelAgentImage

	yurttunnelServerImage, err := flags.GetString("yurt-tunnel-server-image")
	if err != nil {
		return err
	}
	o.YurttunnelServerImage = yurttunnelServerImage

	dt, err := flags.GetBool("deploy-yurt-tunnel")
	if err != nil {
		return err
	}
	o.DeployTunnel = dt

	et, err := flags.GetBool("deploy-app-manager")
	if err != nil {
		return err
	}
	o.DeployAppManager = et

	ipStr, err := flags.GetString("ignore-preflight-errors")
	if err != nil {
		return err
	}
	if ipStr != "" {
		ipStr = strings.ToLower(ipStr)
		o.IgnorePreflightErrors = sets.NewString(strings.Split(ipStr, ",")...)
	}

	CRISocket, err := components.DetectCRISocket()
	if err != nil {
		return err
	}
	o.CRISocket = CRISocket
	return nil
}
