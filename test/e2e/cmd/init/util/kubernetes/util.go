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
	"context"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeclientset "k8s.io/client-go/kubernetes"
	bootstrapapi "k8s.io/cluster-bootstrap/token/api"
	bootstraputil "k8s.io/cluster-bootstrap/token/util"
	"k8s.io/klog/v2"
	kubectllogs "k8s.io/kubectl/pkg/cmd/logs"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	bootstraptokenv1 "github.com/openyurtio/openyurt/pkg/util/kubernetes/kubeadm/app/apis/bootstraptoken/v1"
	kubeadmconstants "github.com/openyurtio/openyurt/pkg/util/kubernetes/kubeadm/app/constants"
	nodetoken "github.com/openyurtio/openyurt/pkg/util/kubernetes/kubeadm/app/phases/bootstraptoken/node"
)

const (
	SystemNamespace = "kube-system"
	// DefaultWaitServantJobTimeout specifies the timeout value of waiting for the ServantJob to be succeeded
	DefaultWaitServantJobTimeout = time.Minute * 5
)

var (
	// PropagationPolicy defines the propagation policy used when deleting a resource
	PropagationPolicy = metav1.DeletePropagationBackground
	// CheckServantJobPeriod defines the time interval between two successive ServantJob status inspection
	CheckServantJobPeriod = time.Second * 10
)

func AddEdgeWorkerLabelAndAutonomyAnnotation(
	cliSet kubeclientset.Interface,
	node *corev1.Node,
	lVal, aVal string,
) (*corev1.Node, error) {
	node.Labels[projectinfo.GetEdgeWorkerLabelKey()] = lVal
	node.Annotations[projectinfo.GetNodeAutonomyDurationAnnotation()] = aVal
	newNode, err := cliSet.CoreV1().Nodes().Update(context.Background(), node, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}
	return newNode, nil
}

// RunJobAndCleanup runs the job, wait for it to be complete, and delete it
func RunJobAndCleanup(cliSet kubeclientset.Interface, job *batchv1.Job, timeout, period time.Duration) error {
	job, err := cliSet.BatchV1().Jobs(job.GetNamespace()).Create(context.Background(), job, metav1.CreateOptions{})
	if err != nil {
		return err
	}

	err = wait.PollUntilContextTimeout(context.Background(), period, timeout, true, jobIsCompleted(cliSet, job))
	if err != nil {
		klog.Errorf("Error job(%s/%s) is not completed, %v", job.Namespace, job.Name, err)
		return err
	}

	return cliSet.BatchV1().Jobs(job.GetNamespace()).Delete(context.Background(), job.GetName(), metav1.DeleteOptions{
		PropagationPolicy: &PropagationPolicy,
	})
}

func jobIsCompleted(clientset kubeclientset.Interface, job *batchv1.Job) wait.ConditionWithContextFunc {
	return func(ctx context.Context) (bool, error) {
		newJob, err := clientset.BatchV1().
			Jobs(job.GetNamespace()).
			Get(context.Background(), job.GetName(), metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, err
			}

			// kube-apiserver maybe not work currently, so we should skip other errors
			return false, nil
		}

		if newJob.Status.Succeeded == *newJob.Spec.Completions {
			return true, nil
		}

		return false, nil
	}
}

func DumpPod(client kubeclientset.Interface, pod *corev1.Pod, w io.Writer) error {
	klog.Infof("dump pod(%s/%s) info:", pod.Namespace, pod.Name)
	url := client.CoreV1().RESTClient().Get().Resource("pods").Namespace(pod.Namespace).Name(pod.Name).URL()
	podRequest := client.CoreV1().RESTClient().Get().AbsPath(url.Path)
	if err := kubectllogs.DefaultConsumeRequest(context.TODO(), podRequest, w); err != nil {
		klog.Errorf("failed to print pod(%s/%s) info, %v", pod.Namespace, pod.Name, err)
		return err
	}

	klog.Infof("start to print logs for pod(%s/%s):", pod.Namespace, pod.Name)
	req := client.CoreV1().Pods(pod.GetNamespace()).GetLogs(pod.Name, &corev1.PodLogOptions{})
	if err := kubectllogs.DefaultConsumeRequest(context.TODO(), req, w); err != nil {
		klog.Errorf("failed to print logs for pod(%s/%s), %v", pod.Namespace, pod.Name, err)
	}

	klog.Infof("start to print events for pod(%s/%s):", pod.Namespace, pod.Name)
	fieldSelector := "involvedObject.name=" + pod.Name
	eventList, err := client.CoreV1().Events(pod.Namespace).List(context.Background(), metav1.ListOptions{
		FieldSelector: fieldSelector,
	})
	if err != nil {
		klog.Errorf("failed to dump events for pod(%s/%s), %v", pod.Namespace, pod.Name, err)
		return err
	}

	for _, event := range eventList.Items {
		klog.Infof(
			"Pod(%s/%s) Event: %v, Type: %v, Reason: %v, Message: %v",
			pod.Namespace,
			pod.Name,
			event.Name,
			event.Type,
			event.Reason,
			event.Message,
		)
	}

	return nil
}

// RunServantJobs launch servant jobs on specified nodes and wait all jobs to finish.
// Succeed jobs will be deleted when finished. Failed jobs are preserved for diagnosis.
func RunServantJobs(
	cliSet kubeclientset.Interface,
	waitServantJobTimeout time.Duration,
	getJob func(nodeName string) (*batchv1.Job, error),
	nodeNames []string, ww io.Writer) error {
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
			if err := RunJobAndCleanup(cliSet, job, waitServantJobTimeout, CheckServantJobPeriod); err != nil {
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

	return sliceEqual(token.Usages, kubeadmconstants.DefaultTokenUsages) &&
		sliceEqual(token.Groups, kubeadmconstants.DefaultTokenGroups)
}
