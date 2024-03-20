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

package options

import (
	"fmt"
	"net"
	"path/filepath"
	"time"

	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	componentbaseconfig "k8s.io/component-base/config"
	"k8s.io/klog/v2"
	utilnet "k8s.io/utils/net"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/certificate"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	DefaultDummyIfIP4 = "169.254.2.1"
	DefaultDummyIfIP6 = "fd00::2:1"
	DummyIfCIDR4      = "169.254.0.0/16"
	ExclusiveCIDR     = "169.254.31.0/24"
)

// YurtHubOptions is the main settings for the yurthub
type YurtHubOptions struct {
	ServerAddr                string
	YurtHubHost               string // YurtHub server host (e.g.: expose metrics API)
	YurtHubProxyHost          string // YurtHub proxy server host
	YurtHubPort               int
	YurtHubProxyPort          int
	YurtHubProxySecurePort    int
	YurtHubNamespace          string
	GCFrequency               int
	YurtHubCertOrganizations  []string
	NodeName                  string
	NodePoolName              string
	LBMode                    string
	HeartbeatFailedRetry      int
	HeartbeatHealthyThreshold int
	HeartbeatTimeoutSeconds   int
	HeartbeatIntervalSeconds  int
	MaxRequestInFlight        int
	JoinToken                 string
	BootstrapMode             string
	BootstrapFile             string
	RootDir                   string
	Version                   bool
	EnableProfiling           bool
	EnableDummyIf             bool
	EnableIptables            bool
	HubAgentDummyIfIP         string
	HubAgentDummyIfName       string
	DiskCachePath             string
	EnableResourceFilter      bool
	DisabledResourceFilters   []string
	WorkingMode               string
	KubeletHealthGracePeriod  time.Duration
	EnableNodePool            bool
	MinRequestTimeout         time.Duration
	CACertHashes              []string
	UnsafeSkipCAVerification  bool
	ClientForTest             kubernetes.Interface
	EnableCoordinator         bool
	CoordinatorServerAddr     string
	CoordinatorStoragePrefix  string
	CoordinatorStorageAddr    string
	LeaderElection            componentbaseconfig.LeaderElectionConfiguration
	EnablePoolServiceTopology bool
}

// NewYurtHubOptions creates a new YurtHubOptions with a default config.
func NewYurtHubOptions() *YurtHubOptions {
	o := &YurtHubOptions{
		YurtHubHost:               "127.0.0.1",
		YurtHubProxyHost:          "127.0.0.1",
		YurtHubProxyPort:          util.YurtHubProxyPort,
		YurtHubPort:               util.YurtHubPort,
		YurtHubProxySecurePort:    util.YurtHubProxySecurePort,
		YurtHubNamespace:          util.YurtHubNamespace,
		GCFrequency:               120,
		YurtHubCertOrganizations:  make([]string, 0),
		LBMode:                    "rr",
		HeartbeatFailedRetry:      3,
		HeartbeatHealthyThreshold: 2,
		HeartbeatTimeoutSeconds:   2,
		HeartbeatIntervalSeconds:  10,
		MaxRequestInFlight:        250,
		BootstrapMode:             certificate.TokenBoostrapMode,
		RootDir:                   filepath.Join("/var/lib/", projectinfo.GetHubName()),
		EnableProfiling:           true,
		EnableDummyIf:             true,
		EnableIptables:            false,
		HubAgentDummyIfName:       fmt.Sprintf("%s-dummy0", projectinfo.GetHubName()),
		DiskCachePath:             disk.CacheBaseDir,
		EnableResourceFilter:      true,
		DisabledResourceFilters:   make([]string, 0),
		WorkingMode:               string(util.WorkingModeEdge),
		KubeletHealthGracePeriod:  time.Second * 40,
		EnableNodePool:            true,
		MinRequestTimeout:         time.Second * 1800,
		CACertHashes:              make([]string, 0),
		UnsafeSkipCAVerification:  true,
		CoordinatorServerAddr:     fmt.Sprintf("https://%s:%s", util.DefaultYurtCoordinatorAPIServerSvcName, util.DefaultYurtCoordinatorAPIServerSvcPort),
		CoordinatorStorageAddr:    fmt.Sprintf("https://%s:%s", util.DefaultYurtCoordinatorEtcdSvcName, util.DefaultYurtCoordinatorEtcdSvcPort),
		CoordinatorStoragePrefix:  "/registry",
		LeaderElection: componentbaseconfig.LeaderElectionConfiguration{
			LeaderElect:       true,
			LeaseDuration:     metav1.Duration{Duration: 15 * time.Second},
			RenewDeadline:     metav1.Duration{Duration: 10 * time.Second},
			RetryPeriod:       metav1.Duration{Duration: 2 * time.Second},
			ResourceLock:      resourcelock.LeasesResourceLock,
			ResourceName:      projectinfo.GetHubName(),
			ResourceNamespace: "kube-system",
		},
		EnablePoolServiceTopology: false,
	}
	return o
}

// Validate validates YurtHubOptions
func (options *YurtHubOptions) Validate() error {
	if len(options.NodeName) == 0 {
		return fmt.Errorf("node name is empty")
	}

	if len(options.ServerAddr) == 0 {
		return fmt.Errorf("server-address is empty")
	}

	if options.BootstrapMode != certificate.KubeletCertificateBootstrapMode {
		if len(options.JoinToken) == 0 && len(options.BootstrapFile) == 0 {
			return fmt.Errorf("bootstrap token and bootstrap file are empty, one of them must be set")
		}
	}

	if !util.IsSupportedLBMode(options.LBMode) {
		return fmt.Errorf("lb mode(%s) is not supported", options.LBMode)
	}

	if !util.IsSupportedWorkingMode(util.WorkingMode(options.WorkingMode)) {
		return fmt.Errorf("working mode %s is not supported", options.WorkingMode)
	}

	if err := options.verifyDummyIP(); err != nil {
		return fmt.Errorf("dummy ip %s is not invalid, %w", options.HubAgentDummyIfIP, err)
	}

	if len(options.HubAgentDummyIfName) > 15 {
		return fmt.Errorf("dummy name %s length should not be more than 15", options.HubAgentDummyIfName)
	}

	if len(options.CACertHashes) == 0 && !options.UnsafeSkipCAVerification {
		return fmt.Errorf("set --discovery-token-unsafe-skip-ca-verification flag as true or pass CACertHashes to continue")
	}

	return nil
}

// AddFlags returns flags for a specific yurthub by section name
func (o *YurtHubOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.YurtHubHost, "bind-address", o.YurtHubHost, "the IP address of YurtHub Server")
	fs.IntVar(&o.YurtHubPort, "serve-port", o.YurtHubPort, "the port on which to serve HTTP requests(like profiling, metrics) for hub agent.")
	fs.StringVar(&o.YurtHubProxyHost, "bind-proxy-address", o.YurtHubProxyHost, "the IP address of YurtHub Proxy Server")
	fs.IntVar(&o.YurtHubProxyPort, "proxy-port", o.YurtHubProxyPort, "the port on which to proxy HTTP requests to kube-apiserver")
	fs.IntVar(&o.YurtHubProxySecurePort, "proxy-secure-port", o.YurtHubProxySecurePort, "the port on which to proxy HTTPS requests to kube-apiserver")
	fs.StringVar(&o.YurtHubNamespace, "namespace", o.YurtHubNamespace, "the namespace of YurtHub Server")
	fs.StringVar(&o.ServerAddr, "server-addr", o.ServerAddr, "the address of Kubernetes kube-apiserver,the format is: \"server1,server2,...\"")
	fs.StringSliceVar(&o.YurtHubCertOrganizations, "hub-cert-organizations", o.YurtHubCertOrganizations, "Organizations that will be added into hub's apiserver client certificate, the format is: certOrg1,certOrg2,...")
	fs.IntVar(&o.GCFrequency, "gc-frequency", o.GCFrequency, "the frequency to gc cache in storage(unit: minute).")
	fs.StringVar(&o.NodeName, "node-name", o.NodeName, "the name of node that runs hub agent")
	fs.StringVar(&o.LBMode, "lb-mode", o.LBMode, "the mode of load balancer to connect remote servers(rr, priority)")
	fs.IntVar(&o.HeartbeatFailedRetry, "heartbeat-failed-retry", o.HeartbeatFailedRetry, "number of heartbeat request retry after having failed.")
	fs.IntVar(&o.HeartbeatHealthyThreshold, "heartbeat-healthy-threshold", o.HeartbeatHealthyThreshold, "minimum consecutive successes for the heartbeat to be considered healthy after having failed.")
	fs.IntVar(&o.HeartbeatTimeoutSeconds, "heartbeat-timeout-seconds", o.HeartbeatTimeoutSeconds, " number of seconds after which the heartbeat times out.")
	fs.IntVar(&o.HeartbeatIntervalSeconds, "heartbeat-interval-seconds", o.HeartbeatIntervalSeconds, " number of seconds for omitting one time heartbeat to remote server.")
	fs.IntVar(&o.MaxRequestInFlight, "max-requests-in-flight", o.MaxRequestInFlight, "the maximum number of parallel requests.")
	fs.StringVar(&o.JoinToken, "join-token", o.JoinToken, "the Join token for bootstrapping hub agent.")
	fs.MarkDeprecated("join-token", "It is planned to be removed from OpenYurt in the version v1.5. Please use --bootstrap-file to bootstrap hub agent.")
	fs.StringVar(&o.BootstrapMode, "bootstrap-mode", o.BootstrapMode, "the mode for bootstrapping hub agent(token, kubeletcertificate).")
	fs.StringVar(&o.BootstrapFile, "bootstrap-file", o.BootstrapFile, "the bootstrap file for bootstrapping hub agent.")
	fs.StringVar(&o.RootDir, "root-dir", o.RootDir, "directory path for managing hub agent files(pki, cache etc).")
	fs.BoolVar(&o.Version, "version", o.Version, "print the version information.")
	fs.BoolVar(&o.EnableProfiling, "profiling", o.EnableProfiling, "enable profiling via web interface host:port/debug/pprof/")
	fs.BoolVar(&o.EnableDummyIf, "enable-dummy-if", o.EnableDummyIf, "enable dummy interface or not")
	fs.BoolVar(&o.EnableIptables, "enable-iptables", o.EnableIptables, "enable iptables manager to setup rules for accessing hub agent")
	fs.MarkDeprecated("enable-iptables", "It is planned to be removed from OpenYurt in the future version")
	fs.StringVar(&o.HubAgentDummyIfIP, "dummy-if-ip", o.HubAgentDummyIfIP, "the ip address of dummy interface that used for container connect hub agent(exclusive ips: 169.254.31.0/24, 169.254.1.1/32)")
	fs.StringVar(&o.HubAgentDummyIfName, "dummy-if-name", o.HubAgentDummyIfName, "the name of dummy interface that is used for hub agent")
	fs.StringVar(&o.DiskCachePath, "disk-cache-path", o.DiskCachePath, "the path for kubernetes to storage metadata")
	fs.BoolVar(&o.EnableResourceFilter, "enable-resource-filter", o.EnableResourceFilter, "enable to filter response that comes back from reverse proxy")
	fs.StringSliceVar(&o.DisabledResourceFilters, "disabled-resource-filters", o.DisabledResourceFilters, "disable resource filters to handle response")
	fs.StringVar(&o.NodePoolName, "nodepool-name", o.NodePoolName, "the name of node pool that runs hub agent")
	fs.StringVar(&o.WorkingMode, "working-mode", o.WorkingMode, "the working mode of yurthub(edge, cloud).")
	fs.DurationVar(&o.KubeletHealthGracePeriod, "kubelet-health-grace-period", o.KubeletHealthGracePeriod, "the amount of time which we allow kubelet to be unresponsive before stop renew node lease")
	fs.BoolVar(&o.EnableNodePool, "enable-node-pool", o.EnableNodePool, "enable list/watch nodepools resource or not for filters(only used for testing)")
	fs.MarkDeprecated("enable-node-pool", "It is planned to be removed from OpenYurt in the future version, please use --enable-pool-service-topology instead")
	fs.DurationVar(&o.MinRequestTimeout, "min-request-timeout", o.MinRequestTimeout, "An optional field indicating at least how long a proxy handler must keep a request open before timing it out. Currently only honored by the local watch request handler(use request parameter timeoutSeconds firstly), which picks a randomized value above this number as the connection timeout, to spread out load.")
	fs.StringSliceVar(&o.CACertHashes, "discovery-token-ca-cert-hash", o.CACertHashes, "For token-based discovery, validate that the root CA public key matches this hash (format: \"<type>:<value>\").")
	fs.BoolVar(&o.UnsafeSkipCAVerification, "discovery-token-unsafe-skip-ca-verification", o.UnsafeSkipCAVerification, "For token-based discovery, allow joining without --discovery-token-ca-cert-hash pinning.")
	fs.BoolVar(&o.EnableCoordinator, "enable-coordinator", o.EnableCoordinator, "make yurthub aware of the yurt coordinator")
	fs.StringVar(&o.CoordinatorServerAddr, "coordinator-server-addr", o.CoordinatorServerAddr, "Coordinator APIServer address in format https://host:port")
	fs.StringVar(&o.CoordinatorStoragePrefix, "coordinator-storage-prefix", o.CoordinatorStoragePrefix, "Yurt-Coordinator etcd storage prefix, same as etcd-prefix of Kube-APIServer")
	fs.StringVar(&o.CoordinatorStorageAddr, "coordinator-storage-addr", o.CoordinatorStorageAddr, "Address of Yurt-Coordinator etcd, in the format host:port")
	bindFlags(&o.LeaderElection, fs)
	fs.BoolVar(&o.EnablePoolServiceTopology, "enable-pool-service-topology", o.EnablePoolServiceTopology, "enable service topology feature in the node pool.")
}

// bindFlags binds the LeaderElectionConfiguration struct fields to a flagset
func bindFlags(l *componentbaseconfig.LeaderElectionConfiguration, fs *pflag.FlagSet) {
	fs.BoolVar(&l.LeaderElect, "leader-elect", l.LeaderElect, ""+
		"Start a leader election client and gain leadership based on yurt coordinator")
	fs.DurationVar(&l.LeaseDuration.Duration, "leader-elect-lease-duration", l.LeaseDuration.Duration, ""+
		"The duration that non-leader candidates will wait after observing a leadership "+
		"renewal until attempting to acquire leadership of a led but unrenewed leader "+
		"slot. This is effectively the maximum duration that a leader can be stopped "+
		"before it is replaced by another candidate. This is only applicable if leader "+
		"election is enabled.")
	fs.DurationVar(&l.RenewDeadline.Duration, "leader-elect-renew-deadline", l.RenewDeadline.Duration, ""+
		"The interval between attempts by the acting master to renew a leadership slot "+
		"before it stops leading. This must be less than or equal to the lease duration. "+
		"This is only applicable if leader election is enabled.")
	fs.DurationVar(&l.RetryPeriod.Duration, "leader-elect-retry-period", l.RetryPeriod.Duration, ""+
		"The duration the clients should wait between attempting acquisition and renewal "+
		"of a leadership. This is only applicable if leader election is enabled.")
	fs.StringVar(&l.ResourceLock, "leader-elect-resource-lock", l.ResourceLock, ""+
		"The type of resource object that is used for locking during "+
		"leader election. Supported options are `leases` (default), `endpoints` and `configmaps`.")
	fs.StringVar(&l.ResourceName, "leader-elect-resource-name", l.ResourceName, ""+
		"The name of resource object that is used for locking during "+
		"leader election.")
	fs.StringVar(&l.ResourceNamespace, "leader-elect-resource-namespace", l.ResourceNamespace, ""+
		"The namespace of resource object that is used for locking during "+
		"leader election.")
}

// verifyDummyIP verify the specified ip is valid or not and set the default ip if empty
func (o *YurtHubOptions) verifyDummyIP() error {
	if o.HubAgentDummyIfIP == "" {
		if utilnet.IsIPv6String(o.YurtHubHost) {
			o.HubAgentDummyIfIP = DefaultDummyIfIP6
		} else {
			o.HubAgentDummyIfIP = DefaultDummyIfIP4
		}
		klog.Infof("dummy ip not set, will use %s as default", o.HubAgentDummyIfIP)
		return nil
	}

	dummyIP := o.HubAgentDummyIfIP
	dip := net.ParseIP(dummyIP)
	if dip == nil {
		return fmt.Errorf("dummy ip %s is invalid", dummyIP)
	}

	if utilnet.IsIPv6(dip) {
		return nil
	}

	_, dummyIfIPNet, err := net.ParseCIDR(DummyIfCIDR4)
	if err != nil {
		return fmt.Errorf("cidr(%s) is invalid, %w", DummyIfCIDR4, err)
	}

	if !dummyIfIPNet.Contains(dip) {
		return fmt.Errorf("dummy ip %s is not in cidr(%s)", dummyIP, DummyIfCIDR4)
	}

	_, exclusiveIPNet, err := net.ParseCIDR(ExclusiveCIDR)
	if err != nil {
		return fmt.Errorf("cidr(%s) is invalid, %w", ExclusiveCIDR, err)
	}

	if exclusiveIPNet.Contains(dip) {
		return fmt.Errorf("dummy ip %s is in reserved cidr(%s)", dummyIP, ExclusiveCIDR)
	}

	if dummyIP == "169.254.1.1" {
		return fmt.Errorf("dummy ip is a reserved ip(%s)", dummyIP)
	}

	return nil
}
