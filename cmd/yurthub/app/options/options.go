package options

import (
	"fmt"

	"github.com/alibaba/openyurt/pkg/yurthub/util"
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
}

// NewYurtHubOptions creates a new YurtHubOptions with a default config.
func NewYurtHubOptions() *YurtHubOptions {
	o := &YurtHubOptions{
		YurtHubHost:               "127.0.0.1",
		YurtHubPort:               10261,
		GCFrequency:               120,
		CertMgrMode:               "kubelet",
		LBMode:                    "rr",
		HeartbeatFailedRetry:      3,
		HeartbeatHealthyThreshold: 2,
		HeartbeatTimeoutSeconds:   2,
		MaxRequestInFlight:        250,
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
	fs.StringVar(&o.YurtHubHost, "yurt-hub-host", o.YurtHubHost, "the host that used to connect yurthub.")
	fs.IntVar(&o.YurtHubPort, "yurt-hub-port", o.YurtHubPort, "the port that used to connect yurthub.")
	fs.StringVar(&o.ServerAddr, "server-addr", o.ServerAddr, "the address of Kubernetes kube-apiserver,the format is: \"server1,server2,...\"")
	fs.StringVar(&o.CertMgrMode, "cert-mgr-mode", o.CertMgrMode, "the cert manager mode, kubelet: use certificates that belongs to kubelet")
	fs.IntVar(&o.GCFrequency, "gc-frequency", o.GCFrequency, "the frequency to gc cache in storage(unit: minute).")
	fs.StringVar(&o.NodeName, "node-name", o.NodeName, "the name of node that runs yurthub")
	fs.StringVar(&o.LBMode, "lb-mode", o.LBMode, "the mode of load balancer to connect remote servers(rr, priority)")
	fs.IntVar(&o.HeartbeatFailedRetry, "heartbeat-failed-retry", o.HeartbeatFailedRetry, "number of heartbeat request retry after having failed.")
	fs.IntVar(&o.HeartbeatHealthyThreshold, "heartbeat-healthy-threshold", o.HeartbeatHealthyThreshold, "minimum consecutive successes for the heartbeat to be considered healthy after having failed.")
	fs.IntVar(&o.HeartbeatTimeoutSeconds, "heartbeat-timeout-seconds", o.HeartbeatTimeoutSeconds, " number of seconds after which the heartbeat times out.")
	fs.IntVar(&o.MaxRequestInFlight, "max-requests-in-flight", o.MaxRequestInFlight, "the maximum number of parallel requests.")
}
