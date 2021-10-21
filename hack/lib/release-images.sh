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

YURT_IMAGE_DIR=${YURT_OUTPUT_DIR}/images
YURTCTL_SERVANT_DIR=${YURT_ROOT}/config/yurtctl-servant
DOCKER_BUILD_BASE_IDR=$YURT_ROOT/dockerbuild
YURT_BUILD_IMAGE="golang:1.15-alpine"
#REPO="openyurt"
#TAG="v0.2.0"

readonly -a YURT_BIN_TARGETS=(
    yurthub
    yurt-controller-manager
    yurtctl
    yurt-tunnel-server
    yurt-tunnel-agent
)

readonly -a SUPPORTED_ARCH=(
    amd64
    arm
    arm64
)

readonly SUPPORTED_OS=linux

readonly -a bin_targets=(${WHAT[@]:-${YURT_BIN_TARGETS[@]}})
readonly -a bin_targets_process_servant=("${bin_targets[@]/yurtctl-servant/yurtctl}")
readonly -a target_arch=(${ARCH[@]:-${SUPPORTED_ARCH[@]}})
readonly region=${REGION:-us}

# Parameters
# $1: component name
# $2: arch
function get_image_name {
    # If ${GIT_COMMIT} is not at a tag, add commit to the image tag. 
    if [[ -z $(git tag --points-at ${GIT_COMMIT}) ]]; then
        yurt_component_image="${REPO}/$1:${TAG}-$2-$(echo ${GIT_COMMIT} | cut -c 1-7)"
    else
        yurt_component_image="${REPO}/$1:${TAG}-$2"
    fi    

    echo ${yurt_component_image}
}

function build_multi_arch_binaries() {
    local docker_run_opts=(
        "-i"
        "--rm"
        "--network host"
        "-v ${YURT_ROOT}:/opt/src"
        "--env CGO_ENABLED=0"
        "--env GOOS=${SUPPORTED_OS}"
        "--env PROJECT_PREFIX=${PROJECT_PREFIX}"
        "--env LABEL_PREFIX=${LABEL_PREFIX}"
        "--env GIT_VERSION=${GIT_VERSION}"
        "--env GIT_COMMIT=${GIT_COMMIT}"
        "--env BUILD_DATE=${BUILD_DATE}"
        "--env HOST_PLATFORM=$(host_platform)"
    )
    # use goproxy if build from inside mainland China
    [[ $region == "cn" ]] && docker_run_opts+=("--env GOPROXY=https://goproxy.cn")

    local docker_run_cmd=(
        "/bin/sh"
        "-xe"
        "-c"
    )

    local sub_commands="sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories; \
        apk --no-cache add bash git; \
        cd /opt/src; umask 0022; \
        rm -rf ${YURT_LOCAL_BIN_DIR}/* ;"
    for arch in ${target_arch[@]}; do
        sub_commands+="GOARCH=$arch bash ./hack/make-rules/build.sh $(echo ${bin_targets_process_servant[@]}); "
    done
    sub_commands+="chown -R $(id -u):$(id -g) /opt/src/_output"

    docker run ${docker_run_opts[@]} ${YURT_BUILD_IMAGE} ${docker_run_cmd[@]} "${sub_commands}"
}

function build_docker_image() {
    for arch in ${target_arch[@]}; do
        for binary in "${bin_targets_process_servant[@]}"; do
           local binary_name=$(get_output_name $binary)
           local binary_path=${YURT_LOCAL_BIN_DIR}/${SUPPORTED_OS}/${arch}/${binary_name}
           if [ -f ${binary_path} ]; then
               local docker_build_path=${DOCKER_BUILD_BASE_IDR}/${SUPPORTED_OS}/${arch}
               local docker_file_path=${docker_build_path}/Dockerfile.${binary_name}-${arch}
               mkdir -p ${docker_build_path}

               local yurt_component_name
               local base_image
               if [[ ${binary} =~ yurtctl ]]
               then
                 yurt_component_name="yurtctl-servant"
                 case $arch in
                  amd64)
                      base_image="amd64/alpine:3.9"
                      ;;
                  arm64)
                      base_image="arm64v8/alpine:3.9"
                      ;;
                  arm)
                      base_image="arm32v7/alpine:3.9"
                      ;;
                  *)
                      echo unknown arch $arch
                      exit 1
                 esac
                 cat << EOF > $docker_file_path
FROM ${base_image}
ADD ${binary_name} /usr/local/bin/yurtctl
EOF
               else
                 yurt_component_name=${binary_name}
                 base_image="k8s.gcr.io/debian-iptables-${arch}:v11.0.2"
                 cat <<EOF > "${docker_file_path}"
FROM ${base_image}
COPY ${binary_name} /usr/local/bin/${binary_name}
ENTRYPOINT ["/usr/local/bin/${binary_name}"]
EOF
               fi

               yurt_component_image=$(get_image_name ${yurt_component_name} ${arch})
               ln "${binary_path}" "${docker_build_path}/${binary_name}"
               docker build --no-cache -t "${yurt_component_image}" -f "${docker_file_path}" ${docker_build_path}
               docker save ${yurt_component_image} > ${YURT_IMAGE_DIR}/${yurt_component_name}-${SUPPORTED_OS}-${arch}.tar
            fi
        done
    done
}

build_images() {
    # Always clean first
    rm -Rf ${YURT_OUTPUT_DIR}
    rm -Rf ${DOCKER_BUILD_BASE_IDR}
    mkdir -p ${YURT_LOCAL_BIN_DIR}
    mkdir -p ${YURT_IMAGE_DIR}
    mkdir -p ${DOCKER_BUILD_BASE_IDR}

    build_multi_arch_binaries
    build_docker_image
}