#!/usr/bin/env bash

YURT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
source "${YURT_ROOT}/hack/lib/init.sh" 

readonly YURT_E2E_TARGETS="test/e2e/yurt-e2e-test"

function build_e2e() {
    local goflags goldflags gcflags
    goldflags="${GOLDFLAGS:--s -w $(project_info)}"
    gcflags="${GOGCFLAGS:-}"
    goflags=${GOFLAGS:-}


    local target_bin_dir=$(get_binary_dir_with_arch ${YURT_BIN_DIR})
    mkdir -p ${target_bin_dir}
    cd ${target_bin_dir}
    echo "Building ${YURT_E2E_TARGETS}"
    local testpkg="$(dirname ${YURT_E2E_TARGETS})"
    local filename="$(basename ${YURT_E2E_TARGETS})"
    go test -c  -gcflags "${gcflags:-}" ${goflags} -o $filename "$YURT_ROOT/${testpkg}"
}

build_e2e 
