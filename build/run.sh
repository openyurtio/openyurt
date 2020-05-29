#!/usr/bin/env bash
set -xe

goldflags="${GOLDFLAGS=-s -w}"
gcflags="${GOGCFLAGS:-}"
goflags=${GOFLAGS:-}

readonly YURT_IMAGE_TARGETS=(
    yurthub
    yurt-controller-manager
)

ORG_PATH="github.com/alibaba"
export REPO_PATH="${ORG_PATH}/openyurt"

if [ ! -h gopath/src/${REPO_PATH} ]; then
	mkdir -p gopath/src/${ORG_PATH}
	ln -s ../../../.. gopath/src/${REPO_PATH} || exit 255
fi

export GO15VENDOREXPERIMENT=1
export GOPATH=${PWD}/gopath
export GOPROXY=https://goproxy.cn

for binary in "${YURT_IMAGE_TARGETS[@]}"; do
    mkdir -p "${PWD}/_output/bin/"
    echo "Building ${binary}"
    go build -ldflags "${goldflags:-}" -gcflags "${gcflags:-}" ${goflags} -o "${PWD}/_output/bin/${binary}" "$REPO_PATH/cmd/${binary}"
done

