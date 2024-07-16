/*
Copyright 2022 The OpenYurt Authors.

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

package storage

import "strings"

type Key interface {
	Key() string
}

type KeyBuildInfo struct {
	Component string
	Namespace string
	Name      string
	Resources string
	Group     string
	Version   string
}

type ClusterInfoKey struct {
	ClusterInfoType
	UrlPath string
}

type ClusterInfoType string

func (key *ClusterInfoKey) Key() string {
	switch key.ClusterInfoType {
	case APIsInfo, Version:
		return string(key.ClusterInfoType)
	case APIResourcesInfo:
		return strings.ReplaceAll(key.UrlPath, "/", "_")
	default:
		return ""
	}
}
