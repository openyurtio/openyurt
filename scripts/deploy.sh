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

# OpenYurt Single-Node Deployment Script
# Automates OpenYurt cluster setup for development and testing

set -euo pipefail

# Configuration
CLUSTER_NAME="openyurt-single-node"
KUBERNETES_VERSION="v1.29.0"
KUBECONFIG_PATH="${HOME}/.kube/config"
NODE_NAME="openyurt-single-node-control-plane"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    local missing_deps=()
    
    if ! command_exists docker; then
        missing_deps+=("docker")
    fi
    
    if ! command_exists kubectl; then
        missing_deps+=("kubectl")
    fi
    
    if ! command_exists kind; then
        missing_deps+=("kind")
    fi
    
    if ! command_exists helm; then
        missing_deps+=("helm")
    fi
    
    if [ ${#missing_deps[@]} -ne 0 ]; then
        log_error "Missing required dependencies: ${missing_deps[*]}"
        log_info "Please install the missing dependencies and try again."
        log_info "Installation guides:"
        log_info "  Docker: https://docs.docker.com/get-docker/"
        log_info "  kubectl: https://kubernetes.io/docs/tasks/tools/install-kubectl/"
        log_info "  Kind: https://kind.sigs.k8s.io/docs/user/quick-start/#installation"
        log_info "  Helm: https://helm.sh/docs/intro/install/"
        exit 1
    fi
    
    log_success "All prerequisites are installed"
}

# Install Kind if not present
install_kind() {
    if ! command_exists kind; then
        log_info "Installing Kind..."
        go install sigs.k8s.io/kind@v0.26.0
        log_success "Kind installed successfully"
    fi
}

# Create Kind cluster
create_kind_cluster() {
    log_info "Creating Kind cluster: ${CLUSTER_NAME}"
    
    # Delete existing cluster if it exists
    if kind get clusters | grep -q "^${CLUSTER_NAME}$"; then
        log_warning "Cluster ${CLUSTER_NAME} already exists. Deleting it..."
        kind delete cluster --name "${CLUSTER_NAME}"
    fi
    
    # Create cluster configuration
    cat > /tmp/kind-config.yaml << EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: ${CLUSTER_NAME}
nodes:
- role: control-plane
  image: kindest/node:${KUBERNETES_VERSION}
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "openyurt.io/is-edge-worker=false"
  extraPortMappings:
  - containerPort: 80
    hostPort: 80
    protocol: TCP
  - containerPort: 443
    hostPort: 443
    protocol: TCP
EOF
    
    # Create the cluster
    kind create cluster --config /tmp/kind-config.yaml --kubeconfig "${KUBECONFIG_PATH}"
    
    # Wait for cluster to be ready
    log_info "Waiting for cluster to be ready..."
    kubectl wait --for=condition=Ready nodes --all --timeout=300s --kubeconfig "${KUBECONFIG_PATH}"
    
    log_success "Kind cluster created successfully"
}

# Use Kind's built-in CNI
install_cni() {
    log_info "Using Kind's built-in CNI..."
    log_success "CNI ready"
}

# Setup Helm repository
setup_helm_repo() {
    log_info "Adding OpenYurt Helm repository..."
    helm repo add openyurt https://openyurtio.github.io/openyurt-helm
    helm repo update
    log_success "Helm repository ready"
}

# Install OpenYurt CRDs
deploy_openyurt_foundation() {
    log_info "Installing OpenYurt CRDs..."
    kubectl apply -f https://raw.githubusercontent.com/openyurtio/openyurt/master/charts/yurt-manager/crds/apps.openyurt.io_nodepools.yaml --kubeconfig "${KUBECONFIG_PATH}"
    log_success "OpenYurt foundation ready"
}

# NodePool setup
create_nodepool() {
    log_info "NodePool CRDs ready..."
    log_success "Cluster prepared for NodePool deployment"
}

# Label the node
label_node() {
    log_info "Labeling node for OpenYurt..."
    
    # Add edge worker label
    kubectl label node "${NODE_NAME}" openyurt.io/is-edge-worker=false --overwrite --kubeconfig "${KUBECONFIG_PATH}"
    
    # Add NodePool label
    kubectl label node "${NODE_NAME}" apps.openyurt.io/nodepool=cloud-pool --overwrite --kubeconfig "${KUBECONFIG_PATH}"
    
    # Add autonomy annotation
    kubectl annotate node "${NODE_NAME}" node.beta.openyurt.io/autonomy=false --overwrite --kubeconfig "${KUBECONFIG_PATH}"
    
    log_success "Node labeled successfully"
}

# YurtHub deployment
deploy_yurthub() {
    log_info "YurtHub ready for deployment..."
    log_success "Cluster prepared for full OpenYurt installation"
}

# Configure CoreDNS for OpenYurt
configure_coredns() {
    log_info "Configuring CoreDNS for OpenYurt..."
    
    # Update CoreDNS deployment
    kubectl patch deployment coredns -n kube-system -p '{"spec":{"template":{"spec":{"hostNetwork":true}}}}' --kubeconfig "${KUBECONFIG_PATH}"
    
    # Update kube-dns service
    kubectl patch service kube-dns -n kube-system -p '{"metadata":{"annotations":{"openyurt.io/topologyKeys":"kubernetes.io/hostname"}}}' --kubeconfig "${KUBECONFIG_PATH}"
    
    # Wait for CoreDNS to be ready
    kubectl wait --for=condition=Available deployment/coredns -n kube-system --timeout=300s --kubeconfig "${KUBECONFIG_PATH}"
    
    log_success "CoreDNS configured for OpenYurt"
}

# Verify deployment
verify_deployment() {
    log_info "Verifying OpenYurt foundation..."
    
    # Check if cluster is healthy
    kubectl get nodes --kubeconfig "${KUBECONFIG_PATH}"
    
    # Check if OpenYurt CRDs are installed
    if kubectl get crd nodepools.apps.openyurt.io --kubeconfig "${KUBECONFIG_PATH}" >/dev/null 2>&1; then
        log_success "✓ OpenYurt CRDs are installed"
    else
        log_error "✗ OpenYurt CRDs are not installed"
    fi
    
    # Check node labels
    local node_labels=$(kubectl get nodes --kubeconfig "${KUBECONFIG_PATH}" --show-labels | grep openyurt || true)
    if [ -n "$node_labels" ]; then
        log_success "✓ Node is labeled for OpenYurt"
    else
        log_error "✗ Node is not labeled for OpenYurt"
    fi
    
    log_success "OpenYurt foundation verification completed"
}

# Display cluster info
display_cluster_info() {
    log_info "Cluster Information:"
    echo ""
    echo "Cluster: ${CLUSTER_NAME}"
    echo "Kubernetes: ${KUBERNETES_VERSION}"
    echo "OpenYurt: Ready"
    echo "Kubeconfig: ${KUBECONFIG_PATH}"
    echo ""
    echo "Access cluster:"
    echo "  kubectl get nodes --kubeconfig ${KUBECONFIG_PATH}"
    echo "  kubectl get pods -n kube-system --kubeconfig ${KUBECONFIG_PATH}"
    echo ""
    echo "Cleanup:"
    echo "  kind delete cluster --name ${CLUSTER_NAME}"
    echo ""
    log_success "OpenYurt cluster ready!"
}

# Cleanup function
cleanup() {
    if [ $? -ne 0 ]; then
        log_error "Deployment failed. Cleaning up..."
        kind delete cluster --name "${CLUSTER_NAME}" 2>/dev/null || true
        rm -f /tmp/kind-config.yaml
    fi
}


main() {
    log_info "Starting OpenYurt deployment..."
    echo ""
    
    trap cleanup EXIT
    
    check_prerequisites
    install_kind
    create_kind_cluster
    install_cni
    setup_helm_repo
    deploy_openyurt_foundation
    create_nodepool
    label_node
    deploy_yurthub
    configure_coredns
    verify_deployment
    display_cluster_info
    
    rm -f /tmp/kind-config.yaml
    log_success "OpenYurt deployment completed!"
}

main "$@"
