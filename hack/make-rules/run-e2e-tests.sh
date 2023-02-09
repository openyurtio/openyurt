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
cloudNodeContainerName="openyurt-e2e-test-control-plane"
edgeNodeContainerName="openyurt-e2e-test-worker"
edgeNodeContainer2Name="openyurt-e2e-test-worker2"
KUBECONFIG=${KUBECONFIG:-${HOME}/.kube/config}
TARGET_PLATFORM=${TARGET_PLATFORMS:-linux/amd64}
ENABLE_AUTONOMY_TESTS=${ENABLE_AUTONOMY_TESTS:-true}

function set_flags() {
    goldflags="${GOLDFLAGS:--s -w $(project_info)}"
    gcflags="${GOGCFLAGS:-}"
    goflags=${GOFLAGS:-}

#    set kubeconfig
    docker exec -d $edgeNodeContainerName /bin/bash -c 'mkdir /root/.kube/ -p'
    docker exec -d $edgeNodeContainerName /bin/bash -c  "echo 'export KUBECONFIG=/root/.kube/config' >> /etc/profile && source /etc/profile" 
    docker cp $KUBECONFIG $edgeNodeContainerName:/root/.kube/config
}

# set up network
function set_up_network() {
    # set up bridge cni plugins for every node
    if [ "$TARGET_PLATFORM" = "linux/amd64" ]; then
        wget -O /tmp/cni.tgz https://github.com/containernetworking/plugins/releases/download/v1.1.1/cni-plugins-linux-amd64-v1.1.1.tgz
    else
        wget -O /tmp/cni.tgz https://github.com/containernetworking/plugins/releases/download/v1.1.1/cni-plugins-linux-arm64-v1.1.1.tgz
    fi

    docker cp /tmp/cni.tgz $cloudNodeContainerName:/opt/cni/bin/
    docker exec -t $cloudNodeContainerName /bin/bash -c 'cd /opt/cni/bin && tar -zxf cni.tgz'

    docker cp /tmp/cni.tgz $edgeNodeContainerName:/opt/cni/bin/
    docker exec -t $edgeNodeContainerName /bin/bash -c 'cd /opt/cni/bin && tar -zxf cni.tgz'

    docker cp /tmp/cni.tgz $edgeNodeContainer2Name:/opt/cni/bin/
    docker exec -t $edgeNodeContainer2Name /bin/bash -c 'cd /opt/cni/bin && tar -zxf cni.tgz'

    # deploy flannel DaemonSet
    local flannelYaml="https://raw.githubusercontent.com/flannel-io/flannel/master/Documentation/kube-flannel.yml"
    local flannelDs="kube-flannel-ds"
    local flannelNameSpace="kube-flannel"
    local POD_CREATE_TIMEOUT=120s
    curl -o /tmp/flannel.yaml $flannelYaml
    kubectl apply -f /tmp/flannel.yaml
    # check if flannel on every node is ready, if so, "daemon set "kube-flannel-ds" successfully rolled out"
    kubectl rollout status daemonset kube-flannel-ds -n kube-flannel --timeout=${POD_CREATE_TIMEOUT}
}

function cleanup {
    rm -rf "$YURT_ROOT/test/e2e/e2e.test"
}

# install gingko
function get_ginkgo() {
    go install github.com/onsi/ginkgo/v2/ginkgo@v2.1.4
}

function build_e2e_binary() {
    echo "Begin to build e2e binary"
    local goflags goldflags gcflags
    goldflags="${GOLDFLAGS:--s -w $(project_info)}"
    gcflags="${GOGCFLAGS:-}"
    goflags=${GOFLAGS:-}

    local arg
    for arg; do
      if [[ "${arg}" == -* ]]; then
        # Assume arguments starting with a dash are flags to pass to go.
        goflags+=("${arg}")
      fi
    done

    ginkgo build $YURT_ROOT/test/e2e \
    --gcflags "${gcflags:-}" ${goflags} --ldflags "${goldflags}"
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
    $YURT_ROOT/test/e2e/e2e.test e2e --ginkgo.label-filter='!edge-autonomy' --ginkgo.v
}

function run_e2e_edge_autonomy_tests {
     # check kubeconfig
    if [ ! -f "${KUBECONFIG}" ]; then
        echo "kubeconfig does not exist at ${KUBECONFIG}"
        exit -1
    fi
    # run edge-autonomy-e2e-tests
    cd $YURT_ROOT/test/e2e/
    $YURT_ROOT/test/e2e/e2e.test e2e --ginkgo.label-filter='edge-autonomy' --ginkgo.v
}

function prepare_autonomy_tests {
#   run a nginx pod as static pod on each edge node
    local nginxYamlPath="${YURT_ROOT}/test/e2e/yamls/nginx.yaml"
    local nginxServiceYamlPath="${YURT_ROOT}/test/e2e/yamls/nginxService.yaml"
    local staticPodPath="/etc/kubernetes/manifests/"
    local POD_CREATE_TIMEOUT=240s

#   create service for nginx pods
    kubectl apply -f $nginxServiceYamlPath
    docker cp $nginxYamlPath $edgeNodeContainerName:$staticPodPath
    docker cp $nginxYamlPath $edgeNodeContainer2Name:$staticPodPath
#   wait confirm that nginx is running
    kubectl wait --for=condition=Ready pod/yurt-e2e-test-nginx-openyurt-e2e-test-worker --timeout=${POD_CREATE_TIMEOUT}
    kubectl wait --for=condition=Ready pod/yurt-e2e-test-nginx-openyurt-e2e-test-worker2 --timeout=${POD_CREATE_TIMEOUT}

#   set up dig in edge node1 
    docker exec -t $edgeNodeContainerName /bin/bash -c "sed -i -r 's/([a-z]{2}.)?archive.ubuntu.com/old-releases.ubuntu.com/g' /etc/apt/sources.list"
    docker exec -t $edgeNodeContainerName /bin/bash -c "sed -i -r 's/security.ubuntu.com/old-releases.ubuntu.com/g' /etc/apt/sources.list"
    docker exec -t $edgeNodeContainerName /bin/bash -c "sed -i -r 's/ports.ubuntu.com\/ubuntu-ports/old-releases.ubuntu.com\/ubuntu/g' /etc/apt/sources.list"
    docker exec -t $edgeNodeContainerName /bin/bash -c "sed -i -r 's/old-releases.ubuntu.com\/ubuntu-ports/old-releases.ubuntu.com\/ubuntu/g' /etc/apt/sources.list"
    docker exec -t $edgeNodeContainerName /bin/bash -c "apt-get update && apt-get install dnsutils -y"

#   set up dig in edge node2
    docker exec -t $edgeNodeContainer2Name /bin/bash -c "sed -i -r 's/([a-z]{2}.)?archive.ubuntu.com/old-releases.ubuntu.com/g' /etc/apt/sources.list"
    docker exec -t $edgeNodeContainer2Name /bin/bash -c "sed -i -r 's/security.ubuntu.com/old-releases.ubuntu.com/g' /etc/apt/sources.list"
    docker exec -t $edgeNodeContainer2Name /bin/bash -c "sed -i -r 's/ports.ubuntu.com\/ubuntu-ports/old-releases.ubuntu.com\/ubuntu/g' /etc/apt/sources.list"
    docker exec -t $edgeNodeContainer2Name /bin/bash -c "sed -i -r 's/old-releases.ubuntu.com\/ubuntu-ports/old-releases.ubuntu.com\/ubuntu/g' /etc/apt/sources.list"
    docker exec -t $edgeNodeContainer2Name /bin/bash -c "apt-get update && apt-get install dnsutils -y"
}

GOOS=${LOCAL_OS} GOARCH=${LOCAL_ARCH} set_flags

set_up_network

cleanup

get_ginkgo

GOOS=${LOCAL_OS} GOARCH=${LOCAL_ARCH} build_e2e_binary

run_non_edge_autonomy_e2e_tests

if [ "$ENABLE_AUTONOMY_TESTS" = "true" ]; then
    prepare_autonomy_tests
    run_e2e_edge_autonomy_tests
fi