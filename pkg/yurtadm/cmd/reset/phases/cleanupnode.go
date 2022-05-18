/*
Copyright 2019 The Kubernetes Authors.

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

package phases

import (
	"errors"
	"os"
	"path/filepath"

	"k8s.io/klog/v2"
	utilsexec "k8s.io/utils/exec"

	"github.com/openyurtio/openyurt/pkg/yurtadm/kubernetes/kubeadm/app/cmd/options"
	"github.com/openyurtio/openyurt/pkg/yurtadm/kubernetes/kubeadm/app/cmd/phases/workflow"
	kubeadmconstants "github.com/openyurtio/openyurt/pkg/yurtadm/kubernetes/kubeadm/app/constants"
	kubeutil "github.com/openyurtio/openyurt/pkg/yurtadm/kubernetes/kubeadm/app/phases/kubelet"
	utilruntime "github.com/openyurtio/openyurt/pkg/yurtadm/kubernetes/kubeadm/app/util/runtime"
)

// NewCleanupNodePhase creates a kubeadm workflow phase that cleanup the node
func NewCleanupNodePhase() workflow.Phase {
	return workflow.Phase{
		Name:    "cleanup-node",
		Aliases: []string{"cleanupnode"},
		Short:   "Run cleanup node.",
		Run:     runCleanupNode,
		InheritFlags: []string{
			options.NodeCRISocket,
		},
	}
}

func runCleanupNode(c workflow.RunData) error {
	r, ok := c.(resetData)
	if !ok {
		return errors.New("cleanup-node phase invoked with an invalid data struct")
	}

	// Try to stop the kubelet service
	klog.Infoln("[reset] Stopping the kubelet service")
	kubeutil.TryStopKubelet()

	// Try to unmount mounted directories under kubeadmconstants.KubeletRunDirectory in order to be able to remove the kubeadmconstants.KubeletRunDirectory directory later
	klog.Infof("[reset] Unmounting mounted directories in %q", kubeadmconstants.KubeletRunDirectory)
	// In case KubeletRunDirectory holds a symbolic link, evaluate it
	kubeletRunDir, err := absoluteKubeletRunDirectory()
	if err == nil {
		// Only clean absoluteKubeletRunDirectory if umountDirsCmd passed without error
		r.AddDirsToClean(kubeletRunDir)
	}

	klog.V(1).Info("[reset] Removing Kubernetes-managed containers")
	if err := removeContainers(utilsexec.New(), r.CRISocketPath()); err != nil {
		klog.Warningf("[reset] Failed to remove containers: %v", err)
	}

	r.AddDirsToClean("/var/lib/dockershim", "/var/run/kubernetes", "/var/lib/cni")

	// Remove contents from the config and pki directories
	klog.V(1).Infoln("[reset] Removing contents from the config and pki directories")
	certsDir := filepath.Join(kubeadmconstants.KubernetesDir, "pki")
	resetConfigDir(kubeadmconstants.KubernetesDir, certsDir)

	return nil
}

func absoluteKubeletRunDirectory() (string, error) {
	absoluteKubeletRunDirectory, err := filepath.EvalSymlinks(kubeadmconstants.KubeletRunDirectory)
	if err != nil {
		klog.Warningf("[reset] Failed to evaluate the %q directory. Skipping its unmount and cleanup: %v", kubeadmconstants.KubeletRunDirectory, err)
		return "", err
	}
	err = unmountKubeletDirectory(absoluteKubeletRunDirectory)
	if err != nil {
		klog.Warningf("[reset] Failed to unmount mounted directories in %s", kubeadmconstants.KubeletRunDirectory)
		return "", err
	}
	return absoluteKubeletRunDirectory, nil
}

func removeContainers(execer utilsexec.Interface, criSocketPath string) error {
	containerRuntime, err := utilruntime.NewContainerRuntime(execer, criSocketPath)
	if err != nil {
		return err
	}
	containers, err := containerRuntime.ListKubeContainers()
	if err != nil {
		return err
	}
	return containerRuntime.RemoveContainers(containers)
}

// resetConfigDir is used to cleanup the files kubeadm writes in /etc/kubernetes/.
func resetConfigDir(configPathDir, pkiPathDir string) {
	dirsToClean := []string{
		filepath.Join(configPathDir, kubeadmconstants.ManifestsSubDirName),
		pkiPathDir,
	}
	klog.Infof("[reset] Deleting contents of config directories: %v", dirsToClean)
	for _, dir := range dirsToClean {
		if err := CleanDir(dir); err != nil {
			klog.Warningf("[reset] Failed to delete contents of %q directory: %v", dir, err)
		}
	}

	filesToClean := []string{
		filepath.Join(configPathDir, kubeadmconstants.KubeletKubeConfigFileName),
	}
	klog.Infof("[reset] Deleting files: %v", filesToClean)
	for _, path := range filesToClean {
		if err := os.RemoveAll(path); err != nil {
			klog.Warningf("[reset] Failed to remove file: %q [%v]", path, err)
		}
	}
}

// CleanDir removes everything in a directory, but not the directory itself
func CleanDir(filePath string) error {
	// If the directory doesn't even exist there's nothing to do, and we do
	// not consider this an error
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil
	}

	d, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer d.Close()
	names, err := d.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, name := range names {
		if err = os.RemoveAll(filepath.Join(filePath, name)); err != nil {
			return err
		}
	}
	return nil
}
