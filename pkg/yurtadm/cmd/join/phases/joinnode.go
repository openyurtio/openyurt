/*
Copyright 2021 The OpenYurt Authors.
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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/version"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	kubeletconfigv1beta1 "k8s.io/kubelet/config/v1beta1"

	"github.com/openyurtio/openyurt/pkg/util/kubernetes/kubeadm/app/cmd/options"
	"github.com/openyurtio/openyurt/pkg/util/kubernetes/kubeadm/app/cmd/phases/workflow"
	"github.com/openyurtio/openyurt/pkg/util/kubernetes/kubeadm/app/constants"
	kubeutil "github.com/openyurtio/openyurt/pkg/util/kubernetes/kubeadm/app/phases/kubelet"
	"github.com/openyurtio/openyurt/pkg/util/kubernetes/kubeadm/app/util/apiclient"
	kubeletconfig "github.com/openyurtio/openyurt/pkg/util/kubernetes/kubelet/apis/config"
	kubeletscheme "github.com/openyurtio/openyurt/pkg/util/kubernetes/kubelet/apis/config/scheme"
	kubeletcodec "github.com/openyurtio/openyurt/pkg/util/kubernetes/kubelet/kubeletconfig/util/codec"
	"github.com/openyurtio/openyurt/pkg/util/templates"
	"github.com/openyurtio/openyurt/pkg/yurtadm/cmd/join/joindata"
	yurtconstants "github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
)

const (
	LvsCareStaticPodName = "kube-lvscare"
	LvsCareCommand       = "/usr/bin/lvscare"
	DefaultLvsCareImage  = "fanux/lvscare:latest"
	LvsStaticPodFileName = "kube-lvscare.yaml"
)

// NewEdgeNodePhase creates a yurtadm workflow phase that start kubelet on a edge node.
func NewEdgeNodePhase() workflow.Phase {
	return workflow.Phase{
		Name:  "Join node to OpenYurt cluster. ",
		Short: "Join node",
		Run:   runJoinNode,
		InheritFlags: []string{
			options.TokenStr,
			options.NodeCRISocket,
			options.NodeName,
			options.IgnorePreflightErrors,
		},
	}
}

// runJoinNode executes the node join process.
func runJoinNode(c workflow.RunData) error {
	data, ok := c.(joindata.YurtJoinData)
	if !ok {
		return fmt.Errorf("Join edge-node phase invoked with an invalid data struct. ")
	}

	err := writeKubeletConfigFile(data.BootstrapClient(), data)
	if err != nil {
		return err
	}

	if err := addLvsStaticPodYaml(data, filepath.Join(constants.KubernetesDir, constants.ManifestsSubDirName)); err != nil {
		return err
	}

	if err := addYurthubStaticYaml(data, filepath.Join(constants.KubernetesDir, constants.ManifestsSubDirName)); err != nil {
		return err
	}
	klog.Info("[kubelet-start] Starting the kubelet")
	kubeutil.TryStartKubelet()
	return nil
}

// writeKubeletConfigFile write kubelet configuration into local disk.
func writeKubeletConfigFile(bootstrapClient *clientset.Clientset, data joindata.YurtJoinData) error {
	kubeletVersion, err := version.ParseSemantic(data.KubernetesVersion())
	if err != nil {
		return err
	}

	// Write the configuration for the kubelet (using the bootstrap token credentials) to disk so the kubelet can start
	_, err = downloadConfig(bootstrapClient, kubeletVersion, constants.KubeletRunDirectory)
	if err != nil {
		return err
	}

	if err := kubeutil.WriteKubeletDynamicEnvFile(data, constants.KubeletRunDirectory); err != nil {
		return err
	}
	return nil
}

// downloadConfig downloads the kubelet configuration from a ConfigMap and writes it to disk.
// Used at "kubeadm join" time
func downloadConfig(client clientset.Interface, kubeletVersion *version.Version, kubeletDir string) (*kubeletconfig.KubeletConfiguration, error) {
	// Download the ConfigMap from the cluster based on what version the kubelet is
	configMapName := constants.GetKubeletConfigMapName(kubeletVersion)

	klog.Infof("[kubelet-start] Downloading configuration for the kubelet from the %q ConfigMap in the %s namespace",
		configMapName, metav1.NamespaceSystem)

	kubeletCfg, err := apiclient.GetConfigMapWithRetry(client, metav1.NamespaceSystem, configMapName)
	// If the ConfigMap wasn't found and the kubelet version is v1.10.x, where we didn't support the config file yet
	// just return, don't error out
	if apierrors.IsNotFound(err) && kubeletVersion.Minor() == 10 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// populate static pod path of kubelet configuration in OpenYurt
	_, kubeletCodecs, err := kubeletscheme.NewSchemeAndCodecs()
	if err != nil {
		return nil, err
	}
	kc, err := kubeletcodec.DecodeKubeletConfiguration(kubeletCodecs, []byte(kubeletCfg.Data[constants.KubeletBaseConfigurationConfigMapKey]))
	if err != nil {
		return nil, err
	}
	if kc.StaticPodPath == "" {
		kc.StaticPodPath = filepath.Join(constants.KubernetesDir, constants.ManifestsSubDirName)
	}

	data, err := kubeletcodec.EncodeKubeletConfig(kc, kubeletconfigv1beta1.SchemeGroupVersion)
	if err != nil {
		return nil, err
	}

	return kc, writeConfigBytesToDisk(data, kubeletDir)
}

// writeConfigBytesToDisk writes a byte slice down to disk at the specific location of the kubelet config file
func writeConfigBytesToDisk(b []byte, kubeletDir string) error {
	configFile := filepath.Join(kubeletDir, constants.KubeletConfigurationFileName)
	klog.Infof("[kubelet-start] Writing kubelet configuration to file %q", configFile)

	// creates target folder if not already exists
	if err := os.MkdirAll(kubeletDir, 0700); err != nil {
		return errors.Wrapf(err, "failed to create directory %q", kubeletDir)
	}

	if err := os.WriteFile(configFile, b, 0644); err != nil {
		return errors.Wrapf(err, "failed to write kubelet configuration to the file %q", configFile)
	}
	return nil
}

// LvsStaticPodYaml return lvs care static pod yaml
func LvsStaticPodYaml(vip string, masters []string, image string) string {
	if vip == "" || len(masters) == 0 {
		return ""
	}
	if image == "" {
		image = DefaultLvsCareImage
	}
	args := []string{"care", "--vs", vip + ":6443", "--health-path", "/healthz", "--health-schem", "https"}
	for _, m := range masters {
		args = append(args, "--rs")
		args = append(args, m)
	}
	flag := true
	pod := componentPod(v1.Container{
		Name:            LvsCareStaticPodName,
		Image:           image,
		Command:         []string{LvsCareCommand},
		Args:            args,
		ImagePullPolicy: v1.PullIfNotPresent,
		SecurityContext: &v1.SecurityContext{Privileged: &flag},
	})
	yaml, err := podToYaml(pod)
	if err != nil {
		logrus.Errorf("failed to decode lvs care static pod yaml: %s", err)
		return ""
	}
	return string(yaml)
}

// componentPod returns a Pod object from the container and volume specifications
func componentPod(container v1.Container) v1.Pod {
	hostPathType := v1.HostPathUnset
	mountName := "lib-modules"
	volumes := []v1.Volume{
		{Name: mountName, VolumeSource: v1.VolumeSource{
			HostPath: &v1.HostPathVolumeSource{
				Path: "/lib/modules",
				Type: &hostPathType,
			},
		}},
	}
	container.VolumeMounts = []v1.VolumeMount{
		{Name: mountName, ReadOnly: true, MountPath: "/lib/modules"},
	}

	return v1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      container.Name,
			Namespace: metav1.NamespaceSystem,
		},
		Spec: v1.PodSpec{
			Containers:  []v1.Container{container},
			HostNetwork: true,
			Volumes:     volumes,
		},
	}
}

func podToYaml(pod v1.Pod) ([]byte, error) {
	codecs := scheme.Codecs
	gv := v1.SchemeGroupVersion
	const mediaType = runtime.ContentTypeYAML
	info, ok := runtime.SerializerInfoForMediaType(codecs.SupportedMediaTypes(), mediaType)
	if !ok {
		return []byte{}, errors.Errorf("unsupported media type %q", mediaType)
	}

	encoder := codecs.EncoderForVersion(info.Serializer, gv)
	return runtime.Encode(encoder, &pod)
}

// addLvsStaticPodYaml generate lvscare static yaml for worker node.
func addLvsStaticPodYaml(data joindata.YurtJoinData, podManifestPath string) error {
	klog.Info("[join-node] Adding lvscare static yaml")
	if _, err := os.Stat(podManifestPath); err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(podManifestPath, os.ModePerm)
			if err != nil {
				return err
			}
		} else {
			klog.Errorf("Describe dir %s fail: %v", podManifestPath, err)
			return err
		}
	}

	yaml := LvsStaticPodYaml(yurtconstants.DefaultVIP, strings.Split(data.ServerAddr(), ","), DefaultLvsCareImage)
	if err := os.WriteFile(filepath.Join(podManifestPath, LvsStaticPodFileName), []byte(yaml), 0600); err != nil {
		return err
	}

	klog.Info("[join-node] Add lvscare static yaml is ok")
	return nil
}

// addYurthubStaticYaml generate YurtHub static yaml for worker node.
func addYurthubStaticYaml(data joindata.YurtJoinData, podManifestPath string) error {
	klog.Info("[join-node] Adding edge hub static yaml")
	if _, err := os.Stat(podManifestPath); err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(podManifestPath, os.ModePerm)
			if err != nil {
				return err
			}
		} else {
			klog.Errorf("Describe dir %s fail: %v", podManifestPath, err)
			return err
		}
	}

	// convert
	// 192.168.152.131:6443,192.168.152.132:6443
	// to
	// https://192.168.152.131:6443,https://192.168.152.132:6443
	serverAddrs := strings.Split(data.ServerAddr(), ",")
	for i := 0; i < len(serverAddrs); i++ {
		serverAddrs[i] = fmt.Sprintf("https://%s", serverAddrs[i])
	}
	kubernetesServerAddrs := strings.Join(serverAddrs, ",")

	ctx := map[string]string{
		"kubernetesServerAddr": kubernetesServerAddrs,
		"image":                data.YurtHubImage(),
		"joinToken":            data.JoinToken(),
		"workingMode":          data.NodeRegistration().WorkingMode,
		"organizations":        data.NodeRegistration().Organizations,
	}

	yurthubTemplate, err := templates.SubsituteTemplate(edgenode.YurthubTemplate, ctx)
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(podManifestPath, yurtconstants.YurthubStaticPodFileName), []byte(yurthubTemplate), 0600); err != nil {
		return err
	}
	klog.Info("[join-node] Add hub agent static yaml is ok")
	return nil
}
