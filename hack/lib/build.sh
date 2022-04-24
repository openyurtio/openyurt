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
    rm -rf ${target_bin_dir}
    mkdir -p ${target_bin_dir}
    cd ${target_bin_dir}
    for binary in "${targets[@]}"; do
      echo "Building ${binary}"
      go build -o $(get_output_name $binary) \
          -ldflags "${goldflags:-}" \
          -gcflags "${gcflags:-}" ${goflags} $YURT_ROOT/cmd/$(canonicalize_target $binary)
    done

    local yurtctl_binary=$(get_output_name yurtctl)
    if is_build_on_host; then
      if [ -f ${target_bin_dir}/${yurtctl_binary} ]; then
          rm -rf "${YURT_BIN_DIR}"
          mkdir -p "${YURT_BIN_DIR}"
          ln -s "${target_bin_dir}/${yurtctl_binary}" "${YURT_BIN_DIR}/${yurtctl_binary}"
      fi
    fi
}

# gen_yamls generates yaml files for user specified components by
# subsituting the place holders with envs
gen_yamls() {
    local -a yaml_targets=()
    for arg; do
        # ignoring go flags
        [[ "$arg" == -* ]] && continue
        target=$(basename $arg)
        # only add target that is in the ${YURT_YAML_TARGETS} list
        if [[ "${YURT_YAML_TARGETS[@]}" =~ "$target" ]]; then
            yaml_targets+=("$target")
        fi
    done
    # if not specified, generate yaml for default yaml targets
    if [ ${#yaml_targets[@]} -eq 0 ]; then
        yaml_targets=("${YURT_YAML_TARGETS[@]}")
    fi
    echo $yaml_targets

    local yaml_dir=$YURT_OUTPUT_DIR/setup/
    mkdir -p $yaml_dir
    for yaml_target in "${yaml_targets[@]}"; do
        oup_file=${yaml_target/yurt/$PROJECT_PREFIX}
        echo "generating yaml file for $oup_file"
        sed "s|__project_prefix__|${PROJECT_PREFIX}|g;
        s|__label_prefix__|$LABEL_PREFIX|g;
        s|__repo__|$REPO|g;
        s|__tag__|$TAG|g;" \
            $YURT_ROOT/config/yaml-template/$yaml_target.yaml > \
            $yaml_dir/$oup_file.yaml
    done
}

function build_e2e() {
    local goflags goldflags gcflags
    goldflags="${GOLDFLAGS:--s -w $(project_info)}"
    gcflags="${GOGCFLAGS:-}"
    goflags=${GOFLAGS:-}

    local target_bin_dir=$(get_binary_dir_with_arch ${YURT_LOCAL_BIN_DIR})
    mkdir -p ${target_bin_dir}
    cd ${target_bin_dir}
    echo "Building ${YURT_E2E_TARGETS}"
    local testpkg="$(dirname ${YURT_E2E_TARGETS})"
    local filename="$(basename ${YURT_E2E_TARGETS})"
    go test -c  -gcflags "${gcflags:-}" ${goflags} -o $filename "$YURT_ROOT/${testpkg}"
}
