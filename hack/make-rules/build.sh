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

YURT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
source "${YURT_ROOT}/hack/lib/init.sh"
source "${YURT_ROOT}/hack/lib/build.sh"

readonly YURT_ALL_TARGETS=(
    yurtadm
    yurt-node-servant
    yurthub
    yurt-manager
    yurt-iot-dock
)

# clean old binaries at GOOS and GOARCH
# eg. _output/local/bin/linux/amd64
rm -rf $(get_binary_dir_with_arch ${YURT_LOCAL_BIN_DIR})

build_binaries "$@"
