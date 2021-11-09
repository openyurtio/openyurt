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

YURT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)
source ${YURT_ROOT}/hack/make-rules/build-e2e.sh

KUBECONFIG=${KUBECONFIG:-${HOME}/.kube/config}

# run e2e tests
function run_e2e_tests {
    # check kubeconfig
    if [ ! -f "${KUBECONFIG}" ]; then
        echo "kubeconfig does not exist at ${KUBECONFIG}"
        exit -1
    fi

    local target_bin_dir=$(get_binary_dir_with_arch ${YURT_LOCAL_BIN_DIR})
    local e2e_test_file_name=$(basename ${YURT_E2E_TARGETS})
    ${target_bin_dir}/${e2e_test_file_name} -kubeconfig ${KUBECONFIG}
}

run_e2e_tests