#!/usr/bin/env bash

# Copyright 2025 The OpenYurt Authors.
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

#!/bin/bash

# This script automates the setup of a K8s-on-K8s tenant cluster.
# It deploys the tenant control plane on a host cluster and configures the new tenant cluster.
# Then you can use yurtadm join to access a node to tenant cluster.

# Exit immediately if a command exits with a non-zero status.
set -e

# user customer configuration
CONFIG_FILE="config.env"

# check if config.env if exists
if [ ! -f "${CONFIG_FILE}" ]; then
    echo "error: CONFIG_FILE '${CONFIG_FILE}' is not exist"
    exit 1
fi

# load parameters from config.env
source "${CONFIG_FILE}"
echo "load parameters from '${CONFIG_FILE}' succeeds"

# export loaded parameters, for envsubst can use them
export TENANT_APISERVER_SERVICE
export TENANT_APISERVER_SERVICE_PORT
export TENANT_K8S_VERSION
export HOST_K8S_CONTEXT
export TENANT_ADMIN_KUBECONFIG_PATH
export HOST_CONTROL_PLANE_ADDR
export YURTHUB_BINARY_URL

# export randomly generated bootstarp token
export BOOTSTRAP_TOKEN_ID=$(tr -dc 'a-z0-9' < /dev/urandom | head -c 6)
export BOOTSTRAP_TOKEN_SECRET=$(tr -dc 'a-z0-9' < /dev/urandom | head -c 16)
export BOOTSTRAP_TOKEN_EXPIRATION=$(date -d '24 hour' +'%Y-%m-%dT%H:%M:%SZ')

# envsubst inplaces this env parameters in yaml template
SHELL_FORMAT='${TENANT_K8S_VERSION} ${TENANT_APISERVER_SERVICE} ${TENANT_APISERVER_SERVICE_PORT}'

# The namespace in the host cluster where the tenant control-plane components will be deployed.
# DO NOT change this namespace for now. TODO: TENANT_NAMESPACE for user customer configuration.
TENANT_NAMESPACE="tenant-control-plane"

# Helper Functions for Colored Output
C_RED='\033[0;31m'
C_GREEN='\033[0;32m'
C_YELLOW='\033[0;33m'
C_BLUE='\033[0;34m'
C_NC='\033[0m' # No Color

info() {
    echo -e "${C_BLUE}INFO: $1${C_NC}"
}
success() {
    echo -e "${C_GREEN}SUCCESS: $1${C_NC}"
}
warn() {
    echo -e "${C_YELLOW}WARNING: $1${C_NC}"
}
error() {
    echo -e "${C_RED}ERROR: $1${C_NC}" >&2
    exit 1
}

# --- Pre-flight Checks ---
check_dependencies() {
    info "Running pre-flight checks..."
    command -v kubectl >/dev/null 2>&1 || error "kubectl is not installed. Please install it first."
    
    HOST_FILES=(
        "tenant-pki-generator.yaml.template" "etcd.yaml.template" "tenant-apiserver.yaml.template"
        "tenant-scheduler.yaml.template" "tenant-controller-manager.yaml.template"
        "yurthub-local-ep-reader.yaml"
    )
    POST_INSTALL_FILES=(
        "post-install/bootstrap-secret.yaml.template" "post-install/kube-proxy.yaml.template"
        "post-install/kubeadm-config.yaml.template" "post-install/kubelet-config.yaml" "post-install/rbac.yaml"
    )

    for f in "${HOST_FILES[@]}"; do
        [ -f "$f" ] || error "'$f' not found in the current directory."
    done

    for f in "${POST_INSTALL_FILES[@]}"; do
        [ -f "$f" ] || error "'$f' not found. Make sure it is inside the 'post-install' directory."
    done
    
    if ! kubectl config get-contexts "${HOST_K8S_CONTEXT}" >/dev/null 2>&1; then
      error "Context '${HOST_K8S_CONTEXT}' does not exist. Please check your kubeconfig and the HOST_K8S_CONTEXT variable. Use 'kubectl config get-contexts' to see available contexts."
    fi

    success "All dependencies are satisfied."
}

# --- Main Functions ---

deploy_host_components() {
    info "Step 1: Deploying tenant control-plane components in the host cluster..."
    
    info "Creating namespace '${TENANT_NAMESPACE}' if it doesn't exist..."
    kubectl --context="${HOST_K8S_CONTEXT}" create namespace "${TENANT_NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

    info "Creating host-k8s-ca-key-pair secret..."
    kubectl --context="${HOST_K8S_CONTEXT}" create secret generic host-k8s-ca-key-pair \
      -n "${TENANT_NAMESPACE}" \
      --from-file=ca.crt=/etc/kubernetes/pki/ca.crt \
      --from-file=ca.key=/etc/kubernetes/pki/ca.key \
      --dry-run=client -o yaml | kubectl --context="${HOST_K8S_CONTEXT}" apply -f -

    envsubst "${SHELL_FORMAT}" < tenant-pki-generator.yaml.template | kubectl --context="${HOST_K8S_CONTEXT}" apply -f -

    info "Waiting for PKI generation job to complete in namespace '${TENANT_NAMESPACE}'..."
    kubectl --context="${HOST_K8S_CONTEXT}" wait --for=condition=complete job/tenant-pki-generator -n "${TENANT_NAMESPACE}" --timeout=300s

    info "Applying local-path-provisioner and etcd..."
    kubectl --context="${HOST_K8S_CONTEXT}" apply -f etcd.yaml.template
    info "Waiting for etcd to be ready..."
    kubectl --context="${HOST_K8S_CONTEXT}" rollout status statefulset/etcd -n "${TENANT_NAMESPACE}" --timeout=5m
    success "etcd is ready."

    info "Applying tenant-apiserver..."
    envsubst "${SHELL_FORMAT}" < tenant-apiserver.yaml.template | kubectl --context="${HOST_K8S_CONTEXT}" apply -f -
    info "Waiting for tenant-apiserver to be ready..."
    kubectl --context="${HOST_K8S_CONTEXT}" rollout status daemonset/tenant-apiserver -n "${TENANT_NAMESPACE}" --timeout=5m
    success "Tenant APIServer is ready."

    info "Applying scheduler and controller-manager..."
    envsubst "${SHELL_FORMAT}" < tenant-scheduler.yaml.template | kubectl --context="${HOST_K8S_CONTEXT}" apply -f -
    envsubst "${SHELL_FORMAT}" < tenant-controller-manager.yaml.template | kubectl --context="${HOST_K8S_CONTEXT}" apply -f -
    info "Waiting for tenant-scheduler and tenant-controller-manager to be ready..."
    kubectl --context="${HOST_K8S_CONTEXT}" rollout status deployment/tenant-scheduler -n "${TENANT_NAMESPACE}" --timeout=5m
    kubectl --context="${HOST_K8S_CONTEXT}" rollout status deployment/tenant-controller-manager -n "${TENANT_NAMESPACE}" --timeout=5m 
    success "Tenant Scheduler and Tenant Controller Manager is ready."    

    info "Applying yurthub-local-ep-reader RBAC for yurthub-local list-watch apiserver endpoints..."
    kubectl --context="${HOST_K8S_CONTEXT}" apply -f yurthub-local-ep-reader.yaml

    info "Extracting tenant admin kubeconfig..."
    kubectl --context="${HOST_K8S_CONTEXT}" get secret tenant-admin-kubeconfig -n "${TENANT_NAMESPACE}" -o jsonpath='{.data.kubeconfig}' | base64 --decode > "${TENANT_ADMIN_KUBECONFIG_PATH}"

    success "Host components deployed. Tenant kubeconfig saved to: ${TENANT_ADMIN_KUBECONFIG_PATH}"
}

generate_cluster_info_configmap() {
    info "--> Generating and applying the 'cluster-info' ConfigMap to the tenant cluster..."

    local tenant_api_server="https://${TENANT_APISERVER_SERVICE}:6443"
    local ca_secret="host-k8s-ca-key-pair"

    info "Extracting CA certificate from secret '${ca_secret}' on the host cluster..."
    local ca_cert_base64
    ca_cert_base64=$(kubectl --context="${HOST_K8S_CONTEXT}" get secret "${ca_secret}" -n "${TENANT_NAMESPACE}" -o jsonpath='{.data.ca\.crt}')

    if [ -z "$ca_cert_base64" ]; then
        error "Failed to retrieve ca.crt from Secret '${ca_secret}'. Make sure the secret exists in namespace '${TENANT_NAMESPACE}' on the host cluster."
    fi

    info "Applying the ConfigMap to the tenant cluster..."
    cat <<EOF | kubectl --kubeconfig="${TENANT_ADMIN_KUBECONFIG_PATH}" apply -f -
apiVersion: v1
kind: ConfigMap
metadata:
  name: cluster-info
  namespace: kube-public
data:
  kubeconfig: |
    apiVersion: v1
    clusters:
    - cluster:
        certificate-authority-data: ${ca_cert_base64}
        server: ${tenant_api_server}
      name: ""
    contexts: null
    current-context: ""
    kind: Config
    preferences: {}
    users: null
EOF

    success "--> Successfully applied 'cluster-info' ConfigMap."
}

configure_tenant_cluster() {
    info "Step 2: Deploying necessary configs into the new tenant K8s cluster..."
    
    if [ ! -f "${TENANT_ADMIN_KUBECONFIG_PATH}" ]; then
        error "Tenant admin kubeconfig file not found at ${TENANT_ADMIN_KUBECONFIG_PATH}"
    fi

    KUBECTL_TENANT="kubectl --kubeconfig=${TENANT_ADMIN_KUBECONFIG_PATH}"

    info "Applying tenant cluster configurations from 'post-install/' directory..."
    generate_cluster_info_configmap
    envsubst '${BOOTSTRAP_TOKEN_ID} ${BOOTSTRAP_TOKEN_SECRET} ${BOOTSTRAP_TOKEN_EXPIRATION}' < ./post-install/bootstrap-secret.yaml.template | $KUBECTL_TENANT apply -f -
    envsubst '${TENANT_K8S_VERSION} ${TENANT_APISERVER_SERVICE}' < ./post-install/kube-proxy.yaml.template | $KUBECTL_TENANT apply -f -
    envsubst '${TENANT_K8S_VERSION}' < ./post-install/kubeadm-config.yaml.template | $KUBECTL_TENANT apply -f -
    $KUBECTL_TENANT apply -f ./post-install/kubelet-config.yaml
    $KUBECTL_TENANT apply -f ./post-install/rbac.yaml

    success "Tenant cluster configured successfully."
}

show_join_command() {
    info "Step 3: Preparing instructions for joining a node..."
    
    info "You may need to build it first with: make build WHAT=cmd/yurtadm"

    local tenant_apiserver_addr
    tenant_apiserver_addr=${TENANT_APISERVER_SERVICE}
    
    echo -e "\n${C_GREEN}--- ACTION REQUIRED: JOIN NODE ---${C_NC}"
    echo "Run the following command on the node you wish to join to the tenant cluster:"
    echo -e "${C_YELLOW}\n yurtadm join ${tenant_apiserver_addr}:6443 --node-type=local --yurthub-binary-url=${YURTHUB_BINARY_URL} --host-control-plane-addr=${HOST_CONTROL_PLANE_ADDR} --token=${BOOTSTRAP_TOKEN_ID}.${BOOTSTRAP_TOKEN_SECRET} --discovery-token-unsafe-skip-ca-verification --cri-socket=/run/containerd/containerd.sock --v=5\n"
}

# --- Main Execution ---
main() {
    check_dependencies
    deploy_host_components
    configure_tenant_cluster
    show_join_command
    success "K8s-on-K8s setup script finished!"
    info "You can now optionally deploy your CNI, CoreDNS, etc., and manage the tenant cluster by using the kubeconfig at: ${TENANT_ADMIN_KUBECONFIG_PATH}"
}

main
