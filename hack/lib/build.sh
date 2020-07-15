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

#!/usr/bin/env bash

readonly YURT_ALL_TARGETS=(
    cmd/yurtctl
    cmd/yurthub
    cmd/yurt-controller-manager
)

# project_info generates the project information and the corresponding valuse 
# for 'ldflags -X' option
project_info() {
    PROJECT_PREFIX=${PROJECT_PREFIX:-yurt}
    GIT_VERSION="v0.1.1"
    GIT_COMMIT=$(git rev-parse HEAD)
    BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
   
    PROJECT_INFO_PKG=${YURT_MOD}/pkg/yurttunnel/projectinfo
    echo "-X ${PROJECT_INFO_PKG}.projectPrefix=${PROJECT_PREFIX}"
    echo "-X ${PROJECT_INFO_PKG}.gitVersion=${GIT_VERSION}"
    echo "-X ${PROJECT_INFO_PKG}.gitCommit=${GIT_COMMIT}"
    echo "-X ${PROJECT_INFO_PKG}.buildDate=${BUILD_DATE}"
}

# get_output_name generates the executable's name. If the $PROJECT_PREFIX
# is set, it subsitutes the prefix of the executable's name with the env, 
# otherwise the basename of the target is used
get_output_name() {
    local oup_name=$(basename $1)
    PROJECT_PREFIX=${PROJECT_PREFIX:-}
    if [ ! -z $PROJECT_PREFIX ]; then
        oup_name=${oup_name/yurt/$PROJECT_PREFIX}
    fi
    echo $oup_name
}

# get_binary_dir_with_arch generated the binary's directory with GOOS and GOARCH.
# eg: ./_output/bin/darwin/arm64/
get_binary_dir_with_arch(){
    echo $1/$(go env GOOS)/$(go env GOARCH)/
}

build_binaries() {
    local goflags goldflags gcflags
    goldflags="${GOLDFLAGS:--s -w $(project_info)}"
    gcflags="${GOGCFLAGS:-}"
    goflags=${GOFLAGS:-}

    local -a targets=()
    local arg

    for arg; do
      if [[ "${arg}" == -* ]]; then
        # Assume arguments starting with a dash are flags to pass to go.
        goflags+=("${arg}")
      else
        targets+=("${arg}")
      fi
    done

    if [[ ${#targets[@]} -eq 0 ]]; then
      targets=("${YURT_ALL_TARGETS[@]}")
    fi

    local target_bin_dir=$(get_binary_dir_with_arch ${YURT_BIN_DIR})
    mkdir -p ${target_bin_dir}
    cd ${target_bin_dir}
    for binary in "${targets[@]}"; do
      echo "Building ${binary}"
      go build -o $(get_output_name $binary) -ldflags "${goldflags:-}" -gcflags "${gcflags:-}" ${goflags} $YURT_ROOT/${binary}
    done
}
