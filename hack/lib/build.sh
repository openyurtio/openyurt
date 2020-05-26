#!/usr/bin/env bash

readonly YURT_ALL_TARGETS=(
    cmd/yurtctl
    cmd/yurthub
    cmd/yurt-controller-manager
)

build_binaries() {
    local goflags goldflags gcflags
    goldflags="${GOLDFLAGS=-s -w}"
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

    mkdir -p ${YURT_BIN_DIR}
    cd ${YURT_BIN_DIR}
    for binary in "${targets[@]}"; do
      echo "Building ${binary}"
      go build -ldflags "${goldflags:-}" -gcflags "${gcflags:-}" ${goflags} $YURT_ROOT/${binary}
    done
}
