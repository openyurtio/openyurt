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

# Checks, from inside the cluster, that a node satisfies the host-level
# requirements of OpenYurt v1.7.0 label-driven node conversion, and reports
# OpenYurt artifacts already present on the host. Creates a short-lived Job in
# kube-system that mounts the host root read-only; no SSH is used.
#
# Exit codes: 0 PASS, 1 FAIL, 2 usage/environment error, 3 TIMEOUT.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "${SCRIPT_DIR}/lib.sh"

usage() {
  cat <<'EOF'
Usage: preflight-node.sh --context <kube-context> <node> [--timeout <seconds>] [--image <image>]

Runs a read-only host check on <node> through an ephemeral Job and prints one
CHECK/ARTIFACT line per item, followed by RESULT=PASS|FAIL|TIMEOUT.

  --timeout   total time budget in seconds (default 120)
  --image     image providing busybox sh/chroot (default busybox:1.36.1)
EOF
}

NODE=""
TIMEOUT=120
IMAGE="busybox:1.36.1"
while [[ $# -gt 0 ]]; do
  case "$1" in
    --context) NY_CTX="${2:-}"; shift 2 ;;
    --timeout) TIMEOUT="${2:-}"; shift 2 ;;
    --image) IMAGE="${2:-}"; shift 2 ;;
    -h | --help) usage; exit 0 ;;
    -*) usage >&2; ny_die 2 USAGE_ERROR "unknown flag: $1" ;;
    *) NODE="$1"; shift ;;
  esac
done
[[ -n "$NODE" ]] || { usage >&2; ny_die 2 USAGE_ERROR "node name is required"; }
case "$TIMEOUT" in
  '' | *[!0-9]*) ny_die 2 USAGE_ERROR "--timeout must be an integer" ;;
  *) ;;
esac
ny_require_tools kubectl
ny_require_context
k get node "$NODE" >/dev/null || ny_die 2 ENV_ERROR "node not found: $NODE"

JOB="openyurt-preflight-$(ny_dns_label "$NODE" 37)-$(ny_rand 5)"
ny_kv CONTEXT "$NY_CTX"
ny_kv NODE "$NODE"
ny_kv JOB "${NY_NS}/${JOB}"

cleanup() {
  k -n "${NY_NS}" delete job "$JOB" --ignore-not-found --wait=false --cascade=background >/dev/null 2>&1 || true
}
trap cleanup EXIT

# The in-container script mirrors the host checks node-servant v1.7.0 performs:
#   pkg/node-servant/components/kubelet.go   GetAPIServerAddress, appendConfig, restartKubeletService
#   pkg/node-servant/components/util.go      GetDefaultKubeadmConfPath
#   pkg/node-servant/components/runtime.go   NewContainerRuntimeForImage, DetectCRISocket
# It always exits 0 so the Job completes; the verdict is computed client-side.
read -r -d '' HOST_SCRIPT <<'EOS' || true
H=/host
HOST_PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
say() { echo "CHECK $1=$2 $3"; }
art() { echo "ARTIFACT $1=$2"; }
in_host() { chroot "$H" /bin/sh -c "export PATH=$HOST_PATH; $1" 2>/dev/null; }

if [ -d "$H/run/systemd/system" ]; then say systemd PASS "host is managed by systemd"; else say systemd FAIL "/run/systemd/system not found; yurthub.service cannot be installed"; fi
if [ -x "$H/bin/sh" ]; then
  for t in crictl systemctl bash; do
    p=$(in_host "command -v $t")
    if [ -n "$p" ]; then say "tool_$t" PASS "$p"; else say "tool_$t" FAIL "$t not found in host PATH $HOST_PATH"; fi
  done
else
  say host_shell FAIL "/bin/sh not found on host; cannot resolve host PATH"
fi

DROPIN=""
for f in /usr/lib/systemd/system/kubelet.service.d/10-kubeadm.conf /etc/systemd/system/kubelet.service.d/10-kubeadm.conf; do
  if [ -f "$H$f" ]; then DROPIN="$f"; break; fi
done
if [ -z "$DROPIN" ]; then
  say kubeadm_dropin FAIL "no 10-kubeadm.conf under /usr/lib or /etc systemd kubelet.service.d (kubeadm layout required)"
else
  say kubeadm_dropin PASS "$DROPIN"
  KCFG=$(grep -o -- '--kubeconfig=[^ "]*kubelet.conf' "$H$DROPIN" | head -n1 | cut -d= -f2)
  if [ -z "$KCFG" ]; then
    say kubelet_kubeconfig FAIL "$DROPIN has no --kubeconfig=...kubelet.conf"
  elif [ ! -f "$H$KCFG" ]; then
    say kubelet_kubeconfig FAIL "$KCFG referenced by $DROPIN does not exist"
  elif grep -qE 'server: ' "$H$KCFG"; then
    say kubelet_kubeconfig PASS "$KCFG"
  else
    say kubelet_kubeconfig FAIL "$KCFG has no 'server:' entry"
  fi
fi

FLAGS=/var/lib/kubelet/kubeadm-flags.env
if [ ! -f "$H$FLAGS" ]; then
  say kubeadm_flags FAIL "$FLAGS not found"
elif grep -qE 'KUBELET_KUBEADM_ARGS=".+"' "$H$FLAGS"; then
  say kubeadm_flags PASS "$FLAGS"
else
  say kubeadm_flags FAIL "$FLAGS has no KUBELET_KUBEADM_ARGS=\"...\" line"
fi

# CRI endpoint resolution, same order as DetectCRISocket().
norm() { s=$(echo "$1" | sed -e "s/^[\"']//" -e "s/[\"']\$//" -e 's#^unix://##'); [ "$s" = /var/run/docker.sock ] && s=/var/run/dockershim.sock; echo "$s"; }
reachable() { if [ "$1" = /var/run/dockershim.sock ]; then [ -S "$H/var/run/docker.sock" ] || [ -S "$H$1" ]; else [ -S "$H$1" ]; fi; }
SRCS=""
for f in /usr/lib/systemd/system/kubelet.service.d/10-kubeadm.conf /etc/systemd/system/kubelet.service.d/10-kubeadm.conf /var/lib/kubelet/kubeadm-flags.env /etc/default/kubelet /etc/sysconfig/kubelet; do
  [ -f "$H$f" ] && SRCS="$SRCS $H$f"
done
lastflag() { [ -n "$SRCS" ] || return 0; cat $SRCS 2>/dev/null | grep -oE -- "--$1(=|[[:space:]]+)[^\"'[:space:]]+" | tail -n1 | sed -E "s/^--$1(=|[[:space:]]+)//"; }
SOCK=""; SRC=""
EP=$(lastflag container-runtime-endpoint)
if [ -n "$EP" ]; then
  EP=$(norm "$EP")
  if reachable "$EP"; then SOCK="$EP"; SRC="kubelet flag --container-runtime-endpoint"; else say cri_flag_endpoint WARN "kubelet flag endpoint $EP is not a socket on host; falling back"; fi
fi
if [ -z "$SOCK" ]; then
  CFG=$(lastflag config)
  for c in $CFG /var/lib/kubelet/config.yaml; do
    [ -f "$H$c" ] || continue
    v=$(grep -E '^[[:space:]]*containerRuntimeEndpoint:' "$H$c" | head -n1 | sed -E "s/^[[:space:]]*containerRuntimeEndpoint:[[:space:]]*//; s/[[:space:]#].*$//")
    [ -n "$v" ] || continue
    v=$(norm "$v")
    if reachable "$v"; then SOCK="$v"; SRC="kubelet config $c"; break; else say cri_config_endpoint WARN "endpoint $v in $c is not a socket on host; falling back"; fi
  done
fi
if [ -z "$SOCK" ]; then
  found=""
  if [ -S "$H/var/run/docker.sock" ]; then found="/var/run/dockershim.sock"; elif [ -S "$H/run/containerd/containerd.sock" ]; then found="/run/containerd/containerd.sock"; fi
  [ -S "$H/var/run/crio/crio.sock" ] && found="$found /var/run/crio/crio.sock"
  set -- $found
  if [ $# -eq 0 ]; then
    say cri_socket FAIL "no CRI socket found; node-servant would fall back to dockershim"
  elif [ $# -gt 1 ]; then
    say cri_socket FAIL "multiple CRI sockets found ($found); node-servant refuses to choose"
  else
    SOCK="$1"; SRC="default socket probing"
  fi
fi
if [ -n "$SOCK" ]; then
  case "$SOCK" in
    /var/run/dockershim.sock) say cri_socket FAIL "resolved to docker ($SRC); dockershim is not supported" ;;
    /run/containerd/containerd.sock | /var/run/containerd/containerd.sock) say cri_socket PASS "containerd $SOCK ($SRC)" ;;
    /var/run/crio/crio.sock | /run/crio/crio.sock) say cri_socket PASS "cri-o $SOCK ($SRC)" ;;
    *) say cri_socket WARN "untested runtime endpoint $SOCK ($SRC)" ;;
  esac
  if [ -x "$H/bin/sh" ] && [ "$SOCK" != /var/run/dockershim.sock ]; then
    if in_host "crictl --timeout 5s -r unix://$SOCK version" >/dev/null; then say crictl_connect PASS "crictl reached $SOCK"; else say crictl_connect WARN "crictl could not query $SOCK (may still work from the privileged node-servant Job)"; fi
  fi
fi

# Host artifacts left by an earlier (possibly partial) conversion.
for f in /etc/systemd/system/yurthub.service /etc/systemd/system/yurthub.service.d/10-yurthub.conf /usr/local/bin/yurthub /var/lib/openyurt/kubelet.conf; do
  if [ -e "$H$f" ]; then art "$f" present; else art "$f" absent; fi
done
if [ -f "$H$FLAGS" ] && grep -q -- '--kubeconfig=/var/lib/openyurt/kubelet.conf' "$H$FLAGS"; then art "kubeadm-flags.env:redirect" present; else art "kubeadm-flags.env:redirect" absent; fi
echo PREFLIGHT_DONE
EOS

# Indent the host script for embedding as a YAML block scalar.
INDENTED_SCRIPT=$(printf '%s\n' "$HOST_SCRIPT" | sed 's/^/          /')

k create -f - >/dev/null <<EOF || ny_die 2 ENV_ERROR "failed to create preflight Job"
apiVersion: batch/v1
kind: Job
metadata:
  name: ${JOB}
  namespace: ${NY_NS}
  labels:
    app.kubernetes.io/name: openyurt-preflight
spec:
  backoffLimit: 0
  activeDeadlineSeconds: ${TIMEOUT}
  ttlSecondsAfterFinished: 300
  template:
    metadata:
      labels:
        app.kubernetes.io/name: openyurt-preflight
    spec:
      nodeName: ${NODE}
      restartPolicy: Never
      automountServiceAccountToken: false
      tolerations:
      - operator: Exists
      containers:
      - name: preflight
        image: ${IMAGE}
        imagePullPolicy: IfNotPresent
        command: ["/bin/sh", "-c"]
        args:
        - |
${INDENTED_SCRIPT}
        volumeMounts:
        - name: host-root
          mountPath: /host
          readOnly: true
      volumes:
      - name: host-root
        hostPath:
          path: /
          type: Directory
EOF

print_pod_events() {
  echo "--- pods and events for ${JOB} ---"
  k -n "${NY_NS}" get pods -l "job-name=${JOB}" -o wide 2>/dev/null || true
  k -n "${NY_NS}" get events --field-selector "involvedObject.kind=Pod" 2>/dev/null | grep -- "${JOB}" || true
}

deadline=$((SECONDS + TIMEOUT + 15))
while :; do
  ny_job_snapshot "$JOB" || true
  case "${JOB_STATE}" in
    succeeded | failed) break ;;
    *) ;;
  esac
  if [[ "$SECONDS" -ge "$deadline" ]]; then
    print_pod_events
    ny_kv RESULT TIMEOUT
    exit 3
  fi
  sleep 3
done

LOGS=$(k -n "${NY_NS}" logs "job/${JOB}" 2>/dev/null || true)
if ! printf '%s\n' "$LOGS" | grep -q '^PREFLIGHT_DONE$'; then
  printf '%s\n' "$LOGS"
  print_pod_events
  reason=$(k -n "${NY_NS}" get job "$JOB" -o jsonpath='{.status.conditions[?(@.type=="Failed")].reason}' 2>/dev/null || true)
  if [[ "$reason" = "DeadlineExceeded" ]]; then
    ny_kv RESULT TIMEOUT
    exit 3
  fi
  ny_die 2 ENV_ERROR "preflight Job did not finish its checks (job state: ${JOB_STATE}, reason: ${reason:-none})"
fi
printf '%s\n' "$LOGS" | grep -E '^(CHECK|ARTIFACT) '
if printf '%s\n' "$LOGS" | grep -qE '^CHECK [a-z_]+=FAIL '; then
  ny_kv RESULT FAIL
  exit 1
fi
ny_kv RESULT PASS
