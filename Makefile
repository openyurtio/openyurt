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

.PHONY: clean all release build

all: test build

# Build binaries in the host environment
build: 
	bash hack/make-rules/build.sh $(WHAT)

# generate yaml files 
gen-yaml:
	hack/make-rules/genyaml.sh $(WHAT)

# Run test
test: fmt vet
	go test ./pkg/... ./cmd/... -coverprofile cover.out
	go test -v  -coverpkg=./pkg/yurttunnel/...  -coverprofile=yurttunnel-cover.out ./test/integration/yurttunnel_test.go

# Run go fmt against code
fmt:
	go fmt ./pkg/... ./cmd/...

# Run go vet against code
vet:
	go vet ./pkg/... ./cmd/...

# Build binaries and docker images.  
# NOTE: this rule can take time, as we build binaries inside containers
#
# ARGS:
#   WHAT: list of components that will be compiled.
#   ARCH: list of target architectures.
#   REGION: in which region this rule is executed, if in mainland China,
#   	set it as cn.
#
# Examples:
#   # compile yurthub, yurt-controller-manager and yurtctl-servant with 
#   # architectures arm64 and arm in the mainland China
#   make release WHAT="yurthub yurt-controller-manager yurtctl-servant" ARCH="arm64 arm" REGION=cn
#
#   # compile all components with all architectures (i.e., amd64, arm64, arm)
#   make relase 
release:
	bash hack/make-rules/release-images.sh

clean: 
	-rm -Rf _output
	-rm -Rf dockerbuild

e2e: 
	hack/make-rules/build-e2e.sh 

# Generate manifests e.g. CRD, RBAC etc.
gen-app-manager-manifests: app-manager-controller-gen
	$(CONTROLLER_GEN) crd:trivialVersions=true rbac:roleName=manager-role webhook paths="./pkg/yurtappmanager/..." output:crd:artifacts:config=config/yurt-app-manager/crd/bases  output:rbac:artifacts:config=config/yurt-app-manager/rbac output:webhook:artifacts:config=config/yurt-app-manager/webhook

# Generate code
gen-app-manager-client: app-manager-controller-gen
	hack/make-rules/generate_client.sh
	$(CONTROLLER_GEN) object:headerFile="./pkg/yurtappmanager/hack/boilerplate.go.txt" paths="./pkg/yurtappmanager/apis/..."

# find or download controller-gen
# download controller-gen if necessary
app-manager-controller-gen:
ifeq (, $(shell which controller-gen-openyurt))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	echo "replace sigs.k8s.io/controller-tools => github.com/openkruise/controller-tools v0.2.9-kruise" >> go.mod ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.9 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	mv $(GOPATH)/bin/controller-gen $(GOPATH)/bin/controller-gen-openyurt ;\
	}
CONTROLLER_GEN=$(GOPATH)/bin/controller-gen-openyurt
else
CONTROLLER_GEN=$(shell which controller-gen-openyurt)
endif
