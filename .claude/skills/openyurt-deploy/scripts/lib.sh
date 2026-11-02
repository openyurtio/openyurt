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

# shellcheck disable=SC2034 # variables set here are consumed by the sourcing scripts

# Shared helpers for the openyurt-deploy skill scripts. Sourced, not executed.
# Every constant below mirrors OpenYurt v1.7.0 source; see the comments for
# the file each value comes from. Keep this file compatible with bash 3.2
# (macOS default): no associative arrays, no mapfile, no ${var,,}.

# pkg/node-servant/constant.go
NY_NS="kube-system"
NY_JOB_PREFIX="node-servant-conversion"
# pkg/projectinfo/projectinfo.go, pkg/apis/apps/well_known_labels_annotations.go
NY_LABEL_NODEPOOL="apps.openyurt.io/nodepool"
NY_LABEL_DESIRED="apps.openyurt.io/desired-nodepool"
NY_LABEL_EDGE="openyurt.io/is-edge-worker"
# suffix used by cleanup-node.sh; keeps the node-servant self-exclusion prefix
NY_CLEANUP_INFIX="rv"

NY_CTX=""

ny_kv() {
  # ny_kv KEY VALUE -> prints KEY=VALUE on one line
  printf '%s=%s\n' "$1" "$2"
}

ny_die() {
  # ny_die EXIT_CODE RESULT MESSAGE
  printf 'ERROR=%s\n' "$3" >&2
  ny_kv RESULT "$2"
  exit "$1"
}

k() {
  kubectl --context "${NY_CTX}" "$@"
}

ny_require_tools() {
  local t
  for t in "$@"; do
    command -v "$t" >/dev/null 2>&1 || ny_die 2 ENV_ERROR "required tool not found: $t"
  done
}

ny_require_context() {
  [ -n "${NY_CTX}" ] || ny_die 2 USAGE_ERROR "--context is required"
  kubectl config get-contexts "${NY_CTX}" >/dev/null 2>&1 ||
    ny_die 2 ENV_ERROR "kube context not found: ${NY_CTX}"
}

ny_job_name() {
  printf '%s-%s' "${NY_JOB_PREFIX}" "$1"
}

ny_rand() {
  # ny_rand LEN -> lowercase hex string (LEN <= 16). Reads a fixed number of
  # bytes so that no pipeline stage is killed by SIGPIPE under pipefail.
  od -An -N8 -tx1 /dev/urandom | tr -d ' \n' | cut -c "1-$1"
}

ny_dns_label() {
  # lowercase, replace characters invalid in a DNS label, trim to $2 chars
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9-' '-' | cut -c "1-$2" | sed 's/-*$//'
}

# ny_node_snapshot NODE
# Sets: NODE_NP NODE_DESIRED NODE_EDGE NODE_UNSCHED COND_STATUS COND_REASON COND_MSG
ny_node_snapshot() {
  local out
  out=$(k get node "$1" -o jsonpath='{.metadata.labels.apps\.openyurt\.io/nodepool}{"\n"}{.metadata.labels.apps\.openyurt\.io/desired-nodepool}{"\n"}{.metadata.labels.openyurt\.io/is-edge-worker}{"\n"}{.spec.unschedulable}{"\n"}{.status.conditions[?(@.type=="YurtNodeConversionFailed")].status}{"\n"}{.status.conditions[?(@.type=="YurtNodeConversionFailed")].reason}{"\n"}{.status.conditions[?(@.type=="YurtNodeConversionFailed")].message}') ||
    return 1
  {
    IFS= read -r NODE_NP
    IFS= read -r NODE_DESIRED
    IFS= read -r NODE_EDGE
    IFS= read -r NODE_UNSCHED
    IFS= read -r COND_STATUS
    IFS= read -r COND_REASON
    IFS= read -r COND_MSG
  } <<EOF
$out
EOF
  [ -n "${NODE_UNSCHED}" ] || NODE_UNSCHED="false"
  return 0
}

# ny_job_snapshot JOB_NAME
# Sets: JOB_EXISTS(yes|no) JOB_UID JOB_ACTION(convert|revert|unknown) JOB_STATE(active|succeeded|failed|pending)
ny_job_snapshot() {
  local out args conds active
  JOB_EXISTS="no"; JOB_UID="none"; JOB_ACTION=""; JOB_STATE=""
  out=$(k -n "${NY_NS}" get job "$1" --ignore-not-found -o jsonpath='{.metadata.uid}{"\n"}{.spec.template.spec.containers[0].args[0]}{"\n"}{.status.active}{"\n"}{range .status.conditions[?(@.status=="True")]}{.type}{","}{end}') ||
    return 1
  [ -n "$out" ] || return 0
  {
    IFS= read -r JOB_UID
    IFS= read -r args
    IFS= read -r active
    IFS= read -r conds
  } <<EOF
$out
EOF
  JOB_EXISTS="yes"
  # mirrors jobAction() in yurt_node_conversion_controller.go
  case "$args" in
    "/usr/local/bin/entry.sh convert"*) JOB_ACTION="convert" ;;
    "/usr/local/bin/entry.sh revert"*) JOB_ACTION="revert" ;;
    *) JOB_ACTION="unknown" ;;
  esac
  case ",$conds" in
    *,Failed,*) JOB_STATE="failed" ;;
    *,Complete,*) JOB_STATE="succeeded" ;;
    *)
      if [ -n "$active" ] && [ "$active" != "0" ]; then JOB_STATE="active"; else JOB_STATE="pending"; fi
      ;;
  esac
  return 0
}

# ny_job_active_pods JOB_NAME -> prints number of Pending/Running pods of the Job
ny_job_active_pods() {
  k -n "${NY_NS}" get pods -l "job-name=$1" \
    -o jsonpath='{range .items[*]}{.status.phase}{"\n"}{end}' 2>/dev/null |
    grep -cE '^(Pending|Running)$' || true
}

# isConvertedStable() in yurt_node_conversion_controller.go; needs ny_node_snapshot
ny_is_converted_stable() {
  [ -n "${NODE_NP}" ] && [ "${NODE_EDGE}" = "true" ] && [ "${NODE_UNSCHED}" != "true" ] &&
    [ "${COND_STATUS}" = "False" ] && [ "${COND_REASON}" = "Converted" ]
}

# isRevertedStable() in yurt_node_conversion_controller.go; needs ny_node_snapshot
ny_is_reverted_stable() {
  [ -z "${NODE_NP}" ] && [ "${NODE_EDGE}" != "true" ] && [ "${NODE_UNSCHED}" != "true" ] &&
    [ "${COND_STATUS}" = "False" ] && [ "${COND_REASON}" = "Reverted" ]
}

# ny_cleanup_jobs NODE -> prints "<name> <state>" for cleanup Jobs created by cleanup-node.sh
ny_cleanup_jobs() {
  local prefix name
  prefix="$(ny_job_name "$1")-${NY_CLEANUP_INFIX}-"
  k -n "${NY_NS}" get jobs -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null |
    while IFS= read -r name; do
      case "$name" in
        "$prefix"????)
          ny_job_snapshot "$name" >/dev/null 2>&1 || continue
          printf '%s %s\n' "$name" "${JOB_STATE}"
          ;;
      esac
    done
}

# ny_classify_node NODE -> sets NODE_STATE and NODE_STATE_NOTE; needs ny_node_snapshot first
# States (see references/openyurt-deploy-recovery.md):
#   NONE REVERTED CONVERTED CONVERTING REVERTING CONVERT_NOT_STARTED REVERT_NOT_STARTED
#   DESIRED_ONLY CONVERT_FAILED REVERT_FAILED CLEANING CLEANUP_FAILED CLEANUP_PENDING
#   CLEANUP_UNVERIFIED EDGE_WITHOUT_RECORD UNKNOWN
ny_classify_node() {
  local node="$1" cleanups
  NODE_STATE="UNKNOWN"; NODE_STATE_NOTE=""
  ny_job_snapshot "$(ny_job_name "$node")" || return 1
  cleanups=$(ny_cleanup_jobs "$node")

  if printf '%s\n' "$cleanups" | grep -q ' failed$'; then
    NODE_STATE="CLEANUP_FAILED"; NODE_STATE_NOTE="host cleanup Job failed; manual handling required"; return 0
  fi
  if printf '%s\n' "$cleanups" | grep -qE ' (active|pending)$'; then
    NODE_STATE="CLEANING"; return 0
  fi
  if [ "${JOB_STATE}" = "active" ] || [ "${JOB_STATE}" = "pending" ]; then
    if [ "${JOB_ACTION}" = "revert" ]; then NODE_STATE="REVERTING"; else NODE_STATE="CONVERTING"; fi
    return 0
  fi
  if ny_is_converted_stable; then NODE_STATE="CONVERTED"; return 0; fi
  if ny_is_reverted_stable; then NODE_STATE="REVERTED"; return 0; fi

  if [ "${COND_STATUS}" = "True" ] && [ "${COND_REASON}" = "RevertFailed" ]; then
    NODE_STATE="REVERT_FAILED"; NODE_STATE_NOTE="failed revert Job is retried automatically after its TTL expires"; return 0
  fi
  if [ "${COND_STATUS}" = "True" ] && [ "${COND_REASON}" = "ConvertFailed" ]; then
    if [ -n "${NODE_NP}" ] || [ -n "${NODE_DESIRED}" ]; then
      NODE_STATE="CONVERT_FAILED"; NODE_STATE_NOTE="labels still request conversion; controller retries after the failed Job's TTL expires"
    elif [ "${NODE_UNSCHED}" = "true" ]; then
      NODE_STATE="CLEANUP_PENDING"; NODE_STATE_NOTE="conversion stopped but host cleanup not done"
    else
      NODE_STATE="CLEANUP_UNVERIFIED"; NODE_STATE_NOTE="conversion stopped and node uncordoned; verify host artifacts with preflight-node.sh"
    fi
    return 0
  fi
  if [ -n "${NODE_NP}" ] && [ "${NODE_EDGE}" != "true" ]; then
    NODE_STATE="CONVERT_NOT_STARTED"; NODE_STATE_NOTE="nodepool label set but no conversion round; check NodePool type and yurt-manager"; return 0
  fi
  if [ -z "${NODE_NP}" ] && [ "${NODE_EDGE}" = "true" ]; then
    if [ -z "${COND_REASON}" ]; then
      NODE_STATE="EDGE_WITHOUT_RECORD"; NODE_STATE_NOTE="edge node without conversion record (e.g. joined by yurtadm); removing labels triggers a revert Job"
    else
      NODE_STATE="REVERT_NOT_STARTED"; NODE_STATE_NOTE="revert requested but no revert round observed"
    fi
    return 0
  fi
  if [ -z "${NODE_NP}" ] && [ -n "${NODE_DESIRED}" ]; then
    NODE_STATE="DESIRED_ONLY"; NODE_STATE_NOTE="desired-nodepool set but nodepool label missing (node webhook was not applied)"; return 0
  fi
  if [ -n "${NODE_NP}" ] && [ "${NODE_EDGE}" = "true" ] && [ -z "${COND_REASON}" ]; then
    NODE_STATE="EDGE_WITHOUT_RECORD"; NODE_STATE_NOTE="edge node without conversion record (e.g. joined by yurtadm)"; return 0
  fi
  if [ -z "${NODE_NP}" ] && [ -z "${NODE_DESIRED}" ] && [ "${NODE_EDGE}" != "true" ] && [ -z "${COND_REASON}" ]; then
    NODE_STATE="NONE"; return 0
  fi
  NODE_STATE_NOTE="nodepool=${NODE_NP} edge=${NODE_EDGE} unschedulable=${NODE_UNSCHED} condition=${COND_STATUS}/${COND_REASON}"
  return 0
}

# ny_state_blocks_uninstall STATE -> 0 when the state must be resolved before uninstalling
ny_state_blocks_uninstall() {
  case "$1" in
    NONE | REVERTED) return 1 ;;
    *) return 0 ;;
  esac
}
