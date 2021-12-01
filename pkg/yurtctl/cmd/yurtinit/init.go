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

package yurtinit

import (
	"bytes"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/lithammer/dedent"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcertutil "k8s.io/client-go/util/cert"
	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	kubeadmscheme "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/scheme"
	kubeadmapiv1beta2 "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/v1beta2"
	"k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm/validation"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/options"
	kubephases "k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/init"
	"k8s.io/kubernetes/cmd/kubeadm/app/cmd/phases/workflow"
	cmdutil "k8s.io/kubernetes/cmd/kubeadm/app/cmd/util"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	"k8s.io/kubernetes/cmd/kubeadm/app/features"
	certsphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/certs"
	kubeconfigphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/kubeconfig"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/apiclient"
	configutil "k8s.io/kubernetes/cmd/kubeadm/app/util/config"
	kubeconfigutil "k8s.io/kubernetes/cmd/kubeadm/app/util/kubeconfig"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/pubkeypin"

	yurtphase "github.com/openyurtio/openyurt/pkg/yurtctl/cmd/yurtinit/phases"
)

var (
	initDoneTempl = template.Must(template.New("init").Parse(dedent.Dedent(`
		Your OpenYurt cluster control-plane has initialized successfully!

		To start using your cluster, you need to run the following as a regular user:

		  mkdir -p $HOME/.kube
		  sudo cp -i {{.KubeConfigPath}} $HOME/.kube/config
		  sudo chown $(id -u):$(id -g) $HOME/.kube/config

		Then you can join any number of edge-nodes by running the following on each as root:

		{{.joinEdgeNodeCommand}}

        And you can join any number of cloud-nodes by running the following on each as root:
 
        {{.joinCloudNodeCommand}}

		`)))

	joinCommandTemplate = template.Must(template.New("join-edge-node").Parse(`` +
		`yurtctl join {{.ControlPlaneHostPort}} --token {{.Token}} \
    {{range $h := .CAPubKeyPins}}--discovery-token-ca-cert-hash {{$h}} {{end}} --node-type={{.NodeType}}`,
	))
)

// initOptions defines all the init options exposed via flags by kubeadm init.
// Please note that this structure includes the public kubeadm config API, but only a subset of the options
// supported by this api will be exposed as a flag.
type initOptions struct {
	cfgPath                     string
	kubeconfigDir               string
	kubeconfigPath              string
	featureGatesString          string
	ignorePreflightErrors       []string
	bto                         *options.BootstrapTokenOptions
	externalInitCfg             *kubeadmapiv1beta2.InitConfiguration
	externalClusterCfg          *kubeadmapiv1beta2.ClusterConfiguration
	kustomizeDir                string
	installCNIFile              string
	isConvertOpenYurtCluster    bool
	openyurtImageRegistry       string
	openyurtVersion             string
	openyurtTunnelServerAddress string
}

// compile-time assert that the local data object satisfies the phases data interface.
var _ yurtphase.YurtInitData = &initData{}

// initData defines all the runtime information used when running the kubeadm init workflow;
// this data is shared across all the phases that are included in the workflow.
type initData struct {
	cfg                         *kubeadmapi.InitConfiguration
	skipTokenPrint              bool
	dryRun                      bool
	kubeconfigDir               string
	kubeconfigPath              string
	ignorePreflightErrors       sets.String
	certificatesDir             string
	dryRunDir                   string
	externalCA                  bool
	client                      clientset.Interface
	outputWriter                io.Writer
	uploadCerts                 bool
	skipCertificateKeyPrint     bool
	kustomizeDir                string
	cniFileName                 string
	isConvertOpenYurtCluster    bool
	openyurtImageRegistry       string
	openyurtVersion             string
	openyurtTunnelServerAddress string
}

// NewCmdInit returns "kubeadm init" command.
// NB. initOptions is exposed as parameter for allowing unit testing of
//     the newInitOptions method, that implements all the command options validation logic
func NewCmdInit(out io.Writer, initOptions *initOptions) *cobra.Command {
	if initOptions == nil {
		initOptions = newInitOptions()
	}
	initRunner := workflow.NewRunner()

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Run this command in order to set up the Kubernetes control plane",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := initRunner.InitData(args)
			if err != nil {
				return err
			}

			data := c.(*initData)
			fmt.Printf("[init] Using Kubernetes version: %s\n", data.cfg.KubernetesVersion)

			if err := initRunner.Run(args); err != nil {
				return err
			}

			return showJoinCommand(data, out)
		},
		Args: cobra.NoArgs,
	}

	// adds flags to the init command
	// init command local flags could be eventually inherited by the sub-commands automatically generated for phases
	AddInitConfigFlags(cmd.Flags(), initOptions.externalInitCfg)
	AddClusterConfigFlags(cmd.Flags(), initOptions.externalClusterCfg, &initOptions.featureGatesString)
	AddInitOtherFlags(cmd.Flags(), initOptions)
	initOptions.bto.AddTokenFlag(cmd.Flags())
	initOptions.bto.AddTTLFlag(cmd.Flags())
	options.AddImageMetaFlags(cmd.Flags(), &initOptions.externalClusterCfg.ImageRepository)

	// defines additional flag that are not used by the init command but that could be eventually used
	// by the sub-commands automatically generated for phases
	initRunner.SetAdditionalFlags(func(flags *flag.FlagSet) {
		options.AddKubeConfigFlag(flags, &initOptions.kubeconfigPath)
		options.AddKubeConfigDirFlag(flags, &initOptions.kubeconfigDir)
		options.AddControlPlanExtraArgsFlags(flags, &initOptions.externalClusterCfg.APIServer.ExtraArgs, &initOptions.externalClusterCfg.ControllerManager.ExtraArgs, &initOptions.externalClusterCfg.Scheduler.ExtraArgs)
	})

	// initialize the workflow runner with the list of phases
	initRunner.AppendPhase(yurtphase.NewPreparePhase())
	initRunner.AppendPhase(kubephases.NewPreflightPhase())
	initRunner.AppendPhase(kubephases.NewKubeletStartPhase())
	initRunner.AppendPhase(kubephases.NewCertsPhase())
	initRunner.AppendPhase(kubephases.NewKubeConfigPhase())
	initRunner.AppendPhase(yurtphase.NewAllInAoneControlPlanePhase())
	initRunner.AppendPhase(kubephases.NewWaitControlPlanePhase())
	initRunner.AppendPhase(kubephases.NewUploadConfigPhase())
	initRunner.AppendPhase(yurtphase.NewMarkCloudNode())
	initRunner.AppendPhase(kubephases.NewBootstrapTokenPhase())
	initRunner.AppendPhase(kubephases.NewKubeletFinalizePhase())
	initRunner.AppendPhase(kubephases.NewAddonPhase())
	initRunner.AppendPhase(yurtphase.NewInstallCNIPhase())
	initRunner.AppendPhase(yurtphase.NewInstallYurtAddonsPhase())

	// sets the data builder function, that will be used by the runner
	// both when running the entire workflow or single phases
	initRunner.SetDataInitializer(func(cmd *cobra.Command, args []string) (workflow.RunData, error) {
		return newInitData(cmd, args, initOptions, out)
	})

	// binds the Runner to kubeadm init command by altering
	// command help, adding --skip-phases flag and by adding phases subcommands
	initRunner.BindToCommand(cmd)

	return cmd
}

// AddInitConfigFlags adds init flags bound to the config to the specified flagset
func AddInitConfigFlags(flagSet *flag.FlagSet, cfg *kubeadmapiv1beta2.InitConfiguration) {
	flagSet.StringVar(
		&cfg.LocalAPIEndpoint.AdvertiseAddress, options.APIServerAdvertiseAddress, cfg.LocalAPIEndpoint.AdvertiseAddress,
		"The IP address the API Server will advertise it's listening on. If not set the default network interface will be used.",
	)
	flagSet.Int32Var(
		&cfg.LocalAPIEndpoint.BindPort, options.APIServerBindPort, cfg.LocalAPIEndpoint.BindPort,
		"Port for the API Server to bind to.",
	)
	cmdutil.AddCRISocketFlag(flagSet, &cfg.NodeRegistration.CRISocket)
}

// AddClusterConfigFlags adds cluster flags bound to the config to the specified flagset
func AddClusterConfigFlags(flagSet *flag.FlagSet, cfg *kubeadmapiv1beta2.ClusterConfiguration, featureGatesString *string) {
	flagSet.StringVar(
		&cfg.Networking.ServiceSubnet, options.NetworkingServiceSubnet, cfg.Networking.ServiceSubnet,
		"Use alternative range of IP address for service VIPs.",
	)
	flagSet.StringVar(
		&cfg.Networking.PodSubnet, options.NetworkingPodSubnet, cfg.Networking.PodSubnet,
		"Specify range of IP addresses for the pod network. If set, the control plane will automatically allocate CIDRs for every node.",
	)
	flagSet.StringVar(
		&cfg.Networking.DNSDomain, options.NetworkingDNSDomain, cfg.Networking.DNSDomain,
		`Use alternative domain for services, e.g. "myorg.internal".`,
	)
	flagSet.StringVar(
		&cfg.ControlPlaneEndpoint, options.ControlPlaneEndpoint, cfg.ControlPlaneEndpoint,
		`Specify a stable IP address or DNS name for the control plane.`,
	)
	options.AddKubernetesVersionFlag(flagSet, &cfg.KubernetesVersion)
	flagSet.StringSliceVar(
		&cfg.APIServer.CertSANs, options.APIServerCertSANs, cfg.APIServer.CertSANs,
		`Optional extra Subject Alternative Names (SANs) to use for the API Server serving certificate. Can be both IP addresses and DNS names.`,
	)
	options.AddFeatureGatesStringFlag(flagSet, featureGatesString)
}

// AddInitOtherFlags adds init flags that are not bound to a configuration file to the given flagset
// Note: All flags that are not bound to the cfg object should be allowed in cmd/kubeadm/app/apis/kubeadm/validation/validation.go
func AddInitOtherFlags(flagSet *flag.FlagSet, initOptions *initOptions) {
	options.AddConfigFlag(flagSet, &initOptions.cfgPath)
	flagSet.StringSliceVar(
		&initOptions.ignorePreflightErrors, options.IgnorePreflightErrors, initOptions.ignorePreflightErrors,
		"A list of checks whose errors will be shown as warnings. Example: 'IsPrivilegedUser,Swap'. Value 'all' ignores errors from all checks.",
	)
	flagSet.StringVar(&initOptions.openyurtImageRegistry, "yurt-image-registry", "",
		"Choose a container registry to pull OpenYurt component images from")
	flagSet.StringVar(&initOptions.openyurtVersion, "yurt-version", "",
		"Choose a specific OpenYurt version")
	flagSet.StringVar(&initOptions.openyurtTunnelServerAddress, "yurt-tunnel-server-address", "",
		"Choose an accessible address for tunnelAgent when deployed in an isolated network")
	flagSet.StringVar(&initOptions.installCNIFile, "install-cni-file", "",
		"Configure install cni yaml file.")
	flagSet.BoolVar(
		&initOptions.isConvertOpenYurtCluster, "is-convert-openyurt", true, "Convert kubernetes cluster to OpenYurt cluster.")
	options.AddKustomizePodsFlag(flagSet, &initOptions.kustomizeDir)
}

// newInitOptions returns a struct ready for being used for creating cmd init flags.
func newInitOptions() *initOptions {
	// initialize the public kubeadm config API by applying defaults
	externalInitCfg := &kubeadmapiv1beta2.InitConfiguration{}
	kubeadmscheme.Scheme.Default(externalInitCfg)

	externalClusterCfg := &kubeadmapiv1beta2.ClusterConfiguration{}
	kubeadmscheme.Scheme.Default(externalClusterCfg)

	// Create the options object for the bootstrap token-related flags, and override the default value for .Description
	bto := options.NewBootstrapTokenOptions()
	bto.Description = "The default bootstrap token generated by 'kubeadm init'."

	return &initOptions{
		externalInitCfg:    externalInitCfg,
		externalClusterCfg: externalClusterCfg,
		bto:                bto,
		kubeconfigDir:      kubeadmconstants.KubernetesDir,
		kubeconfigPath:     kubeadmconstants.GetAdminKubeConfigPath(),
	}
}

// newInitData returns a new initData struct to be used for the execution of the kubeadm init workflow.
// This func takes care of validating initOptions passed to the command, and then it converts
// options into the internal InitConfiguration type that is used as input all the phases in the kubeadm init workflow
func newInitData(cmd *cobra.Command, args []string, options *initOptions, out io.Writer) (*initData, error) {
	// Re-apply defaults to the public kubeadm API (this will set only values not exposed/not set as a flags)
	kubeadmscheme.Scheme.Default(options.externalInitCfg)
	kubeadmscheme.Scheme.Default(options.externalClusterCfg)

	// Validate standalone flags values and/or combination of flags and then assigns
	// validated values to the public kubeadm config API when applicable
	var err error
	if options.externalClusterCfg.FeatureGates, err = features.NewFeatureGate(&features.InitFeatureGates, options.featureGatesString); err != nil {
		return nil, err
	}

	if err = validation.ValidateMixedArguments(cmd.Flags()); err != nil {
		return nil, err
	}

	if err = options.bto.ApplyTo(options.externalInitCfg); err != nil {
		return nil, err
	}

	// Either use the config file if specified, or convert public kubeadm API to the internal InitConfiguration
	// and validates InitConfiguration
	cfg, err := configutil.LoadOrDefaultInitConfiguration(options.cfgPath, options.externalInitCfg, options.externalClusterCfg)
	if err != nil {
		return nil, err
	}

	ignorePreflightErrorsSet, err := validation.ValidateIgnorePreflightErrors(options.ignorePreflightErrors, cfg.NodeRegistration.IgnorePreflightErrors)
	if err != nil {
		return nil, err
	}
	// Also set the union of pre-flight errors to InitConfiguration, to provide a consistent view of the runtime configuration:
	cfg.NodeRegistration.IgnorePreflightErrors = ignorePreflightErrorsSet.List()

	// override node name and CRI socket from the command line options
	if options.externalInitCfg.NodeRegistration.Name != "" {
		cfg.NodeRegistration.Name = options.externalInitCfg.NodeRegistration.Name
	}
	if options.externalInitCfg.NodeRegistration.CRISocket != "" {
		cfg.NodeRegistration.CRISocket = options.externalInitCfg.NodeRegistration.CRISocket
	}

	if err := configutil.VerifyAPIServerBindAddress(cfg.LocalAPIEndpoint.AdvertiseAddress); err != nil {
		return nil, err
	}
	if err := features.ValidateVersion(features.InitFeatureGates, cfg.FeatureGates, cfg.KubernetesVersion); err != nil {
		return nil, err
	}

	// Checks if an external CA is provided by the user (when the CA Cert is present but the CA Key is not)
	externalCA, err := certsphase.UsingExternalCA(&cfg.ClusterConfiguration)
	if externalCA {
		// In case the certificates signed by CA (that should be provided by the user) are missing or invalid,
		// returns, because kubeadm can't regenerate them without the CA Key
		if err != nil {
			return nil, errors.Wrapf(err, "invalid or incomplete external CA")
		}

		// Validate that also the required kubeconfig files exists and are invalid, because
		// kubeadm can't regenerate them without the CA Key
		kubeconfigDir := options.kubeconfigDir
		if err := kubeconfigphase.ValidateKubeconfigsForExternalCA(kubeconfigDir, cfg); err != nil {
			return nil, err
		}
	}

	// Checks if an external Front-Proxy CA is provided by the user (when the Front-Proxy CA Cert is present but the Front-Proxy CA Key is not)
	externalFrontProxyCA, err := certsphase.UsingExternalFrontProxyCA(&cfg.ClusterConfiguration)
	if externalFrontProxyCA {
		// In case the certificates signed by Front-Proxy CA (that should be provided by the user) are missing or invalid,
		// returns, because kubeadm can't regenerate them without the Front-Proxy CA Key
		if err != nil {
			return nil, errors.Wrapf(err, "invalid or incomplete external front-proxy CA")
		}
	}

	if options.openyurtTunnelServerAddress != "" {
		_, _, err = net.SplitHostPort(options.openyurtTunnelServerAddress)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid yurt tunnel server address")
		}
	}

	return &initData{
		cfg:                         cfg,
		certificatesDir:             cfg.CertificatesDir,
		skipTokenPrint:              false,
		kubeconfigDir:               options.kubeconfigDir,
		kubeconfigPath:              options.kubeconfigPath,
		ignorePreflightErrors:       ignorePreflightErrorsSet,
		externalCA:                  externalCA,
		outputWriter:                out,
		uploadCerts:                 false,
		skipCertificateKeyPrint:     false,
		kustomizeDir:                options.kustomizeDir,
		isConvertOpenYurtCluster:    options.isConvertOpenYurtCluster,
		openyurtVersion:             options.openyurtVersion,
		cniFileName:                 options.installCNIFile,
		openyurtImageRegistry:       options.openyurtImageRegistry,
		openyurtTunnelServerAddress: options.openyurtTunnelServerAddress,
	}, nil
}

// UploadCerts returns Uploadcerts flag.
func (d *initData) UploadCerts() bool {
	return d.uploadCerts
}

// CertificateKey returns the key used to encrypt the certs.
func (d *initData) CertificateKey() string {
	return d.cfg.CertificateKey
}

// SetCertificateKey set the key used to encrypt the certs.
func (d *initData) SetCertificateKey(key string) {
	d.cfg.CertificateKey = key
}

// SkipCertificateKeyPrint returns the skipCertificateKeyPrint flag.
func (d *initData) SkipCertificateKeyPrint() bool {
	return d.skipCertificateKeyPrint
}

// Cfg returns initConfiguration.
func (d *initData) Cfg() *kubeadmapi.InitConfiguration {
	return d.cfg
}

// DryRun returns the DryRun flag.
func (d *initData) DryRun() bool {
	return d.dryRun
}

// SkipTokenPrint returns the SkipTokenPrint flag.
func (d *initData) SkipTokenPrint() bool {
	return d.skipTokenPrint
}

// IgnorePreflightErrors returns the IgnorePreflightErrors flag.
func (d *initData) IgnorePreflightErrors() sets.String {
	return d.ignorePreflightErrors
}

// CertificateWriteDir returns the path to the certificate folder or the temporary folder path in case of DryRun.
func (d *initData) CertificateWriteDir() string {
	if d.dryRun {
		return d.dryRunDir
	}
	return d.certificatesDir
}

// CertificateDir returns the CertificateDir as originally specified by the user.
func (d *initData) CertificateDir() string {
	return d.certificatesDir
}

// KubeConfigDir returns the path of the Kubernetes configuration folder or the temporary folder path in case of DryRun.
func (d *initData) KubeConfigDir() string {
	if d.dryRun {
		return d.dryRunDir
	}
	return d.kubeconfigDir
}

// KubeConfigPath returns the path to the kubeconfig file to use for connecting to Kubernetes
func (d *initData) KubeConfigPath() string {
	if d.dryRun {
		d.kubeconfigPath = filepath.Join(d.dryRunDir, kubeadmconstants.AdminKubeConfigFileName)
	}
	return d.kubeconfigPath
}

// ManifestDir returns the path where manifest should be stored or the temporary folder path in case of DryRun.
func (d *initData) ManifestDir() string {
	if d.dryRun {
		return d.dryRunDir
	}
	return kubeadmconstants.GetStaticPodDirectory()
}

// KubeletDir returns path of the kubelet configuration folder or the temporary folder in case of DryRun.
func (d *initData) KubeletDir() string {
	if d.dryRun {
		return d.dryRunDir
	}
	return kubeadmconstants.KubeletRunDirectory
}

// ExternalCA returns true if an external CA is provided by the user.
func (d *initData) ExternalCA() bool {
	return d.externalCA
}

// OutputWriter returns the io.Writer used to write output to by this command.
func (d *initData) OutputWriter() io.Writer {
	return d.outputWriter
}

//IsConvertYurtCluster return whether to convert to OpenYurt cluster.
func (d *initData) IsConvertYurtCluster() bool {
	return d.isConvertOpenYurtCluster
}

//OpenYurtImageRegistry return the image registry to install OpenYurt component.
func (d *initData) OpenYurtImageRegistry() string {
	return d.openyurtImageRegistry
}

//OpenYurtVersion return the OpenYurt version.
func (d *initData) OpenYurtVersion() string {
	return d.openyurtVersion
}

//CNIFileName return the cni install yaml.
func (d *initData) CNIFileName() string {
	return d.cniFileName
}

// YurtTunnelAddress return the openyurtTunnelServerAddress
func (d *initData) YurtTunnelAddress() string {
	return d.openyurtTunnelServerAddress
}

// Client returns a Kubernetes client to be used by kubeadm.
// This function is implemented as a singleton, thus avoiding to recreate the client when it is used by different phases.
// Important. This function must be called after the admin.conf kubeconfig file is created.
func (d *initData) Client() (clientset.Interface, error) {
	if d.client == nil {
		if d.dryRun {
			svcSubnetCIDR, err := kubeadmconstants.GetKubernetesServiceCIDR(d.cfg.Networking.ServiceSubnet, features.Enabled(d.cfg.FeatureGates, features.IPv6DualStack))
			if err != nil {
				return nil, errors.Wrapf(err, "unable to get internal Kubernetes Service IP from the given service CIDR (%s)", d.cfg.Networking.ServiceSubnet)
			}
			// If we're dry-running, we should create a faked client that answers some GETs in order to be able to do the full init flow and just logs the rest of requests
			dryRunGetter := apiclient.NewInitDryRunGetter(d.cfg.NodeRegistration.Name, svcSubnetCIDR.String())
			d.client = apiclient.NewDryRunClient(dryRunGetter, os.Stdout)
		} else {
			// If we're acting for real, we should create a connection to the API server and wait for it to come up
			var err error
			d.client, err = kubeconfigutil.ClientSetFromFile(d.KubeConfigPath())
			if err != nil {
				return nil, err
			}
		}
	}
	return d.client, nil
}

// Tokens returns an array of token strings.
func (d *initData) Tokens() []string {
	tokens := []string{}
	for _, bt := range d.cfg.BootstrapTokens {
		tokens = append(tokens, bt.Token.String())
	}
	return tokens
}

// KustomizeDir returns the folder where kustomize patches for static pod manifest are stored
func (d *initData) KustomizeDir() string {
	return d.kustomizeDir
}

func printJoinCommand(out io.Writer, adminKubeConfigPath, token string, i *initData) error {
	joinEdgeNodeCommand, err := getJoinCommand(adminKubeConfigPath, token, "edge-node")
	if err != nil {
		return err
	}
	joinCloudNodeCommand, err := getJoinCommand(adminKubeConfigPath, token, "cloud-node")
	if err != nil {
		return err
	}

	ctx := map[string]interface{}{
		"KubeConfigPath":       adminKubeConfigPath,
		"ControlPlaneEndpoint": i.Cfg().ControlPlaneEndpoint,
		"joinEdgeNodeCommand":  joinEdgeNodeCommand,
		"joinCloudNodeCommand": joinCloudNodeCommand,
	}

	return initDoneTempl.Execute(out, ctx)
}

// showJoinCommand prints the join command after all the phases in init have finished
func showJoinCommand(i *initData, out io.Writer) error {
	adminKubeConfigPath := i.KubeConfigPath()

	// Prints the join command, multiple times in case the user has multiple tokens
	for _, token := range i.Tokens() {
		if err := printJoinCommand(out, adminKubeConfigPath, token, i); err != nil {
			return errors.Wrap(err, "failed to print join command")
		}
	}

	return nil
}

//getJoinCommand returns the yurtctl join command for a given token and nodeType.
func getJoinCommand(kubeConfigFile, token, nodeType string) (string, error) {
	// load the kubeconfig file to get the CA certificate and endpoint
	config, err := clientcmd.LoadFromFile(kubeConfigFile)
	if err != nil {
		return "", errors.Wrap(err, "failed to load kubeconfig")
	}

	// load the default cluster config
	clusterConfig := kubeconfigutil.GetClusterFromKubeConfig(config)
	if clusterConfig == nil {
		return "", errors.New("failed to get default cluster config")
	}

	// load CA certificates from the kubeconfig (either from PEM data or by file path)
	var caCerts []*x509.Certificate
	if clusterConfig.CertificateAuthorityData != nil {
		caCerts, err = clientcertutil.ParseCertsPEM(clusterConfig.CertificateAuthorityData)
		if err != nil {
			return "", errors.Wrap(err, "failed to parse CA certificate from kubeconfig")
		}
	} else if clusterConfig.CertificateAuthority != "" {
		caCerts, err = clientcertutil.CertsFromFile(clusterConfig.CertificateAuthority)
		if err != nil {
			return "", errors.Wrap(err, "failed to load CA certificate referenced by kubeconfig")
		}
	} else {
		return "", errors.New("no CA certificates found in kubeconfig")
	}

	// hash all the CA certs and include their public key pins as trusted values
	publicKeyPins := make([]string, 0, len(caCerts))
	for _, caCert := range caCerts {
		publicKeyPins = append(publicKeyPins, pubkeypin.Hash(caCert))
	}

	ctx := map[string]interface{}{
		"Token":                token,
		"CAPubKeyPins":         publicKeyPins,
		"ControlPlaneHostPort": strings.Replace(clusterConfig.Server, "https://", "", -1),
		"NodeType":             nodeType,
	}

	var out bytes.Buffer
	err = joinCommandTemplate.Execute(&out, ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to render join command template")
	}
	return out.String(), nil
}
