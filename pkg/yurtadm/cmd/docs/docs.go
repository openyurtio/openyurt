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

package docs

import (
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

var docsPath string

// TODO use the follow command generate docs for openyurt.io
/*
cd docs/api/yurtadm
for f in *; do
  suffix=".md"
  name=${f%"$suffix"}
  echo "\"reference/yurtadm/$name\","
done
rm -rf docs/api/yurtadm
*/

func NewDocsCmd(rootCmd *cobra.Command) *cobra.Command {
	var docsCmd = &cobra.Command{
		Use:     "docs",
		Short:   "generate API reference",
		Example: `yurtadm docs`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return doc.GenMarkdownTree(rootCmd, docsPath)
		},
	}
	docsCmd.Flags().StringVarP(&docsPath, "path", "p", "./docs/api/yurtadm", "path to output docs")

	return docsCmd
}
