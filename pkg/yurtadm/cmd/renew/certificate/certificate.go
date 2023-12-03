/*
Copyright 2023 The OpenYurt Authors.

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

package certificate

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"k8s.io/klog/v2"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	yurtconstants "github.com/openyurtio/openyurt/pkg/yurtadm/constants"
	"github.com/openyurtio/openyurt/pkg/yurtadm/util/yurthub"
)

type certificateOptions struct {
	token                    string
	caCertHashes             []string
	unsafeSkipCAVerification bool
	serverAddr               string
	yurthubServer            string
}

// newCertificateOptions returns a struct ready for being used for creating cmd renew flags.
func newCertificateOptions() *certificateOptions {
	return &certificateOptions{
		caCertHashes:             make([]string, 0),
		unsafeSkipCAVerification: true,
		yurthubServer:            yurtconstants.DefaultYurtHubServerAddr,
	}
}

// NewCmdCertificate returns "yurtadm renew certificate" command.
func NewCmdCertificate() *cobra.Command {
	o := newCertificateOptions()

	certificateCmd := &cobra.Command{
		Use:   "certificate",
		Short: "Create bootstrap file for yurthub to update certificate",
		RunE: func(certificateCmd *cobra.Command, args []string) error {
			if err := o.validate(); err != nil {
				klog.Fatalf("validate options: %v", err)
			}

			// Check if YurtHub's certificate is ready or not.
			// No update if ready.
			if ok := yurthub.CheckYurthubReadyzOnce(o.yurthubServer); ok {
				klog.Infoln("The certificate is still valid and does not need to be renewed")
				return nil
			}

			us, err := parseRemoteServers(o.serverAddr)
			if err != nil {
				return err
			}

			// 1.Create a temporary bootstrap file and set to /var/lib/yurthub directory.
			if err := yurthub.SetHubBootstrapConfig(us[0].Host, o.token, o.caCertHashes); err != nil {
				return err
			}

			// 2.Check if YurtHub's certificates is ready or not.
			if err := yurthub.CheckYurthubReadyz(o.yurthubServer); err != nil {
				return err
			}

			// 3.Delete temporary bootstrap file.
			if err := yurthub.CleanHubBootstrapConfig(); err != nil {
				return err
			}

			klog.Infoln("Certificate renewed successfully")

			return nil
		},
	}

	addCertificateConfigFlags(certificateCmd.Flags(), o)
	return certificateCmd
}

func (options *certificateOptions) validate() error {
	if len(options.serverAddr) == 0 {
		return fmt.Errorf("server-address is empty")
	}

	if len(options.token) == 0 {
		return fmt.Errorf("bootstrap token is empty")
	}

	if len(options.caCertHashes) == 0 && !options.unsafeSkipCAVerification {
		return fmt.Errorf("set --discovery-token-unsafe-skip-ca-verification flag as true or pass CACertHashes to continue")
	}

	return nil
}

func parseRemoteServers(serverAddr string) ([]*url.URL, error) {
	servers := strings.Split(serverAddr, ",")
	us := make([]*url.URL, 0, len(servers))
	remoteServers := make([]string, 0, len(servers))
	for _, server := range servers {
		u, err := url.Parse(server)
		if err != nil {
			klog.Errorf("could not parse server address %s, %v", servers, err)
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

// addCertificateConfigFlags adds certificate flags bound to the config to the specified flagset
func addCertificateConfigFlags(flagSet *flag.FlagSet, certificateOptions *certificateOptions) {
	flagSet.StringVar(
		&certificateOptions.token, yurtconstants.TokenStr, certificateOptions.token,
		"Use this token for bootstrapping yurthub.",
	)
	flagSet.StringSliceVar(
		&certificateOptions.caCertHashes, yurtconstants.TokenDiscoveryCAHash, certificateOptions.caCertHashes,
		"For token-based discovery, validate that the root CA public key matches this hash (format: \"<type>:<value>\").",
	)
	flagSet.BoolVar(
		&certificateOptions.unsafeSkipCAVerification, yurtconstants.TokenDiscoverySkipCAHash, certificateOptions.unsafeSkipCAVerification,
		"For token-based discovery, allow joining without --discovery-token-ca-cert-hash pinning.",
	)
	flagSet.StringVar(
		&certificateOptions.serverAddr, yurtconstants.ServerAddr, certificateOptions.serverAddr,
		"The address of Kubernetes kube-apiserver,the format is: \"server1,server2,...\"",
	)
	flagSet.StringVar(
		&certificateOptions.yurthubServer, yurtconstants.YurtHubServerAddr, certificateOptions.yurthubServer,
		"Sets the address for yurthub server addr",
	)
}
