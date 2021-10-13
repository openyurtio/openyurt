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
# KUBERNETESVERSION declares the kubernetes version the cluster will use. The format is "1.XX". 
# Now only 1.18, 1.19 and 1.20 are supported. The default value is 1.20.
#
# TIMEOUT
# TIMEOUT represents the time to wait for the kind control-plane, yurt-tunnel-server and
# yurt-tunnel-agent to be ready. If they are not ready after the duration, the shell will exit.
# The default value is 120s.
#
# YURTTUNNEL
# If set YURTTUNNEL=disable, the yurt-tunnel-agent and yurt-tunnel-server will not be
# deployed in the openyurt cluster. The default value is "enable".


set -x
set -e
set -u

YURT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
KIND_KUBECONFIG=${KIND_KUBECONFIG:-${HOME}/.kube/config}

readonly KIND_NODE_IMAGES=(
    kindest/node:v1.18.19@sha256:7af1492e19b3192a79f606e43c35fb741e520d195f96399284515f077b3b622c
    kindest/node:v1.19.11@sha256:07db187ae84b4b7de440a73886f008cf903fcf5764ba8106a9fd5243d6f32729
    kindest/node:v1.20.7@sha256:cbeaf907fc78ac97ce7b625e4bf0de16e3ea725daf6b04f930bd14c67c671ff9
)

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
)

readonly LOCAL_ARCH=$(go env GOHOSTARCH)
readonly LOCAL_OS=$(go env GOHOSTOS)
readonly IMAGES_DIR=${YURT_ROOT}/_output/images
readonly CLUSTER_NAME="openyurt-e2e-test"
readonly TIMEOUT=${TIMEOUT:-"120s"}
readonly KUBERNETESVERSION=${KUBERNETESVERSION:-"1.20"}
readonly KIND_CONFIG=${YURT_ROOT}/_output/kind-config-v${KUBERNETESVERSION}.yaml
readonly NODES_NUM=${NODES_NUM:-2}
readonly KIND_CONTEXT="kind-${CLUSTER_NAME}"

master=
edgenodes=

# $1 string to escape
function escape_slash {
    echo $1 | sed "s/\//\\\\\//g"
}

function gen_kind_config {
    # check if number of nodes is valid
    if [[ ${NODES_NUM} -lt 2 ]]; then
        echo "NODES_NUM should be greater than 2"
        exit -1 
    fi
       
    # check if kubernetes version is valid
    local k8s_version=($(echo ${KUBERNETESVERSION} | sed "s/\./ /g"))
    local major=${k8s_version[0]}
    local minor=${k8s_version[1]}
    if [[ ${major} -ne 1 ]] || [[ ${minor} -gt 20 ]] || [[ ${minor} -lt 18 ]]; then
        echo "Invalid KUBERNETESVERSION, it should be between 1.18 and 1.20."
        exit -1
    fi

    # create and init kind config file
    local gen_config_path=${KIND_CONFIG}
    cat ${YURT_ROOT}/hack/kind-config-template.yaml > ${gen_config_path}
    
    # add additional node spec into kind config
    for ((count=2; count<${NODES_NUM}; count++)); do
        echo -e "\n  - role: worker\n    image: |fill image here|" >> ${gen_config_path}
    done

    # fill name:tag of images and fill bin dir
    local bindir=$(escape_slash ${YURT_LOCAL_BIN_DIR}/${LOCAL_OS}/${LOCAL_ARCH})
    local node_image=$(escape_slash $(echo ${KIND_NODE_IMAGES[${minor}-18]}))
    sed -i "s/image: |fill image here|$/image: ${node_image}/g 
        s/- hostPath: |fill local bin dir|/- hostPath: ${bindir}/g" \
            ${gen_config_path}    
}

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

function kind_load_images {
    local postfix="${LOCAL_OS}-${LOCAL_ARCH}.tar"

    for bin in ${BUILD_TARGETS[@]}; do
        local imagename="${bin}-${postfix}"
        
        if [[ "${bin}" = "yurtctl" ]]; then
            imagename="yurtctl-servant-${postfix}"
        fi
        
        echo "loading image ${imagename} to nodes"
        local nodesarg=$(echo ${master} ${edgenodes[@]} | sed "s/ /,/g")
        kind load image-archive ${IMAGES_DIR}/${imagename} \
            --name ${CLUSTER_NAME} --nodes ${nodesarg}
    done
}

function local_up_cluster {
    echo "Creating kubernetes cluster with ${NODES_NUM} nodes"
    gen_kind_config
    
    # output kind config before kind cmd for the convenience of debugging
    echo $(cat ${KIND_CONFIG})
    kind create cluster --config ${KIND_CONFIG}

    echo "Waiting for the control-plane ready..."
    kubectl wait --for=condition=Ready node/${CLUSTER_NAME}-control-plane --context ${KIND_CONTEXT} --timeout=${TIMEOUT}
    master=$(kubectl get node -A -o custom-columns=NAME:.metadata.name --context ${KIND_CONTEXT} | grep control-plane)
    edgenodes=$(kubectl get node -A -o custom-columns=NAME:.metadata.name --context ${KIND_CONTEXT} | grep work)
}

# $1 yurt-tunnel-agent image with the format of "name:tag"
function deploy_yurt_tunnel_agent {
    local yurt_tunnel_agent_yaml=${YURT_OUTPUT_DIR}/yurt-tunnel-agent.yaml
    # revise the yurt-tunnel-agent.yaml
    cat ${YURT_ROOT}/config/setup/yurt-tunnel-agent.yaml |
        sed "s/image: openyurt\/yurt-tunnel-agent:latest/image: $(escape_slash ${1})/" |
            tee ${yurt_tunnel_agent_yaml}
            
    kubectl apply -f ${yurt_tunnel_agent_yaml} --context ${KIND_CONTEXT}
}

# $1 yurt-tunnel-server image with the format of "name:tag"
function deploy_yurt_tunnel_server {
    local yurt_tunnel_server_yaml=${YURT_OUTPUT_DIR}/yurt-tunnel-server.yaml
    # revise the yurt-tunnel-server.yaml
    cat ${YURT_ROOT}/config/setup/yurt-tunnel-server.yaml | 
        sed "s/image: openyurt\/yurt-tunnel-server:latest/image: $(escape_slash ${1})/;
            s/imagePullPolicy: Always/imagePullPolicy: IfNotPresent/" | 
                tee ${yurt_tunnel_server_yaml}

    kubectl apply -f ${yurt_tunnel_server_yaml} --context ${KIND_CONTEXT}
}

function deploy_yurt_tunnel {
    deploy_yurt_tunnel_agent $(get_image_name "yurt-tunnel-agent" ${LOCAL_ARCH})
    deploy_yurt_tunnel_server $(get_image_name "yurt-tunnel-server" ${LOCAL_ARCH})
}

function convert_to_openyurt {
    echo "converting to openyurt cluster "
    
    local yurtctl_dir="/root/openyurt"
    # local yurtctl_dir=${YURT_LOCAL_BIN_DIR}/${LOCAL_OS}/${LOCAL_ARCH}
    docker exec ${master} \
        ${yurtctl_dir}/yurtctl convert --provider kubeadm --cloud-nodes ${master} \
            --yurthub-image=$(get_image_name "yurthub" ${LOCAL_ARCH}) \
            --yurt-controller-manager-image=$(get_image_name "yurt-controller-manager" ${LOCAL_ARCH}) \
            --yurtctl-servant-image=$(get_image_name "yurtctl-servant" ${LOCAL_ARCH})
            # --yurt-tunnel-server-image=$(get_image_name "yurt-tunnel-server" ${LOCAL_ARCH}) \
            # --yurt-tunnel-agent-image=$(get_image_name "yurt-tunnel-agent" ${LOCAL_ARCH})
    
    local nodearg=$(echo ${edgenodes[@]} | sed "s/ /,/g")
    docker exec ${master} \
        ${yurtctl_dir}/yurtctl markautonomous -a "${nodearg}"
    
    if [ ! "${YURTTUNNEL:-"enable"}" = "disable" ]; then
        deploy_yurt_tunnel    
        kubectl rollout status deploy yurt-tunnel-server -n kube-system \
            --timeout=${TIMEOUT} --context ${KIND_CONTEXT}
        kubectl rollout status daemonset yurt-tunnel-agent -n kube-system \
            --timeout=${TIMEOUT} --context ${KIND_CONTEXT}
    fi
}

function get_kubeconfig {
    mkdir -p ${HOME}/.kube 
    kind get kubeconfig --name ${CLUSTER_NAME} > ${KIND_KUBECONFIG} 
}

function cleanup {
    rm -rf ${YURT_ROOT}/_output
    rm -rf ${YURT_ROOT}/dockerbuild
    rm -f ${KIND_CONFIG} 
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
local_up_cluster
kind_load_images
convert_to_openyurt
get_kubeconfig