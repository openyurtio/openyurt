/*
Copyright 2023 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the License);
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an AS IS BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package options

import (
	"fmt"

	"github.com/spf13/pflag"

	"github.com/openyurtio/openyurt/pkg/yurtmanager/controller/nodebucket/config"
)

type NodeBucketControllerOptions struct {
	*config.NodeBucketControllerConfiguration
}

func NewNodeBucketControllerOptions() *NodeBucketControllerOptions {
	return &NodeBucketControllerOptions{
		&config.NodeBucketControllerConfiguration{
			MaxNodesPerBucket: 100,
		},
	}
}

// AddFlags adds flags related to nodebucket for yurt-manager to the specified FlagSet.
func (n *NodeBucketControllerOptions) AddFlags(fs *pflag.FlagSet) {
	if n == nil {
		return
	}

	fs.Int32Var(&n.MaxNodesPerBucket, "max-nodes-per-bucket", n.MaxNodesPerBucket, "The maximum number of nodes that will be added to a NodeBucket. More nodes per bucket will result in less node buckets, but larger resources. Defaults to 100.")
}

// ApplyTo fills up nodebucket config with options.
func (o *NodeBucketControllerOptions) ApplyTo(cfg *config.NodeBucketControllerConfiguration) error {
	if o == nil {
		return nil
	}

	cfg.MaxNodesPerBucket = o.MaxNodesPerBucket

	return nil
}

// Validate checks validation of NodeBucketControllerOptions.
func (o *NodeBucketControllerOptions) Validate() []error {
	if o == nil {
		return nil
	}

	errs := []error{}
	if o.MaxNodesPerBucket <= 0 {
		errs = append(errs, fmt.Errorf("max-nodes-per-bucket(%d) is invalid, should greater than 0", o.MaxNodesPerBucket))
	}
	return errs
}
