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


# This shell will create a openyurt cluster locally with kind. The yurt-tunnel will be
# automatically deployed, and the autonomous mode will be active.
#
# It uses the following env variables:
# REGION
# REGION affects the GOPROXY to use. You can set it to "cn" to use GOPROXY="https://goproxy.cn".
# Default value is "us", which means using GOPROXY="https://goproxy.io".
#
# KIND_KUBECONFIG
# KIND_KUBECONFIG represents the path to store the kubeconfig file of the cluster
# which is created by this shell. The default value is "$HOME/.kube/config".
#
# NODES_NUM
# NODES_NUM represents the number of nodes to set up in the new-created cluster.
# There is one control-plane node and NODES_NUM-1 worker nodes. Thus, NODES_NUM must
# not be less than 2. The default value is 2.
#
# KUBERNETESVERSION
# KUBERNETESVERSION declares the kubernetes version the cluster will use. The format is "v1.XX". 
# Now only v1.17, v1.18, v1.19, v1.20 and v1.21 are supported. The default value is v1.21.


set -x
set -e
set -u

YURT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"

readonly REQUIRED_CMD=(
    go
    docker
    kubectl
    kind
)

readonly BUILD_TARGETS=(
    yurthub
    yurt-controller-manager
    yurtctl
    yurt-tunnel-server
    yurt-tunnel-agent
    yurt-node-servant
)

readonly LOCAL_ARCH=$(go env GOHOSTARCH)
readonly LOCAL_OS=$(go env GOHOSTOS)
readonly CLUSTER_NAME="openyurt-e2e-test"
readonly KUBERNETESVERSION=${KUBERNETESVERSION:-"v1.22"}
readonly NODES_NUM=${NODES_NUM:-2}
readonly KIND_KUBECONFIG=${KIND_KUBECONFIG:-${HOME}/.kube/config}

function install_kind {
    echo "Begin to install kind"
    GO111MODULE="on" go get sigs.k8s.io/kind@v0.11.1
}

function install_docker {
    echo "docker should be installed first"
    return -1
}

function install_kubectl {
    echo "kubectl should be installed first"
    return -1
} 

function install_go {
    echo "go should be installed first"
    return -1
}

function preflight {
    echo "Preflight Check..."
    for bin in "${REQUIRED_CMD[@]}"; do
        command -v ${bin} > /dev/null 2>&1
        if [[ $? -ne 0 ]]; then
            echo "Cannot find command ${bin}."
            install_${bin}
            if [[ $? -ne 0 ]]; then
                echo "Error occurred, exit"
                exit -1
            fi
        fi
    done
}

function build_target_binaries_and_images {
    echo "Begin to build binaries and images"

    export WHAT=${BUILD_TARGETS[@]}
    export ARCH=${LOCAL_ARCH}

    source ${YURT_ROOT}/hack/make-rules/release-images.sh    
}

function local_up_openyurt {
    echo "Begin to setup OpenYurt cluster"
    openyurt_version=$(get_version ${LOCAL_ARCH})
    ${YURT_LOCAL_BIN_DIR}/${LOCAL_OS}/${LOCAL_ARCH}/yurtctl test init \
      --kubernetes-version=${KUBERNETESVERSION} --kube-config=${KIND_KUBECONFIG} \
      --cluster-name=${CLUSTER_NAME} --openyurt-version=${openyurt_version} --use-local-images --ignore-error \
      --node-num=${NODES_NUM}
}

function cleanup {
    rm -rf ${YURT_ROOT}/_output
    rm -rf ${YURT_ROOT}/dockerbuild
    kind delete clusters ${CLUSTER_NAME}
}

function cleanup_on_err {
    if [[ $? -ne 0 ]]; then
        cleanup
    fi
}


trap cleanup_on_err EXIT

cleanup
preflight
build_target_binaries_and_images
local_up_openyurt