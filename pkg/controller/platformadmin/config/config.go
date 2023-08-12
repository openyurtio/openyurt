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

package config

import (
	"embed"
	"encoding/json"
	"path/filepath"

	"gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	util "github.com/openyurtio/openyurt/pkg/controller/platformadmin/utils"
)

type EdgeXConfig struct {
	Versions []*Version `yaml:"versions,omitempty" json:"versions,omitempty"`
}

type Version struct {
	Name       string             `yaml:"versionName" json:"versionName"`
	ConfigMaps []corev1.ConfigMap `yaml:"configMaps,omitempty" json:"configMaps,omitempty"`
	Components []*Component       `yaml:"components,omitempty" json:"components,omitempty"`
}

type Component struct {
	Name       string                 `yaml:"name" json:"name"`
	Service    *corev1.ServiceSpec    `yaml:"service,omitempty" json:"service,omitempty"`
	Deployment *appsv1.DeploymentSpec `yaml:"deployment,omitempty" json:"deployment,omitempty"`
}

var (
	//go:embed EdgeXConfig
	EdgeXFS      embed.FS
	folder       = "EdgeXConfig/"
	ManifestPath = filepath.Join(folder, "manifest.yaml")
	securityFile = filepath.Join(folder, "config.json")
	nosectyFile  = filepath.Join(folder, "config-nosecty.json")
)

// PlatformAdminControllerConfiguration contains elements describing PlatformAdminController.
type PlatformAdminControllerConfiguration struct {
	Manifest           util.Manifest
	SecurityComponents map[string][]*Component
	NoSectyComponents  map[string][]*Component
	SecurityConfigMaps map[string][]corev1.ConfigMap
	NoSectyConfigMaps  map[string][]corev1.ConfigMap
}

func NewPlatformAdminControllerConfiguration() *PlatformAdminControllerConfiguration {
	var (
		edgexconfig        = EdgeXConfig{}
		edgexnosectyconfig = EdgeXConfig{}
		conf               = PlatformAdminControllerConfiguration{
			Manifest:           util.Manifest{},
			SecurityComponents: make(map[string][]*Component),
			NoSectyComponents:  make(map[string][]*Component),
			SecurityConfigMaps: make(map[string][]corev1.ConfigMap),
			NoSectyConfigMaps:  make(map[string][]corev1.ConfigMap),
		}
	)

	// Read the EdgeX configuration file
	manifestContent, err := EdgeXFS.ReadFile(ManifestPath)
	if err != nil {
		klog.Errorf("File to open the embed EdgeX manifest file: %v", err)
		return nil
	}
	securityContent, err := EdgeXFS.ReadFile(securityFile)
	if err != nil {
		klog.Errorf("Fail to open the embed EdgeX security config: %v", err)
		return nil
	}
	nosectyContent, err := EdgeXFS.ReadFile(nosectyFile)
	if err != nil {
		klog.Errorf("Fail to open the embed EdgeX nosecty config: %v", err)
		return nil
	}

	// Unmarshal the EdgeX configuration file
	if err := yaml.Unmarshal(manifestContent, &conf.Manifest); err != nil {
		klog.Errorf("Error manifest EdgeX configuration file: %v", err)
		return nil
	}
	if err = json.Unmarshal(securityContent, &edgexconfig); err != nil {
		klog.Errorf("Fail to unmarshal the embed EdgeX security config: %v", err)
		return nil
	}
	for _, version := range edgexconfig.Versions {
		conf.SecurityComponents[version.Name] = version.Components
		conf.SecurityConfigMaps[version.Name] = version.ConfigMaps
	}

	if err := json.Unmarshal(nosectyContent, &edgexnosectyconfig); err != nil {
		klog.Errorf("Fail to unmarshal the embed EdgeX nosecty config: %v", err)
		return nil
	}
	for _, version := range edgexnosectyconfig.Versions {
		conf.NoSectyComponents[version.Name] = version.Components
		conf.NoSectyConfigMaps[version.Name] = version.ConfigMaps
	}

	return &conf
}
