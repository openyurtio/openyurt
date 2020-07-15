#!/usr/bin/env bash
set -xe

YURT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
YURT_OUTPUT_DIR=_output
YURT_BIN_DIR=${YURT_OUTPUT_DIR}/bin
YURT_IMAGE_DIR=${YURT_OUTPUT_DIR}/images
YURTCTL_SERVANT_DIR=${YURT_ROOT}/config/yurtctl-servant
DOCKER_BUILD_BASE_IDR=dockerbuild

YURT_BUILD_IMAGE="golang:1.13-alpine"
REPO="openyurt"
TAG="v0.1.1"
readonly YURT_BIN_TARGETS=(
    yurthub
    yurt-controller-manager
)

readonly SUPPORTED_ARCH=(
    amd64
    arm
    arm64
)

readonly SUPPORTED_OS=linux

# Always clean first
rm -Rf ${YURT_OUTPUT_DIR}
rm -Rf ${DOCKER_BUILD_BASE_IDR}
mkdir -p ${YURT_BIN_DIR}
mkdir -p ${YURT_IMAGE_DIR}
mkdir -p ${DOCKER_BUILD_BASE_IDR}

target_arch=${1:-${SUPPORTED_ARCH[@]}}

function build_multi_arch_binaries() {
    local docker_run_opts=(
        "-i"
        "--rm"
        "--network host"
        "-v ${YURT_ROOT}:/opt/src"
        "--env CGO_ENABLED=0"
        "--env GOPROXY=https://goproxy.cn"
        "--env GOOS=${SUPPORTED_OS}"
    )

    local docker_run_cmd=(
        "/bin/sh"
        "-xe"
        "-c"
    )

    local sub_commands="sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories; \
        apk --no-cache add bash git; \
        cd /opt/src; umask 0022; \
        rm -rf ${YURT_BIN_DIR}/* ;"
    for arch in ${target_arch}; do
        sub_commands+="GOARCH=$arch ./hack/make-rules/build.sh; "
    done
    sub_commands+="chown -R $(id -u):$(id -g) ${YURT_OUTPUT_DIR}"

    docker run ${docker_run_opts[@]} ${YURT_BUILD_IMAGE} ${docker_run_cmd[@]} "${sub_commands}"

}

function build_docker_image() {
    for arch in ${target_arch}; do
        for binary in "${YURT_BIN_TARGETS[@]}"; do
           local binary_path=${YURT_BIN_DIR}/${SUPPORTED_OS}/${arch}/${binary}
           if [ -f ${binary_path} ]; then
               local docker_build_path=${DOCKER_BUILD_BASE_IDR}/${SUPPORTED_OS}/${arch}
               local docker_file_path=${docker_build_path}/Dockerfile.${binary}-${arch}
               mkdir -p ${docker_build_path}

               local yurt_component_image="${REPO}/${binary}:${TAG}-${arch}"
               local base_image="k8s.gcr.io/debian-iptables-${arch}:v11.0.2"
               cat <<EOF > "${docker_file_path}"
FROM ${base_image}
COPY ${binary} /usr/local/bin/${binary}
ENTRYPOINT ["/usr/local/bin/${binary}"]
EOF

               ln "${binary_path}" "${docker_build_path}/${binary}"
               docker build --no-cache -t "${yurt_component_image}" -f "${docker_file_path}" ${docker_build_path}
               docker save ${yurt_component_image} > ${YURT_IMAGE_DIR}/${binary}-${SUPPORTED_OS}-${arch}.tar
               rm -rf ${docker_build_path}
            fi
        done
    done
}

function build_yurtctl_servant_image() {
    local yurtctl_servant_image=${REPO}/yurtctl-servant:$TAG
    docker build -t ${yurtctl_servant_image} ${YURTCTL_SERVANT_DIR}
    docker save ${yurtctl_servant_image} > ${YURT_IMAGE_DIR}/yurtctl-servant.tar
}

build_multi_arch_binaries
build_docker_image
build_yurtctl_servant_image
