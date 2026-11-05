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

# Local only (no cluster access). Extracts the yurt-manager and yurthub charts
# exactly as tagged in v1.7.0 and renders the shared yurthub resources
# (templates/yurthub-cfg.yaml) for namespace kube-system, the same way the
# v1.7.0 e2e does (test/e2e/cmd/init/converter.go installYurthubByHelm).
#
# The charts must come from the tag: the working tree of a checkout may be on
# another branch, and the published openyurt-helm repository has no 1.7.0 chart.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "${SCRIPT_DIR}/lib.sh"

TAG="v1.7.0"
CHART_VERSION="1.7.0"
TARBALL_URL="https://github.com/openyurtio/openyurt/archive/refs/tags/${TAG}.tar.gz"

usage() {
  cat <<EOF
Usage: prepare-charts.sh --dest <empty-dir> [--repo <openyurt-git-checkout>]

  --repo   use 'git archive ${TAG}' from a local clone (no network); otherwise
           download ${TARBALL_URL}
EOF
}

DEST=""; REPO=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dest) DEST="${2:-}"; shift 2 ;;
    --repo) REPO="${2:-}"; shift 2 ;;
    -h | --help) usage; exit 0 ;;
    *) usage >&2; ny_die 2 USAGE_ERROR "unknown argument: $1" ;;
  esac
done
[[ -n "$DEST" ]] || ny_die 2 USAGE_ERROR "--dest is required"
ny_require_tools helm tar
mkdir -p "$DEST"
[[ -z "$(ls -A "$DEST")" ]] || ny_die 2 USAGE_ERROR "--dest must be an empty directory: $DEST"
DEST="$(cd "$DEST" && pwd)"

if [[ -n "$REPO" ]]; then
  ny_require_tools git
  git -C "$REPO" rev-parse -q --verify "refs/tags/${TAG}^{commit}" >/dev/null ||
    ny_die 2 ENV_ERROR "tag ${TAG} not found in ${REPO} (git fetch --tags first)"
  git -C "$REPO" archive "$TAG" charts/yurt-manager charts/yurthub | tar -x -C "$DEST"
  ny_kv SOURCE "git archive ${TAG} from ${REPO}"
else
  ny_require_tools curl
  tmp="$DEST/.download"
  mkdir -p "$tmp"
  curl -fsSL --proto '=https' --proto-redir '=https' "$TARBALL_URL" -o "$tmp/src.tar.gz" || ny_die 2 ENV_ERROR "download failed: $TARBALL_URL"
  tar -xzf "$tmp/src.tar.gz" -C "$tmp"
  src=$(find "$tmp" -mindepth 1 -maxdepth 1 -type d | head -n1)
  mkdir -p "$DEST/charts"
  cp -R "$src/charts/yurt-manager" "$src/charts/yurthub" "$DEST/charts/"
  rm -rf "$tmp"
  ny_kv SOURCE "$TARBALL_URL"
fi

for c in yurt-manager yurthub; do
  f="$DEST/charts/$c/Chart.yaml"
  [[ -f "$f" ]] || ny_die 2 ENV_ERROR "missing $f"
  grep -qx "version: ${CHART_VERSION}" "$f" || ny_die 2 ENV_ERROR "$f is not version ${CHART_VERSION}"
  grep -qx "appVersion: \"${TAG}\"" "$f" || ny_die 2 ENV_ERROR "$f is not appVersion ${TAG}"
done

MANIFEST="$DEST/yurthub-cfg.yaml"
helm template yurthub "$DEST/charts/yurthub" --namespace "$NY_NS" \
  --show-only templates/yurthub-cfg.yaml >"$MANIFEST"
grep -q "name: yurt-hub-cfg" "$MANIFEST" || ny_die 2 ENV_ERROR "rendered manifest has no yurt-hub-cfg ConfigMap"
grep -q "namespace: ${NY_NS}" "$MANIFEST" || ny_die 2 ENV_ERROR "rendered manifest is not in namespace ${NY_NS}"

ny_kv YURT_MANAGER_CHART "$DEST/charts/yurt-manager"
ny_kv YURTHUB_CFG_MANIFEST "$MANIFEST"
ny_kv RESULT READY
