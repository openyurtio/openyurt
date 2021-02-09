package options

import (
	"fmt"
	"path/filepath"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
	"github.com/spf13/pflag"
)

// YurtHubOptions is the main settings for the yurthub
type YurtHubOptions struct {
	ServerAddr                string
	YurtHubHost               string
	YurtHubPort               int
	GCFrequency               int
	CertMgrMode               string
	NodeName                  string
	LBMode                    string
	HeartbeatFailedRetry      int
	HeartbeatHealthyThreshold int
	HeartbeatTimeoutSeconds   int
	MaxRequestInFlight        int
	JoinToken                 string
	RootDir                   string
	Version                   bool
}

// NewYurtHubOptions creates a new YurtHubOptions with a default config.
func NewYurtHubOptions() *YurtHubOptions {
	o := &YurtHubOptions{
		YurtHubHost:               "127.0.0.1",
		YurtHubPort:               10261,
		GCFrequency:               120,
		CertMgrMode:               "hubself",
		LBMode:                    "rr",
		HeartbeatFailedRetry:      3,
		HeartbeatHealthyThreshold: 2,
		HeartbeatTimeoutSeconds:   2,
		MaxRequestInFlight:        250,
		RootDir:                   filepath.Join("/var/lib/", projectinfo.GetHubName()),
	}

	return o
}

// ValidateOptions validates YurtHubOptions
func ValidateOptions(options *YurtHubOptions) error {
	if len(options.NodeName) == 0 {
		return fmt.Errorf("node name is empty")
	}

	if len(options.ServerAddr) == 0 {
		return fmt.Errorf("server-address is empty")
	}

	if !util.IsSupportedLBMode(options.LBMode) {
		return fmt.Errorf("lb mode(%s) is not supported", options.LBMode)
	}

	if !util.IsSupportedCertMode(options.CertMgrMode) {
		return fmt.Errorf("cert manage mode %s is not supported", options.CertMgrMode)
	}

	return nil
}

// AddFlags returns flags for a specific yurthub by section name
func (o *YurtHubOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.YurtHubHost, "bind-address", o.YurtHubHost, "the IP address on which to listen for the --serve-port port.")
	fs.IntVar(&o.YurtHubPort, "serve-port", o.YurtHubPort, "the port on which to serve HTTP.")
	fs.StringVar(&o.ServerAddr, "server-addr", o.ServerAddr, "the address of Kubernetes kube-apiserver,the format is: \"server1,server2,...\"")
	fs.StringVar(&o.CertMgrMode, "cert-mgr-mode", o.CertMgrMode, "the cert manager mode, kubelet: use certificates that belongs to kubelet, hubself: auto generate client cert for hub agent.")
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
}
