package convert

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog"

	"github.com/alibaba/openyurt/pkg/yurtctl/constants"
	kubeutil "github.com/alibaba/openyurt/pkg/yurtctl/util/kubernetes"
	strutil "github.com/alibaba/openyurt/pkg/yurtctl/util/strings"
)

// Provider signifies the provider type
type Provider string

const (
	// ProviderMinikube is used if the target kubernetes is run on minikube
	ProviderMinikube Provider = "minikube"
	// ProviderACK is used if the target kubernetes is run on ack
	ProviderACK Provider = "ack"
)

// ConvertOptions has the information that required by convert operation
type ConvertOptions struct {
	clientSet  *kubernetes.Clientset
	CloudNodes []string
	Provider   Provider
}

// NewConvertOptions creates a new ConvertOptions
func NewConvertOptions() *ConvertOptions {
	return &ConvertOptions{}
}

// NewConvertCmd generates a new convert command
func NewConvertCmd() *cobra.Command {
	co := NewConvertOptions()
	cmd := &cobra.Command{
		Use:   "convert -c CLOUDNODES",
		Short: "Converts the kubernetes cluster to a yurt cluster",
		Run: func(cmd *cobra.Command, _ []string) {
			if err := co.Complete(cmd.Flags()); err != nil {
				klog.Fatalf("fail to complete the convert option: %s", err)
			}
			if err := co.Validate(); err != nil {
				klog.Fatalf("convert option is invalid: %s", err)
			}
			if err := co.RunConvert(); err != nil {
				klog.Fatalf("fail to convert kubernetes to yurt: %s", err)
			}
		},
	}

	cmd.Flags().StringP("cloud-nodes", "c", "",
		"The list of cloud nodes.(e.g. -c cloudnode1,cloudnode2)")
	cmd.Flags().StringP("provider", "p", "ack",
		"The provider of the original Kubernetes cluster.")

	return cmd
}

// Complete completes all the required options
func (co *ConvertOptions) Complete(flags *pflag.FlagSet) error {
	cnStr, err := flags.GetString("cloud-nodes")
	if err != nil {
		return err
	}
	co.CloudNodes = strings.Split(cnStr, ",")

	pStr, err := flags.GetString("provider")
	if err != nil {
		return err
	}
	co.Provider = Provider(pStr)

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

	co.clientSet, err = kubernetes.NewForConfig(restCfg)
	if err != nil {
		return err
	}
	return nil
}

// Validate makes sure provided values for ConvertOptions are valid
func (co *ConvertOptions) Validate() error {
	if co.Provider != ProviderMinikube &&
		co.Provider != ProviderACK {
		return fmt.Errorf("unknown provider: %s, valid providers are: minikube, ack",
			co.Provider)
	}
	return nil
}

// RunConvert performs the conversion
func (co *ConvertOptions) RunConvert() error {
	// 1. check the server version
	if err := kubeutil.ValidateServerVersion(co.clientSet); err != nil {
		return err
	}
	// 2. label nodes as cloud node or edge node
	nodeLst, err := co.clientSet.CoreV1().Nodes().List(metav1.ListOptions{})
	if err != nil {
		return err
	}
	var edgeNodeNames []string
	for _, node := range nodeLst.Items {
		if strutil.IsInStringLst(co.CloudNodes, node.GetName()) {
			// label node as cloud node
			klog.Infof("mark %s as the cloud-node", node.GetName())
			if _, err := kubeutil.LabelNode(co.clientSet,
				&node, constants.LabelEdgeWorker, "false"); err != nil {
				return err
			}
			continue
		}
		// label node as edge node
		klog.Infof("mark %s as the edge-node", node.GetName())
		edgeNodeNames = append(edgeNodeNames, node.GetName())
		edgeNode, err := kubeutil.LabelNode(co.clientSet,
			&node, constants.LabelEdgeWorker, "true")
		if err != nil {
			return err
		}
		// 3. annotate all edge nodes as autonomous
		// NOTE pods running on an non-autonomous will be evicted, even though
		// the node is marked as an edge node.
		// TODO should we allow user to decide if a node is autonomous or not ?
		klog.Infof("mark %s as autonomous node", node.GetName())
		if _, err := kubeutil.AnnotateNode(co.clientSet,
			edgeNode, constants.AnnotationAutonomy, "true"); err != nil {
			return err
		}
	}

	// 4. deploy yurt controller manager
	dpObj, err := kubeutil.YamlToObject([]byte(constants.YurtControllerManagerDeployment))
	if err != nil {
		return err
	}
	ecmDp, ok := dpObj.(*appsv1.Deployment)
	if !ok {
		return errors.New("fail to assert YurtControllerManagerDeployment")
	}
	if _, err := co.clientSet.AppsV1().Deployments("kube-system").Create(ecmDp); err != nil {
		return err
	}
	klog.Info("deploy the yurt controller manager")

	// 5. delete the node-controller service account to disable node-controller
	if err := co.clientSet.CoreV1().ServiceAccounts("kube-system").
		Delete("node-controller", &metav1.DeleteOptions{
			PropagationPolicy: &kubeutil.PropagationPolicy,
		}); err != nil {
		klog.Errorf("fail to delete ServiceAccount(node-controller): %s", err)
		return err
	}

	// 6. deploy yurt-hub and reset the kubelet service
	klog.Infof("deploying the yurt-hub and resetting the kubelet service...")
	if err := kubeutil.RunServantJobs(co.clientSet, map[string]string{
		"provider": string(co.Provider),
		"action":   "convert",
	}, edgeNodeNames); err != nil {
		klog.Errorf("fail to run ServantJobs: %s", err)
		return err
	}

	return nil
}
