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

KUBERNETESVERSION ?=v1.32
GOLANGCILINT_VERSION ?= v1.64.7
GLOBAL_GOLANGCILINT := $(shell which golangci-lint)
GOBIN := $(shell go env GOPATH)/bin
GOBIN_GOLANGCILINT := $(shell which $(GOBIN)/golangci-lint)
TARGET_PLATFORMS ?= linux/amd64
BRANCH_TAG = $(shell git describe --abbrev=0 --tags)
IMAGE_REPO ?= openyurt
IMAGE_TAG ?= $(BRANCH_TAG)
GIT_COMMIT = $(shell git rev-parse HEAD)
ENABLE_AUTONOMY_TESTS ?=true
BUILD_KUSTOMIZE ?= _output/manifest
GOPROXY ?= $(shell go env GOPROXY)

# Dynamic detection of operating system and architecture
OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
ARCH := $(shell uname -m)
ifeq ($(ARCH),x86_64)
	ARCH := amd64
endif

ifeq ($(IMAGE_TAG),$(BRANCH_TAG))
	ifeq ($(shell git tag --points-at ${GIT_COMMIT}),)
		IMAGE_TAG :=$(IMAGE_TAG)-$(shell echo ${GIT_COMMIT} | cut -c 1-7)
	endif
endif

GIT_VERSION := $(IMAGE_TAG)

DOCKER_BUILD_ARGS = --build-arg GIT_VERSION=${GIT_VERSION}

ifeq (${REGION}, cn)
GOPROXY=https://goproxy.cn
DOCKER_BUILD_ARGS += --build-arg GOPROXY=$(GOPROXY) --build-arg MIRROR_REPO=mirrors.aliyun.com
endif

ifneq (${http_proxy},)
DOCKER_BUILD_ARGS += --build-arg http_proxy='${http_proxy}'
endif

ifneq (${https_proxy},)
DOCKER_BUILD_ARGS += --build-arg https_proxy='${https_proxy}'
endif

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

KUSTOMIZE_VERSION ?= v4.5.7
## Tool Binaries
KUSTOMIZE ?= $(LOCALBIN)/kustomize

KUBECTL_VERSION ?= v1.30.1
KUBECTL ?= $(LOCALBIN)/kubectl

YQ_VERSION := 4.13.2
YQ := $(shell command -v $(LOCALBIN)/yq 2> /dev/null)

HELM_VERSION ?= v3.9.3
HELM ?= $(LOCALBIN)/helm
HELM_BINARY_URL := https://get.helm.sh/helm-$(HELM_VERSION)-$(OS)-$(ARCH).tar.gz

.PHONY: clean all build test print-version

all: test build

print-version:
	@echo "GIT_VERSION is $(GIT_VERSION), IMAGE_TAG is $(IMAGE_TAG)"

# Build binaries in the host environment
build:
	GOPROXY=$(GOPROXY) GIT_VERSION=$(GIT_VERSION) bash hack/make-rules/build.sh $(WHAT)

# Run test
test:
	go test -v ./pkg/... ./cmd/... -coverprofile cover.out
	go test -v -coverpkg=./pkg/yurttunnel/...  -coverprofile=yurttunnel-cover.out ./test/integration/yurttunnel_test.go

clean:
	-rm -Rf _output

# verify will verify the code.
verify: verify-mod verify-license verify_manifests

verify_manifests:
	hack/make-rules/verify_manifests.sh

# verify-license will check if license has been added to files. 
verify-license:
	hack/make-rules/check_license.sh

# verify-mod will check if go.mod has been tidied.
verify-mod:
	hack/make-rules/verify_mod.sh

# Start up OpenYurt cluster on local machine based on a Kind cluster
local-up-openyurt: install-helm
	KUBERNETESVERSION=${KUBERNETESVERSION} YURT_VERSION=$(GIT_VERSION) bash hack/make-rules/local-up-openyurt.sh

# Build all OpenYurt components images and then start up OpenYurt cluster on local machine based on a Kind cluster
# And you can run the following command on different env by specify TARGET_PLATFORMS, default platform is linux/amd64
#   - on centos env: make docker-build-and-up-openyurt
#   - on MACBook Pro M1: make docker-build-and-up-openyurt TARGET_PLATFORMS=linux/arm64
docker-build-and-up-openyurt: docker-build
	KUBERNETESVERSION=${KUBERNETESVERSION} YURT_VERSION=$(GIT_VERSION) bash hack/make-rules/local-up-openyurt.sh

# Start up e2e tests for OpenYurt
# And you can run the following command on different env by specify TARGET_PLATFORMS, default platform is linux/amd64
#   - on centos env: make e2e-tests
#   - on MACBook Pro M1: make e2e-tests TARGET_PLATFORMS=linux/arm64
e2e-tests:
	ENABLE_AUTONOMY_TESTS=${ENABLE_AUTONOMY_TESTS} TARGET_PLATFORMS=${TARGET_PLATFORMS} hack/make-rules/run-e2e-tests.sh


install-helm: $(LOCALBIN)
	@echo "Checking Helm installation..."
	@HELM_CURRENT_VERSION=$$($(HELM) version --template="{{ .Version }}" 2>/dev/null || echo ""); \
	if [ "$$HELM_CURRENT_VERSION" != "$(HELM_VERSION)" ]; then \
		echo "Installing or upgrading Helm to version $(HELM_VERSION) into $(LOCALBIN)"; \
		curl -fsSL -o helm.tar.gz "$(HELM_BINARY_URL)"; \
		tar -xzf helm.tar.gz; \
		mv $(OS)-$(ARCH)/helm $(HELM); \
		rm -rf $(OS)-$(ARCH); \
		rm helm.tar.gz; \
	else \
		echo "Helm version $(HELM_VERSION) is already installed."; \
	fi

install-golint: ## check golint if not exist install golint tools
ifeq ($(shell $(GLOBAL_GOLANGCILINT) version --format short), $(GOLANGCILINT_VERSION))
GOLINT_BIN=$(GLOBAL_GOLANGCILINT)
else ifeq ($(shell $(GOBIN_GOLANGCILINT) version --format short), $(GOLANGCILINT_VERSION))
GOLINT_BIN=$(GOBIN_GOLANGCILINT)
else
	@{ \
    set -e ;\
    echo 'installing golangci-lint-$(GOLANGCILINT_VERSION)' ;\
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCILINT_VERSION) ;\
    echo 'Successfully installed' ;\
    }
GOLINT_BIN=$(GOBIN)/golangci-lint
endif

lint: install-golint ## Run go lint against code.
	$(GOLINT_BIN) run -v

# Build the docker images only one arch(specify arch by TARGET_PLATFORMS env)
# otherwise the platform of host will be used. 
# e.g.
#     - build linux/amd64 docker images:
#       $# make docker-build TARGET_PLATFORMS=linux/amd64
#     - build linux/arm64 docker images:
#       $# make docker-build TARGET_PLATFORMS=linux/arm64
#     - build a specific image:
#       $# make docker-build WHAT=yurthub
#     - build with proxy, maybe useful for Chinese users
#       $# REGION=cn make docker-build
docker-build:
	TARGET_PLATFORMS=${TARGET_PLATFORMS} IMAGE_REPO=$(IMAGE_REPO) IMAGE_TAG=$(IMAGE_TAG) GIT_VERSION=$(GIT_VERSION) hack/make-rules/image_build.sh $(WHAT)


# Build and Push the docker images with multi-arch
docker-push: docker-push-yurthub docker-push-node-servant docker-push-yurt-manager docker-push-yurt-tunnel-server docker-push-yurt-tunnel-agent docker-push-yurt-iot-dock
	@echo "release openyurt images completed~~"

docker-buildx-builder:
	if ! docker buildx ls | grep -q container-builder; then\
		docker buildx create --name container-builder --use;\
	fi
	# enable qemu for arm64 build
	# https://github.com/docker/buildx/issues/464#issuecomment-741507760
	docker run --privileged --rm tonistiigi/binfmt --uninstall qemu-aarch64
	docker run --rm --privileged tonistiigi/binfmt --install all

docker-push-yurthub: docker-buildx-builder
	docker buildx build --no-cache --push ${DOCKER_BUILD_ARGS}  --platform ${TARGET_PLATFORMS} -f hack/dockerfiles/release/Dockerfile.yurthub . -t ${IMAGE_REPO}/yurthub:${IMAGE_TAG}

docker-push-node-servant: docker-buildx-builder
	docker buildx build --no-cache --push ${DOCKER_BUILD_ARGS}  --platform ${TARGET_PLATFORMS} -f hack/dockerfiles/release/Dockerfile.node-servant . -t ${IMAGE_REPO}/node-servant:${IMAGE_TAG}

docker-push-yurt-manager: docker-buildx-builder
	docker buildx build --no-cache --push ${DOCKER_BUILD_ARGS}  --platform ${TARGET_PLATFORMS} -f hack/dockerfiles/release/Dockerfile.yurt-manager . -t ${IMAGE_REPO}/yurt-manager:${IMAGE_TAG}

docker-push-yurt-tunnel-server: docker-buildx-builder
	docker buildx build --no-cache --push ${DOCKER_BUILD_ARGS}  --platform ${TARGET_PLATFORMS} -f hack/dockerfiles/release/Dockerfile.yurt-tunnel-server . -t ${IMAGE_REPO}/yurt-tunnel-server:${IMAGE_TAG}

docker-push-yurt-tunnel-agent: docker-buildx-builder
	docker buildx build --no-cache --push ${DOCKER_BUILD_ARGS}  --platform ${TARGET_PLATFORMS} -f hack/dockerfiles/release/Dockerfile.yurt-tunnel-agent . -t ${IMAGE_REPO}/yurt-tunnel-agent:${IMAGE_TAG}

docker-push-yurt-iot-dock: docker-buildx-builder
	docker buildx build --no-cache --push ${DOCKER_BUILD_ARGS}  --platform ${TARGET_PLATFORMS} -f hack/dockerfiles/release/Dockerfile.yurt-iot-dock . -t ${IMAGE_REPO}/yurt-iot-dock:${IMAGE_TAG}

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
#	hack/make-rule/generate_openapi.sh // TODO by kadisi
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./pkg/apis/..."

.PHONY: manifests
manifests: kustomize kubectl yq generate ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	rm -rf $(BUILD_KUSTOMIZE)
	hack/make-rules/generate_manifests.sh
	hack/make-rules/kustomize_to_chart.sh --crd $(BUILD_KUSTOMIZE)/auto_generate/crd  --webhook $(BUILD_KUSTOMIZE)/auto_generate/webhook --rbac $(BUILD_KUSTOMIZE)/auto_generate/rbac --output $(BUILD_KUSTOMIZE)/kustomize --chartDir charts/yurt-manager

# newcontroller
# .e.g
# make newcontroller GROUP=apps VERSION=v1beta1 KIND=example SHORTNAME=examples SCOPE=Namespaced 
# make newcontroller GROUP=apps VERSION=v1beta1 KIND=example SHORTNAME=examples SCOPE=Cluster
.PHONY: newcontroller
newcontroller:
	hack/make-rules/add_controller.sh --group $(GROUP) --version $(VERSION) --kind $(KIND) --shortname $(SHORTNAME) --scope $(SCOPE)

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
.PHONY: controller-gen
controller-gen: ## Download controller-gen locally if necessary.
ifeq ("$(shell $(CONTROLLER_GEN) --version 2> /dev/null)", "Version: v0.16.5")
else
	rm -rf $(CONTROLLER_GEN)
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.5)
endif

.PHONY: kubectl
kubectl: $(KUBECTL) ## Download kustomize locally if necessary. If wrong version is installed, it will be removed before downloading.
$(KUBECTL): $(LOCALBIN)
	@if test -x $(LOCALBIN)/kubectl && ! $(LOCALBIN)/kubectl version | grep -q $(KUBECTL_VERSION); then \
		echo "$(LOCALBIN)/kubectl version is not expected $(KUBECTL_VERSION). Removing it before installing."; \
		rm -rf $(LOCALBIN)/kubectl; \
	fi
	test -s $(LOCALBIN)/kubectl || curl https://storage.googleapis.com/kubernetes-release/release/$(KUBECTL_VERSION)/bin/$(shell go env GOOS)/$(shell go env GOARCH)/kubectl -o $(KUBECTL)
	chmod +x $(KUBECTL)

KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary. If wrong version is installed, it will be removed before downloading.
$(KUSTOMIZE): $(LOCALBIN)
	@if test -x $(LOCALBIN)/kustomize && ! $(LOCALBIN)/kustomize version | grep -q $(KUSTOMIZE_VERSION); then \
		echo "$(LOCALBIN)/kustomize version is not expected $(KUSTOMIZE_VERSION). Removing it before installing."; \
		rm -f $(LOCALBIN)/kustomize; \
	fi
	test -s $(LOCALBIN)/kustomize || { curl -Ss $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN); }

.PHONY: yq
yq:
ifndef YQ
	@echo "Installing yq..."
	test -s $(LOCALBIN)/yq || curl -k -L https://github.com/mikefarah/yq/releases/download/v${YQ_VERSION}/yq_$(shell go env GOOS)_$(shell go env GOARCH) -o $(LOCALBIN)/yq
	chmod +x $(LOCALBIN)/yq
else
	@echo "yq is already installed"
endif

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go install $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

fmt:
	go fmt ./...
	find . -name '*.go' | grep -Ev 'vendor|thrift_gen' | xargs goimports -w

vet:
	GO111MODULE=${GO_MODULE} go list ./... | grep -v "vendor" | xargs go vet
