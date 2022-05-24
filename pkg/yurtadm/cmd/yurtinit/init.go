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

package yurtinit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"k8s.io/klog/v2"

	strutil "github.com/openyurtio/openyurt/pkg/util/strings"
	tmplutil "github.com/openyurtio/openyurt/pkg/util/templates"
	"github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
)

const (
	// APIServerAdvertiseAddress flag sets the IP address the API Server will advertise it's listening on. Specify '0.0.0.0' to use the address of the default network interface.
	APIServerAdvertiseAddress = "apiserver-advertise-address"
	//YurttunnelServerAddress flag sets the IP address of Yurttunnel Server.
	YurttunnelServerAddress = "yurt-tunnel-server-address"
	// NetworkingServiceSubnet flag sets the range of IP address for service VIPs.
	NetworkingServiceSubnet = "service-cidr"
	// NetworkingPodSubnet flag sets the range of IP addresses for the pod network. If set, the control plane will automatically allocate CIDRs for every node.
	NetworkingPodSubnet = "pod-network-cidr"
	// OpenYurtVersion flag sets the OpenYurt version for the control plane.
	OpenYurtVersion = "openyurt-version"
	// ImageRepository flag sets the container registry to pull control plane images from.
	ImageRepository = "image-repository"
	// PassWd flag is the password of master server.
	PassWd = "passwd"

	TmpDownloadDir = "/tmp"

	SealerUrlFormat      = "https://github.com/alibaba/sealer/releases/download/%s/sealer-%s-linux-%s.tar.gz"
	DefaultSealerVersion = "v0.6.1"

	InitClusterImage = "%s/openyurt-cluster:%s"
	SealerRunCmd     = "sealer apply -f %s/Clusterfile"

	OpenYurtClusterfile = `
apiVersion: sealer.cloud/v2
kind: Cluster
metadata:
  name: my-cluster
spec:
  hosts:
  - ips:
    - {{.apiserver_address}}
    roles:
    - master
  image: {{.cluster_image}}
  ssh:
    passwd: {{.passwd}}
    pk: /root/.ssh/id_rsa
    user: root
  env:
  - YurttunnelServerAddress={{.yurttunnel_server_address}}
---
apiVersion: sealer.cloud/v2
kind: KubeadmConfig
metadata:
  name: default-kubernetes-config
spec:
  networking:
    {{if .pod_subnet }}
    podSubnet: {{.pod_subnet}}
    {{end}}
    {{if .service_subnet}}
    serviceSubnet: {{.service_subnet}}
    {{end}}
  controllerManager:
    extraArgs:
      controllers: -nodelifecycle,*,bootstrapsigner,tokencleaner
`
)

var (
	ValidSealerVersions = []string{
		"v0.6.1",
	}
)

// clusterInitializer init a node to master of openyurt cluster
type clusterInitializer struct {
	InitOptions
}

// NewCmdInit use tool sealer to initializer a master of OpenYurt cluster.
// It will deploy all openyurt components, such as yurt-app-manager, yurt-tunnel-server, etc.
func NewCmdInit() *cobra.Command {
	o := NewInitOptions()

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Run this command in order to set up the OpenYurt control plane",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := o.Validate(); err != nil {
				return err
			}
			initializer := NewInitializerWithOptions(o)
			if err := initializer.Run(); err != nil {
				return err
			}
			return nil
		},
		Args: cobra.NoArgs,
	}

	addFlags(cmd.Flags(), o)
	return cmd
}

func addFlags(flagset *flag.FlagSet, o *InitOptions) {
	flagset.StringVarP(
		&o.AdvertiseAddress, APIServerAdvertiseAddress, "", o.AdvertiseAddress,
		"The IP address the API Server will advertise it's listening on.",
	)
	flagset.StringVarP(
		&o.YurttunnelServerAddress, YurttunnelServerAddress, "", o.YurttunnelServerAddress,
		"The yurt-tunnel-server address.")
	flagset.StringVarP(
		&o.ServiceSubnet, NetworkingServiceSubnet, "", o.ServiceSubnet,
		"Use alternative range of IP address for service VIPs.",
	)
	flagset.StringVarP(
		&o.PodSubnet, NetworkingPodSubnet, "", o.PodSubnet,
		"Specify range of IP addresses for the pod network. If set, the control plane will automatically allocate CIDRs for every node.",
	)
	flagset.StringVarP(&o.Password, PassWd, "p", o.Password,
		"set master server ssh password",
	)
	flagset.StringVarP(
		&o.OpenYurtVersion, OpenYurtVersion, "", o.OpenYurtVersion,
		`Choose a specific OpenYurt version for the control plane.`,
	)
	flagset.StringVarP(&o.ImageRepository, ImageRepository, "", o.ImageRepository,
		"Choose a registry to pull cluster images from",
	)
}

func NewInitializerWithOptions(o *InitOptions) *clusterInitializer {
	return &clusterInitializer{
		*o,
	}
}

// Run use sealer to initialize the master node.
func (ci *clusterInitializer) Run() error {
	if err := CheckAndInstallSealer(); err != nil {
		return err
	}

	if err := ci.PrepareClusterfile(); err != nil {
		return err
	}

	if err := ci.InstallCluster(); err != nil {
		return err
	}
	return nil
}

// CheckAndInstallSealer install sealer, skip install if it exists
func CheckAndInstallSealer() error {
	klog.Infof("Check and install sealer")
	sealerExist := false
	if _, err := exec.LookPath("sealer"); err == nil {
		if b, err := exec.Command("sealer", "version").CombinedOutput(); err == nil {
			info := make(map[string]string)
			if err := json.Unmarshal(b, &info); err != nil {
				return fmt.Errorf("Can't get the existing sealer version: %w", err)
			}
			sealerVersion := info["gitVersion"]
			if strutil.IsInStringLst(ValidSealerVersions, sealerVersion) {
				klog.Infof("Sealer %s already exist, skip install.", sealerVersion)
				sealerExist = true
			} else {
				return fmt.Errorf("The existing sealer version %s is not supported, please clean it. Valid server versions are %v.", sealerVersion, ValidSealerVersions)
			}
		}
	}

	if !sealerExist {
		// download and install sealer
		packageUrl := fmt.Sprintf(SealerUrlFormat, DefaultSealerVersion, DefaultSealerVersion, runtime.GOARCH)
		savePath := fmt.Sprintf("%s/sealer-%s-linux-%s.tar.gz", TmpDownloadDir, DefaultSealerVersion, runtime.GOARCH)
		klog.V(1).Infof("Download sealer from: %s", packageUrl)
		if err := util.DownloadFile(packageUrl, savePath, 3); err != nil {
			return fmt.Errorf("Download sealer fail: %w", err)
		}
		if err := util.Untar(savePath, TmpDownloadDir); err != nil {
			return err
		}
		comp := "sealer"
		target := fmt.Sprintf("/usr/bin/%s", comp)
		if err := edgenode.CopyFile(TmpDownloadDir+"/"+comp, target, constants.DirMode); err != nil {
			return err
		}
	}
	return nil
}

// InstallCluster initialize the master of openyurt cluster by calling sealer
func (ci *clusterInitializer) InstallCluster() error {
	klog.Infof("init an openyurt cluster")
	runCmd := fmt.Sprintf(SealerRunCmd, TmpDownloadDir)
	cmd := exec.Command("bash", "-c", runCmd)
	return execCmd(cmd)
}

// PrepareClusterfile fill the template and write the Clusterfile to the /tmp
func (ci *clusterInitializer) PrepareClusterfile() error {
	klog.Infof("generate Clusterfile for openyurt")
	err := os.MkdirAll(TmpDownloadDir, constants.DirMode)
	if err != nil {
		return err
	}

	clusterfile, err := tmplutil.SubsituteTemplate(OpenYurtClusterfile, map[string]string{
		"apiserver_address":         ci.AdvertiseAddress,
		"cluster_image":             fmt.Sprintf(InitClusterImage, ci.ImageRepository, ci.OpenYurtVersion),
		"passwd":                    ci.Password,
		"pod_subnet":                ci.PodSubnet,
		"service_subnet":            ci.ServiceSubnet,
		"yurttunnel_server_address": ci.YurttunnelServerAddress,
	})
	if err != nil {
		return err
	}

	err = os.WriteFile(fmt.Sprintf("%s/Clusterfile", TmpDownloadDir), []byte(clusterfile), constants.FileMode)
	if err != nil {
		return err
	}
	return nil
}

func execCmd(cmd *exec.Cmd) error {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	outStr, errStr := string(stdout.Bytes()), string(stderr.Bytes())
	fmt.Printf(outStr)
	if err != nil {
		pos := strings.Index(errStr, "Usage:")
		fmt.Printf(errStr[:pos])
	}
	return err
}
