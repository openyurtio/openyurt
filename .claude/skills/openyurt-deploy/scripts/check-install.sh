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

# Read-only check that an existing OpenYurt installation can be reused for
# v1.7.0 label-driven node conversion, optionally probing the Node webhooks of
# a target node with server-side dry-run requests (nothing is persisted).
#
# RESULT / exit code:
#   OK 0 | NOT_INSTALLED 10 | MISSING_SHARED 11 | INCOMPATIBLE 12
#   PROBE_UNAVAILABLE 13 | WEBHOOK_UNHEALTHY 14 | usage/env error 2

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
. "${SCRIPT_DIR}/lib.sh"

EXPECTED_CHART="yurt-manager-1.7.0"
EXPECTED_TAG="v1.7.0"
RELEASE="yurt-manager"
MUTATING_CFG="yurt-manager-mutating-webhook-configuration"
VALIDATING_CFG="yurt-manager-validating-webhook-configuration"
MUTATING_NODE_HOOK="mutate.core.v1.node.openyurt.io"
VALIDATING_NODE_HOOK="validate.core.v1.node.openyurt.io"

usage() {
  cat <<'EOF'
Usage: check-install.sh --context <kube-context> [--require-autonomy]
                        [--probe-node <node> [--probe-pool <nodepool>]]

  --require-autonomy  also require pod-binding-controller (needed for autonomy-duration)
  --probe-node        dry-run the Node webhooks against this node (no changes are persisted)
  --probe-pool        pool to probe with when the node is not in a pool yet
  --self-test         run the built-in controller/webhook flag parsing cases and exit
EOF
}

# --- flag semantics --------------------------------------------------------

# canonical controller name; mirrors YurtManagerControllerAliases() in
# cmd/yurt-manager/names/controller_names.go (v1.7.0)
canonical_controller() {
  case "$1" in
    csrapprover) echo csr-approver-controller ;;
    daemonpodupdater) echo daemon-pod-updater-controller ;;
    nodepool) echo nodepool-controller ;;
    platformadmin) echo platform-admin-controller ;;
    servicetopologyendpoints) echo service-topology-endpoints-controller ;;
    servicetopologyendpointslices) echo service-topology-endpointslice-controller ;;
    yurtappset) echo yurt-app-set-controller ;;
    yurtstaticset) echo yurt-static-set-controller ;;
    delegatelease) echo delegate-lease-controller ;;
    podbinding) echo pod-binding-controller ;;
    gatewaypickup) echo gateway-pickup-controller ;;
    gatewayinternalservice) echo gateway-internal-service-controller ;;
    gatewaypublicservice) echo gateway-public-service-controller ;;
    gatewaydns) echo gateway-dns-controller ;;
    nodelifecycle) echo node-life-cycle-controller ;;
    nodebucket) echo node-bucket-controller ;;
    yurtnodeconversion) echo yurt-node-conversion-controller ;;
    loadbalancerset) echo load-balancer-set-controller ;;
    hubleader) echo hubleader-controller ;;
    hubleaderconfig) echo hubleaderconfig-controller ;;
    hubleaderrbac) echo hubleaderrbac-controller ;;
    imagepreheat) echo image-preheat-controller ;;
    *) echo "$1" ;;
  esac
}

# controller_enabled NAME CSV -> 0 if enabled. Mirrors
# k8s.io/controller-manager/app.IsControllerEnabled after the alias
# normalization in cmd/yurt-manager/app/options/generic.go. v1.7.0 has no
# controllers that are disabled by default.
controller_enabled() {
  local name="$1" csv="$2" item c has_star=no
  # read items line by line so that '*' is never subject to pathname expansion
  while IFS= read -r item; do
    item=$(printf '%s' "$item" | tr -d '[:space:]')
    case "$item" in
      '') ;;
      '*') has_star=yes ;;
      -*) c=$(canonical_controller "${item#-}"); [ "$c" = "$name" ] && return 1 ;;
      *) c=$(canonical_controller "$item"); [ "$c" = "$name" ] && return 0 ;;
    esac
  done <<EOF
$(printf '%s' "$csv" | tr ',' '\n')
EOF
  [ "$has_star" = yes ]
}

# webhook_disabled NAME CSV -> 0 if disabled. Mirrors IsWebhookDisabled in
# pkg/yurtmanager/webhook/util/util.go (exact name or '*').
webhook_disabled() {
  local name="$1" csv="$2" item
  while IFS= read -r item; do
    item=$(printf '%s' "$item" | tr -d '[:space:]')
    if [ "$item" = "$name" ] || [ "$item" = '*' ]; then return 0; fi
  done <<EOF
$(printf '%s' "$csv" | tr ',' '\n')
EOF
  return 1
}

# slice_flag_value FLAG ARGS... -> CSV value with pflag StringSlice semantics
# (the first occurrence replaces the default, later occurrences append).
# Prints "__UNSET__" when the flag is absent.
slice_flag_value() {
  local flag="$1" value="__UNSET__" expect_value=no a
  shift
  for a in "$@"; do
    if [ "$expect_value" = yes ]; then
      if [ "$value" = "__UNSET__" ]; then value="$a"; else value="$value,$a"; fi
      expect_value=no
      continue
    fi
    case "$a" in
      "--$flag="*)
        if [ "$value" = "__UNSET__" ]; then value="${a#--"$flag"=}"; else value="$value,${a#--"$flag"=}"; fi
        ;;
      "--$flag") expect_value=yes ;;
    esac
  done
  printf '%s' "$value"
}

self_test() {
  local fails=0
  t() { # t EXPECT(on|off) NAME CSV
    local got=off
    if controller_enabled "$2" "$3"; then got=on; fi
    if [ "$got" = "$1" ]; then echo "ok   controllers=[$3] $2 -> $got"; else echo "FAIL controllers=[$3] $2 -> $got (want $1)"; fails=$((fails + 1)); fi
  }
  w() { # w EXPECT(disabled|enabled) CSV
    local got=enabled
    if webhook_disabled node "$2"; then got=disabled; fi
    if [ "$got" = "$1" ]; then echo "ok   disable-independent-webhooks=[$2] node -> $got"; else echo "FAIL disable-independent-webhooks=[$2] node -> $got (want $1)"; fails=$((fails + 1)); fi
  }
  t on yurt-node-conversion-controller '-nodelifecycle,*'
  t off yurt-node-conversion-controller 'nodepool'
  t on nodepool-controller 'nodepool'
  t off yurt-node-conversion-controller '-yurtnodeconversion,*'
  t on yurt-node-conversion-controller 'yurt-node-conversion-controller'
  t off yurt-node-conversion-controller '*,-yurtnodeconversion'
  t on yurt-node-conversion-controller 'yurtnodeconversion,-yurtnodeconversion'
  t off pod-binding-controller '-podbinding,*'
  t off yurt-node-conversion-controller ''
  w disabled 'node'
  w disabled '*'
  w disabled 'pod,node'
  w enabled ''
  w enabled 'pod,endpoints'
  local v
  v=$(slice_flag_value controllers --v=4 --controllers=a,b --x --controllers c)
  if [ "$v" = "a,b,c" ]; then echo "ok   slice_flag_value -> $v"; else echo "FAIL slice_flag_value -> $v (want a,b,c)"; fails=$((fails + 1)); fi
  v=$(slice_flag_value controllers --v=4)
  if [ "$v" = "__UNSET__" ]; then echo "ok   slice_flag_value unset"; else echo "FAIL slice_flag_value unset -> $v"; fails=$((fails + 1)); fi
  [ "$fails" -eq 0 ]
}

# --- argument parsing ------------------------------------------------------

REQUIRE_AUTONOMY=no
PROBE_NODE=""
PROBE_POOL=""
while [ $# -gt 0 ]; do
  case "$1" in
    --context) NY_CTX="${2:-}"; shift 2 ;;
    --require-autonomy) REQUIRE_AUTONOMY=yes; shift ;;
    --probe-node) PROBE_NODE="${2:-}"; shift 2 ;;
    --probe-pool) PROBE_POOL="${2:-}"; shift 2 ;;
    --self-test) if self_test; then exit 0; else exit 1; fi ;;
    -h | --help) usage; exit 0 ;;
    *) usage >&2; ny_die 2 USAGE_ERROR "unknown argument: $1" ;;
  esac
done
ny_require_tools kubectl helm
ny_require_context

INCOMPATIBLE=""
MISSING=""
incompatible() { echo "CHECK $1=FAIL $2"; INCOMPATIBLE="$INCOMPATIBLE $1"; }
missing() { echo "CHECK $1=MISSING $2"; MISSING="$MISSING $1"; }
pass() { echo "CHECK $1=PASS $2"; }
warn() { echo "CHECK $1=WARN $2"; }

ny_kv CONTEXT "$NY_CTX"

# --- release and deployment -----------------------------------------------

# Helm v3 lists only deployed/failed releases unless --all is given; Helm v4
# lists every status by default and no longer accepts --all.
helm_all=()
case "$(helm version --short 2>/dev/null)" in v3.*) helm_all=(--all) ;; esac
release_yaml=$(helm --kube-context "$NY_CTX" --namespace "$NY_NS" list ${helm_all[@]+"${helm_all[@]}"} --filter "^${RELEASE}\$" -o yaml) ||
  ny_die 2 ENV_ERROR "helm list failed"
other_ns=$(helm --kube-context "$NY_CTX" list --all-namespaces ${helm_all[@]+"${helm_all[@]}"} --filter "^${RELEASE}\$" -o yaml 2>/dev/null |
  sed -n 's/^  namespace: //p' | grep -vx "$NY_NS" || true)
deploy_found=$(k -n "$NY_NS" get deployment yurt-manager --ignore-not-found -o name)

if printf '%s\n' "$release_yaml" | grep -q "name: ${RELEASE}\$"; then
  chart=$(printf '%s\n' "$release_yaml" | sed -n 's/^  chart: //p' | head -n1)
  status=$(printf '%s\n' "$release_yaml" | sed -n 's/^  status: //p' | head -n1)
  if [ "$chart" = "$EXPECTED_CHART" ]; then pass helm_chart "$chart"; else incompatible helm_chart "release ${NY_NS}/${RELEASE} uses chart '$chart', want $EXPECTED_CHART"; fi
  if [ "$status" = "deployed" ]; then pass helm_status "$status"; else incompatible helm_status "release status is '$status', want deployed"; fi
else
  if [ -n "$other_ns" ]; then
    incompatible helm_namespace "release ${RELEASE} found in namespace(s) $(echo "$other_ns" | tr '\n' ' ')- this skill requires it in ${NY_NS}"
  elif [ -n "$deploy_found" ]; then
    incompatible helm_release "deployment ${NY_NS}/yurt-manager exists but is not managed by a helm release in ${NY_NS}"
  else
    echo "CHECK yurt_manager=ABSENT no helm release or deployment in ${NY_NS}"
    ny_kv RESULT NOT_INSTALLED
    exit 10
  fi
fi

if [ -n "$deploy_found" ]; then
  image=$(k -n "$NY_NS" get deployment yurt-manager -o jsonpath='{.spec.template.spec.containers[?(@.name=="yurt-manager")].image}')
  available=$(k -n "$NY_NS" get deployment yurt-manager -o jsonpath='{.status.availableReplicas}')
  args=()
  while IFS= read -r line; do args+=("$line"); done < <(k -n "$NY_NS" get deployment yurt-manager \
    -o jsonpath='{range .spec.template.spec.containers[?(@.name=="yurt-manager")].args[*]}{@}{"\n"}{end}')

  case "$image" in
    *":${EXPECTED_TAG}") pass yurt_manager_image "$image" ;;
    *) incompatible yurt_manager_image "image '$image', want tag ${EXPECTED_TAG}" ;;
  esac
  if [ -n "$available" ] && [ "$available" -ge 1 ]; then pass yurt_manager_available "$available"; else incompatible yurt_manager_available "no available replicas"; fi

  servant=$(slice_flag_value node-servant-image ${args[@]+"${args[@]}"})
  case "$servant" in
    *":${EXPECTED_TAG}") pass node_servant_image "$servant" ;;
    __UNSET__) incompatible node_servant_image "--node-servant-image is not set" ;;
    *) incompatible node_servant_image "'$servant' (pin nodeServant.image.tag=${EXPECTED_TAG}; 'latest' drifts from the controller version)" ;;
  esac

  ns_arg=$(slice_flag_value working-namespace ${args[@]+"${args[@]}"})
  if [ "$ns_arg" = "__UNSET__" ] || [ "$ns_arg" = "$NY_NS" ]; then pass working_namespace "$NY_NS"; else incompatible working_namespace "--working-namespace=$ns_arg, want $NY_NS"; fi

  controllers=$(slice_flag_value controllers ${args[@]+"${args[@]}"})
  [ "$controllers" = "__UNSET__" ] && controllers=""
  for c in yurt-node-conversion-controller nodepool-controller; do
    if controller_enabled "$c" "$controllers"; then pass "controller_$c" "enabled by --controllers=$controllers"; else incompatible "controller_$c" "disabled by --controllers=$controllers"; fi
  done
  if controller_enabled pod-binding-controller "$controllers"; then
    pass controller_pod-binding-controller "enabled"
  elif [ "$REQUIRE_AUTONOMY" = yes ]; then
    incompatible controller_pod-binding-controller "disabled by --controllers=$controllers; node.openyurt.io/autonomy-duration has no effect"
  else
    warn controller_pod-binding-controller "disabled; only matters if node autonomy is enabled later"
  fi

  disabled_hooks=$(slice_flag_value disable-independent-webhooks ${args[@]+"${args[@]}"})
  [ "$disabled_hooks" = "__UNSET__" ] && disabled_hooks=""
  if webhook_disabled node "$disabled_hooks"; then
    incompatible node_webhook_flag "disabled by --disable-independent-webhooks=$disabled_hooks"
  else
    pass node_webhook_flag "not disabled"
  fi
fi

# --- webhook configuration, CRD, shared yurthub resources -----------------

hook_rules() { # hook_rules KIND CONFIG HOOK
  k get "$1" "$2" --ignore-not-found \
    -o jsonpath="{range .webhooks[?(@.name==\"$3\")]}{.rules[*].resources}{\" \"}{.rules[*].operations}{end}" 2>/dev/null || true
}
for spec in "mutatingwebhookconfiguration ${MUTATING_CFG} ${MUTATING_NODE_HOOK}" \
  "validatingwebhookconfiguration ${VALIDATING_CFG} ${VALIDATING_NODE_HOOK}"; do
  read -r kind cfg hook <<<"$spec"
  rules=$(hook_rules "$kind" "$cfg" "$hook")
  if printf '%s' "$rules" | grep -q '"nodes"' && printf '%s' "$rules" | grep -q '"UPDATE"'; then
    pass "webhook_$hook" "registered for nodes UPDATE"
  else
    incompatible "webhook_$hook" "missing from $kind/$cfg or not covering nodes UPDATE"
  fi
done
endpoints=$(k -n "$NY_NS" get endpoints yurt-manager-webhook-service --ignore-not-found -o jsonpath='{.subsets[*].addresses[*].ip}')
if [ -n "$endpoints" ]; then pass webhook_endpoints "$endpoints"; else incompatible webhook_endpoints "service ${NY_NS}/yurt-manager-webhook-service has no ready endpoints"; fi

served=$(k get crd nodepools.apps.openyurt.io --ignore-not-found -o jsonpath='{.spec.versions[?(@.name=="v1beta2")].served}')
if [ "$served" = "true" ]; then pass crd_nodepool_v1beta2 "served"; else incompatible crd_nodepool_v1beta2 "nodepools.apps.openyurt.io does not serve v1beta2"; fi

for obj in "configmap -n ${NY_NS} yurt-hub-cfg" "clusterrole yurt-hub" "clusterrolebinding yurt-hub"; do
  # shellcheck disable=SC2086
  if [ -n "$(k get $obj --ignore-not-found -o name)" ]; then pass "shared_$(echo "$obj" | awk '{print $NF}')_$(echo "$obj" | awk '{print $1}')" "present"; else missing "shared_$(echo "$obj" | awk '{print $NF}')_$(echo "$obj" | awk '{print $1}')" "not found (render charts/yurthub templates/yurthub-cfg.yaml from v1.7.0)"; fi
done

if [ -n "$INCOMPATIBLE" ]; then
  ny_kv RESULT INCOMPATIBLE
  exit 12
fi
if [ -n "$MISSING" ]; then
  ny_kv RESULT MISSING_SHARED
  exit 11
fi

# --- functional webhook probe (server-side dry-run, nothing persisted) ---

if [ -n "$PROBE_NODE" ]; then
  ny_node_snapshot "$PROBE_NODE" || ny_die 2 ENV_ERROR "node not found: $PROBE_NODE"
  pool="${NODE_NP:-$PROBE_POOL}"
  if [ -z "$pool" ]; then
    echo "CHECK webhook_probe=UNAVAILABLE node $PROBE_NODE is not in a pool and --probe-pool was not given"
    ny_kv RESULT PROBE_UNAVAILABLE
    exit 13
  fi
  if [ -n "$NODE_NP" ] && [ -n "$PROBE_POOL" ] && [ "$PROBE_POOL" != "$NODE_NP" ]; then
    echo "CHECK webhook_probe=INFO node is in pool $NODE_NP; probing with its current pool instead of $PROBE_POOL"
  fi
  if [ -z "$(k get nodepools.apps.openyurt.io "$pool" --ignore-not-found -o name)" ]; then
    echo "CHECK webhook_probe=UNAVAILABLE NodePool $pool does not exist (the defaulting webhook needs it)"
    ny_kv RESULT PROBE_UNAVAILABLE
    exit 13
  fi

  # (a) mutating: drop nodepool and set desired-nodepool in one patch; the
  # defaulting webhook (node_default.go) must put nodepool back.
  probe_args=("${NY_LABEL_DESIRED}=${pool}")
  [ -n "$NODE_NP" ] && probe_args=("${NY_LABEL_NODEPOOL}-" "${probe_args[@]}")
  errfile=$(mktemp)
  if got=$(k label node "$PROBE_NODE" "${probe_args[@]}" --overwrite --dry-run=server \
    -o jsonpath='{.metadata.labels.apps\.openyurt\.io/nodepool}' 2>"$errfile"); then :; else
    got="error: $(tr '\n' ' ' <"$errfile")"
  fi
  rm -f "$errfile"
  if [ "$got" = "$pool" ]; then
    pass webhook_probe_mutating "desired-nodepool=$pool was defaulted to nodepool=$pool"
    mutating_ok=yes
  else
    echo "CHECK webhook_probe_mutating=FAIL expected nodepool=$pool in the dry-run result, got '$got'"
    mutating_ok=no
  fi

  # (b) validating: changing is-edge-worker as a regular user must be denied by
  # node_validation.go. The webhook returns apierrors.NewInvalid, so the API
  # reports Invalid (422); match the webhook name and the field, not Forbidden.
  if [ "$NODE_EDGE" = "true" ]; then flip="false"; else flip="true"; fi
  if out=$(k label node "$PROBE_NODE" "${NY_LABEL_EDGE}=${flip}" --overwrite --dry-run=server 2>&1); then
    echo "CHECK webhook_probe_validating=FAIL dry-run change of ${NY_LABEL_EDGE} was accepted"
    validating_ok=no
  elif printf '%s' "$out" | grep -q "$VALIDATING_NODE_HOOK" && printf '%s' "$out" | grep -q "$NY_LABEL_EDGE"; then
    pass webhook_probe_validating "denied by $VALIDATING_NODE_HOOK"
    validating_ok=yes
  else
    echo "CHECK webhook_probe_validating=FAIL request failed for another reason: $(printf '%s' "$out" | tr '\n' ' ')"
    validating_ok=no
  fi

  if [ "$mutating_ok" != yes ] || [ "$validating_ok" != yes ]; then
    ny_kv RESULT WEBHOOK_UNHEALTHY
    exit 14
  fi
fi

ny_kv RESULT OK
