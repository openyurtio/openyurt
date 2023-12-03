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

package node_servant

import (
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"

	tmplutil "github.com/openyurtio/openyurt/pkg/util/templates"
)

// RenderNodeServantJob return k8s job
// to start k8s job to run convert/revert on specific node
func RenderNodeServantJob(action string, renderCtx map[string]string, nodeName string) (*batchv1.Job, error) {
	tmplCtx := make(map[string]string)
	for k, v := range renderCtx {
		tmplCtx[k] = v
	}
	if err := validate(action, tmplCtx, nodeName); err != nil {
		return nil, err
	}

	var servantJobTemplate, jobBaseName string
	switch action {
	case "convert":
		servantJobTemplate = ConvertServantJobTemplate
		jobBaseName = ConvertJobNameBase
	case "revert":
		servantJobTemplate = RevertServantJobTemplate
		jobBaseName = RevertJobNameBase
	}

	tmplCtx["jobName"] = jobBaseName + "-" + nodeName
	tmplCtx["nodeName"] = nodeName
	jobYaml, err := tmplutil.SubsituteTemplate(servantJobTemplate, tmplCtx)
	if err != nil {
		return nil, err
	}

	srvJobObj, err := YamlToObject([]byte(jobYaml))
	if err != nil {
		return nil, err
	}
	srvJob, ok := srvJobObj.(*batchv1.Job)
	if !ok {
		return nil, fmt.Errorf("could not assert node-servant job")
	}

	return srvJob, nil
}

// YamlToObject deserializes object in yaml format to a runtime.Object
func YamlToObject(yamlContent []byte) (k8sruntime.Object, error) {
	decode := serializer.NewCodecFactory(scheme.Scheme).UniversalDeserializer().Decode
	obj, _, err := decode(yamlContent, nil, nil)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

func validate(action string, tmplCtx map[string]string, nodeName string) error {
	if nodeName == "" {
		return fmt.Errorf("nodeName empty")
	}

	switch action {
	case "convert":
		keysMustHave := []string{"node_servant_image", "yurthub_image", "joinToken"}
		return checkKeys(keysMustHave, tmplCtx)
	case "revert":
		keysMustHave := []string{"node_servant_image"}
		return checkKeys(keysMustHave, tmplCtx)
	default:
		return fmt.Errorf("action invalied: %s ", action)
	}
}

func checkKeys(arr []string, tmplCtx map[string]string) error {
	for _, k := range arr {
		if _, ok := tmplCtx[k]; !ok {
			return fmt.Errorf("key %s not found", k)
		}
	}
	return nil
}
