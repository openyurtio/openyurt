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

# Read-only. Waits for the convert/revert round that YurtNodeConversionController
# (OpenYurt v1.7.0) starts after a label change.
#
#   baseline: run BEFORE changing labels; prints the UID of the current
#             node-servant-conversion-<node> Job (or "none").
#   wait:     identifies the round by a Job whose UID differs from the
#             baseline and whose command matches --action. Failure is decided
#             only by that Job reaching Failed (re-read to confirm UID and
#             action); success only by isConvertedStable/isRevertedStable.
#             The node condition is printed for diagnosis, never used to fail,
#             so a stale ConvertFailed/RevertFailed from an earlier round
#             cannot end the wait early.
#
# Exit codes: 0 STABLE|ALREADY_STABLE, 1 JOB_FAILED, 2 usage/env error,
#             3 TIMEOUT, 4 NOT_STARTED.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "${SCRIPT_DIR}/lib.sh"

usage() {
  cat <<'EOF'
Usage:
  wait-node-conversion.sh baseline --context <kube-context> <node>
  wait-node-conversion.sh wait --context <kube-context> <node> --action convert|revert
                          --baseline <job-uid|none> [--start-timeout 120] [--timeout 900]
EOF
}

CMD="${1:-}"
[[ $# -gt 0 ]] && shift
NODE=""; ACTION=""; BASELINE=""; START_TIMEOUT=120; TIMEOUT=900
while [[ $# -gt 0 ]]; do
  case "$1" in
    --context) NY_CTX="${2:-}"; shift 2 ;;
    --action) ACTION="${2:-}"; shift 2 ;;
    --baseline) BASELINE="${2:-}"; shift 2 ;;
    --start-timeout) START_TIMEOUT="${2:-}"; shift 2 ;;
    --timeout) TIMEOUT="${2:-}"; shift 2 ;;
    -h | --help) usage; exit 0 ;;
    -*) usage >&2; ny_die 2 USAGE_ERROR "unknown flag: $1" ;;
    *) NODE="$1"; shift ;;
  esac
done
case "$CMD" in baseline | wait) ;; -h | --help) usage; exit 0 ;; *) usage >&2; ny_die 2 USAGE_ERROR "first argument must be 'baseline' or 'wait'" ;; esac
[[ -n "$NODE" ]] || ny_die 2 USAGE_ERROR "node name is required"
ny_require_tools kubectl
ny_require_context
JOB=$(ny_job_name "$NODE")

if [[ "$CMD" = "baseline" ]]; then
  ny_job_snapshot "$JOB" || ny_die 2 ENV_ERROR "cannot read Job ${NY_NS}/${JOB}"
  ny_node_snapshot "$NODE" || ny_die 2 ENV_ERROR "node not found: $NODE"
  ny_kv NODE "$NODE"
  ny_kv JOB "${NY_NS}/${JOB}"
  ny_kv JOB_STATE "${JOB_STATE:-none}"
  ny_kv CONDITION "${COND_STATUS:-none}/${COND_REASON:-none}"
  ny_kv BASELINE "${JOB_UID}"
  exit 0
fi

case "$ACTION" in convert | revert) ;; *) ny_die 2 USAGE_ERROR "--action must be convert or revert" ;; esac
[[ -n "$BASELINE" ]] || ny_die 2 USAGE_ERROR "--baseline is required (run the baseline subcommand before changing labels)"
for n in "$START_TIMEOUT" "$TIMEOUT"; do
  case "$n" in
    '' | *[!0-9]*) ny_die 2 USAGE_ERROR "timeouts must be integers" ;;
    *) ;;
  esac
done

stable() {
  if [[ "$ACTION" = "convert" ]]; then ny_is_converted_stable; else ny_is_reverted_stable; fi
}

ny_node_snapshot "$NODE" || ny_die 2 ENV_ERROR "node not found: $NODE"
if stable; then
  ny_kv CONDITION "${COND_STATUS}/${COND_REASON}"
  ny_kv RESULT ALREADY_STABLE
  exit 0
fi

ROUND_UID=""
last=""
start_deadline=$((SECONDS + START_TIMEOUT))
deadline=$((SECONDS + TIMEOUT))
while :; do
  ny_node_snapshot "$NODE" || ny_die 2 ENV_ERROR "node disappeared: $NODE"
  if stable; then
    ny_kv ROUND_JOB_UID "${ROUND_UID:-not-observed}"
    ny_kv CONDITION "${COND_STATUS}/${COND_REASON}"
    ny_kv RESULT STABLE
    exit 0
  fi

  ny_job_snapshot "$JOB" || true
  if [[ -z "$ROUND_UID" ]] && [[ "${JOB_EXISTS}" = "yes" ]] && [[ "${JOB_UID}" != "$BASELINE" ]] && [[ "${JOB_ACTION}" = "$ACTION" ]]; then
    ROUND_UID="${JOB_UID}"
    ny_kv ROUND_JOB_UID "$ROUND_UID"
  fi

  if [[ -n "$ROUND_UID" ]] && [[ "${JOB_UID}" = "$ROUND_UID" ]] && [[ "${JOB_STATE}" = "failed" ]]; then
    # re-read so that a single inconsistent snapshot cannot decide the result
    ny_job_snapshot "$JOB" || true
    if [[ "${JOB_UID}" = "$ROUND_UID" ]] && [[ "${JOB_ACTION}" = "$ACTION" ]] && [[ "${JOB_STATE}" = "failed" ]]; then
      ny_kv CONDITION "${COND_STATUS}/${COND_REASON}"
      ny_kv CONDITION_MESSAGE "${COND_MSG}"
      ny_kv RESULT JOB_FAILED
      exit 1
    fi
  fi

  line="nodepool=${NODE_NP:-} desired=${NODE_DESIRED:-} edge=${NODE_EDGE:-} unschedulable=${NODE_UNSCHED} condition=${COND_STATUS:-}/${COND_REASON:-} job=${JOB_STATE:-none}:${JOB_ACTION:-}"
  if [[ "$line" != "$last" ]]; then
    echo "STATE $line"
    last="$line"
  fi

  if [[ -z "$ROUND_UID" ]] && [[ "$SECONDS" -ge "$start_deadline" ]]; then
    ny_kv JOB_STATE "${JOB_STATE:-none}"
    ny_kv RESULT NOT_STARTED
    exit 4
  fi
  if [[ "$SECONDS" -ge "$deadline" ]]; then
    ny_kv RESULT TIMEOUT
    exit 3
  fi
  sleep 5
done
