package config

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/alibaba/openyurt/cmd/yurthub/app/options"

	"k8s.io/klog"
)

// YurtHubConfiguration represents configuration of yurthub
type YurtHubConfiguration struct {
	LBMode                    string
	RemoteServers             []*url.URL
	YurtHubHost               string
	YurtHubPort               int
	GCFrequency               int
	CertMgrMode               string
	NodeName                  string
	HeartbeatFailedRetry      int
	HeartbeatHealthyThreshold int
	HeartbeatTimeoutSeconds   int
	MaxRequestInFlight        int
}

// Complete converts *options.YurtHubOptions to *YurtHubConfiguration
func Complete(options *options.YurtHubOptions) (*YurtHubConfiguration, error) {
	us, err := parseRemoteServers(options.ServerAddr)
	if err != nil {
		return nil, err
	}

	cfg := &YurtHubConfiguration{
		LBMode:                    options.LBMode,
		RemoteServers:             us,
		YurtHubHost:               options.YurtHubHost,
		YurtHubPort:               options.YurtHubPort,
		GCFrequency:               options.GCFrequency,
		CertMgrMode:               options.CertMgrMode,
		NodeName:                  options.NodeName,
		HeartbeatFailedRetry:      options.HeartbeatFailedRetry,
		HeartbeatHealthyThreshold: options.HeartbeatHealthyThreshold,
		HeartbeatTimeoutSeconds:   options.HeartbeatTimeoutSeconds,
		MaxRequestInFlight:        options.MaxRequestInFlight,
	}

	return cfg, nil
}

func parseRemoteServers(serverAddr string) ([]*url.URL, error) {
	servers := strings.Split(serverAddr, ",")
	us := make([]*url.URL, 0, len(servers))
	remoteServers := make([]string, 0, len(servers))
	for _, server := range servers {
		u, err := url.Parse(server)
		if err != nil {
			klog.Errorf("failed to parse server address %s, %v", servers, err)
			return us, err
		}
		if u.Scheme == "" {
			u.Scheme = "https"
		} else if u.Scheme != "https" {
			return us, fmt.Errorf("only https scheme is supported for server address(%s)", serverAddr)
		}
		us = append(us, u)
		remoteServers = append(remoteServers, u.String())
	}

	if len(us) < 1 {
		return us, fmt.Errorf("no server address is set, can not connect remote server")
	}
	klog.Infof("yurthub would connect remote servers: %s", strings.Join(remoteServers, ","))

	return us, nil
}
