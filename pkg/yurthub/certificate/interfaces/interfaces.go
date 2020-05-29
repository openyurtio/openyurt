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

package interfaces

import (
	"github.com/alibaba/openyurt/cmd/yurthub/app/config"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/certificate"
)

// YurtCertificateManager is responsible for managing node certificate for yurthub
type YurtCertificateManager interface {
	certificate.Manager
	Update(cfg *config.YurtHubConfiguration) error
	GetRestConfig() *rest.Config
	GetCaFile() string
	NotExpired() bool
}
