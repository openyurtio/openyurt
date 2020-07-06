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

package kubernetes

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/spf13/pflag"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog"

	"github.com/alibaba/openyurt/pkg/yurtctl/constants"
	strutil "github.com/alibaba/openyurt/pkg/yurtctl/util/strings"
	tmplutil "github.com/alibaba/openyurt/pkg/yurtctl/util/templates"
)

const (
	// ConvertJobNameBase is the prefix of the convert ServantJob name
	ConvertJobNameBase = "yurtctl-servant-convert"
	// RevertJobNameBase is the prefix of the revert ServantJob name
	RevertJobNameBase = "yurtctl-servant-revert"
)

var (
	// PropagationPolicy defines the propagation policy used when deleting a resource
	PropagationPolicy = metav1.DeletePropagationForeground
	// WaitServantJobTimeout specifies the timeout value of waiting for the ServantJob to be succeeded
	WaitServantJobTimeout = time.Minute * 2
	// CheckServantJobPeriod defines the time interval between two successive ServantJob statu's inspection
	CheckServantJobPeriod = time.Second * 10
	// ValidServerVersion contains all compatable server version
	// yurtctl only support Kubernetes 1.12+ - 1.16+ for now
	ValidServerVersions = []string{
		"1.12", "1.12+",
		"1.13", "1.13+",
		"1.14", "1.14+",
		"1.16", "1.16+"}
)

// YamlToObject deserializes object in yaml format to a runtime.Object
func YamlToObject(yamlContent []byte) (runtime.Object, error) {
	decode := serializer.NewCodecFactory(scheme.Scheme).UniversalDeserializer().Decode
	obj, _, err := decode(yamlContent, nil, nil)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// LabelNode add a new label (<key>=<val>) to the given node
func LabelNode(cliSet *kubernetes.Clientset, node *v1.Node, key, val string) (*v1.Node, error) {
	node.Labels[key] = val
	newNode, err := cliSet.CoreV1().Nodes().Update(node)
	if err != nil {
		return nil, err
	}
	return newNode, nil
}

// AnnotateNode add a new annotation (<key>=<val>) to the given node
func AnnotateNode(cliSet *kubernetes.Clientset, node *v1.Node, key, val string) (*v1.Node, error) {
	node.Annotations[key] = val
	newNode, err := cliSet.CoreV1().Nodes().Update(node)
	if err != nil {
		return nil, err
	}
	return newNode, nil
}

// RunJobAndCleanup runs the job, wait for it to be complete, and delete it
func RunJobAndCleanup(cliSet *kubernetes.Clientset, job *batchv1.Job, timeout, period time.Duration) error {
	job, err := cliSet.BatchV1().Jobs(job.GetNamespace()).Create(job)
	if err != nil {
		return err
	}
	waitJobTimeout := time.After(timeout)
	for {
		select {
		case <-waitJobTimeout:
			return errors.New("wait for job to be complete timeout")
		case <-time.After(period):
			job, err := cliSet.BatchV1().Jobs(job.GetNamespace()).
				Get(job.GetName(), metav1.GetOptions{})
			if err != nil {
				klog.Errorf("fail to get job(%s) when waiting for it to be succeeded: %s",
					job.GetName(), err)
				return err
			}
			if job.Status.Succeeded == *job.Spec.Completions {
				if err := cliSet.BatchV1().Jobs(job.GetNamespace()).
					Delete(job.GetName(), &metav1.DeleteOptions{
						PropagationPolicy: &PropagationPolicy,
					}); err != nil {
					klog.Errorf("fail to delete succeeded servant job(%s): %s",
						job.GetName(), err)
					return err
				}
				return nil
			}
			continue
		}
	}
}

// RunServantJobs launchs servant jobs on specified edge nodes
func RunServantJobs(cliSet *kubernetes.Clientset, tmplCtx map[string]string, edgeNodeNames []string) error {
	var wg sync.WaitGroup
	for _, nodeName := range edgeNodeNames {
		action, exist := tmplCtx["action"]
		if !exist {
			return errors.New("action is not specified")
		}

		switch action {
		case "convert":
			tmplCtx["jobName"] = ConvertJobNameBase + "-" + nodeName
		case "revert":
			tmplCtx["jobName"] = RevertJobNameBase + "-" + nodeName
		default:
			return fmt.Errorf("unknown action: %s", action)
		}
		tmplCtx["nodeName"] = nodeName

		jobYaml, err := tmplutil.SubsituteTemplate(constants.ServantJobTemplate, tmplCtx)
		if err != nil {
			return err
		}
		srvJobObj, err := YamlToObject([]byte(jobYaml))
		if err != nil {
			return err
		}
		srvJob, ok := srvJobObj.(*batchv1.Job)
		if !ok {
			return errors.New("fail to assert yurtctl-servant job")
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := RunJobAndCleanup(cliSet, srvJob,
				WaitServantJobTimeout, CheckServantJobPeriod); err != nil {
				klog.Errorf("fail to run servant job(%s): %s",
					srvJob.GetName(), err)
			} else {
				klog.Infof("servant job(%s) has succeeded", srvJob.GetName())
			}
		}()
	}
	wg.Wait()
	return nil
}

// ValidateServerVersion checks if the target server's version is supported
func ValidateServerVersion(cliSet *kubernetes.Clientset) error {
	serverVersion, err := discovery.
		NewDiscoveryClient(cliSet.RESTClient()).ServerVersion()
	if err != nil {
		return err
	}
	completeVersion := serverVersion.Major + "." + serverVersion.Minor
	if !strutil.IsInStringLst(ValidServerVersions, completeVersion) {
		return fmt.Errorf("server version(%s) is not supported, valid server versions are %v",
			completeVersion, ValidServerVersions)
	}
	return nil
}

// GenClientSet generates the clientset based on command option, environment variable or
// the default kubeconfig file
func GenClientSet(flags *pflag.FlagSet) (*kubernetes.Clientset, error) {
	kbCfgPath, err := flags.GetString("kubeconfig")
	if err != nil {
		return nil, err
	}

	if kbCfgPath == "" {
		kbCfgPath = os.Getenv("KUBECONFIG")
	}

	if kbCfgPath == "" {
		if home := homedir.HomeDir(); home != "" {
			kbCfgPath = filepath.Join(home, ".kube", "config")
		}
	}

	if kbCfgPath == "" {
		return nil, errors.New("either '--kubeconfig', '$HOME/.kube/config' or '$KUBECONFIG' need to be set")
	}

	restCfg, err := clientcmd.BuildConfigFromFlags("", kbCfgPath)
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(restCfg)
}
