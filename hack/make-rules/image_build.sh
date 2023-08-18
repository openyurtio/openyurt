#!/usr/bin/env bash

# Copyright 2023 The OpenYurt Authors.
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
source "${YURT_ROOT}/hack/lib/init.sh"
source "${YURT_ROOT}/hack/lib/build.sh"

readonly IMAGE_TARGETS=(
    yurt-node-servant
    yurthub
    yurt-manager
    yurt-iot-dock
)

http_proxy=${http_proxy:-}
https_proxy=${https_proxy:-}
targets=${@:-${IMAGE_TARGETS[@]}}
REGION=${REGION:-}
IMAGE_REPO=${IMAGE_REPO:-"openyurt"}
IMAGE_TAG=${IMAGE_TAG:-$(get_image_tag)}
DOCKER_BUILD_ARGS=""
DOCKER_EXTRA_ENVS=""
BUILD_BASE_IMAGE="golang:1.18"
BUILD_GOPROXY=$(go env GOPROXY)
GOPROXY_CN="https://goproxy.cn"
APKREPO_MIRROR_CN="mirrors.aliyun.com"

if [ "${REGION}" == "cn" ]; then
    BUILD_GOPROXY=${GOPROXY_CN}
    DOCKER_BUILD_ARGS="${DOCKER_BUILD_ARGS} --build-arg MIRROR_REPO=${APKREPO_MIRROR_CN}"
fi

if [[ ! -z ${http_proxy} ]]; then 
    DOCKER_BUILD_ARGS="${DOCKER_BUILD_ARGS} --build-arg http_proxy=${http_proxy}"
    DOCKER_EXTRA_ENVS="--env http_proxy=${http_proxy}"
fi

if [[ ! -z ${https_proxy} ]]; then
    DOCKER_BUILD_ARGS="${DOCKER_BUILD_ARGS} --build-arg https_proxy=${https_proxy}"
    DOCKER_EXTRA_ENVS="${DOCKER_EXTRA_ENVS=} --env https_proxy=${https_proxy}"
fi

if [[ ! -z ${TARGET_PLATFORMS} ]]; then
    IFS="/" read -r TARGETOS TARGETARCH <<< "${TARGET_PLATFORMS}"
else
    echo "TARGET_PLATFORMS should be specified"
    exit -1
fi

# build binaries within docker container

# It uses the following two lines:
# --env GOCACHE=/tmp/
# --user $(id -u ${USER}):$(id -g ${USER})
# to enable the docker container to build binaries with the
# same user:group as the current user:group of host.
docker run \
    --rm --name openyurt-build \
    --mount type=bind,dst=/build/,src=${YURT_ROOT} \
    --workdir=/build/ \
    --env GOPROXY=${BUILD_GOPROXY} \
    --env GOOS=${TARGETOS} \
    --env GOARCH=${TARGETARCH} \
    --env GOCACHE=/tmp/ \
    ${DOCKER_EXTRA_ENVS} \
    --user $(id -u ${USER}):$(id -g ${USER}) \
    ${BUILD_BASE_IMAGE} \
    ./hack/make-rules/build.sh ${targets[@]}

# build images
for image in ${targets[@]}; do
    # The image name and binary name of node-servant
    # are not same. So we have to this translation.
    if [[ ${image} == "yurt-node-servant" ]]; then
        image="node-servant"
    fi 

    docker buildx build \
    --no-cache \
    --load ${DOCKER_BUILD_ARGS} \
    --platform ${TARGET_PLATFORMS} \
    --file ${YURT_ROOT}/hack/dockerfiles/build/Dockerfile.${image} \
    --tag ${IMAGE_REPO}/${image}:${IMAGE_TAG} \
    ${YURT_ROOT}
done
