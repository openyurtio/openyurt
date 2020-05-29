#!/usr/bin/env bash
set -xe

YURT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
YURT_OUTPUT_DIR=${YURT_ROOT}/_output/
YURT_BIN_DIR=${YURT_OUTPUT_DIR}/bin/
YURT_IMAGE_DIR=${YURT_OUTPUT_DIR}/images/
BUILDFLAGS="-a --ldflags '-extldflags \"-static\"'"

REPO="openyurt"
VERSION=$("v0.1.0":${YURT_VERSION})

DOCKER_BUILD_BASE_IDR=dockerbuild
USER_ID=$(id -u)
GROUP_ID=$(id -g)

# Always clean first
rm -Rf ${YURT_OUTPUT_DIR}
rm -Rf ${DOCKER_BUILD_BASE_IDR}
mkdir -p ${YURT_BIN_DIR}
mkdir -p ${YURT_IMAGES_DIR}
mkdir -p ${DOCKER_BUILD_BASE_IDR}

docker run -i -v ${YURT_ROOT}:/opt/src --rm golang:1.13-alpine \
/bin/sh -xe -c "\
    apk --no-cache add bash tar;
    cd /opt/src; umask 0022;
    for arch in amd64; do \
        rm -f ${YURT_OUTPUT_DIR}/\$arch/*; \
        CGO_ENABLED=0 GOOS=linux GOARCH=\$arch ./build/run.sh ${BUILDFLAGS}; \
    done; \
    chown -R ${USER_ID}:${GROUP_ID} ${OUTPUT_DIR}"


function build_docker_image() {
    for arch in amd64; do
       for binary in yurthub yurt-controller-manager; do
	       if [ -f ${YURT_OUTPUT_DIR}/${arch}/${binary} ]; then
	           local docker_build_path=${DOCKER_BUILD_BASE_IDR}/${arch}
	           local docker_file_path="${docker_build_path}/Dockerfile"
	           mkdir -p ${docker_build_path}

	           local yurt_component_image="${REPO}/${binary}:${VERSION}.${arch}"
		       local base_image="k8s.gcr.io/debian-iptables-${arch}:v11.0.2"
	           cat <<EOF > "${docker_file_path}"
FROM ${base_image}
COPY ${docker_build_path}/${binary} /usr/local/bin/${binary}
ENTRYPOINT ["/usr/local/bin/${binary}"]
EOF

               ln "${YURT_OUTPUT_DIR}/${arch}/${binary}" "${docker_build_path}/${binary}"
	           docker build --no-cache -t "${yurt_component_image}" -f "${docker_file_path}" .
	           docker save ${yurt_component_image} > ${YURT_IMAGES_DIR}
	        fi
	   fi
    done
}

build_docker_image