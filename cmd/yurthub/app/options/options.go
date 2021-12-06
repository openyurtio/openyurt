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
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter"
	"github.com/openyurtio/openyurt/pkg/yurthub/filter/register"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

const (
	DummyIfCIDR   = "169.254.0.0/16"
	ExclusiveCIDR = "169.254.31.0/24"
)

// YurtHubOptions is the main settings for the yurthub
type YurtHubOptions struct {
	ServerAddr                   string
	YurtHubHost                  string
	YurtHubPort                  string
	YurtHubProxyPort             string
	YurtHubProxySecurePort       string
	GCFrequency                  int
	CertMgrMode                  string
	YurtHubCertOrganizations     string
	KubeletRootCAFilePath        string
	KubeletPairFilePath          string
	NodeName                     string
	NodePoolName                 string
	LBMode                       string
	HeartbeatFailedRetry         int
	HeartbeatHealthyThreshold    int
	HeartbeatTimeoutSeconds      int
	MaxRequestInFlight           int
	JoinToken                    string
	RootDir                      string
	Version                      bool
	EnableProfiling              bool
	EnableDummyIf                bool
	EnableIptables               bool
	HubAgentDummyIfIP            string
	HubAgentDummyIfName          string
	DiskCachePath                string
	AccessServerThroughHub       bool
	EnableResourceFilter         bool
	DisabledResourceFilters      []string
	WorkingMode                  string
	KubeletHealthGracePeriod     time.Duration
	Filters                      *filter.Filters
	ServiceTopologyFilterEnabled bool
}

// NewYurtHubOptions creates a new YurtHubOptions with a default config.
func NewYurtHubOptions() *YurtHubOptions {
	o := &YurtHubOptions{
		YurtHubHost:               "127.0.0.1",
		YurtHubProxyPort:          "10261",
		YurtHubPort:               "10267",
		YurtHubProxySecurePort:    "10268",
		GCFrequency:               120,
		CertMgrMode:               util.YurtHubCertificateManagerName,
		KubeletRootCAFilePath:     util.DefaultKubeletRootCAFilePath,
		KubeletPairFilePath:       util.DefaultKubeletPairFilePath,
		LBMode:                    "rr",
		HeartbeatFailedRetry:      3,
		HeartbeatHealthyThreshold: 2,
		HeartbeatTimeoutSeconds:   2,
		MaxRequestInFlight:        250,
		RootDir:                   filepath.Join("/var/lib/", projectinfo.GetHubName()),
		EnableProfiling:           true,
		EnableDummyIf:             true,
		EnableIptables:            true,
		HubAgentDummyIfIP:         "169.254.2.1",
		HubAgentDummyIfName:       fmt.Sprintf("%s-dummy0", projectinfo.GetHubName()),
		DiskCachePath:             disk.CacheBaseDir,
		AccessServerThroughHub:    true,
		EnableResourceFilter:      true,
		DisabledResourceFilters:   make([]string, 0),
		WorkingMode:               string(util.WorkingModeEdge),
		KubeletHealthGracePeriod:  time.Second * 40,

		ServiceTopologyFilterEnabled: false,
		Filters:                      nil,
	}
	return o
}

// ValidateOptions validates YurtHubOptions
func (o *YurtHubOptions) ValidateOptions() error {
	if len(o.NodeName) == 0 {
		return fmt.Errorf("node name is empty")
	}

	if len(o.ServerAddr) == 0 {
		return fmt.Errorf("server-address is empty")
	}

	if !util.IsSupportedLBMode(o.LBMode) {
		return fmt.Errorf("lb mode(%s) is not supported", o.LBMode)
	}

	if !util.IsSupportedCertMode(o.CertMgrMode) {
		return fmt.Errorf("cert manage mode %s is not supported", o.CertMgrMode)
	}

	if !util.IsSupportedWorkingMode(util.WorkingMode(o.WorkingMode)) {
		return fmt.Errorf("working mode %s is not supported", o.WorkingMode)
	}

	if err := verifyDummyIP(o.HubAgentDummyIfIP); err != nil {
		return fmt.Errorf("dummy ip %s is not invalid, %v", o.HubAgentDummyIfIP, err)
	}

	if o.EnableResourceFilter {
		invalidFilters := []string{}
		registeredFilters := sets.NewString(o.Filters.Registered()...)
		for _, name := range o.DisabledResourceFilters {
			if !registeredFilters.Has(name) && name != "*" {
				invalidFilters = append(invalidFilters, name)
			}
		}
		if len(invalidFilters) != 0 {
			return fmt.Errorf("disable-resource-filters %v is unknown", invalidFilters)
		}
	}

	return nil
}

// AddFlags returns flags for a specific yurthub by section name
func (o *YurtHubOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.YurtHubHost, "bind-address", o.YurtHubHost, "the IP address on which to listen for the --serve-port port.")
	fs.StringVar(&o.YurtHubPort, "serve-port", o.YurtHubPort, "the port on which to serve HTTP requests(like profiling, metrics) for hub agent.")
	fs.StringVar(&o.YurtHubProxyPort, "proxy-port", o.YurtHubProxyPort, "the port on which to proxy HTTP requests to kube-apiserver")
	fs.StringVar(&o.YurtHubProxySecurePort, "proxy-secure-port", o.YurtHubProxySecurePort, "the port on which to proxy HTTPS requests to kube-apiserver")
	fs.StringVar(&o.ServerAddr, "server-addr", o.ServerAddr, "the address of Kubernetes kube-apiserver,the format is: \"server1,server2,...\"")
	fs.StringVar(&o.CertMgrMode, "cert-mgr-mode", o.CertMgrMode, "the cert manager mode, hubself: auto generate client cert for hub agent.")
	fs.StringVar(&o.YurtHubCertOrganizations, "hub-cert-organizations", o.YurtHubCertOrganizations, "Organizations that will be added into hub's certificate in hubself cert-mgr-mode, the format is: certOrg1,certOrg1,...")
	fs.StringVar(&o.KubeletRootCAFilePath, "kubelet-ca-file", o.KubeletRootCAFilePath, "the ca file path used by kubelet.")
	fs.StringVar(&o.KubeletPairFilePath, "kubelet-client-certificate", o.KubeletPairFilePath, "the path of kubelet client certificate file.")
	fs.IntVar(&o.GCFrequency, "gc-frequency", o.GCFrequency, "the frequency to gc cache in storage(unit: minute).")
	fs.StringVar(&o.NodeName, "node-name", o.NodeName, "the name of node that runs hub agent")
	fs.StringVar(&o.LBMode, "lb-mode", o.LBMode, "the mode of load balancer to connect remote servers(rr, priority)")
	fs.IntVar(&o.HeartbeatFailedRetry, "heartbeat-failed-retry", o.HeartbeatFailedRetry, "number of heartbeat request retry after having failed.")
	fs.IntVar(&o.HeartbeatHealthyThreshold, "heartbeat-healthy-threshold", o.HeartbeatHealthyThreshold, "minimum consecutive successes for the heartbeat to be considered healthy after having failed.")
	fs.IntVar(&o.HeartbeatTimeoutSeconds, "heartbeat-timeout-seconds", o.HeartbeatTimeoutSeconds, " number of seconds after which the heartbeat times out.")
	fs.IntVar(&o.MaxRequestInFlight, "max-requests-in-flight", o.MaxRequestInFlight, "the maximum number of parallel requests.")
	fs.StringVar(&o.JoinToken, "join-token", o.JoinToken, "the Join token for bootstrapping hub agent when --cert-mgr-mode=hubself.")
	fs.StringVar(&o.RootDir, "root-dir", o.RootDir, "directory path for managing hub agent files(pki, cache etc).")
	fs.BoolVar(&o.Version, "version", o.Version, "print the version information.")
	fs.BoolVar(&o.EnableProfiling, "profiling", o.EnableProfiling, "enable profiling via web interface host:port/debug/pprof/")
	fs.BoolVar(&o.EnableDummyIf, "enable-dummy-if", o.EnableDummyIf, "enable dummy interface or not")
	fs.BoolVar(&o.EnableIptables, "enable-iptables", o.EnableIptables, "enable iptables manager to setup rules for accessing hub agent")
	fs.StringVar(&o.HubAgentDummyIfIP, "dummy-if-ip", o.HubAgentDummyIfIP, "the ip address of dummy interface that used for container connect hub agent(exclusive ips: 169.254.31.0/24, 169.254.1.1/32)")
	fs.StringVar(&o.HubAgentDummyIfName, "dummy-if-name", o.HubAgentDummyIfName, "the name of dummy interface that is used for hub agent")
	fs.StringVar(&o.DiskCachePath, "disk-cache-path", o.DiskCachePath, "the path for kubernetes to storage metadata")
	fs.BoolVar(&o.AccessServerThroughHub, "access-server-through-hub", o.AccessServerThroughHub, "enable pods access kube-apiserver through yurthub or not")
	fs.BoolVar(&o.EnableResourceFilter, "enable-resource-filter", o.EnableResourceFilter, "enable to filter response that comes back from reverse proxy")
	fs.StringSliceVar(&o.DisabledResourceFilters, "disabled-resource-filters", o.DisabledResourceFilters, "disable resource filters to handle response")
	fs.StringVar(&o.NodePoolName, "nodepool-name", o.NodePoolName, "the name of node pool that runs hub agent")
	fs.StringVar(&o.WorkingMode, "working-mode", o.WorkingMode, "the working mode of yurthub(edge, cloud).")
	fs.DurationVar(&o.KubeletHealthGracePeriod, "kubelet-health-grace-period", o.KubeletHealthGracePeriod, "the amount of time which we allow kubelet to be unresponsive before stop renew node lease")
}

// verifyDummyIP verify the specified ip is valid or not
func verifyDummyIP(dummyIP string) error {
	//169.254.2.1/32
	dip := net.ParseIP(dummyIP)
	if dip == nil {
		return fmt.Errorf("dummy ip %s is invalid", dummyIP)
	}

	_, dummyIfIPNet, err := net.ParseCIDR(DummyIfCIDR)
	if err != nil {
		return fmt.Errorf("cidr(%s) is invalid, %v", DummyIfCIDR, err)
	}

	if !dummyIfIPNet.Contains(dip) {
		return fmt.Errorf("dummy ip %s is not in cidr(%s)", dummyIP, DummyIfCIDR)
	}

	_, exclusiveIPNet, err := net.ParseCIDR(ExclusiveCIDR)
	if err != nil {
		return fmt.Errorf("cidr(%s) is invalid, %v", ExclusiveCIDR, err)
	}

	if exclusiveIPNet.Contains(dip) {
		return fmt.Errorf("dummy ip %s is in reserved cidr(%s)", dummyIP, ExclusiveCIDR)
	}

	if dummyIP == "169.254.1.1" {
		return fmt.Errorf("dummy ip is a reserved ip(%s)", dummyIP)
	}

	return nil
}

// CompletedYurtHubOptions is a wrapper that enforces a call of Complete().
type CompletedYurtHubOptions struct {
	*YurtHubOptions
}

func Complete(o *YurtHubOptions) *CompletedYurtHubOptions {
	if o.EnableResourceFilter {
		o.Filters = filter.NewFilters()
		register.RegisterAllFilters(o.Filters)

		if o.WorkingMode == string(util.WorkingModeCloud) {
			o.DisabledResourceFilters = append(o.DisabledResourceFilters, filter.DisabledInCloudMode...)
		}

		disableFilters := sets.NewString(o.DisabledResourceFilters...)
		o.ServiceTopologyFilterEnabled = filter.Enabled(disableFilters, filter.ServiceTopologyFilterName)
	}
	return &CompletedYurtHubOptions{o}
}
