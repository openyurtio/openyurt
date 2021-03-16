package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/openyurtio/openyurt/cmd/yurthub/app/options"
	"github.com/openyurtio/openyurt/pkg/projectinfo"

	"k8s.io/klog"
)

// YurtHubConfiguration represents configuration of yurthub
type YurtHubConfiguration struct {
	LBMode                    string
	RemoteServers             []*url.URL
	YurtHubServerAddr         string
	YurtHubProxyServerAddr    string
	GCFrequency               int
	CertMgrMode               string
	NodeName                  string
	HeartbeatFailedRetry      int
	HeartbeatHealthyThreshold int
	HeartbeatTimeoutSeconds   int
	MaxRequestInFlight        int
	JoinToken                 string
	RootDir                   string
	EnableProfiling           bool
}

// Complete converts *options.YurtHubOptions to *YurtHubConfiguration
func Complete(options *options.YurtHubOptions) (*YurtHubConfiguration, error) {
	us, err := parseRemoteServers(options.ServerAddr)
	if err != nil {
		return nil, err
	}

	hubServerAddr := net.JoinHostPort(options.YurtHubHost, options.YurtHubPort)
	proxyServerAddr := net.JoinHostPort(options.YurtHubHost, options.YurtHubProxyPort)
	cfg := &YurtHubConfiguration{
		LBMode:                    options.LBMode,
		RemoteServers:             us,
		YurtHubServerAddr:         hubServerAddr,
		YurtHubProxyServerAddr:    proxyServerAddr,
		GCFrequency:               options.GCFrequency,
		CertMgrMode:               options.CertMgrMode,
		NodeName:                  options.NodeName,
		HeartbeatFailedRetry:      options.HeartbeatFailedRetry,
		HeartbeatHealthyThreshold: options.HeartbeatHealthyThreshold,
		HeartbeatTimeoutSeconds:   options.HeartbeatTimeoutSeconds,
		MaxRequestInFlight:        options.MaxRequestInFlight,
		JoinToken:                 options.JoinToken,
		RootDir:                   options.RootDir,
		EnableProfiling:           options.EnableProfiling,
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
	klog.Infof("%s would connect remote servers: %s", projectinfo.GetHubName(), strings.Join(remoteServers, ","))

	return us, nil
}
