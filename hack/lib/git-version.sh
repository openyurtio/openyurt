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
Usage: hack/lib/git-version.sh <latest-tag|version|image-tag|short-commit>
EOF
    return 0
}

latest_tag() {
    git describe --abbrev=0 --tags 2>/dev/null || true
    return 0
}

short_commit() {
    git rev-parse --short=7 HEAD
    return 0
}

version() {
    local tag
    tag=$(latest_tag)
    if [[ -n "${tag}" ]]; then
        echo "${tag}"
        return
    fi

    echo "dev-$(short_commit)"
    return 0
}

image_tag() {
    local tag
    tag=$(latest_tag)
    if [[ -z "${tag}" ]]; then
        echo "dev-$(short_commit)"
        return
    fi

    if git tag --points-at HEAD | grep -q .; then
        echo "${tag}"
        return
    fi

    echo "${tag}-$(short_commit)"
    return 0
}

main() {
    if [[ $# -ne 1 ]]; then
        usage >&2
        exit 1
    fi

    local command="$1"

    case "${command}" in
        latest-tag)
            latest_tag
            ;;
        version)
            version
            ;;
        image-tag)
            image_tag
            ;;
        short-commit)
            short_commit
            ;;
        *)
            usage >&2
            exit 1
            ;;
    esac

    return 0
}

main "$@"
