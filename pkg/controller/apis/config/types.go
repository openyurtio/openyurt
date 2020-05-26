package config

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubectrlmgrconfig "k8s.io/kubernetes/pkg/controller/apis/config"
)

// YurtControllerManagerConfiguration contains elements describing yurt-controller manager.
type YurtControllerManagerConfiguration struct {
	metav1.TypeMeta

	// Generic holds configuration for a generic controller-manager
	Generic kubectrlmgrconfig.GenericControllerManagerConfiguration

	// NodeLifecycleControllerConfiguration holds configuration for
	// NodeLifecycleController related features.
	NodeLifecycleController kubectrlmgrconfig.NodeLifecycleControllerConfiguration
}
