package revert

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog"

	"github.com/alibaba/openyurt/pkg/yurtctl/constants"
	kubeutil "github.com/alibaba/openyurt/pkg/yurtctl/util/kubernetes"
)

type RevertOptions struct {
	clientSet *kubernetes.Clientset
}

func NewRevertOptions() *RevertOptions {
	return &RevertOptions{}
}

func NewRevertCmd() *cobra.Command {
	ro := NewRevertOptions()
	cmd := &cobra.Command{
		Use:   "revert",
		Short: "Reverts the yurt cluster back to a Kubernetes cluster",
		Run: func(cmd *cobra.Command, _ []string) {
			if err := ro.Complete(cmd.Flags()); err != nil {
				klog.Fatalf("fail to complete the convert option: %s", err)
			}
			if err := ro.RunRevert(); err != nil {
				klog.Fatalf("fail to convert kubernetes to yurt: %s", err)
			}
		},
	}
	return cmd
}

func (ro *RevertOptions) Complete(flags *pflag.FlagSet) error {
	// parse kubeconfig and generate the clientset
	kbCfgPath, err := flags.GetString("kubeconfig")
	if err != nil {
		return err
	}

	if kbCfgPath == "" {
		if home := homedir.HomeDir(); home != "" {
			kbCfgPath = filepath.Join(home, ".kube", "config")
		}
	}

	if kbCfgPath == "" {
		kbCfgPath = os.Getenv("KUBECONFIG")
	}

	if kbCfgPath == "" {
		return errors.New("either '--kubeconfig', '$HOME/.kube/config' or '$KUBECONFIG' need to be set")
	}

	restCfg, err := clientcmd.BuildConfigFromFlags("", kbCfgPath)
	if err != nil {
		return err
	}

	ro.clientSet, err = kubernetes.NewForConfig(restCfg)
	if err != nil {
		return err
	}
	return nil
}

func (ro *RevertOptions) RunRevert() error {
	// 1. check the server version
	if err := kubeutil.ValidateServerVersion(ro.clientSet); err != nil {
		return err
	}
	// 2. remove labels from nodes
	nodeLst, err := ro.clientSet.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return err
	}

	var edgeNodeNames []string
	for _, node := range nodeLst.Items {
		isEdgeNode, ok := node.Labels[constants.LabelEdgeWorker]
		if ok && isEdgeNode == "true" {
			edgeNodeNames = append(edgeNodeNames, node.GetName())
		}
		if ok {
			delete(node.Labels, constants.LabelEdgeWorker)
			if _, err := ro.clientSet.CoreV1().Nodes().Update(&node); err != nil {
				return err
			}
		}
	}
	klog.Info("label alibabacloud.com/is-edge-worker is removed")

	// 3. remove the yurt controller manager
	if err := ro.clientSet.AppsV1().Deployments("kube-system").
		Delete("yurt-controller-manager", &metav1.DeleteOptions{
			PropagationPolicy: &kubeutil.PropagationPolicy,
		}); err != nil && !apierrors.IsNotFound(err) {
		klog.Errorf("fail to remove yurt controller manager: %s", err)
		return err
	}
	klog.Info("yurt controller manager is removed")

	// 4. recreate the node-controller service account
	ncSa := &v1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "node-controller",
			Namespace: "kube-system",
		},
	}
	if _, err := ro.clientSet.CoreV1().
		ServiceAccounts(ncSa.GetNamespace()).Create(ncSa); err != nil {
		klog.Errorf("fail to create node-controller service account: %s", err)
		return err
	}
	klog.Info("ServiceAccount node-controller is created")

	// 5. remove yurt-hub and revert kubelet service
	if err := kubeutil.RunServantJobs(ro.clientSet,
		map[string]string{"action": "revert"},
		edgeNodeNames); err != nil {
		klog.Errorf("fail to revert edge node: %s", err)
		return err
	}
	klog.Info("yurt-hub is removed, kubelet service is reset")

	return nil
}
