/*
Copyright 2022 The OpenYurt Authors.

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
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/openyurtio/openyurt/pkg/projectinfo"
	"github.com/openyurtio/openyurt/pkg/yurthub/storage/disk"
	"github.com/openyurtio/openyurt/pkg/yurthub/util"
)

func TestNewYurtHubOptions(t *testing.T) {
	expectOptions := YurtHubOptions{
		YurtHubHost:               "127.0.0.1",
		YurtHubProxyHost:          "127.0.0.1",
		YurtHubProxyPort:          util.YurtHubProxyPort,
		YurtHubPort:               util.YurtHubPort,
		YurtHubProxySecurePort:    util.YurtHubProxySecurePort,
		GCFrequency:               120,
		YurtHubCertOrganizations:  make([]string, 0),
		LBMode:                    "rr",
		HeartbeatFailedRetry:      3,
		HeartbeatHealthyThreshold: 2,
		HeartbeatTimeoutSeconds:   2,
		HeartbeatIntervalSeconds:  10,
		MaxRequestInFlight:        250,
		RootDir:                   filepath.Join("/var/lib/", projectinfo.GetHubName()),
		EnableProfiling:           true,
		EnableDummyIf:             true,
		EnableIptables:            true,
		HubAgentDummyIfName:       fmt.Sprintf("%s-dummy0", projectinfo.GetHubName()),
		DiskCachePath:             disk.CacheBaseDir,
		AccessServerThroughHub:    true,
		EnableResourceFilter:      true,
		DisabledResourceFilters:   make([]string, 0),
		WorkingMode:               string(util.WorkingModeEdge),
		KubeletHealthGracePeriod:  time.Second * 40,
		EnableNodePool:            true,
		MinRequestTimeout:         time.Second * 1800,
		CACertHashes:              make([]string, 0),
		UnsafeSkipCAVerification:  true,
	}

	options := NewYurtHubOptions()
	options.AddFlags((&cobra.Command{}).Flags())
	if !reflect.DeepEqual(expectOptions, *options) {
		t.Errorf("expect options: %#+v, but got %#+v", expectOptions, *options)
	}
}

func TestValidate(t *testing.T) {
	testcases := map[string]struct {
		options *YurtHubOptions
		isErr   bool
	}{
		"don't set node name": {
			options: NewYurtHubOptions(),
			isErr:   true,
		},
		"don't set server addr": {
			options: &YurtHubOptions{
				NodeName: "foo",
			},
			isErr: true,
		},
		"don't set join token": {
			options: &YurtHubOptions{
				NodeName:   "foo",
				ServerAddr: "1.2.3.4:56",
			},
			isErr: true,
		},
		"invalid lb mode": {
			options: &YurtHubOptions{
				NodeName:   "foo",
				ServerAddr: "1.2.3.4:56",
				JoinToken:  "xxxx",
				LBMode:     "invalid mode",
			},
			isErr: true,
		},
		"invalid working mode": {
			options: &YurtHubOptions{
				NodeName:    "foo",
				ServerAddr:  "1.2.3.4:56",
				JoinToken:   "xxxx",
				LBMode:      "rr",
				WorkingMode: "invalid mode",
			},
			isErr: true,
		},
		"invalid dummy ip": {
			options: &YurtHubOptions{
				NodeName:          "foo",
				ServerAddr:        "1.2.3.4:56",
				JoinToken:         "xxxx",
				LBMode:            "rr",
				WorkingMode:       "cloud",
				HubAgentDummyIfIP: "invalid ip",
			},
			isErr: true,
		},
		"not in dummy cidr": {
			options: &YurtHubOptions{
				NodeName:          "foo",
				ServerAddr:        "1.2.3.4:56",
				JoinToken:         "xxxx",
				LBMode:            "rr",
				WorkingMode:       "cloud",
				HubAgentDummyIfIP: "169.250.0.0",
			},
			isErr: true,
		},
		"in exclusive cidr": {
			options: &YurtHubOptions{
				NodeName:          "foo",
				ServerAddr:        "1.2.3.4:56",
				JoinToken:         "xxxx",
				LBMode:            "rr",
				WorkingMode:       "cloud",
				HubAgentDummyIfIP: "169.254.31.1",
			},
			isErr: true,
		},
		"ip is 169.254.1.1": {
			options: &YurtHubOptions{
				NodeName:          "foo",
				ServerAddr:        "1.2.3.4:56",
				JoinToken:         "xxxx",
				LBMode:            "rr",
				WorkingMode:       "cloud",
				HubAgentDummyIfIP: "169.254.1.1",
			},
			isErr: true,
		},
		"no ca cert hashes": {
			options: &YurtHubOptions{
				NodeName:                 "foo",
				ServerAddr:               "1.2.3.4:56",
				JoinToken:                "xxxx",
				LBMode:                   "rr",
				WorkingMode:              "cloud",
				UnsafeSkipCAVerification: false,
			},
			isErr: true,
		},
		"normal options": {
			options: &YurtHubOptions{
				NodeName:                 "foo",
				ServerAddr:               "1.2.3.4:56",
				JoinToken:                "xxxx",
				LBMode:                   "rr",
				WorkingMode:              "cloud",
				UnsafeSkipCAVerification: true,
			},
			isErr: false,
		},
		"normal options with ipv4": {
			options: &YurtHubOptions{
				NodeName:                 "foo",
				ServerAddr:               "1.2.3.4:56",
				JoinToken:                "xxxx",
				LBMode:                   "rr",
				WorkingMode:              "cloud",
				UnsafeSkipCAVerification: true,
				HubAgentDummyIfIP:        "fd00::2:1",
			},
			isErr: false,
		},
		"normal options with ipv6": {
			options: &YurtHubOptions{
				NodeName:                 "foo",
				ServerAddr:               "1.2.3.4:56",
				JoinToken:                "xxxx",
				LBMode:                   "rr",
				WorkingMode:              "cloud",
				UnsafeSkipCAVerification: true,
				HubAgentDummyIfIP:        "169.254.2.1",
			},
			isErr: false,
		},
	}

	for k, tc := range testcases {
		t.Run(k, func(t *testing.T) {
			err := tc.options.Validate()
			if tc.isErr && err == nil {
				t.Errorf("expect return err, but got nil")
			} else if !tc.isErr && err != nil {
				t.Errorf("expect return nil, but got %v", err)
			}
		})
	}
}
