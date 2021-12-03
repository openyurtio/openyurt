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


# Parameters
# $1: component name
function get_manifest_name() {
      # If ${GIT_COMMIT} is not at a tag, add commit to the image tag.
      if [[ -z $(git tag --points-at ${GIT_COMMIT}) ]]; then
          yurt_component_manifest="${REPO}/$1:${TAG}-$(echo ${GIT_COMMIT} | cut -c 1-7)"
      else
          yurt_component_manifest="${REPO}/$1:${TAG}"
      fi
      echo ${yurt_component_manifest}
}

function build_docker_manifest() {
    # Always clean first
    rm -Rf ${DOCKER_BUILD_BASE_IDR}
    mkdir -p ${DOCKER_BUILD_BASE_IDR}

    for binary in "${bin_targets_process_servant[@]}"; do
      local binary_name=$(get_output_name $binary)
      local yurt_component_name=$(get_component_name $binary_name)
      local yurt_component_manifest=$(get_manifest_name $yurt_component_name)
      echo ${yurt_component_manifest} >> ${DOCKER_BUILD_BASE_IDR}/manifest.list
      # Remove existing manifest.
      docker manifest rm ${yurt_component_manifest} || true
      for arch in ${target_arch[@]}; do
        case $arch in
          amd64)
              ;;
          arm64)
              ;;
          arm)
              ;;
          *)
              echo unknown arch $arch
              exit 1
         esac
         yurt_component_image=$(get_image_name ${yurt_component_name} ${arch})
         docker manifest create ${yurt_component_manifest} --amend  ${yurt_component_image}
         docker manifest annotate ${yurt_component_manifest} ${yurt_component_image} --os ${SUPPORTED_OS} --arch ${arch}
      done
    done
}

push_manifest() {
    cat ${DOCKER_BUILD_BASE_IDR}/manifest.list | xargs -I % sh -c 'echo pushing manifest %; docker manifest push --purge %; echo'
}