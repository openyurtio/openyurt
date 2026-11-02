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

# MUTATING. Out-of-band host cleanup for a node whose conversion failed and was
# abandoned (recovery flow B, "abandon"): runs `node-servant revert` once on the
# node through a Job that YurtNodeConversionController does not track.
#
# Naming: the Job is node-servant-conversion-<node>-rv-<4 hex>. It MUST keep the
# node-servant-conversion-<node> prefix: revert restarts every non-pause
# container on the node except pods whose name starts with that prefix
# (RestartNonPauseContainers in pkg/node-servant/components/workload.go), so
# the prefix is what keeps the cleanup container from stopping itself. The Job
# carries no openyurt.io/conversion-node label so the controller ignores it.
#
# This script never uncordons the node and never deletes Jobs on failure.
#
# Exit codes: 0 CLEANED, 1 CLEANUP_FAILED, 2 usage/env error or precondition
#             not met (REFUSED), 3 CLEANUP_TIMEOUT.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "${SCRIPT_DIR}/lib.sh"

usage() {
  cat <<'EOF'
Usage: cleanup-node.sh --context <kube-context> <node> [--image <node-servant-image>]
                       [--timeout 600] [--diag-dir ./openyurt-diag]

  --image     defaults to the --node-servant-image of the running yurt-manager
  --timeout   activeDeadlineSeconds of the cleanup Job; the client waits timeout+60s
EOF
}

NODE=""; IMAGE=""; TIMEOUT=600; DIAG_DIR="./openyurt-diag"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --context) NY_CTX="${2:-}"; shift 2 ;;
    --image) IMAGE="${2:-}"; shift 2 ;;
    --timeout) TIMEOUT="${2:-}"; shift 2 ;;
    --diag-dir) DIAG_DIR="${2:-}"; shift 2 ;;
    -h | --help) usage; exit 0 ;;
    -*) usage >&2; ny_die 2 USAGE_ERROR "unknown flag: $1" ;;
    *) NODE="$1"; shift ;;
  esac
done
[[ -n "$NODE" ]] || ny_die 2 USAGE_ERROR "node name is required"
case "$TIMEOUT" in
  '' | *[!0-9]*) ny_die 2 USAGE_ERROR "--timeout must be an integer" ;;
  *) ;;
esac
ny_require_tools kubectl
ny_require_context

refuse() {
  local reason="$1"
  ny_die 2 REFUSED "$reason"
}

# --- preconditions ---------------------------------------------------------

PREFIX=$(ny_job_name "$NODE")
CLEANUP_JOB="${PREFIX}-${NY_CLEANUP_INFIX}-$(ny_rand 4)"
# Job names are limited to 63 characters; the generated Pod name keeps only the
# first 58 characters of "<job>-" (MaxGeneratedNameLength), so the prefix must
# fit in 58 characters for the self-exclusion to match.
[[ "${#CLEANUP_JOB}" -le 63 ]] || refuse "cleanup Job name ${CLEANUP_JOB} exceeds 63 characters; clean the host manually"
[[ "${#PREFIX}" -le 58 ]] || refuse "prefix ${PREFIX} exceeds 58 characters; node-servant self-exclusion would not match"

ny_node_snapshot "$NODE" || refuse "node not found: $NODE"
[[ -z "${NODE_NP}" ]] || refuse "node still has ${NY_LABEL_NODEPOOL}=${NODE_NP}; remove both pool labels first (conversion not stopped)"
[[ -z "${NODE_DESIRED}" ]] || refuse "node still has ${NY_LABEL_DESIRED}=${NODE_DESIRED}; remove both pool labels first"
[[ "${NODE_EDGE}" != "true" ]] || refuse "node has ${NY_LABEL_EDGE}=true; this is a converted node, use the normal revert or the RevertFailed flow"
[[ "${NODE_UNSCHED}" = "true" ]] || refuse "node is not cordoned; cordon it before cleaning the host"

ny_job_snapshot "$PREFIX" || refuse "cannot read Job ${NY_NS}/${PREFIX}"
if [[ "${JOB_EXISTS}" = "yes" ]]; then
  case "${JOB_STATE}" in succeeded | failed) ;; *) refuse "conversion Job ${PREFIX} is still ${JOB_STATE}" ;; esac
fi
active=$(ny_job_active_pods "$PREFIX")
[[ "${active:-0}" -eq 0 ]] || refuse "conversion Job ${PREFIX} still has ${active} active pod(s)"
if ny_cleanup_jobs "$NODE" | grep -qE ' (active|pending)$'; then
  refuse "another cleanup Job is running for ${NODE}"
fi

if [[ -z "$IMAGE" ]]; then
  while IFS= read -r a; do
    case "$a" in
      --node-servant-image=*) IMAGE="${a#--node-servant-image=}" ;;
      *) ;;
    esac
  done < <(k -n "$NY_NS" get deployment yurt-manager --ignore-not-found \
    -o jsonpath='{range .spec.template.spec.containers[?(@.name=="yurt-manager")].args[*]}{@}{"\n"}{end}')
fi
[[ -n "$IMAGE" ]] || refuse "cannot determine the node-servant image; pass --image"
image_ref="${IMAGE##*/}"
case "$image_ref" in *:* | *@*) ;; *) refuse "image $IMAGE has no tag; pass a pinned --image" ;; esac
case "$IMAGE" in
  *:latest) refuse "image $IMAGE uses 'latest'; pass a pinned --image (e.g. openyurt/node-servant:v1.7.0)" ;;
  *) ;;
esac

ny_kv CONTEXT "$NY_CTX"
ny_kv NODE "$NODE"
ny_kv CLEANUP_JOB "${NY_NS}/${CLEANUP_JOB}"
ny_kv IMAGE "$IMAGE"

# --- run -------------------------------------------------------------------

# Same pod spec as ServantJobTemplate (pkg/node-servant/constant.go, v1.7.0)
# with the revert command from buildRevertCommand (pkg/node-servant/job.go),
# minus the conversion-node label, plus a bounded lifetime.
k create -f - >/dev/null <<EOF || ny_die 2 ENV_ERROR "failed to create cleanup Job"
apiVersion: batch/v1
kind: Job
metadata:
  name: ${CLEANUP_JOB}
  namespace: ${NY_NS}
  labels:
    app.kubernetes.io/name: openyurt-node-cleanup
spec:
  backoffLimit: 0
  activeDeadlineSeconds: ${TIMEOUT}
  ttlSecondsAfterFinished: 7200
  template:
    metadata:
      labels:
        app.kubernetes.io/name: openyurt-node-cleanup
    spec:
      hostPID: true
      hostNetwork: true
      restartPolicy: Never
      nodeName: ${NODE}
      tolerations:
      - operator: Exists
        effect: NoSchedule
      - operator: Exists
        effect: NoExecute
      volumes:
      - name: host-root
        hostPath:
          path: /
          type: Directory
      containers:
      - name: node-servant
        image: ${IMAGE}
        imagePullPolicy: IfNotPresent
        command:
        - /bin/sh
        - -c
        args:
        - "/usr/local/bin/entry.sh revert --node-name=${NODE}"
        securityContext:
          privileged: true
        volumeMounts:
        - mountPath: /openyurt
          name: host-root
        env:
        - name: NODE_NAME
          valueFrom:
            fieldRef:
              fieldPath: spec.nodeName
EOF

write_diag() {
  local file
  mkdir -p "$DIAG_DIR"
  file="${DIAG_DIR}/${NODE}-$(date +%Y%m%dT%H%M%S).txt"
  {
    echo "# cleanup diagnostics for ${NODE} (context ${NY_CTX})"
    echo "## job"; k -n "$NY_NS" describe job "$CLEANUP_JOB" 2>&1 || true
    echo "## pods"; k -n "$NY_NS" get pods -l "job-name=${CLEANUP_JOB}" -o wide 2>&1 || true
    echo "## logs"; k -n "$NY_NS" logs "job/${CLEANUP_JOB}" --tail=500 2>&1 || true
    echo "## events"; k -n "$NY_NS" get events --field-selector involvedObject.kind=Pod 2>&1 | grep -- "$CLEANUP_JOB" || true
    echo "## node"; k get node "$NODE" -o yaml 2>&1 || true
  } >"$file"
  ny_kv DIAGNOSTICS "$file"
}

deadline=$((SECONDS + TIMEOUT + 60))
while :; do
  ny_job_snapshot "$CLEANUP_JOB" || true
  case "${JOB_STATE}" in
    succeeded | failed) break ;;
    *) ;;
  esac
  if [[ "$SECONDS" -ge "$deadline" ]]; then
    write_diag
    ny_kv NODE_LEFT_CORDONED true
    ny_kv BLOCKS_UNINSTALL true
    ny_kv RESULT CLEANUP_TIMEOUT
    exit 3
  fi
  sleep 5
done

if [[ "${JOB_STATE}" = "succeeded" ]]; then
  excluded=$(k -n "$NY_NS" logs "job/${CLEANUP_JOB}" 2>/dev/null | sed -n 's/.*skippedExcluded=\([0-9][0-9]*\).*/\1/p' | tail -n1)
  if [[ -n "$excluded" ]] && [[ "$excluded" -ge 1 ]]; then ny_kv SELF_EXCLUDED yes; else ny_kv SELF_EXCLUDED unknown; fi
  ny_kv NEXT "delete Jobs ${PREFIX} and ${CLEANUP_JOB}, then uncordon ${NODE} after user confirmation"
  ny_kv RESULT CLEANED
  exit 0
fi

reason=$(k -n "$NY_NS" get job "$CLEANUP_JOB" -o jsonpath='{.status.conditions[?(@.type=="Failed")].reason}' 2>/dev/null || true)
write_diag
ny_kv NODE_LEFT_CORDONED true
ny_kv BLOCKS_UNINSTALL true
ny_kv POSSIBLE_HOST_LEFTOVERS "/etc/systemd/system/yurthub.service* /usr/local/bin/yurthub /var/lib/openyurt/kubelet.conf kubeadm-flags.env redirect"
if [[ "$reason" = "DeadlineExceeded" ]]; then
  ny_kv RESULT CLEANUP_TIMEOUT
  exit 3
fi
ny_kv RESULT CLEANUP_FAILED
exit 1
