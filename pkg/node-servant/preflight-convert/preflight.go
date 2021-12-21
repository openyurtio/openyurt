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
	"fmt"

	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/preflight"
)

// ConvertPreflighter do the preflight-convert-convert job
type ConvertPreflighter struct {
	Options
}

// NewPreflighterWithOptions create nodePreflighter
func NewPreflighterWithOptions(o *Options) *ConvertPreflighter {
	return &ConvertPreflighter{
		*o,
	}
}

func (n *ConvertPreflighter) Do() error {
	klog.Infof("[preflight-convert] Running node-servant pre-flight checks")
	if err := preflight.RunConvertNodeChecks(n, n.IgnorePreflightErrors, n.DeployTunnel); err != nil {
		return err
	}

	fmt.Println("[preflight-convert] Pulling images required for converting a Kubernetes cluster to an OpenYurt cluster")
	fmt.Println("[preflight-convert] This might take a minute or two, depending on the speed of your internet connection")
	if err := preflight.RunPullImagesCheck(n, n.IgnorePreflightErrors); err != nil {
		return err
	}

	return nil
}
