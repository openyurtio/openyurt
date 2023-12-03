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

set -o errexit
set -o nounset
set -o pipefail

YURT_MOD="$(head -1 $YURT_ROOT/go.mod | awk '{print $2}')"
YURT_OUTPUT_DIR=${YURT_ROOT}/_output
YURT_BIN_DIR=${YURT_OUTPUT_DIR}/bin
YURT_LOCAL_BIN_DIR=${YURT_OUTPUT_DIR}/local/bin

PROJECT_PREFIX=${PROJECT_PREFIX:-yurt}
LABEL_PREFIX=${LABEL_PREFIX:-openyurt.io}
GIT_VERSION=${GIT_VERSION:-$(git describe --abbrev=0 --tags)}
GIT_COMMIT=$(git rev-parse HEAD)
BUILD_DATE=$(date -u +'%Y-%m-%dT%H:%M:%SZ')
NODEPOOL_LABEL_KEY=${NODEPOOL_LABEL_KEY:-apps.openyurt.io/nodepool}

source "${YURT_ROOT}/hack/lib/common.sh"