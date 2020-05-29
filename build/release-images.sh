#!/usr/bin/env bash
set -xe

YURT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
YURT_OUTPUT_DIR=_output
YURT_BIN_DIR=${YURT_OUTPUT_DIR}/bin
YURT_IMAGE_DIR=${YURT_OUTPUT_DIR}/images
DOCKER_BUILD_BASE_IDR=dockerbuild

REPO="openyurt"
TAG="v.0.1.0-beta1"
readonly YURT_BIN_TARGETS=(
    yurthub
    yurt-controller-manager
)

USER_ID=$(id -u)
GROUP_ID=$(id -g)

# Always clean first
rm -Rf ${YURT_OUTPUT_DIR}
rm -Rf ${DOCKER_BUILD_BASE_IDR}
mkdir -p ${YURT_BIN_DIR}
mkdir -p ${YURT_IMAGE_DIR}
mkdir -p ${DOCKER_BUILD_BASE_IDR}

docker run -i -v ${YURT_ROOT}:/opt/src --network host --rm golang:1.13-alpine \
/bin/sh -xe -c "\
    apk --no-cache add bash tar; \
    cd /opt/src; umask 0022; \
    rm -f ${YURT_BIN_DIR}/*; \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 ./build/run.sh; \
    chown -R ${USER_ID}:${GROUP_ID} ${YURT_OUTPUT_DIR}"


function build_docker_image() {
   for binary in "${YURT_BIN_TARGETS[@]}"; do
	   if [ -f ${YURT_BIN_DIR}/${binary} ]; then
	       local docker_build_path=${DOCKER_BUILD_BASE_IDR}
	       local docker_file_path="${docker_build_path}/Dockerfile.${binary}"
	       mkdir -p ${docker_build_path}

	       local yurt_component_image="${REPO}/${binary}:${TAG}"
		   local base_image="k8s.gcr.io/debian-iptables-amd64:v11.0.2"
	       cat <<EOF > "${docker_file_path}"
FROM ${base_image}
COPY ${docker_build_path}/${binary} /usr/local/bin/${binary}
ENTRYPOINT ["/usr/local/bin/${binary}"]
EOF

           ln "${YURT_BIN_DIR}/${binary}" "${docker_build_path}/${binary}"
	       docker build --no-cache -t "${yurt_component_image}" -f "${docker_file_path}" .
	       docker save ${yurt_component_image} > ${YURT_IMAGE_DIR}/${binary}.tar
	    fi
    done
}

build_docker_image