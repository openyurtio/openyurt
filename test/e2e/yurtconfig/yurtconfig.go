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

package yurtconfig

import (
	clientset "k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
)

type YurtE2eConfig struct {
	NodeType           string
	RegionID           string
	AccessKeyID        string
	AccessKeySecret    string
	EnableYurtAutonomy bool
	KubeClient         *clientset.Clientset
	RestConfig         *restclient.Config
	ReportDir          string
}

var YurtE2eCfg YurtE2eConfig
