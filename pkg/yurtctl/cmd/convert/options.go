/*
Copyright 2021 The OpenYurt Authors.

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

package convert

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/spf13/pflag"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	enutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/edgenode"
	kubeutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/kubernetes"
	strutil "github.com/openyurtio/openyurt/pkg/yurtctl/util/strings"
)

// Provider signifies the provider type
type Provider string

const (
	// ProviderMinikube is used if the target kubernetes is run on minikube
	ProviderMinikube Provider = "minikube"
	// ProviderACK is used if the target kubernetes is run on ack
	ProviderACK     Provider = "ack"
	ProviderKubeadm Provider = "kubeadm"
	// ProviderKind is used if the target kubernetes is run on kind
	ProviderKind Provider = "kind"

	Amd64 string = "amd64"
	Arm64 string = "arm64"
	Arm   string = "arm"
)

var (
	// ValidServerVersions contains all compatible server version
	// yurtctl only support Kubernetes 1.12+ - 1.16+ for now
	ValidServerVersions = []string{
		"1.12", "1.12+",
		"1.13", "1.13+",
		"1.14", "1.14+",
		"1.16", "1.16+",
		"1.18", "1.18+",
		"1.20", "1.20+"}
)

// ConvertOptions has the information that required by convert operation
type ConvertOptions struct {
	CloudNodes []string
	// AutonomousNodes stores the names of edge nodes that are going to be marked as autonomous.
	// If empty, all edge nodes will be marked as autonomous.
	AutonomousNodes           []string
	YurttunnelServerAddress   string
	KubeConfigPath            string
	KubeadmConfPath           string
	Provider                  Provider
	YurthubHealthCheckTimeout time.Duration
	WaitServantJobTimeout     time.Duration
	IgnorePreflightErrors     sets.String
	DeployTunnel              bool
	EnableAppManager          bool

	SystemArchitecture         string
	YurhubImage                string
	YurtControllerManagerImage string
	NodeServantImage           string
	YurttunnelServerImage      string
	YurttunnelAgentImage       string
	YurtAppManagerImage        string

	PodMainfestPath         string
	ClientSet               *kubernetes.Clientset
	YurtAppManagerClientSet dynamic.Interface
}

// NewConvertOptions creates a new ConvertOptions
func NewConvertOptions() *ConvertOptions {
	return &ConvertOptions{
		CloudNodes:            []string{},
		AutonomousNodes:       []string{},
		IgnorePreflightErrors: sets.NewString(),
	}
}

// Complete completes all the required options
func (co *ConvertOptions) Complete(flags *pflag.FlagSet) error {
	cnStr, err := flags.GetString("cloud-nodes")
	if err != nil {
		return err
	}
	if cnStr != "" {
		co.CloudNodes = strings.Split(cnStr, ",")
	}

	anStr, err := flags.GetString("autonomous-nodes")
	if err != nil {
		return err
	}
	if anStr != "" {
		co.AutonomousNodes = strings.Split(anStr, ",")
	}

	ytsa, err := flags.GetString("yurt-tunnel-server-address")
	if err != nil {
		return err
	}
	co.YurttunnelServerAddress = ytsa

	kcp, err := flags.GetString("kubeadm-conf-path")
	if err != nil {
		return err
	}
	co.KubeadmConfPath = kcp

	pStr, err := flags.GetString("provider")
	if err != nil {
		return err
	}
	co.Provider = Provider(pStr)

	yurthubHealthCheckTimeout, err := flags.GetDuration("yurthub-healthcheck-timeout")
	if err != nil {
		return err
	}
	co.YurthubHealthCheckTimeout = yurthubHealthCheckTimeout

	waitServantJobTimeout, err := flags.GetDuration("wait-servant-job-timeout")
	if err != nil {
		return err
	}
	co.WaitServantJobTimeout = waitServantJobTimeout

	ipStr, err := flags.GetString("ignore-preflight-errors")
	if err != nil {
		return err
	}
	if ipStr != "" {
		ipStr = strings.ToLower(ipStr)
		co.IgnorePreflightErrors = sets.NewString(strings.Split(ipStr, ",")...)
	}

	dt, err := flags.GetBool("deploy-yurttunnel")
	if err != nil {
		return err
	}
	co.DeployTunnel = dt

	eam, err := flags.GetBool("enable-app-manager")
	if err != nil {
		return err
	}
	co.EnableAppManager = eam

	sa, err := flags.GetString("system-architecture")
	if err != nil {
		return err
	}
	co.SystemArchitecture = sa

	yhi, err := flags.GetString("yurthub-image")
	if err != nil {
		return err
	}
	co.YurhubImage = yhi

	ycmi, err := flags.GetString("yurt-controller-manager-image")
	if err != nil {
		return err
	}
	co.YurtControllerManagerImage = ycmi

	nsi, err := flags.GetString("node-servant-image")
	if err != nil {
		return err
	}
	co.NodeServantImage = nsi

	ytsi, err := flags.GetString("yurt-tunnel-server-image")
	if err != nil {
		return err
	}
	co.YurttunnelServerImage = ytsi

	ytai, err := flags.GetString("yurt-tunnel-agent-image")
	if err != nil {
		return err
	}
	co.YurttunnelAgentImage = ytai

	yami, err := flags.GetString("yurt-app-manager-image")
	if err != nil {
		return err
	}
	co.YurtAppManagerImage = yami

	// prepare path of cluster kubeconfig file
	co.KubeConfigPath, err = kubeutil.PrepareKubeConfigPath(flags)
	if err != nil {
		return err
	}

	co.PodMainfestPath = enutil.GetPodManifestPath()

	// parse kubeconfig and generate the clientset
	co.ClientSet, err = kubeutil.GenClientSet(flags)
	if err != nil {
		return err
	}

	// parse kubeconfig and generate the yurtappmanagerclientset
	co.YurtAppManagerClientSet, err = kubeutil.GenDynamicClientSet(flags)
	if err != nil {
		return err
	}

	return nil
}

// Validate makes sure provided values for ConvertOptions are valid
func (co *ConvertOptions) Validate() error {
	if err := ValidateKubeConfig(co.KubeConfigPath); err != nil {
		return err
	}

	if err := ValidateServerVersion(co.ClientSet); err != nil {
		return err
	}

	nodeLst, err := co.ClientSet.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return err
	}
	if err := ValidateCloudNodes(co.CloudNodes, nodeLst); err != nil {
		return err
	}

	edgeNodeNames := getEdgeNodeNames(nodeLst, co.CloudNodes)
	if err := ValidateNodeAutonomy(edgeNodeNames, co.AutonomousNodes); err != nil {
		return err
	}
	// If empty, mark all edge nodes as autonomous
	if len(co.AutonomousNodes) == 0 {
		co.AutonomousNodes = make([]string, len(edgeNodeNames))
		copy(co.AutonomousNodes, edgeNodeNames)
	}

	if err := ValidateYurttunnelServerAddress(co.YurttunnelServerAddress); err != nil {
		return err
	}
	if err := ValidateProvider(co.Provider); err != nil {
		return err
	}
	if err := ValidateYurtHubHealthCheckTimeout(co.YurthubHealthCheckTimeout); err != nil {
		return err
	}
	if err := ValidateWaitServantJobTimeout(co.WaitServantJobTimeout); err != nil {
		return err
	}
	if err := ValidateIgnorePreflightErrors(co.IgnorePreflightErrors); err != nil {
		return err
	}
	if err := ValidateSystemArchitecture(co.SystemArchitecture); err != nil {
		return err
	}

	return nil
}

// ValidateServerVersion checks if the target server's version is supported
func ValidateServerVersion(cliSet *kubernetes.Clientset) error {
	serverVersion, err := discovery.NewDiscoveryClient(cliSet.RESTClient()).ServerVersion()
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

func ValidateCloudNodes(cloudNodeNames []string, nodeLst *v1.NodeList) error {
	if cloudNodeNames == nil || len(cloudNodeNames) == 0 {
		return fmt.Errorf("invalid --cloud-nodes: cannot be empty, please specify the cloud nodes")
	}

	var notExistNodeNames []string
	nodeNameSet := make(map[string]struct{})
	for _, node := range nodeLst.Items {
		nodeNameSet[node.GetName()] = struct{}{}
	}
	for _, name := range cloudNodeNames {
		if _, ok := nodeNameSet[name]; !ok {
			notExistNodeNames = append(notExistNodeNames, name)
		}
	}
	if len(notExistNodeNames) != 0 {
		return fmt.Errorf("invalid --cloud-nodes: the nodes %v are not kubernetes node, can't be converted to cloud node", notExistNodeNames)
	}
	return nil
}

func ValidateNodeAutonomy(edgeNodeNames []string, autonomousNodeNames []string) error {
	var invaildation []string
	for _, name := range autonomousNodeNames {
		if !strutil.IsInStringLst(edgeNodeNames, name) {
			invaildation = append(invaildation, name)
		}
	}
	if len(invaildation) != 0 {
		return fmt.Errorf("invalid --autonomous-nodes: can't make unedge nodes %v autonomous", invaildation)
	}
	return nil
}

func ValidateProvider(provider Provider) error {
	if provider != ProviderMinikube && provider != ProviderACK &&
		provider != ProviderKubeadm && provider != ProviderKind {
		return fmt.Errorf("invalid --provider: %s, valid providers are: minikube, ack, kubeadm, kind",
			provider)
	}
	return nil
}

func ValidateYurttunnelServerAddress(address string) error {
	if address != "" {
		if _, _, err := net.SplitHostPort(address); err != nil {
			return fmt.Errorf("invalid --yurt-tunnel-server-address: %s", err)
		}
	}
	return nil
}

func ValidateSystemArchitecture(arch string) error {
	if arch != Amd64 && arch != Arm64 && arch != Arm {
		return fmt.Errorf("invalid --system-architecture: %s, valid arch are: amd64, arm64, arm", arch)
	}
	return nil
}

func ValidateIgnorePreflightErrors(ignoreErrors sets.String) error {
	if ignoreErrors.Has("all") && ignoreErrors.Len() > 1 {
		return fmt.Errorf("invalid --ignore-preflight-errors: please don't specify individual checks if 'all' is used in option 'ignorePreflightErrors'")
	}
	return nil
}

func ValidateKubeConfig(kbCfgPath string) error {
	if _, err := enutil.FileExists(kbCfgPath); err != nil {
		return fmt.Errorf("invalid kubeconfig path: %v", err)
	}
	return nil

}

func ValidateYurtHubHealthCheckTimeout(t time.Duration) error {
	if t <= 0 {
		return fmt.Errorf("invalid --yurthub-healthcheck-timeout: time must be a valid number(greater than 0)")
	}
	return nil
}

func ValidateWaitServantJobTimeout(t time.Duration) error {
	if t <= 0 {
		return fmt.Errorf("invalid --wait-servant-job-timeout: time must be a valid number(greater than 0)")
	}
	return nil
}
