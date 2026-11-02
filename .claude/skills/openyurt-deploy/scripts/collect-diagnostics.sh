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

# Read-only diagnostics for OpenYurt v1.7.0 node conversion.
#
#   collect-diagnostics.sh --context <ctx> <node>   one node in depth
#   collect-diagnostics.sh --context <ctx> --summary  classify every node
#
# The summary prints one NODE line per node with the recovery state used by
# references/openyurt-deploy-recovery.md and ends with BLOCKS_UNINSTALL=.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "${SCRIPT_DIR}/lib.sh"

usage() {
  cat <<'EOF'
Usage: collect-diagnostics.sh --context <kube-context> (<node> | --summary) [--tail 200]
EOF
}

NODE=""; SUMMARY=no; TAIL=200
while [ $# -gt 0 ]; do
  case "$1" in
    --context) NY_CTX="${2:-}"; shift 2 ;;
    --summary) SUMMARY=yes; shift ;;
    --tail) TAIL="${2:-}"; shift 2 ;;
    -h | --help) usage; exit 0 ;;
    -*) usage >&2; ny_die 2 USAGE_ERROR "unknown flag: $1" ;;
    *) NODE="$1"; shift ;;
  esac
done
ny_require_tools kubectl
ny_require_context
if [ "$SUMMARY" = no ] && [ -z "$NODE" ]; then usage >&2; ny_die 2 USAGE_ERROR "node name or --summary is required"; fi

if [ "$SUMMARY" = yes ]; then
  blocks=no
  while IFS= read -r n; do
    [ -n "$n" ] || continue
    ny_node_snapshot "$n" || continue
    ny_classify_node "$n" || continue
    echo "NODE name=$n state=${NODE_STATE} nodepool=${NODE_NP:-} desired=${NODE_DESIRED:-} edge=${NODE_EDGE:-} unschedulable=${NODE_UNSCHED} condition=${COND_STATUS:-}/${COND_REASON:-} job=${JOB_STATE:-none}:${JOB_ACTION:-}${NODE_STATE_NOTE:+ note=\"${NODE_STATE_NOTE}\"}"
    if ny_state_blocks_uninstall "${NODE_STATE}"; then blocks=yes; fi
  done < <(k get nodes -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}')
  ny_kv BLOCKS_UNINSTALL "$blocks"
  exit 0
fi

JOB=$(ny_job_name "$NODE")
section() { printf '\n===== %s =====\n' "$1"; }

ny_node_snapshot "$NODE" || ny_die 2 ENV_ERROR "node not found: $NODE"
ny_classify_node "$NODE"
section "summary"
ny_kv NODE "$NODE"
ny_kv STATE "${NODE_STATE}"
[ -z "${NODE_STATE_NOTE}" ] || ny_kv NOTE "${NODE_STATE_NOTE}"
ny_kv NODEPOOL "${NODE_NP:-}"
ny_kv DESIRED_NODEPOOL "${NODE_DESIRED:-}"
ny_kv EDGE_WORKER "${NODE_EDGE:-}"
ny_kv UNSCHEDULABLE "${NODE_UNSCHED}"
ny_kv CONDITION "${COND_STATUS:-}/${COND_REASON:-}"
ny_kv CONDITION_MESSAGE "${COND_MSG:-}"
ny_kv CONVERSION_JOB "${JOB_EXISTS}:${JOB_STATE:-}:${JOB_ACTION:-}:${JOB_UID}"

section "node conditions"
k get node "$NODE" -o jsonpath='{range .status.conditions[*]}{.type}{"\t"}{.status}{"\t"}{.reason}{"\t"}{.message}{"\n"}{end}' || true

if [ -n "${NODE_NP}" ]; then
  section "nodepool ${NODE_NP}"
  k get nodepools.apps.openyurt.io "${NODE_NP}" -o jsonpath='{.metadata.name}{" type="}{.spec.type}{" hostNetwork="}{.spec.hostNetwork}{"\n"}' 2>&1 || true
fi

section "conversion job ${NY_NS}/${JOB}"
if [ "${JOB_EXISTS}" = "yes" ]; then
  k -n "$NY_NS" describe job "$JOB" 2>&1 | tail -n 40 || true
  section "conversion job logs (tail ${TAIL})"
  k -n "$NY_NS" logs "job/${JOB}" --tail="$TAIL" 2>&1 || true
else
  echo "no conversion Job (finished Jobs are removed after ttlSecondsAfterFinished=7200)"
fi

section "cleanup jobs"
cleanups=$(ny_cleanup_jobs "$NODE")
if [ -n "$cleanups" ]; then
  printf '%s\n' "$cleanups"
  printf '%s\n' "$cleanups" | while read -r name _; do
    section "cleanup job logs ${name} (tail ${TAIL})"
    k -n "$NY_NS" logs "job/${name}" --tail="$TAIL" 2>&1 || true
  done
else
  echo "none"
fi

section "pods of conversion/cleanup jobs"
k -n "$NY_NS" get pods -o wide 2>/dev/null | grep -E "^NAME|^${JOB}" || echo "none"

section "events mentioning ${NODE}"
k get events -A --field-selector "involvedObject.name=${NODE}" 2>/dev/null | tail -n 30 || true
k -n "$NY_NS" get events 2>/dev/null | grep -- "${JOB}" | tail -n 30 || true

section "yurt-manager logs mentioning ${NODE} (tail ${TAIL})"
k -n "$NY_NS" logs deployment/yurt-manager --tail=5000 2>/dev/null | grep -F -- "$NODE" | tail -n "$TAIL" || echo "none (or yurt-manager not running)"

section "install check"
"${SCRIPT_DIR}/check-install.sh" --context "$NY_CTX" 2>&1 || true
