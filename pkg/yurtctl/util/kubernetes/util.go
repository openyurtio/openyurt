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
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	pkgerrors "github.com/pkg/errors"
	"github.com/spf13/pflag"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	"k8s.io/api/admissionregistration/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/client-go/util/homedir"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	bootstraputil "k8s.io/cluster-bootstrap/token/util"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurtctl/constants"
	kubeadmapi "github.com/openyurtio/openyurt/pkg/yurtctl/kubernetes/kubeadm/app/apis/kubeadm"
	kubeadmconstants "github.com/openyurtio/openyurt/pkg/yurtctl/kubernetes/kubeadm/app/constants"
	nodetoken "github.com/openyurtio/openyurt/pkg/yurtctl/kubernetes/kubeadm/app/phases/bootstraptoken/node"
	"github.com/openyurtio/openyurt/pkg/yurtctl/kubernetes/kubeadm/app/util/apiclient"
	kubeconfigutil "github.com/openyurtio/openyurt/pkg/yurtctl/kubernetes/kubeadm/app/util/kubeconfig"
	"github.com/openyurtio/openyurt/pkg/yurtctl/util"
	"github.com/openyurtio/openyurt/pkg/yurtctl/util/edgenode"
	strutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/strings"
	tmplutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/templates"
)

const (
	// DisableNodeControllerJobNameBase is the prefix of the DisableNodeControllerJob name
	DisableNodeControllerJobNameBase = "yurtctl-disable-node-controller"
	// EnableNodeControllerJobNameBase is the prefix of the EnableNodeControllerJob name
	EnableNodeControllerJobNameBase = "yurtctl-enable-node-controller"
	SystemNamespace                 = "kube-system"
	// DefaultWaitServantJobTimeout specifies the timeout value of waiting for the ServantJob to be succeeded
	DefaultWaitServantJobTimeout = time.Minute * 5
)

var (
	// PropagationPolicy defines the propagation policy used when deleting a resource
	PropagationPolicy = metav1.DeletePropagationBackground
	// CheckServantJobPeriod defines the time interval between two successive ServantJob statu's inspection
	CheckServantJobPeriod = time.Second * 10
	// ValidServerVersions contains all compatible server version
	// yurtctl only support Kubernetes 1.12+ - 1.23+ for now
	ValidServerVersions = []string{
		"1.12", "1.12+",
		"1.13", "1.13+",
		"1.14", "1.14+",
		"1.16", "1.16+",
		"1.18", "1.18+",
		"1.19", "1.19+",
		"1.20", "1.20+",
		"1.21", "1.21+",
		"1.22", "1.22+",
		"1.23", "1.23+"}

	ErrClusterVersionEmpty = errors.New("cluster version should not be empty")
)

func processCreateErr(kind string, name string, err error) error {
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			klog.V(4).Infof("[WARNING] %s/%s is already in cluster, skip to prepare it", kind, name)
			return nil
		}
		return fmt.Errorf("fail to create the %s/%s: %v", kind, name, err)
	}
	klog.V(4).Infof("%s/%s is created", kind, name)
	return nil
}

// CreateServiceAccountFromYaml creates the ServiceAccount from the yaml template.
func CreateServiceAccountFromYaml(cliSet *kubernetes.Clientset, ns, saTmpl string) error {
	obj, err := YamlToObject([]byte(saTmpl))
	if err != nil {
		return err
	}
	sa, ok := obj.(*corev1.ServiceAccount)
	if !ok {
		return fmt.Errorf("fail to assert serviceaccount: %v", err)
	}
	_, err = cliSet.CoreV1().ServiceAccounts(ns).Create(context.Background(), sa, metav1.CreateOptions{})
	return processCreateErr("serviceaccount", sa.Name, err)
}

// CreateClusterRoleFromYaml creates the ClusterRole from the yaml template.
func CreateClusterRoleFromYaml(cliSet *kubernetes.Clientset, crTmpl string) error {
	obj, err := YamlToObject([]byte(crTmpl))
	if err != nil {
		return err
	}
	cr, ok := obj.(*rbacv1.ClusterRole)
	if !ok {
		return fmt.Errorf("fail to assert clusterrole: %v", err)
	}
	_, err = cliSet.RbacV1().ClusterRoles().Create(context.Background(), cr, metav1.CreateOptions{})
	return processCreateErr("clusterrole", cr.Name, err)
}

// CreateClusterRoleBindingFromYaml creates the ClusterRoleBinding from the yaml template.
func CreateClusterRoleBindingFromYaml(cliSet *kubernetes.Clientset, crbTmpl string) error {
	obj, err := YamlToObject([]byte(crbTmpl))
	if err != nil {
		return err
	}
	crb, ok := obj.(*rbacv1.ClusterRoleBinding)
	if !ok {
		return fmt.Errorf("fail to assert clusterrolebinding: %v", err)
	}
	_, err = cliSet.RbacV1().ClusterRoleBindings().Create(context.Background(), crb, metav1.CreateOptions{})
	return processCreateErr("clusterrolebinding", crb.Name, err)
}

// CreateConfigMapFromYaml creates the ConfigMap from the yaml template.
func CreateConfigMapFromYaml(cliSet *kubernetes.Clientset, ns, cmTmpl string) error {
	obj, err := YamlToObject([]byte(cmTmpl))
	if err != nil {
		return err
	}
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return fmt.Errorf("fail to assert configmap: %v", err)
	}
	_, err = cliSet.CoreV1().ConfigMaps(ns).Create(context.Background(), cm, metav1.CreateOptions{})
	return processCreateErr("configmap", cm.Name, err)
}

// CreateDeployFromYaml creates the Deployment from the yaml template.
func CreateDeployFromYaml(cliSet *kubernetes.Clientset, ns, dplyTmpl string, ctx interface{}) error {
	ycmdp, err := tmplutil.SubsituteTemplate(dplyTmpl, ctx)
	if err != nil {
		return err
	}
	dpObj, err := YamlToObject([]byte(ycmdp))
	if err != nil {
		return err
	}
	dply, ok := dpObj.(*appsv1.Deployment)
	if !ok {
		return errors.New("fail to assert Deployment")
	}
	if _, err = cliSet.AppsV1().Deployments(ns).Create(context.Background(), dply, metav1.CreateOptions{}); err != nil {
		return err
	}
	klog.V(4).Infof("the deployment/%s is deployed", dply.Name)
	return nil
}

// CreateDaemonSetFromYaml creates the DaemonSet from the yaml template.
func CreateDaemonSetFromYaml(cliSet *kubernetes.Clientset, ns, dsTmpl string, ctx interface{}) error {
	var ytadstmp string
	var err error
	if ctx != nil {
		ytadstmp, err = tmplutil.SubsituteTemplate(dsTmpl, ctx)
		if err != nil {
			return err
		}
	} else {
		ytadstmp = dsTmpl
	}

	obj, err := YamlToObject([]byte(ytadstmp))
	if err != nil {
		return err
	}
	ds, ok := obj.(*appsv1.DaemonSet)
	if !ok {
		return fmt.Errorf("fail to assert daemonset: %v", err)
	}
	_, err = cliSet.AppsV1().DaemonSets(ns).Create(context.Background(), ds, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("fail to create the daemonset/%s: %v", ds.Name, err)
	}
	klog.V(4).Infof("daemonset/%s is created", ds.Name)
	return nil
}

// CreateServiceFromYaml creates the Service from the yaml template.
func CreateServiceFromYaml(cliSet *kubernetes.Clientset, ns, svcTmpl string) error {
	obj, err := YamlToObject([]byte(svcTmpl))
	if err != nil {
		return err
	}
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return fmt.Errorf("fail to assert service: %v", err)
	}
	_, err = cliSet.CoreV1().Services(ns).Create(context.Background(), svc, metav1.CreateOptions{})
	return processCreateErr("service", svc.Name, err)
}

//add by yanyhui at 20210611
// CreateRoleFromYaml creates the ClusterRole from the yaml template.
func CreateRoleFromYaml(cliSet *kubernetes.Clientset, ns, crTmpl string) error {
	obj, err := YamlToObject([]byte(crTmpl))
	if err != nil {
		return err
	}
	ro, ok := obj.(*rbacv1.Role)
	if !ok {
		return fmt.Errorf("fail to assert role: %v", err)
	}
	_, err = cliSet.RbacV1().Roles(ns).Create(context.Background(), ro, metav1.CreateOptions{})
	return processCreateErr("role", ro.Name, err)
}

// CreateRoleBindingFromYaml creates the ClusterRoleBinding from the yaml template.
func CreateRoleBindingFromYaml(cliSet *kubernetes.Clientset, ns, crbTmpl string) error {
	obj, err := YamlToObject([]byte(crbTmpl))
	if err != nil {
		return err
	}
	rb, ok := obj.(*rbacv1.RoleBinding)
	if !ok {
		return fmt.Errorf("fail to assert rolebinding: %v", err)
	}
	_, err = cliSet.RbacV1().RoleBindings(ns).Create(context.Background(), rb, metav1.CreateOptions{})
	return processCreateErr("rolebinding", rb.Name, err)
}

// CreateSecretFromYaml creates the Secret from the yaml template.
func CreateSecretFromYaml(cliSet *kubernetes.Clientset, ns, saTmpl string) error {
	obj, err := YamlToObject([]byte(saTmpl))
	if err != nil {
		return err
	}
	se, ok := obj.(*corev1.Secret)
	if !ok {
		return fmt.Errorf("fail to assert secret: %v", err)
	}
	_, err = cliSet.CoreV1().Secrets(ns).Create(context.Background(), se, metav1.CreateOptions{})

	return processCreateErr("secret", se.Name, err)
}

// CreateMutatingWebhookConfigurationFromYaml creates the Service from the yaml template.
func CreateMutatingWebhookConfigurationFromYaml(cliSet *kubernetes.Clientset, svcTmpl string) error {
	obj, err := YamlToObject([]byte(svcTmpl))
	if err != nil {
		return err
	}

	mwv1, ok := obj.(*admissionv1.MutatingWebhookConfiguration)
	if ok {
		_, err = cliSet.AdmissionregistrationV1().MutatingWebhookConfigurations().Create(context.Background(), mwv1, metav1.CreateOptions{})
		return processCreateErr("mutatingwebhookconfiguration", mwv1.Name, err)
	}

	mv, ok := obj.(*v1beta1.MutatingWebhookConfiguration)
	if !ok {
		return fmt.Errorf("fail to assert mutatingwebhookconfiguration: %v", err)
	}
	_, err = cliSet.AdmissionregistrationV1beta1().MutatingWebhookConfigurations().Create(context.Background(), mv, metav1.CreateOptions{})
	return processCreateErr("mutatingwebhookconfiguration", mv.Name, err)
}

// CreateValidatingWebhookConfigurationFromYaml creates the Service from the yaml template.
func CreateValidatingWebhookConfigurationFromYaml(cliSet *kubernetes.Clientset, svcTmpl string) error {
	obj, err := YamlToObject([]byte(svcTmpl))
	if err != nil {
		return err
	}

	vwv1, ok := obj.(*admissionv1.ValidatingWebhookConfiguration)
	if ok {
		_, err = cliSet.AdmissionregistrationV1().ValidatingWebhookConfigurations().Create(context.Background(), vwv1, metav1.CreateOptions{})
		return processCreateErr("validatingwebhookconfiguration", vwv1.Name, err)
	}

	vw, ok := obj.(*v1beta1.ValidatingWebhookConfiguration)
	if !ok {
		return fmt.Errorf("fail to assert validatingwebhookconfiguration: %v", err)
	}
	_, err = cliSet.AdmissionregistrationV1beta1().ValidatingWebhookConfigurations().Create(context.Background(), vw, metav1.CreateOptions{})
	return processCreateErr("validatingwebhookconfiguration", vw.Name, err)
}

func CreateCRDFromYaml(clientset *kubernetes.Clientset, yurtAppManagerClient dynamic.Interface, nameSpace string, filebytes []byte) error {
	var err error
	decoder := yamlutil.NewYAMLOrJSONDecoder(bytes.NewReader(filebytes), 10000)
	var rawObj k8sruntime.RawExtension
	err = decoder.Decode(&rawObj)
	if err != nil {
		return err
	}
	obj, gvk, err := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme).Decode(rawObj.Raw, nil, nil)
	if err != nil {
		return err
	}
	unstructuredMap, err := k8sruntime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return err
	}
	unstructuredObj := &unstructured.Unstructured{Object: unstructuredMap}
	gr, err := restmapper.GetAPIGroupResources(clientset.Discovery())
	if err != nil {
		return err
	}

	mapper := restmapper.NewDiscoveryRESTMapper(gr)
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return err
	}

	var dri dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		if unstructuredObj.GetNamespace() == "" {
			unstructuredObj.SetNamespace(nameSpace)
		}
		dri = yurtAppManagerClient.Resource(mapping.Resource).Namespace(unstructuredObj.GetNamespace())
	} else {
		dri = yurtAppManagerClient.Resource(mapping.Resource)
	}

	objSecond, err := dri.Create(context.Background(), unstructuredObj, metav1.CreateOptions{})
	if err != nil {
		return err
	} else {
		fmt.Printf("%s/%s created", objSecond.GetKind(), objSecond.GetName())
	}
	return nil
}

func DeleteCRDResource(clientset *kubernetes.Clientset, yurtAppManagerClientSet dynamic.Interface, res string, name string, filebytes []byte) error {
	var err error
	decoder := yamlutil.NewYAMLOrJSONDecoder(bytes.NewReader(filebytes), 10000)
	var rawObj k8sruntime.RawExtension
	err = decoder.Decode(&rawObj)
	if err != nil {
		return err
	}
	obj, gvk, err := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme).Decode(rawObj.Raw, nil, nil)
	if err != nil {
		return err
	}
	unstructuredMap, err := k8sruntime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		return err
	}
	unstructuredObj := &unstructured.Unstructured{Object: unstructuredMap}
	gr, err := restmapper.GetAPIGroupResources(clientset.Discovery())
	if err != nil {
		return err
	}

	mapper := restmapper.NewDiscoveryRESTMapper(gr)
	mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return err
	}

	var dri dynamic.ResourceInterface
	if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
		if unstructuredObj.GetNamespace() == "" {
			unstructuredObj.SetNamespace("")
		}
		dri = yurtAppManagerClientSet.Resource(mapping.Resource).Namespace(unstructuredObj.GetNamespace())
	} else {
		dri = yurtAppManagerClientSet.Resource(mapping.Resource)
	}
	err = dri.Delete(context.Background(), name, metav1.DeleteOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}
	} else {
		fmt.Printf("%s/%s is deleted ", res, name)
	}
	return nil
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

// LabelNode add a new label (<key>=<val>) to the given node
func LabelNode(cliSet *kubernetes.Clientset, node *corev1.Node, key, val string) (*corev1.Node, error) {
	node.Labels[key] = val
	newNode, err := cliSet.CoreV1().Nodes().Update(context.Background(), node, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	return newNode, nil
}

// AnnotateNode add a new annotation (<key>=<val>) to the given node
func AnnotateNode(cliSet *kubernetes.Clientset, node *corev1.Node, key, val string) (*corev1.Node, error) {
	node.Annotations[key] = val
	newNode, err := cliSet.CoreV1().Nodes().Update(context.Background(), node, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	return newNode, nil
}

func AddEdgeWorkerLabelAndAutonomyAnnotation(cliSet *kubernetes.Clientset, node *corev1.Node, lVal, aVal string) (*corev1.Node, error) {
	node.Labels[projectinfo.GetEdgeWorkerLabelKey()] = lVal
	node.Annotations[projectinfo.GetAutonomyAnnotation()] = aVal
	newNode, err := cliSet.CoreV1().Nodes().Update(context.Background(), node, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	return newNode, nil
}

// RunJobAndCleanup runs the job, wait for it to be complete, and delete it
func RunJobAndCleanup(cliSet *kubernetes.Clientset, job *batchv1.Job, timeout, period time.Duration) error {
	job, err := cliSet.BatchV1().Jobs(job.GetNamespace()).Create(context.Background(), job, metav1.CreateOptions{})
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
				Get(context.Background(), job.GetName(), metav1.GetOptions{})
			if err != nil {
				klog.Errorf("fail to get job(%s) when waiting for it to be succeeded: %s",
					job.GetName(), err)
				return err
			}
			if job.Status.Succeeded == *job.Spec.Completions {
				if err := cliSet.BatchV1().Jobs(job.GetNamespace()).
					Delete(context.Background(), job.GetName(), metav1.DeleteOptions{
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

// RenderServantJob renders servant job for a specified node.
func RenderServantJob(action string, tmplCtx map[string]string, nodeName string) (*batchv1.Job, error) {
	var jobTemplate string
	switch action {
	case "enable":
		jobTemplate = constants.EnableNodeControllerJobTemplate
		tmplCtx["jobName"] = EnableNodeControllerJobNameBase + "-" + nodeName
	case "disable":
		jobTemplate = constants.DisableNodeControllerJobTemplate
		tmplCtx["jobName"] = DisableNodeControllerJobNameBase + "-" + nodeName
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
	tmplCtx["nodeName"] = nodeName
	jobYaml, err := tmplutil.SubsituteTemplate(jobTemplate, tmplCtx)
	if err != nil {
		return nil, err
	}
	srvJobObj, err := YamlToObject([]byte(jobYaml))
	if err != nil {
		return nil, err
	}
	srvJob, ok := srvJobObj.(*batchv1.Job)
	if !ok {
		return nil, errors.New("fail to assert yurtctl-servant job")
	}
	return srvJob, nil
}

// RunServantJobs launch servant jobs on specified nodes and wait all jobs to finish.
// Succeed jobs will be deleted when finished. Failed jobs are preserved for diagnosis.
func RunServantJobs(
	cliSet *kubernetes.Clientset,
	waitServantJobTimeout time.Duration,
	getJob func(nodeName string) (*batchv1.Job, error),
	nodeNames []string, ww io.Writer) error {
	var wg sync.WaitGroup

	jobByNodeName := make(map[string]*batchv1.Job)
	for _, nodeName := range nodeNames {
		job, err := getJob(nodeName)
		if err != nil {
			return fmt.Errorf("fail to get job for node %s: %s", nodeName, err)
		}
		jobByNodeName[nodeName] = job
	}

	res := make(chan string, len(nodeNames))
	for _, nodeName := range nodeNames {
		wg.Add(1)
		job := jobByNodeName[nodeName]
		go func() {
			defer wg.Done()
			if err := RunJobAndCleanup(cliSet, job,
				waitServantJobTimeout, CheckServantJobPeriod); err != nil {
				msg := fmt.Sprintf("\t[ERROR] fail to run servant job(%s): %s\n", job.GetName(), err)
				res <- msg
			} else {
				msg := fmt.Sprintf("\t[INFO] servant job(%s) has succeeded\n", job.GetName())
				res <- msg
			}
		}()
	}
	wg.Wait()
	close(res)
	for m := range res {
		io.WriteString(ww, m)
	}
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
	kubeconfigPath, err := PrepareKubeConfigPath(flags)
	if err != nil {
		return nil, err
	}

	restCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(restCfg)
}

// GenDynamicClientSet generates the clientset based on command option, environment variable or
// the default kubeconfig file
func GenDynamicClientSet(flags *pflag.FlagSet) (dynamic.Interface, error) {
	kubeconfigPath, err := PrepareKubeConfigPath(flags)
	if err != nil {
		return nil, err
	}

	restCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, err
	}

	return dynamic.NewForConfig(restCfg)
}

// PrepareKubeConfigPath returns the path of cluster kubeconfig file
func PrepareKubeConfigPath(flags *pflag.FlagSet) (string, error) {
	kbCfgPath, err := flags.GetString("kubeconfig")
	if err != nil {
		return "", err
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
		return "", errors.New("either '--kubeconfig', '$HOME/.kube/config' or '$KUBECONFIG' need to be set")
	}

	return kbCfgPath, nil
}

func GetOrCreateJoinTokenString(cliSet *kubernetes.Clientset) (string, error) {
	tokenSelector := fields.SelectorFromSet(
		map[string]string{
			// TODO: We hard-code "type" here until `field_constants.go` that is
			// currently in `pkg/apis/core/` exists in the external API, i.e.
			// k8s.io/api/v1. Should be v1.SecretTypeField
			"type": string(bootstrapapi.SecretTypeBootstrapToken),
		},
	)
	listOptions := metav1.ListOptions{
		FieldSelector: tokenSelector.String(),
	}
	klog.V(1).Infoln("[token] retrieving list of bootstrap tokens")
	secrets, err := cliSet.CoreV1().Secrets(metav1.NamespaceSystem).List(context.Background(), listOptions)
	if err != nil {
		return "", fmt.Errorf("%v%s", err, "failed to list bootstrap tokens")
	}

	for _, secret := range secrets.Items {

		// Get the BootstrapToken struct representation from the Secret object
		token, err := kubeadmapi.BootstrapTokenFromSecret(&secret)
		if err != nil {
			klog.Warningf("%v", err)
			continue
		}
		if !usagesAndGroupsAreValid(token) {
			continue
		}

		return token.Token.String(), nil
		// Get the human-friendly string representation for the token
	}

	tokenStr, err := bootstraputil.GenerateBootstrapToken()
	if err != nil {
		return "", fmt.Errorf("couldn't generate random token, %v", err)
	}
	token, err := kubeadmapi.NewBootstrapTokenString(tokenStr)
	if err != nil {
		return "", err
	}

	klog.V(1).Infoln("[token] creating token")
	if err := nodetoken.CreateNewTokens(cliSet,
		[]kubeadmapi.BootstrapToken{{
			Token:  token,
			Usages: kubeadmconstants.DefaultTokenUsages,
			Groups: kubeadmconstants.DefaultTokenGroups,
		}}); err != nil {
		return "", err
	}
	return tokenStr, nil
}

// usagesAndGroupsAreValid checks if the usages and groups in the given bootstrap token are valid
func usagesAndGroupsAreValid(token *kubeadmapi.BootstrapToken) bool {
	sliceEqual := func(a, b []string) bool {
		if len(a) != len(b) {
			return false
		}
		sort.Strings(a)
		sort.Strings(b)
		for k, v := range b {
			if a[k] != v {
				return false
			}
		}
		return true
	}

	return sliceEqual(token.Usages, kubeadmconstants.DefaultTokenUsages) && sliceEqual(token.Groups, kubeadmconstants.DefaultTokenGroups)
}

// find kube-controller-manager deployed through static file
func GetKubeControllerManagerHANodes(cliSet *kubernetes.Clientset) ([]string, error) {
	var kcmNodeNames []string
	podLst, err := cliSet.CoreV1().Pods(SystemNamespace).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, pod := range podLst.Items {
		kcmPodName := fmt.Sprintf("kube-controller-manager-%s", pod.Spec.NodeName)
		if kcmPodName == pod.Name {
			kcmNodeNames = append(kcmNodeNames, pod.Spec.NodeName)
		}
	}
	return kcmNodeNames, nil
}

//CheckAndInstallKubelet install kubelet and kubernetes-cni, skip install if they exist.
func CheckAndInstallKubelet(kubernetesResourceServer, clusterVersion string) error {
	if strings.Contains(clusterVersion, "-") {
		clusterVersion = strings.Split(clusterVersion, "-")[0]
	}

	klog.Infof("Check and install kubelet %s", clusterVersion)
	if clusterVersion == "" {
		return ErrClusterVersionEmpty
	}
	kubeletExist := false
	if _, err := exec.LookPath("kubelet"); err == nil {
		if b, err := exec.Command("kubelet", "--version").CombinedOutput(); err == nil {
			kubeletVersion := strings.Split(string(b), " ")[1]
			kubeletVersion = strings.TrimSpace(kubeletVersion)
			klog.Infof("kubelet --version: %s", kubeletVersion)
			if strings.Contains(string(b), clusterVersion) {
				klog.Infof("Kubelet %s already exist, skip install.", clusterVersion)
				kubeletExist = true
			} else {
				return fmt.Errorf("The existing kubelet version %s of the node is inconsistent with cluster version %s, please clean it. ", kubeletVersion, clusterVersion)
			}
		}
	}

	if !kubeletExist {
		//download and install kubernetes-node
		packageUrl := fmt.Sprintf(constants.KubeUrlFormat, kubernetesResourceServer, clusterVersion, runtime.GOARCH)
		savePath := fmt.Sprintf("%s/kubernetes-node-linux-%s.tar.gz", constants.TmpDownloadDir, runtime.GOARCH)
		klog.V(1).Infof("Download kubelet from: %s", packageUrl)
		if err := util.DownloadFile(packageUrl, savePath, 3); err != nil {
			return fmt.Errorf("Download kuelet fail: %v", err)
		}
		if err := util.Untar(savePath, constants.TmpDownloadDir); err != nil {
			return err
		}
		for _, comp := range []string{"kubectl", "kubeadm", "kubelet"} {
			target := fmt.Sprintf("/usr/bin/%s", comp)
			if err := edgenode.CopyFile(constants.TmpDownloadDir+"/kubernetes/node/bin/"+comp, target, constants.DirMode); err != nil {
				return err
			}
		}
	}

	if _, err := os.Stat(constants.KubeCniDir); err == nil {
		klog.Infof("Cni dir %s already exist, skip install.", constants.KubeCniDir)
		return nil
	}
	//download and install kubernetes-cni
	cniUrl := fmt.Sprintf(constants.CniUrlFormat, constants.KubeCniVersion, runtime.GOARCH, constants.KubeCniVersion)
	savePath := fmt.Sprintf("%s/cni-plugins-linux-%s-%s.tgz", constants.TmpDownloadDir, runtime.GOARCH, constants.KubeCniVersion)
	klog.V(1).Infof("Download cni from: %s", cniUrl)
	if err := util.DownloadFile(cniUrl, savePath, 3); err != nil {
		return err
	}

	if err := os.MkdirAll(constants.KubeCniDir, 0600); err != nil {
		return err
	}
	if err := util.Untar(savePath, constants.KubeCniDir); err != nil {
		return err
	}
	return nil
}

// SetKubeletService configure kubelet service.
func SetKubeletService() error {
	klog.Info("Setting kubelet service.")
	kubeletServiceDir := filepath.Dir(constants.KubeletServiceFilepath)
	if _, err := os.Stat(kubeletServiceDir); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(kubeletServiceDir, os.ModePerm); err != nil {
				klog.Errorf("Create dir %s fail: %v", kubeletServiceDir, err)
				return err
			}
		} else {
			klog.Errorf("Describe dir %s fail: %v", kubeletServiceDir, err)
			return err
		}
	}
	if err := ioutil.WriteFile(constants.KubeletServiceFilepath, []byte(constants.KubeletServiceContent), 0644); err != nil {
		klog.Errorf("Write file %s fail: %v", constants.KubeletServiceFilepath, err)
		return err
	}
	return nil
}

// SetKubeletUnitConfig configure kubelet startup parameters.
func SetKubeletUnitConfig() error {
	kubeletUnitDir := filepath.Dir(constants.KubeletServiceConfPath)
	if _, err := os.Stat(kubeletUnitDir); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(kubeletUnitDir, os.ModePerm); err != nil {
				klog.Errorf("Create dir %s fail: %v", kubeletUnitDir, err)
				return err
			}
		} else {
			klog.Errorf("Describe dir %s fail: %v", kubeletUnitDir, err)
			return err
		}
	}

	if err := ioutil.WriteFile(constants.KubeletServiceConfPath, []byte(constants.KubeletUnitConfig), 0600); err != nil {
		return err
	}

	return nil
}

// SetKubeletConfigForNode write kubelet.conf for join node.
func SetKubeletConfigForNode() error {
	kubeconfigFilePath := filepath.Join(kubeadmconstants.KubernetesDir, kubeadmconstants.KubeletKubeConfigFileName)
	kubeletConfigDir := filepath.Dir(kubeconfigFilePath)
	if _, err := os.Stat(kubeletConfigDir); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(kubeletConfigDir, os.ModePerm); err != nil {
				klog.Errorf("Create dir %s fail: %v", kubeletConfigDir, err)
				return err
			}
		} else {
			klog.Errorf("Describe dir %s fail: %v", kubeletConfigDir, err)
			return err
		}
	}
	if err := ioutil.WriteFile(kubeconfigFilePath, []byte(constants.KubeletConfForNode), constants.DirMode); err != nil {
		return err
	}
	return nil
}

// SetKubeletCaCert write ca.crt for join node.
func SetKubeletCaCert(config *clientcmdapi.Config) error {
	kubeletCaCertPath := filepath.Join(kubeadmconstants.KubernetesDir, "pki", kubeadmconstants.CACertName)
	kubeletCaCertDir := filepath.Dir(kubeletCaCertPath)
	if _, err := os.Stat(kubeletCaCertDir); err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(kubeletCaCertDir, os.ModePerm); err != nil {
				klog.Errorf("Create dir %s fail: %v", kubeletCaCertDir, err)
				return err
			}
		} else {
			klog.Errorf("Describe dir %s fail: %v", kubeletCaCertDir, err)
			return err
		}
	}

	clusterinfo := kubeconfigutil.GetClusterFromKubeConfig(config)
	if err := ioutil.WriteFile(kubeletCaCertPath, []byte(clusterinfo.CertificateAuthorityData), constants.DirMode); err != nil {
		return err
	}
	return nil
}

// GetKubernetesVersionFromCluster get kubernetes cluster version from master.
func GetKubernetesVersionFromCluster(client kubernetes.Interface) (string, error) {
	var kubernetesVersion string
	// Also, the config map really should be KubeadmConfigConfigMap...
	configMap, err := apiclient.GetConfigMapWithRetry(client, metav1.NamespaceSystem, kubeadmconstants.KubeadmConfigConfigMap)
	if err != nil {
		return kubernetesVersion, pkgerrors.Wrap(err, "failed to get config map")
	}

	// gets ClusterConfiguration from kubeadm-config
	clusterConfigurationData, ok := configMap.Data[kubeadmconstants.ClusterConfigurationConfigMapKey]
	if !ok {
		return kubernetesVersion, pkgerrors.Errorf("unexpected error when reading kubeadm-config ConfigMap: %s key value pair missing", kubeadmconstants.ClusterConfigurationConfigMapKey)
	}

	scanner := bufio.NewScanner(strings.NewReader(clusterConfigurationData))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}

		if strings.Contains(parts[0], "kubernetesVersion") {
			kubernetesVersion = strings.TrimSpace(parts[1])
			break
		}
	}

	if len(kubernetesVersion) == 0 {
		return kubernetesVersion, errors.New("failed to get Kubernetes version")
	}

	klog.Infof("kubernetes version: %s", kubernetesVersion)
	return kubernetesVersion, nil
}
