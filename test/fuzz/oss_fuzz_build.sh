# Copyright 2022 The OpenYurt Authors.
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

#!/usr/bin/env bash

set -euxo pipefail

GOPATH="${GOPATH:-/root/go}"
GO_SRC="${GOPATH}/src"
PROJECT_PATH="github.com/openyurtio/openyurt"

cd "${GO_SRC}"

# Move fuzzer to their respective directories.
# This removes dependency noises from the modules' go.mod and go.sum files.
cp "${PROJECT_PATH}/test/fuzz/yurtappdaemon_fuzzer.go" "${PROJECT_PATH}/pkg/yurtmanager/controller/yurtappdaemon/yurtappdaemon_fuzzer.go"
cp "${PROJECT_PATH}/test/fuzz/yurtappset_fuzzer.go" "${PROJECT_PATH}/pkg/yurtmanager/controller/yurtappset/yurtappset_fuzzer.go"

# compile fuzz test for the runtime module
pushd "${PROJECT_PATH}"

go get -d github.com/AdaLogics/go-fuzz-headers
go mod vendor
go mod tidy
compile_go_fuzzer "${PROJECT_PATH}/pkg/yurtmanager/controller/yurtappdaemon/" FuzzAppDaemonReconcile fuzz_yurtappdaemon_controller
compile_go_fuzzer "${PROJECT_PATH}/pkg/yurtmanager/controller/yurtappset/" FuzzAppSetReconcile fuzz_yurtappset_controller

popd
