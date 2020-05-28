#!/usr/bin/env bash
set -xe

readonly YURT_IMAGE_TARGETS=(
    cmd/yurthub
    cmd/yurt-controller-manager
)

if [ "$(uname)" == "Darwin" ]; then
	export GOOS=linux
fi

ORG_PATH="github.com/alibaba"
export REPO_PATH="${ORG_PATH}/openyurt"

if [ ! -h gopath/src/${REPO_PATH} ]; then
	mkdir -p gopath/src/${ORG_PATH}
	ln -s ../../../.. gopath/src/${REPO_PATH} || exit 255
fi

export GO15VENDOREXPERIMENT=1
export GOPATH=${PWD}/gopath

targets=("${YURT_ALL_TARGETS[@]}")
for binary in "${targets[@]}"; do
    mkdir -p "${PWD}/bin/${GOARCH}"
    echo "Building ${binary}"
    go build -pkgdir "${PWD}/bin/${GOARCH}/" "$@" "$REPO_PATH/${binary}"
done

