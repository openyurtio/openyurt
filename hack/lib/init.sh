#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

YURT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
YURT_OUTPUT_DIR=${YURT_ROOT}/_output/
YURT_BIN_DIR=${YURT_OUTPUT_DIR}/bin/

source "${YURT_ROOT}/hack/lib/build.sh"
