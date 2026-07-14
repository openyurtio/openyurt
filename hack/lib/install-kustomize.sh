#!/usr/bin/env bash

# Copyright 2026 The OpenYurt Authors.
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

usage() {
    cat <<'EOF'
Usage: hack/lib/install-kustomize.sh <version-without-leading-v> <destination-dir>
EOF
    return 0
}

checksum_command() {
    if command -v sha256sum >/dev/null 2>&1; then
        echo "sha256sum"
        return
    fi

    if command -v shasum >/dev/null 2>&1; then
        echo "shasum -a 256"
        return
    fi

    echo "missing checksum tool: need sha256sum or shasum" >&2
    exit 1
}

main() {
    if [[ $# -ne 2 ]]; then
        usage >&2
        exit 1
    fi

    local version="$1"
    local destination_dir="$2"
    local os
    local arch
    os=$(go env GOOS)
    arch=$(go env GOARCH)

    mkdir -p "${destination_dir}"

    local release_tag="kustomize/v${version}"
    local asset_name="kustomize_v${version}_${os}_${arch}.tar.gz"
    local base_url="https://github.com/kubernetes-sigs/kustomize/releases/download/${release_tag}"
    local archive="${destination_dir}/${asset_name}"
    local checksums="${destination_dir}/checksums.txt"
    local checksum_tool
    checksum_tool=$(checksum_command)

    curl -fsSL -o "${archive}" "${base_url}/${asset_name}"
    curl -fsSL -o "${checksums}" "${base_url}/checksums.txt"

    (
        cd "${destination_dir}"
        grep " ${asset_name}\$" checksums.txt | ${checksum_tool} -c -
    )

    tar -xzf "${archive}" -C "${destination_dir}" kustomize
    chmod +x "${destination_dir}/kustomize"
    rm -f "${archive}" "${checksums}"
    return 0
}

main "$@"
