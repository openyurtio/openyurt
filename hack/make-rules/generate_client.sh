#!/usr/bin/env bash

set -x
set -e

YURT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"

TMP_DIR=$(mktemp -d)
mkdir -p "${TMP_DIR}"/src/github.com/alibaba/openyurt/pkg/yurtappmanager/client
cp -r ${YURT_ROOT}/{go.mod,go.sum} "${TMP_DIR}"/src/github.com/alibaba/openyurt/
cp -r ${YURT_ROOT}/pkg/yurtappmanager/{apis,hack} "${TMP_DIR}"/src/github.com/alibaba/openyurt/pkg/yurtappmanager/

(
  cd "${TMP_DIR}"/src/github.com/alibaba/openyurt/;
  HOLD_GO="${TMP_DIR}/src/github.com/alibaba/openyurt/pkg/yurtappmanager/hack/hold.go"
  printf 'package hack\nimport "k8s.io/code-generator"\n' > ${HOLD_GO}
  go mod vendor
  GOPATH=${TMP_DIR} GO111MODULE=off /bin/bash vendor/k8s.io/code-generator/generate-groups.sh all \
    github.com/alibaba/openyurt/pkg/yurtappmanager/client github.com/alibaba/openyurt/pkg/yurtappmanager/apis apps:v1alpha1 -h ./pkg/yurtappmanager/hack/boilerplate.go.txt
)

rm -rf ./pkg/yurtappmanager/client/{clientset,informers,listers}
mv "${TMP_DIR}"/src/github.com/alibaba/openyurt/pkg/yurtappmanager/client/* ./pkg/yurtappmanager/client

