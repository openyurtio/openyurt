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
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes/scheme"

	tmplutil "github.com/openyurtio/openyurt/pkg/util/templates"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
)

const (
	workingModeFlag = "working-mode"
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

	servantCommand, err := buildServantCommand(action, tmplCtx, nodeName)
	if err != nil {
		return nil, err
	}

	tmplCtx["backoffLimit"] = fmt.Sprintf("%d", DefaultConversionJobBackoffLimit)
	tmplCtx["conversionNodeLabelKey"] = ConversionNodeLabelKey
	tmplCtx["jobName"] = ConversionJobNameBase + "-" + nodeName
	tmplCtx["jobNamespace"] = valueOrDefault(tmplCtx["jobNamespace"], DefaultConversionJobNamespace)
	tmplCtx["nodeName"] = nodeName
	tmplCtx["servantCommand"] = servantCommand
	tmplCtx["ttlSecondsAfterFinished"] = fmt.Sprintf("%d", DefaultConversionJobTTLSecondsAfterFinished)

	jobYaml, err := tmplutil.SubstituteTemplate(ServantJobTemplate, tmplCtx)
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
	if strings.TrimSpace(nodeName) == "" {
		return fmt.Errorf("nodeName empty")
	}

	switch action {
	case "convert":
		keysMustHave := []string{"nodeServantImage", "nodePoolName"}
		return checkKeys(keysMustHave, tmplCtx)
	case "revert":
		keysMustHave := []string{"nodeServantImage"}
		return checkKeys(keysMustHave, tmplCtx)
	default:
		return fmt.Errorf("action invalid: %s ", action)
	}
}

func buildServantCommand(action string, tmplCtx map[string]string, nodeName string) (string, error) {
	switch action {
	case "convert":
		return buildConvertCommand(tmplCtx, nodeName), nil
	case "revert":
		return buildRevertCommand(nodeName), nil
	default:
		return "", fmt.Errorf("action invalid: %s ", action)
	}
}

func buildConvertCommand(tmplCtx map[string]string, nodeName string) string {
	args := []string{
		"convert",
		fmt.Sprintf("--%s=%s", constants.NodeName, nodeName),
		fmt.Sprintf("--%s=%s", constants.Namespace, valueOrDefault(tmplCtx["namespace"], DefaultConversionJobNamespace)),
		fmt.Sprintf("--%s=%s", workingModeFlag, valueOrDefault(tmplCtx["workingMode"], defaultWorkingMode)),
		fmt.Sprintf("--%s=%s", constants.NodePoolName, tmplCtx["nodePoolName"]),
	}

	if kubeadmConfPath := tmplCtx["kubeadmConfPath"]; kubeadmConfPath != "" {
		args = append(args, fmt.Sprintf("--kubeadm-conf-path=%s", kubeadmConfPath))
	}
	return strings.Join(args, " ")
}

func buildRevertCommand(nodeName string) string {
	return strings.Join([]string{
		"revert",
		fmt.Sprintf("--%s=%s", constants.NodeName, nodeName),
	}, " ")
}

func checkKeys(arr []string, tmplCtx map[string]string) error {
	for _, k := range arr {
		value, ok := tmplCtx[k]
		if !ok {
			return fmt.Errorf("key %s not found", k)
		}
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("key %s empty", k)
		}
	}
	return nil
}

func valueOrDefault(value, defaultValue string) string {
	if value == "" {
		return defaultValue
	}

	return value
}
