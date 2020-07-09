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

package server

import (
	"errors"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

// StartIPNodeNameManager starts the IPNodename manager that will keep
// updating the global map nodeIP2Name based on the cluster status
func StartIPNodeNameManager(
	cliSet *kubernetes.Clientset,
	nodeIP2Name map[string]string,
	stopCh <-chan struct{}) (cache.Indexer, error) {
	return nil, errors.New("NOT IMPLEMENT YET")
}
