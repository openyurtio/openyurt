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

    local target_bin_dir=$(get_binary_dir_with_arch ${YURT_LOCAL_BIN_DIR})
    mkdir -p ${target_bin_dir}
    cd ${target_bin_dir}
    for binary in "${targets[@]}"; do
      echo "Building ${binary}"
      go build -o $(get_output_name $binary) \
          -ldflags "${goldflags:-}" \
          -gcflags "${gcflags:-}" ${goflags} $YURT_ROOT/cmd/$(canonicalize_target $binary)
    done
}