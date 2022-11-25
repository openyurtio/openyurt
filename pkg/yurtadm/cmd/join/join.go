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

package join

import (
	"fmt"
	"io"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/yurtadm/cmd/join/joindata"
	yurtphases "github.com/openyurtio/openyurt/pkg/yurtadm/cmd/join/phases"
	yurtconstants "github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/edgenode"
	kubeconfigutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/kubeconfig"
	yurtadmutil "github.com/openyurtio/openyurt/pkg/yurtadm/util/kubernetes"
)

type joinOptions struct {
	token                    string
	nodeType                 string
	nodeName                 string
	criSocket                string
	organizations            string
	pauseImage               string
	yurthubImage             string
	caCertHashes             []string
	unsafeSkipCAVerification bool
	ignorePreflightErrors    []string
	nodeLabels               string
	kubernetesResourceServer string
	yurthubServer            string
}

// newJoinOptions returns a struct ready for being used for creating cmd join flags.
func newJoinOptions() *joinOptions {
	return &joinOptions{
		nodeType:                 yurtconstants.EdgeNode,
		criSocket:                yurtconstants.DefaultDockerCRISocket,
		pauseImage:               yurtconstants.PauseImagePath,
		yurthubImage:             fmt.Sprintf("%s/%s:%s", yurtconstants.DefaultOpenYurtImageRegistry, yurtconstants.Yurthub, yurtconstants.DefaultOpenYurtVersion),
		caCertHashes:             make([]string, 0),
		unsafeSkipCAVerification: false,
		ignorePreflightErrors:    make([]string, 0),
		kubernetesResourceServer: yurtconstants.DefaultKubernetesResourceServer,
		yurthubServer:            yurtconstants.DefaultYurtHubServerAddr,
	}
}

type nodeJoiner struct {
	*joinData
	inReader     io.Reader
	outWriter    io.Writer
	outErrWriter io.Writer
}

// NewCmdJoin returns "yurtadm join" command.
func NewCmdJoin(in io.Reader, out io.Writer, outErr io.Writer) *cobra.Command {
	joinOptions := newJoinOptions()

	cmd := &cobra.Command{
		Use:   "join [api-server-endpoint]",
		Short: "Run this on any machine you wish to join an existing cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			o, err := newJoinData(args, joinOptions)
			if err != nil {
				return err
			}

			joiner := newJoinerWithJoinData(o, in, out, outErr)
			if err := joiner.Run(); err != nil {
				return err
			}
			return nil
		},
	}

	addJoinConfigFlags(cmd.Flags(), joinOptions)

	return cmd
}

// addJoinConfigFlags adds join flags bound to the config to the specified flagset
func addJoinConfigFlags(flagSet *flag.FlagSet, joinOptions *joinOptions) {
	flagSet.StringVar(
		&joinOptions.token, yurtconstants.TokenStr, "",
		"Use this token for both discovery-token and tls-bootstrap-token when those values are not provided.",
	)
	flagSet.StringVar(
		&joinOptions.nodeType, yurtconstants.NodeType, joinOptions.nodeType,
		"Sets the node is edge or cloud",
	)
	flagSet.StringVar(
		&joinOptions.nodeName, yurtconstants.NodeName, joinOptions.nodeName,
		`Specify the node name. if not specified, hostname will be used.`,
	)
	flagSet.StringVar(
		&joinOptions.criSocket, yurtconstants.NodeCRISocket, joinOptions.criSocket,
		"Path to the CRI socket to connect",
	)
	flagSet.StringVar(
		&joinOptions.organizations, yurtconstants.Organizations, joinOptions.organizations,
		"Organizations that will be added into hub's client certificate",
	)
	flagSet.StringVar(
		&joinOptions.pauseImage, yurtconstants.PauseImage, joinOptions.pauseImage,
		"Sets the image version of pause container",
	)
	flagSet.StringVar(
		&joinOptions.yurthubImage, yurtconstants.YurtHubImage, joinOptions.yurthubImage,
		"Sets the image version of yurthub component",
	)
	flagSet.StringSliceVar(
		&joinOptions.caCertHashes, yurtconstants.TokenDiscoveryCAHash, joinOptions.caCertHashes,
		"For token-based discovery, validate that the root CA public key matches this hash (format: \"<type>:<value>\").",
	)
	flagSet.BoolVar(
		&joinOptions.unsafeSkipCAVerification, yurtconstants.TokenDiscoverySkipCAHash, false,
		"For token-based discovery, allow joining without --discovery-token-ca-cert-hash pinning.",
	)
	flagSet.StringSliceVar(
		&joinOptions.ignorePreflightErrors, yurtconstants.IgnorePreflightErrors, joinOptions.ignorePreflightErrors,
		"A list of checks whose errors will be shown as warnings. Example: 'IsPrivilegedUser,Swap'. Value 'all' ignores errors from all checks.",
	)
	flagSet.StringVar(
		&joinOptions.nodeLabels, yurtconstants.NodeLabels, joinOptions.nodeLabels,
		"Sets the labels for joining node",
	)
	flagSet.StringVar(
		&joinOptions.kubernetesResourceServer, yurtconstants.KubernetesResourceServer, joinOptions.kubernetesResourceServer,
		"Sets the address for downloading k8s node resources",
	)
	flagSet.StringVar(
		&joinOptions.yurthubServer, yurtconstants.YurtHubServerAddr, joinOptions.yurthubServer,
		"Sets the address for yurthub server addr",
	)
}

func newJoinerWithJoinData(o *joinData, in io.Reader, out io.Writer, outErr io.Writer) *nodeJoiner {
	return &nodeJoiner{
		o,
		in,
		out,
		outErr,
	}
}

// Run use kubeadm to join the node.
func (nodeJoiner *nodeJoiner) Run() error {
	joinData := nodeJoiner.joinData

	if err := yurtphases.RunPrepare(joinData); err != nil {
		return err
	}

	if err := yurtphases.RunJoinNode(joinData, nodeJoiner.outWriter, nodeJoiner.outErrWriter); err != nil {
		return err
	}

	if err := yurtphases.RunPostCheck(joinData); err != nil {
		return err
	}

	return nil
}

type joinData struct {
	joinNodeData             *joindata.NodeRegistration
	apiServerEndpoint        string
	token                    string
	tlsBootstrapCfg          *clientcmdapi.Config
	clientSet                *clientset.Clientset
	ignorePreflightErrors    sets.String
	organizations            string
	pauseImage               string
	yurthubImage             string
	kubernetesVersion        string
	caCertHashes             sets.String
	nodeLabels               map[string]string
	kubernetesResourceServer string
	yurthubServer            string
}

// newJoinData returns a new joinData struct to be used for the execution of the kubeadm join workflow.
// This func takes care of validating joinOptions passed to the command, and then it converts
// options into the internal JoinData type that is used as input all the phases in the kubeadm join workflow
func newJoinData(args []string, opt *joinOptions) (*joinData, error) {
	// if an APIServerEndpoint from which to retrieve cluster information was not provided, unset the Discovery.BootstrapToken object
	var apiServerEndpoint string
	if len(args) == 0 {
		return nil, errors.New("apiServer endpoint is empty")
	} else {
		if len(args) > 1 {
			klog.Warningf("[preflight] WARNING: More than one API server endpoint supplied on command line %v. Using the first one.", args)
		}
		// if join multiple masters, apiServerEndpoint may be like:
		// 1.2.3.4:6443,1.2.3.5:6443,1.2.3.6:6443
		apiServerEndpoint = args[0]
	}

	if len(opt.token) == 0 {
		return nil, errors.New("join token is empty, so unable to bootstrap worker node.")
	}

	if opt.nodeType != yurtconstants.EdgeNode && opt.nodeType != yurtconstants.CloudNode {
		return nil, errors.Errorf("node type(%s) is invalid, only \"edge and cloud\" are supported", opt.nodeType)
	}

	if opt.unsafeSkipCAVerification && len(opt.caCertHashes) != 0 {
		return nil, errors.Errorf("when --discovery-token-ca-cert-hash is specified, --discovery-token-unsafe-skip-ca-verification should be false.")
	} else if len(opt.caCertHashes) == 0 && !opt.unsafeSkipCAVerification {
		return nil, errors.Errorf("when --discovery-token-ca-cert-hash is not specified, --discovery-token-unsafe-skip-ca-verification should be true")
	}

	ignoreErrors := sets.String{}
	for i := range opt.ignorePreflightErrors {
		ignoreErrors.Insert(opt.ignorePreflightErrors[i])
	}

	// Either use specified nodename or get hostname from OS envs
	name, err := edgenode.GetHostname(opt.nodeName)
	if err != nil {
		klog.Errorf("failed to get node name, %v", err)
		return nil, err
	}

	data := &joinData{
		apiServerEndpoint:     apiServerEndpoint,
		token:                 opt.token,
		tlsBootstrapCfg:       nil,
		ignorePreflightErrors: ignoreErrors,
		pauseImage:            opt.pauseImage,
		yurthubImage:          opt.yurthubImage,
		yurthubServer:         opt.yurthubServer,
		caCertHashes:          sets.NewString(opt.caCertHashes...),
		organizations:         opt.organizations,
		nodeLabels:            make(map[string]string),
		joinNodeData: &joindata.NodeRegistration{
			Name:          name,
			WorkingMode:   opt.nodeType,
			CRISocket:     opt.criSocket,
			Organizations: opt.organizations,
		},
		kubernetesResourceServer: opt.kubernetesResourceServer,
	}

	// parse node labels
	if len(opt.nodeLabels) != 0 {
		parts := strings.Split(opt.nodeLabels, ",")
		for i := range parts {
			kv := strings.Split(parts[i], "=")
			if len(kv) != 2 {
				klog.Warningf("node labels(%s) format is invalid, expect k1=v1,k2=v2", parts[i])
				continue
			}
			data.nodeLabels[kv[0]] = kv[1]
		}
	}

	// get tls bootstrap config
	cfg, err := yurtadmutil.RetrieveBootstrapConfig(data)
	if err != nil {
		klog.Errorf("failed to retrieve bootstrap config, %v", err)
		return nil, err
	}
	data.tlsBootstrapCfg = cfg

	// get kubernetes version
	client, err := kubeconfigutil.ToClientSet(cfg)
	if err != nil {
		klog.Errorf("failed to create bootstrap client, %v", err)
		return nil, err
	}
	data.clientSet = client

	k8sVersion, err := yurtadmutil.GetKubernetesVersionFromCluster(client)
	if err != nil {
		klog.Errorf("failed to get kubernetes version, %v", err)
		return nil, err
	}
	data.kubernetesVersion = k8sVersion
	klog.Infof("node join data info: %#+v", *data)

	return data, nil
}

// ServerAddr returns the public address of kube-apiserver.
func (j *joinData) ServerAddr() string {
	return j.apiServerEndpoint
}

// JoinToken returns bootstrap token for joining node
func (j *joinData) JoinToken() string {
	return j.token
}

// PauseImage returns the pause image.
func (j *joinData) PauseImage() string {
	return j.pauseImage
}

// YurtHubImage returns the YurtHub image.
func (j *joinData) YurtHubImage() string {
	return j.yurthubImage
}

// YurtHubServer returns the YurtHub server addr.
func (j *joinData) YurtHubServer() string {
	return j.yurthubServer
}

// KubernetesVersion returns the kubernetes version.
func (j *joinData) KubernetesVersion() string {
	return j.kubernetesVersion
}

// TLSBootstrapCfg returns the cluster-info (kubeconfig).
func (j *joinData) TLSBootstrapCfg() *clientcmdapi.Config {
	return j.tlsBootstrapCfg
}

// BootstrapClient returns the kube clientset.
func (j *joinData) BootstrapClient() *clientset.Clientset {
	return j.clientSet
}

func (j *joinData) NodeRegistration() *joindata.NodeRegistration {
	return j.joinNodeData
}

// IgnorePreflightErrors returns the list of preflight errors to ignore.
func (j *joinData) IgnorePreflightErrors() sets.String {
	return j.ignorePreflightErrors
}

func (j *joinData) CaCertHashes() sets.String {
	return j.caCertHashes
}

func (j *joinData) NodeLabels() map[string]string {
	return j.nodeLabels
}

func (j *joinData) KubernetesResourceServer() string {
	return j.kubernetesResourceServer
}
