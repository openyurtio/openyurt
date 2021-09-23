#!/usr/bin/env bash

# Copyright 2020 The OpenYurt Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -x
set -e

YURT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"

TMP_DIR=$(mktemp -d)
mkdir -p "${TMP_DIR}"/src/github.com/openyurtio/openyurt/pkg/yurtappmanager/client
cp -r ${YURT_ROOT}/{go.mod,go.sum} "${TMP_DIR}"/src/github.com/openyurtio/openyurt/
cp -r ${YURT_ROOT}/pkg/yurtappmanager/{apis,hack} "${TMP_DIR}"/src/github.com/openyurtio/openyurt/pkg/yurtappmanager/

(
  cd "${TMP_DIR}"/src/github.com/openyurtio/openyurt/;
  HOLD_GO="${TMP_DIR}/src/github.com/openyurtio/openyurt/pkg/yurtappmanager/hack/hold.go"
  printf 'package hack\nimport "k8s.io/code-generator"\n' > ${HOLD_GO}
  go mod vendor
  GOPATH=${TMP_DIR} GO111MODULE=off /bin/bash vendor/k8s.io/code-generator/generate-groups.sh all \
    github.com/openyurtio/openyurt/pkg/yurtappmanager/client github.com/openyurtio/openyurt/pkg/yurtappmanager/apis apps:v1alpha1 -h ./pkg/yurtappmanager/hack/boilerplate.go.txt
)

rm -rf ./pkg/yurtappmanager/client/{clientset,informers,listers}
mv "${TMP_DIR}"/src/github.com/openyurtio/openyurt/pkg/yurtappmanager/client/* ./pkg/yurtappmanager/client

