#!/usr/bin/env bash

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

set -o errexit
set -o nounset
set -o pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../" && pwd -P)"
YURT_PACKAGE="github.com/openyurtio/openyurt"
YURT_CLIENT_GO_PACKAGE="github.com/openyurtio/client-go"

#APIS_DIR_FROM_REPO="pkg/controller/apis"
APIS_DIR_FROM_REPO="pkg/apis/"
#OUTPUT_PKG="${YURT_PACKAGE}/staging/src/github.com/openyurtio/api/controller/client"
OUTPUT_PKG="${YURT_CLIENT_GO_PACKAGE}"
APIS_DIR=$PROJECT_DIR/$APIS_DIR_FROM_REPO
APIS_PKG="${YURT_PACKAGE}/${APIS_DIR_FROM_REPO}"

INPUT_DIRS=

for groupDir in $(ls $APIS_DIR) 
do
    if [ -d "$APIS_DIR/$groupDir" ]; then
        for versionDir in $(ls "$APIS_DIR/$groupDir") 
        do
            if [ -d "$APIS_DIR/$groupDir/$versionDir" ]; then
                if [ -z "${INPUT_DIRS}" ]; then
                    INPUT_DIRS="${APIS_PKG}/$groupDir/$versionDir"
                else
                    INPUT_DIRS="${INPUT_DIRS},${APIS_PKG}/$groupDir/$versionDir"
                fi
            fi
        done
    fi
done

gobin="$PROJECT_DIR/bin"

echo "Generating clientset for  at ${OUTPUT_PKG}/clientset"
"${gobin}/client-gen" \
    --go-header-file ${PROJECT_DIR}/hack/boilerplate.go.txt \
    --clientset-name "versioned" \
    --input-base "" \
    --input "${INPUT_DIRS}" \
    --output-package "${OUTPUT_PKG}/clientset" \
    "$@" 

echo "Generating defaulter functions"
"${gobin}/defaulter-gen" \
    --go-header-file ${PROJECT_DIR}/hack/boilerplate.go.txt \
    --input-dirs "${INPUT_DIRS}" \
    -O zz_generated.defaults 

echo "Generating listers for at ${OUTPUT_PKG}/listers"
"${gobin}/lister-gen" \
    --go-header-file ${PROJECT_DIR}/hack/boilerplate.go.txt \
    --input-dirs "${INPUT_DIRS}" \
    --output-package "${OUTPUT_PKG}/listers" 

echo "Generating informers for at ${OUTPUT_PKG}/informers"
"${gobin}/informer-gen" \
    --go-header-file ${PROJECT_DIR}/hack/boilerplate.go.txt \
    --input-dirs "${INPUT_DIRS}" \
    --versioned-clientset-package "${OUTPUT_PKG}/clientset/versioned" \
    --listers-package "${OUTPUT_PKG}/listers" \
    --output-package "${OUTPUT_PKG}/informers" 
