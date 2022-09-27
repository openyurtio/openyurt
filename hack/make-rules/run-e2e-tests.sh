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
set -u

YURT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
source "${YURT_ROOT}/hack/lib/init.sh"
source "${YURT_ROOT}/hack/lib/build.sh"

readonly LOCAL_ARCH=$(go env GOHOSTARCH)
readonly LOCAL_OS=$(go env GOHOSTOS)
readonly YURT_E2E_TARGETS="test/e2e/yurt-e2e-test"
readonly EDGE_AUTONOMY_NODES_NUM=3
edgeNodeContainerName="openyurt-e2e-test-worker"
edgeNodeContainer2Name="openyurt-e2e-test-worker2"
KUBECONFIG=$HOME/.kube/config


function set_flags() {
    goldflags="${GOLDFLAGS:--s -w $(project_info)}"
    gcflags="${GOGCFLAGS:-}"
    goflags=${GOFLAGS:-}

    target_bin_dir=$(get_binary_dir_with_arch ${YURT_LOCAL_BIN_DIR})
    mkdir -p ${target_bin_dir}
    cd ${target_bin_dir}
    echo "Building ${YURT_E2E_TARGETS}"
    testpkg="$(dirname ${YURT_E2E_TARGETS})"
    filename="$(basename ${YURT_E2E_TARGETS})"
#    set kubeconfig
    docker exec -d $edgeNodeContainerName /bin/bash -c 'mkdir /root/.kube/ -p'
    docker exec -d $edgeNodeContainerName /bin/bash -c  "echo 'export KUBECONFIG=/root/.kube/config' >> /etc/profile && source /etc/profile" 
    docker cp $KUBECONFIG $edgeNodeContainerName:/root/.kube/config
}

# install gingko
function getGinkgo() {
    go install github.com/onsi/ginkgo/v2/ginkgo@v2.3.0
    go get github.com/onsi/gomega/...
    go mod tidy
}

# run e2e tests
function run_non_edge_autonomy_e2e_tests {
    # check kubeconfig
        if [ ! -f "${KUBECONFIG}" ]; then
            echo "kubeconfig does not exist at ${KUBECONFIG}"
            exit -1
        fi
    # run non-edge-autonomy-e2e-tests
    cd $YURT_ROOT/test/e2e/
    ginkgo --gcflags "${gcflags:-}" ${goflags} --label-filter='!edge_autonomy' -r
}

function run_e2e_edge_autonomy_tests {
    #run edge-autonomy-e2e-tests
    cd $YURT_ROOT/test/e2e/
    ginkgo --gcflags "${gcflags:-}" ${goflags} --label-filter='edge_autonomy' -r
}

function cpCodeToEdge {
    local edgeNodeContainerName="openyurt-e2e-test-worker"
    local openyurtCodePath=${YURT_ROOT}
    docker cp $openyurtCodePath $edgeNodeContainerName:/
}

function serviceNginx {
#  run a nginx pod as static pod on each edge node
    local nginxYamlPath="${YURT_ROOT}/test/e2e/yamls/nginx.yaml"
    local nginxServiceYamlPath="${YURT_ROOT}/test/e2e/yamls/nginxService.yaml"
    local staticPodPath="/etc/kubernetes/manifests/"
    local POD_CREATE_TIMEOUT=60s

# create service for nginx pods
    kubectl apply -f $nginxServiceYamlPath
    docker cp $nginxYamlPath $edgeNodeContainerName:$staticPodPath
    # docker cp $nginxYamlPath $edgeNodeContainer2Name:$staticPodPath
#  wait for pod create timeout
    echo "wait pod create timeout ${edgeNodeContainerName}"
#  wait confirm that nginx is running
    kubectl wait --for=condition=Ready pod/yurt-e2e-test-nginx-openyurt-e2e-test-worker --timeout=${POD_CREATE_TIMEOUT}
    # kubectl wait --for=condition=Ready pod/openyurt-e2e-test-nginx-openyurt-e2e-test-worker2 --timeout=${POD_CREATE_TIMEOUT}
}

function disconnectCloudNode {
#  disconnect the cloud node by pull its plug out of the kind net bridge
    local cloudNodeName="openyurt-e2e-test-control-plane"
    echo "Disconnecting cloudnode $cloudNodeName"
    docker network disconnect kind $cloudNodeName
}

function reconnectCloudNode {
#  disconnect the cloud node by pull its plug out of the kind net bridge
    local cloudNodeName="openyurt-e2e-test-control-plane"
    echo "Reconnecting cloudnode $cloudNodeName"
    docker network connect kind $cloudNodeName
}

GOOS=${LOCAL_OS} GOARCH=${LOCAL_ARCH} set_flags

getGinkgo

run_non_edge_autonomy_e2e_tests

# copy Openyurt and Golang code to edge nodes
cpCodeToEdge

# install golang to edgenode for further tests run
docker exec -d $edgeNodeContainerName /bin/bash  "/openyurt/hack/lib/installGolang.sh"

# start busybox and expose it with a service
serviceNginx

# disconnect cloud node
disconnectCloudNode

# run edge_autonomy_tests
run_e2e_edge_autonomy_tests

# reconnect cloud node
reconnectCloudNode