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

YURT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
source "${YURT_ROOT}/hack/lib/init.sh" 

readonly YURT_E2E_TARGETS="test/e2e/yurt-e2e-test"

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

build_e2e 
