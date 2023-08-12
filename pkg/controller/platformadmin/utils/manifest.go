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

package util

type Manifest struct {
	Updated       string    `yaml:"updated"`
	Count         int       `yaml:"count"`
	LatestVersion string    `yaml:"latestVersion"`
	Versions      []Version `yaml:"versions"`
}

type Version struct {
	Name               string   `yaml:"name"`
	RequiredComponents []string `yaml:"requiredComponents"`
}

func ExtractVersionsName(manifest *Manifest) []string {
	var versionsName []string
	for _, version := range manifest.Versions {
		versionsName = append(versionsName, version.Name)
	}
	return versionsName
}
