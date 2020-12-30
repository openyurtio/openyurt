package client

import (
	yurtappclientset "github.com/alibaba/openyurt/pkg/yurtappmanager/client/clientset/versioned"
	kubeclientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// GenericClientset defines a generic client
type GenericClientset struct {
	KubeClient    kubeclientset.Interface
	YurtappClient yurtappclientset.Interface
}

// NewForConfig creates a new Clientset for the given config.
func newForConfig(c *rest.Config) (*GenericClientset, error) {
	kubeClient, err := kubeclientset.NewForConfig(c)
	if err != nil {
		return nil, err
	}
	yurtClient, err := yurtappclientset.NewForConfig(c)
	if err != nil {
		return nil, err
	}
	return &GenericClientset{
		KubeClient:    kubeClient,
		YurtappClient: yurtClient,
	}, nil
}
