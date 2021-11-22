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
	"os"

	"github.com/lithammer/dedent"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/klog"
	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	kubeadmscheme "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/scheme"
	kubeadmapiv1beta2 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta2"
	"k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/validation"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/options"
	kubeadmPhase "k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/join"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/workflow"
	cmdutil "k8s.io/kubernetes/cmd/kubeadm/app/cmd/util"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	"k8s.io/kubernetes/cmd/kubeadm/app/discovery"
	configutil "k8s.io/kubernetes/cmd/kubeadm/app/util/config"
	kubeconfigutil "k8s.io/kubernetes/cmd/kubeadm/app/util/kubeconfig"

	yurtphase "github.com/openyurtio/openyurt/pkg/yurtctl/cmd/join/phases"
)

var (
	joinWorkerNodeDoneMsg = dedent.Dedent(`
		This node has joined the cluster:
		* Certificate signing request was sent to apiserver and a response was received.
		* The Kubelet was informed of the new secure connection details.

		Run 'kubectl get nodes' on the control-plane to see this node join the cluster.

		`)
)

type joinOptions struct {
	cfgPath               string
	token                 string
	controlPlane          bool
	ignorePreflightErrors []string
	externalcfg           *kubeadmapiv1beta2.JoinConfiguration
	kustomizeDir          string
	nodeType              string
	yurthubImage          string
	markAutonomous        bool
}

// newJoinOptions returns a struct ready for being used for creating cmd join flags.
func newJoinOptions() *joinOptions {
	// initialize the public kubeadm config API by applying defaults
	externalcfg := &kubeadmapiv1beta2.JoinConfiguration{}

	// Add optional config objects to host flags.
	// un-set objects will be cleaned up afterwards (into newJoinData func)
	externalcfg.Discovery.File = &kubeadmapiv1beta2.FileDiscovery{}
	externalcfg.Discovery.BootstrapToken = &kubeadmapiv1beta2.BootstrapTokenDiscovery{}
	externalcfg.ControlPlane = &kubeadmapiv1beta2.JoinControlPlane{}

	// Apply defaults
	kubeadmscheme.Scheme.Default(externalcfg)

	return &joinOptions{
		externalcfg: externalcfg,
	}
}

// NewJoinCmd returns "yurtctl join" command.
func NewCmdJoin(out io.Writer, joinOptions *joinOptions) *cobra.Command {
	if joinOptions == nil {
		joinOptions = newJoinOptions()
	}
	joinRunner := workflow.NewRunner()

	cmd := &cobra.Command{
		Use:   "join [api-server-endpoint]",
		Short: "Run this on any machine you wish to join an existing cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := joinRunner.InitData(args)
			if err != nil {
				return err
			}
			data := c.(*joinData)
			if err := joinRunner.Run(args); err != nil {
				return err
			}
			fmt.Fprint(data.outputWriter, joinWorkerNodeDoneMsg)
			return nil
		},
	}

	addJoinConfigFlags(cmd.Flags(), joinOptions)

	joinRunner.AppendPhase(yurtphase.NewPreparePhase())
	joinRunner.AppendPhase(kubeadmPhase.NewPreflightPhase())
	joinRunner.AppendPhase(yurtphase.NewCloudNodePhase())
	joinRunner.AppendPhase(yurtphase.NewConvertPhase())
	joinRunner.AppendPhase(yurtphase.NewPostcheckPhase())

	joinRunner.SetDataInitializer(func(cmd *cobra.Command, args []string) (workflow.RunData, error) {
		return newJoinData(cmd, args, joinOptions, out)
	})
	joinRunner.BindToCommand(cmd)
	return cmd
}

// addJoinConfigFlags adds join flags bound to the config to the specified flagset
func addJoinConfigFlags(flagSet *flag.FlagSet, joinOptions *joinOptions) {
	flagSet.StringVar(
		&joinOptions.externalcfg.NodeRegistration.Name, options.NodeName, joinOptions.externalcfg.NodeRegistration.Name,
		`Specify the node name.`,
	)
	flagSet.StringSliceVar(
		&joinOptions.externalcfg.Discovery.BootstrapToken.CACertHashes, options.TokenDiscoveryCAHash, []string{},
		"For token-based discovery, validate that the root CA public key matches this hash (format: \"<type>:<value>\").",
	)
	flagSet.BoolVar(
		&joinOptions.externalcfg.Discovery.BootstrapToken.UnsafeSkipCAVerification, options.TokenDiscoverySkipCAHash, false,
		"For token-based discovery, allow joining without --discovery-token-ca-cert-hash pinning.",
	)
	flagSet.StringSliceVar(
		&joinOptions.ignorePreflightErrors, options.IgnorePreflightErrors, joinOptions.ignorePreflightErrors,
		"A list of checks whose errors will be shown as warnings. Example: 'IsPrivilegedUser,Swap'. Value 'all' ignores errors from all checks.",
	)
	flagSet.StringVar(
		&joinOptions.token, options.TokenStr, "",
		"Use this token for both discovery-token and tls-bootstrap-token when those values are not provided.",
	)
	flagSet.StringVar(
		&joinOptions.nodeType, "node-type", "",
		"Sets the node is edge-node or cloud-node",
	)
	flagSet.StringVar(
		&joinOptions.yurthubImage, "yurthub-image", "",
		"Sets the image version of yurthub component",
	)
	flagSet.BoolVar(
		&joinOptions.markAutonomous, "mark-autonomous", true,
		"Annotate node autonomous if set true, default true.",
	)
	cmdutil.AddCRISocketFlag(flagSet, &joinOptions.externalcfg.NodeRegistration.CRISocket)
}

type joinData struct {
	cfg                   *kubeadmapi.JoinConfiguration
	initCfg               *kubeadmapi.InitConfiguration
	tlsBootstrapCfg       *clientcmdapi.Config
	clientSet             *clientset.Clientset
	ignorePreflightErrors sets.String
	outputWriter          io.Writer
	kustomizeDir          string
	nodeType              string
	yurthubImage          string
	markAutonomous        bool
}

// newJoinData returns a new joinData struct to be used for the execution of the kubeadm join workflow.
// This func takes care of validating joinOptions passed to the command, and then it converts
// options into the internal JoinConfiguration type that is used as input all the phases in the kubeadm join workflow
func newJoinData(cmd *cobra.Command, args []string, opt *joinOptions, out io.Writer) (*joinData, error) {
	// Re-apply defaults to the public kubeadm API (this will set only values not exposed/not set as a flags)
	kubeadmscheme.Scheme.Default(opt.externalcfg)

	// Validate standalone flags values and/or combination of flags and then assigns
	// validated values to the public kubeadm config API when applicable

	// if a token is provided, use this value for both discovery-token and tls-bootstrap-token when those values are not provided
	if len(opt.token) > 0 {
		if len(opt.externalcfg.Discovery.TLSBootstrapToken) == 0 {
			opt.externalcfg.Discovery.TLSBootstrapToken = opt.token
		}
		if len(opt.externalcfg.Discovery.BootstrapToken.Token) == 0 {
			opt.externalcfg.Discovery.BootstrapToken.Token = opt.token
		}
	}

	// if a file or URL from which to load cluster information was not provided, unset the Discovery.File object
	if len(opt.externalcfg.Discovery.File.KubeConfigPath) == 0 {
		opt.externalcfg.Discovery.File = nil
	}

	// if an APIServerEndpoint from which to retrieve cluster information was not provided, unset the Discovery.BootstrapToken object
	if len(args) == 0 {
		opt.externalcfg.Discovery.BootstrapToken = nil
	} else {
		if len(opt.cfgPath) == 0 && len(args) > 1 {
			klog.Warningf("[preflight] WARNING: More than one API server endpoint supplied on command line %v. Using the first one.", args)
		}
		opt.externalcfg.Discovery.BootstrapToken.APIServerEndpoint = args[0]
	}

	// if not joining a control plane, unset the ControlPlane object
	if !opt.controlPlane {
		if opt.externalcfg.ControlPlane != nil {
			klog.Warningf("[preflight] WARNING: JoinControlPane.controlPlane settings will be ignored when %s flag is not set.", options.ControlPlane)
		}
		opt.externalcfg.ControlPlane = nil
	}

	// if the admin.conf file already exists, use it for skipping the discovery process.
	// NB. this case can happen when we are joining a control-plane node only (and phases are invoked atomically)
	var adminKubeConfigPath = kubeadmconstants.GetAdminKubeConfigPath()
	var tlsBootstrapCfg *clientcmdapi.Config
	if _, err := os.Stat(adminKubeConfigPath); err == nil && opt.controlPlane {
		// use the admin.conf as tlsBootstrapCfg, that is the kubeconfig file used for reading the kubeadm-config during discovery
		klog.V(1).Infof("[preflight] found %s. Use it for skipping discovery", adminKubeConfigPath)
		tlsBootstrapCfg, err = clientcmd.LoadFromFile(adminKubeConfigPath)
		if err != nil {
			return nil, errors.Wrapf(err, "Error loading %s", adminKubeConfigPath)
		}
	}

	if err := validation.ValidateMixedArguments(cmd.Flags()); err != nil {
		return nil, err
	}

	// Either use the config file if specified, or convert public kubeadm API to the internal JoinConfiguration
	// and validates JoinConfiguration
	if opt.externalcfg.NodeRegistration.Name == "" {
		klog.V(1).Infoln("[preflight] found NodeName empty; using OS hostname as NodeName")
	}

	if opt.externalcfg.ControlPlane != nil && opt.externalcfg.ControlPlane.LocalAPIEndpoint.AdvertiseAddress == "" {
		klog.V(1).Infoln("[preflight] found advertiseAddress empty; using default interface's IP address as advertiseAddress")
	}

	cfg, err := configutil.LoadOrDefaultJoinConfiguration(opt.cfgPath, opt.externalcfg)
	if err != nil {
		return nil, err
	}

	ignorePreflightErrorsSet, err := validation.ValidateIgnorePreflightErrors(opt.ignorePreflightErrors, cfg.NodeRegistration.IgnorePreflightErrors)
	if err != nil {
		return nil, err
	}
	// Also set the union of pre-flight errors to JoinConfiguration, to provide a consistent view of the runtime configuration:
	cfg.NodeRegistration.IgnorePreflightErrors = ignorePreflightErrorsSet.List()

	// override node name and CRI socket from the command line opt
	if opt.externalcfg.NodeRegistration.Name != "" {
		cfg.NodeRegistration.Name = opt.externalcfg.NodeRegistration.Name
	}
	if opt.externalcfg.NodeRegistration.CRISocket != "" {
		cfg.NodeRegistration.CRISocket = opt.externalcfg.NodeRegistration.CRISocket
	}

	if cfg.ControlPlane != nil {
		if err := configutil.VerifyAPIServerBindAddress(cfg.ControlPlane.LocalAPIEndpoint.AdvertiseAddress); err != nil {
			return nil, err
		}
	}

	return &joinData{
		cfg:                   cfg,
		tlsBootstrapCfg:       tlsBootstrapCfg,
		ignorePreflightErrors: ignorePreflightErrorsSet,
		outputWriter:          out,
		kustomizeDir:          opt.kustomizeDir,
		nodeType:              opt.nodeType,
		yurthubImage:          opt.yurthubImage,
		markAutonomous:        opt.markAutonomous,
	}, nil
}

// CertificateKey returns the key used to encrypt the certs.
func (j *joinData) CertificateKey() string {
	if j.cfg.ControlPlane != nil {
		return j.cfg.ControlPlane.CertificateKey
	}
	return ""
}

// Cfg returns the JoinConfiguration.
func (j *joinData) Cfg() *kubeadmapi.JoinConfiguration {
	return j.cfg
}

// TLSBootstrapCfg returns the cluster-info (kubeconfig).
func (j *joinData) TLSBootstrapCfg() (*clientcmdapi.Config, error) {
	if j.tlsBootstrapCfg != nil {
		return j.tlsBootstrapCfg, nil
	}
	klog.V(1).Infoln("[preflight] Discovering cluster-info")
	tlsBootstrapCfg, err := discovery.For(j.cfg)
	j.tlsBootstrapCfg = tlsBootstrapCfg
	return tlsBootstrapCfg, err
}

// InitCfg returns the InitConfiguration.
func (j *joinData) InitCfg() (*kubeadmapi.InitConfiguration, error) {
	if j.initCfg != nil {
		return j.initCfg, nil
	}
	if _, err := j.TLSBootstrapCfg(); err != nil {
		return nil, err
	}
	for _, cluster := range j.tlsBootstrapCfg.Clusters {
		cluster.Server = fmt.Sprintf("https://%s", j.cfg.Discovery.BootstrapToken.APIServerEndpoint)
	}
	klog.V(1).Infoln("[preflight] Fetching init configuration")
	initCfg, err := fetchInitConfigurationFromJoinConfiguration(j.cfg, j.tlsBootstrapCfg)
	j.initCfg = initCfg
	return initCfg, err
}

// fetchInitConfigurationFromJoinConfiguration retrieves the init configuration from a join configuration, performing the discovery
func fetchInitConfigurationFromJoinConfiguration(cfg *kubeadmapi.JoinConfiguration, tlsBootstrapCfg *clientcmdapi.Config) (*kubeadmapi.InitConfiguration, error) {
	// Retrieves the kubeadm configuration
	klog.V(1).Infoln("[preflight] Retrieving KubeConfig objects")
	initConfiguration, err := fetchInitConfiguration(tlsBootstrapCfg)
	if err != nil {
		return nil, err
	}

	// Create the final KubeConfig file with the cluster name discovered after fetching the cluster configuration
	clusterinfo := kubeconfigutil.GetClusterFromKubeConfig(tlsBootstrapCfg)
	tlsBootstrapCfg.Clusters = map[string]*clientcmdapi.Cluster{
		initConfiguration.ClusterName: clusterinfo,
	}
	tlsBootstrapCfg.Contexts[tlsBootstrapCfg.CurrentContext].Cluster = initConfiguration.ClusterName

	// injects into the kubeadm configuration the information about the joining node
	initConfiguration.NodeRegistration = cfg.NodeRegistration
	if cfg.ControlPlane != nil {
		initConfiguration.LocalAPIEndpoint = cfg.ControlPlane.LocalAPIEndpoint
	}

	return initConfiguration, nil
}

// fetchInitConfiguration reads the cluster configuration from the kubeadm-admin configMap
func fetchInitConfiguration(tlsBootstrapCfg *clientcmdapi.Config) (*kubeadmapi.InitConfiguration, error) {
	// creates a client to access the cluster using the bootstrap token identity
	tlsClient, err := kubeconfigutil.ToClientSet(tlsBootstrapCfg)
	if err != nil {
		return nil, errors.Wrap(err, "unable to access the cluster")
	}

	// Fetches the init configuration
	initConfiguration, err := configutil.FetchInitConfigurationFromCluster(tlsClient, os.Stdout, "preflight", true)
	if err != nil {
		return nil, errors.Wrap(err, "unable to fetch the kubeadm-config ConfigMap")
	}

	return initConfiguration, nil
}

// ClientSet returns the ClientSet for accessing the cluster with the identity defined in admin.conf.
func (j *joinData) ClientSet() (*clientset.Clientset, error) {
	if j.clientSet != nil {
		return j.clientSet, nil
	}
	path := kubeadmconstants.GetAdminKubeConfigPath()
	client, err := kubeconfigutil.ClientSetFromFile(path)
	if err != nil {
		return nil, err
	}
	j.clientSet = client
	return client, nil
}

// IgnorePreflightErrors returns the list of preflight errors to ignore.
func (j *joinData) IgnorePreflightErrors() sets.String {
	return j.ignorePreflightErrors
}

// OutputWriter returns the io.Writer used to write messages such as the "join done" message.
func (j *joinData) OutputWriter() io.Writer {
	return j.outputWriter
}

// KustomizeDir returns the folder where kustomize patches for static pod manifest are stored
func (j *joinData) KustomizeDir() string {
	return j.kustomizeDir
}

//NodeType returns the node is cloud-node or edge-node.
func (j *joinData) NodeType() string {
	return j.nodeType
}

//YurtHubImage returns the YurtHub image.
func (j *joinData) YurtHubImage() string {
	return j.yurthubImage
}

//MarkAutonomous returns markAutonomous setting.
func (j *joinData) MarkAutonomous() bool {
	return j.markAutonomous
}
