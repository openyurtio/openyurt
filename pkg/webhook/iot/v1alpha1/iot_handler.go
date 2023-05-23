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

package v1alpha1

import (
	"gopkg.in/yaml.v3"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/openyurtio/openyurt/pkg/apis/device/v1alpha1"
	"github.com/openyurtio/openyurt/pkg/controller/iot/config"
	"github.com/openyurtio/openyurt/pkg/webhook/util"
)

type Manifest struct {
	Updated       string   `yaml:"updated"`
	Count         int      `yaml:"count"`
	LatestVersion string   `yaml:"latestVersion"`
	Versions      []string `yaml:"versions"`
}

// SetupWebhookWithManager sets up Cluster webhooks. 	mutate path, validatepath, error
func (webhook *IoTHandler) SetupWebhookWithManager(mgr ctrl.Manager) (string, string, error) {
	// init
	webhook.Client = mgr.GetClient()

	gvk, err := apiutil.GVKForObject(&v1alpha1.IoT{}, mgr.GetScheme())
	if err != nil {
		return "", "", err
	}

	if err := webhook.initManifest(); err != nil {
		return "", "", err
	}

	return util.GenerateMutatePath(gvk),
		util.GenerateValidatePath(gvk),
		ctrl.NewWebhookManagedBy(mgr).
			For(&v1alpha1.IoT{}).
			WithDefaulter(webhook).
			WithValidator(webhook).
			Complete()
}

func (webhook *IoTHandler) initManifest() error {
	webhook.Manifests = &Manifest{}

	manifestContent, err := config.EdgeXFS.ReadFile(config.ManifestPath)
	if err != nil {
		klog.Error(err, "File to open the embed EdgeX manifest file")
		return err
	}

	if err := yaml.Unmarshal(manifestContent, webhook.Manifests); err != nil {
		klog.Error(err, "Error manifest edgeX configuration file")
		return err
	}

	return nil
}

// Cluster implements a validating and defaulting webhook for Cluster.
type IoTHandler struct {
	Client    client.Client
	Manifests *Manifest
}

var _ webhook.CustomDefaulter = &IoTHandler{}
var _ webhook.CustomValidator = &IoTHandler{}
