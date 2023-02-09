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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/spf13/pflag"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
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
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"
	kubeclientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	bootstraputil "k8s.io/cluster-bootstrap/token/util"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	bootstraptokenv1 "github.com/openyurtio/openyurt/pkg/util/kubernetes/kubeadm/app/apis/bootstraptoken/v1"
	kubeadmconstants "github.com/openyurtio/openyurt/pkg/util/kubernetes/kubeadm/app/constants"
	nodetoken "github.com/openyurtio/openyurt/pkg/util/kubernetes/kubeadm/app/phases/bootstraptoken/node"
	tmplutil "github.com/openyurtio/openyurt/pkg/util/templates"
)

const (
	SystemNamespace = "kube-system"
	// DefaultWaitServantJobTimeout specifies the timeout value of waiting for the ServantJob to be succeeded
	DefaultWaitServantJobTimeout = time.Minute * 5
)

var (
	// PropagationPolicy defines the propagation policy used when deleting a resource
	PropagationPolicy = metav1.DeletePropagationBackground
	// CheckServantJobPeriod defines the time interval between two successive ServantJob statu's inspection
	CheckServantJobPeriod = time.Second * 10
)

func processCreateErr(kind string, name string, err error) error {
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			klog.V(4).Infof("[WARNING] %s/%s is already in cluster, skip to prepare it", kind, name)
			return nil
		}
		return fmt.Errorf("fail to create the %s/%s: %w", kind, name, err)
	}
	klog.V(4).Infof("%s/%s is created", kind, name)
	return nil
}

// CreateServiceAccountFromYaml creates the ServiceAccount from the yaml template.
func CreateServiceAccountFromYaml(cliSet kubeclientset.Interface, ns, saTmpl string) error {
	obj, err := YamlToObject([]byte(saTmpl))
	if err != nil {
		return err
	}
	sa, ok := obj.(*corev1.ServiceAccount)
	if !ok {
		return fmt.Errorf("fail to assert serviceaccount: %w", err)
	}
	_, err = cliSet.CoreV1().ServiceAccounts(ns).Create(context.Background(), sa, metav1.CreateOptions{})
	return processCreateErr("serviceaccount", sa.Name, err)
}

// CreateClusterRoleFromYaml creates the ClusterRole from the yaml template.
func CreateClusterRoleFromYaml(cliSet kubeclientset.Interface, crTmpl string) error {
	obj, err := YamlToObject([]byte(crTmpl))
	if err != nil {
		return err
	}
	cr, ok := obj.(*rbacv1.ClusterRole)
	if !ok {
		return fmt.Errorf("fail to assert clusterrole: %w", err)
	}
	_, err = cliSet.RbacV1().ClusterRoles().Create(context.Background(), cr, metav1.CreateOptions{})
	return processCreateErr("clusterrole", cr.Name, err)
}

// CreateClusterRoleBindingFromYaml creates the ClusterRoleBinding from the yaml template.
func CreateClusterRoleBindingFromYaml(cliSet kubeclientset.Interface, crbTmpl string) error {
	obj, err := YamlToObject([]byte(crbTmpl))
	if err != nil {
		return err
	}
	crb, ok := obj.(*rbacv1.ClusterRoleBinding)
	if !ok {
		return fmt.Errorf("fail to assert clusterrolebinding: %w", err)
	}
	_, err = cliSet.RbacV1().ClusterRoleBindings().Create(context.Background(), crb, metav1.CreateOptions{})
	return processCreateErr("clusterrolebinding", crb.Name, err)
}

// CreateConfigMapFromYaml creates the ConfigMap from the yaml template.
func CreateConfigMapFromYaml(cliSet kubeclientset.Interface, ns, cmTmpl string) error {
	obj, err := YamlToObject([]byte(cmTmpl))
	if err != nil {
		return err
	}
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return fmt.Errorf("fail to assert configmap: %w", err)
	}
	_, err = cliSet.CoreV1().ConfigMaps(ns).Create(context.Background(), cm, metav1.CreateOptions{})
	return processCreateErr("configmap", cm.Name, err)
}

// CreateDeployFromYaml creates the Deployment from the yaml template.
func CreateDeployFromYaml(cliSet kubeclientset.Interface, ns, dplyTmpl string, ctx interface{}) error {
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
	_, err = cliSet.AppsV1().Deployments(ns).Create(context.Background(), dply, metav1.CreateOptions{})
	return processCreateErr("deployment", dply.Name, err)
}

// CreateDaemonSetFromYaml creates the DaemonSet from the yaml template.
func CreateDaemonSetFromYaml(cliSet kubeclientset.Interface, ns, dsTmpl string, ctx interface{}) error {
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
		return fmt.Errorf("fail to assert daemonset: %w", err)
	}
	_, err = cliSet.AppsV1().DaemonSets(ns).Create(context.Background(), ds, metav1.CreateOptions{})
	return processCreateErr("daemonset", ds.Name, err)
}

// CreateServiceFromYaml creates the Service from the yaml template.
func CreateServiceFromYaml(cliSet kubeclientset.Interface, ns, svcTmpl string) error {
	obj, err := YamlToObject([]byte(svcTmpl))
	if err != nil {
		return err
	}
	svc, ok := obj.(*corev1.Service)
	if !ok {
		return fmt.Errorf("fail to assert service: %w", err)
	}
	_, err = cliSet.CoreV1().Services(ns).Create(context.Background(), svc, metav1.CreateOptions{})
	return processCreateErr("service", svc.Name, err)
}

//add by yanyhui at 20210611
// CreateRoleFromYaml creates the ClusterRole from the yaml template.

func CreateRoleFromYaml(cliSet kubeclientset.Interface, ns, crTmpl string) error {
	obj, err := YamlToObject([]byte(crTmpl))
	if err != nil {
		return err
	}
	ro, ok := obj.(*rbacv1.Role)
	if !ok {
		return fmt.Errorf("fail to assert role: %w", err)
	}
	_, err = cliSet.RbacV1().Roles(ns).Create(context.Background(), ro, metav1.CreateOptions{})
	return processCreateErr("role", ro.Name, err)
}

// CreateRoleBindingFromYaml creates the ClusterRoleBinding from the yaml template.
func CreateRoleBindingFromYaml(cliSet kubeclientset.Interface, ns, crbTmpl string) error {
	obj, err := YamlToObject([]byte(crbTmpl))
	if err != nil {
		return err
	}
	rb, ok := obj.(*rbacv1.RoleBinding)
	if !ok {
		return fmt.Errorf("fail to assert rolebinding: %w", err)
	}
	_, err = cliSet.RbacV1().RoleBindings(ns).Create(context.Background(), rb, metav1.CreateOptions{})
	return processCreateErr("rolebinding", rb.Name, err)
}

// CreateSecretFromYaml creates the Secret from the yaml template.
func CreateSecretFromYaml(cliSet kubeclientset.Interface, ns, saTmpl string) error {
	obj, err := YamlToObject([]byte(saTmpl))
	if err != nil {
		return err
	}
	se, ok := obj.(*corev1.Secret)
	if !ok {
		return fmt.Errorf("fail to assert secret: %w", err)
	}
	_, err = cliSet.CoreV1().Secrets(ns).Create(context.Background(), se, metav1.CreateOptions{})

	return processCreateErr("secret", se.Name, err)
}

// CreateMutatingWebhookConfigurationFromYaml creates the Service from the yaml template.
func CreateMutatingWebhookConfigurationFromYaml(cliSet kubeclientset.Interface, svcTmpl string) error {
	obj, err := YamlToObject([]byte(svcTmpl))
	if err != nil {
		return err
	}
	mw, ok := obj.(*admissionregistrationv1.MutatingWebhookConfiguration)
	if !ok {
		return fmt.Errorf("fail to assert mutatingwebhookconfiguration: %w", err)
	}
	_, err = cliSet.AdmissionregistrationV1().MutatingWebhookConfigurations().Create(context.Background(), mw, metav1.CreateOptions{})
	return processCreateErr("mutatingwebhookconfiguration", mw.Name, err)
}

// CreateValidatingWebhookConfigurationFromYaml creates the Service from the yaml template.
func CreateValidatingWebhookConfigurationFromYaml(cliSet kubeclientset.Interface, svcTmpl string) error {
	obj, err := YamlToObject([]byte(svcTmpl))
	if err != nil {
		return err
	}
	vw, ok := obj.(*admissionregistrationv1.ValidatingWebhookConfiguration)
	if !ok {
		return fmt.Errorf("fail to assert validatingwebhookconfiguration: %w", err)
	}
	_, err = cliSet.AdmissionregistrationV1().ValidatingWebhookConfigurations().Create(context.Background(), vw, metav1.CreateOptions{})
	return processCreateErr("validatingwebhookconfiguration", vw.Name, err)
}

func CreateCRDFromYaml(clientset kubeclientset.Interface, yurtAppManagerClient dynamic.Interface, nameSpace string, filebytes []byte) error {
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

// YamlToObject deserializes object in yaml format to a runtime.Object
func YamlToObject(yamlContent []byte) (k8sruntime.Object, error) {
	decode := serializer.NewCodecFactory(scheme.Scheme).UniversalDeserializer().Decode
	obj, _, err := decode(yamlContent, nil, nil)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// AnnotateNode add a new annotation (<key>=<val>) to the given node
func AnnotateNode(cliSet kubeclientset.Interface, node *corev1.Node, key, val string) (*corev1.Node, error) {
	node.Annotations[key] = val
	newNode, err := cliSet.CoreV1().Nodes().Update(context.Background(), node, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	return newNode, nil
}

func AddEdgeWorkerLabelAndAutonomyAnnotation(cliSet kubeclientset.Interface, node *corev1.Node, lVal, aVal string) (*corev1.Node, error) {
	node.Labels[projectinfo.GetEdgeWorkerLabelKey()] = lVal
	node.Annotations[projectinfo.GetAutonomyAnnotation()] = aVal
	newNode, err := cliSet.CoreV1().Nodes().Update(context.Background(), node, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	return newNode, nil
}

// RunJobAndCleanup runs the job, wait for it to be complete, and delete it
func RunJobAndCleanup(cliSet kubeclientset.Interface, job *batchv1.Job, timeout, period time.Duration, waitForTimeout bool) error {
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
			newJob, err := cliSet.BatchV1().Jobs(job.GetNamespace()).
				Get(context.Background(), job.GetName(), metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					return err
				}

				if waitForTimeout {
					klog.Infof("continue to wait for job(%s) to complete until timeout, even if failed to get job, %v", job.GetName(), err)
					continue
				}
				return err
			}

			if newJob.Status.Succeeded == *newJob.Spec.Completions {
				if err := cliSet.BatchV1().Jobs(job.GetNamespace()).
					Delete(context.Background(), job.GetName(), metav1.DeleteOptions{
						PropagationPolicy: &PropagationPolicy,
					}); err != nil {
					klog.Errorf("fail to delete succeeded servant job(%s): %s", job.GetName(), err)
					return err
				}
				return nil
			}
		}
	}
}

// RunServantJobs launch servant jobs on specified nodes and wait all jobs to finish.
// Succeed jobs will be deleted when finished. Failed jobs are preserved for diagnosis.
func RunServantJobs(
	cliSet kubeclientset.Interface,
	waitServantJobTimeout time.Duration,
	getJob func(nodeName string) (*batchv1.Job, error),
	nodeNames []string, ww io.Writer,
	waitForTimeout bool) error {
	var wg sync.WaitGroup

	jobByNodeName := make(map[string]*batchv1.Job)
	for _, nodeName := range nodeNames {
		job, err := getJob(nodeName)
		if err != nil {
			return fmt.Errorf("fail to get job for node %s: %w", nodeName, err)
		}
		jobByNodeName[nodeName] = job
	}

	res := make(chan string, len(nodeNames))
	errCh := make(chan error, len(nodeNames))
	for _, nodeName := range nodeNames {
		wg.Add(1)
		job := jobByNodeName[nodeName]
		go func() {
			defer wg.Done()
			if err := RunJobAndCleanup(cliSet, job, waitServantJobTimeout, CheckServantJobPeriod, waitForTimeout); err != nil {
				errCh <- fmt.Errorf("[ERROR] fail to run servant job(%s): %w", job.GetName(), err)
			} else {
				res <- fmt.Sprintf("\t[INFO] servant job(%s) has succeeded\n", job.GetName())
			}
		}()
	}
	wg.Wait()
	close(res)
	close(errCh)
	for m := range res {
		io.WriteString(ww, m)
	}

	errs := []error{}
	for err := range errCh {
		errs = append(errs, err)
	}
	if len(errs) != 0 {
		return utilerrors.NewAggregate(errs)
	}

	return nil
}

// GenClientSet generates the clientset based on command option, environment variable or
// the default kubeconfig file
func GenClientSet(flags *pflag.FlagSet) (*kubeclientset.Clientset, error) {
	kubeconfigPath, err := PrepareKubeConfigPath(flags)
	if err != nil {
		return nil, err
	}

	restCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, err
	}

	return kubeclientset.NewForConfig(restCfg)
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

func GetOrCreateJoinTokenString(cliSet kubeclientset.Interface) (string, error) {
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
		return "", fmt.Errorf("%w%s", err, "failed to list bootstrap tokens")
	}

	for _, secret := range secrets.Items {

		// Get the BootstrapToken struct representation from the Secret object
		token, err := bootstraptokenv1.BootstrapTokenFromSecret(&secret)
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
		return "", fmt.Errorf("couldn't generate random token, %w", err)
	}
	token, err := bootstraptokenv1.NewBootstrapTokenString(tokenStr)
	if err != nil {
		return "", err
	}

	klog.V(1).Infoln("[token] creating token")
	if err := nodetoken.CreateNewTokens(cliSet,
		[]bootstraptokenv1.BootstrapToken{{
			Token:  token,
			Usages: kubeadmconstants.DefaultTokenUsages,
			Groups: kubeadmconstants.DefaultTokenGroups,
		}}); err != nil {
		return "", err
	}
	return tokenStr, nil
}

// usagesAndGroupsAreValid checks if the usages and groups in the given bootstrap token are valid
func usagesAndGroupsAreValid(token *bootstraptokenv1.BootstrapToken) bool {
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
