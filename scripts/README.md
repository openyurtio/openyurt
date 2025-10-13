# OpenYurt One-Click Deployment

## Quick Startgit

The simplest way to deploy OpenYurt is using the provided shell script:

```bash
./scripts/deploy.sh
```

This script will:

1. Check prerequisites (Docker, kubectl, Kind, Helm)
2. Create a Kind cluster with Kubernetes v1.29.0
3. Install OpenYurt CRDs
4. Label nodes for OpenYurt
5. Configure CoreDNS for OpenYurt
6. Verify the foundation setup

## Prerequisites

- Docker
- kubectl
- Kind
- Helm
- jq (for JSON processing)

## What Gets Deployed

The one-click deployment creates a single-node OpenYurt foundation with:

- **Kubernetes v1.29.0** running on Kind
- **OpenYurt CRDs**: Custom Resource Definitions
- **Node Labels**: OpenYurt-specific node labeling
- **CoreDNS**: Configured for OpenYurt topology

## Accessing the Cluster

After deployment, you can access your cluster using:

```bash
# Get cluster information
kubectl get nodes --kubeconfig ~/.kube/config

# Check system pods
kubectl get pods -n kube-system --kubeconfig ~/.kube/config

# Check OpenYurt CRDs
kubectl get crd | grep openyurt
```

## Cleanup

To remove the cluster:

```bash
kind delete cluster --name openyurt-single-node
```

## Troubleshooting

### Common Issues

1. **Port conflicts**: Make sure ports 80, 443, and 6443 are not in use
2. **Docker not running**: Ensure Docker daemon is running
3. **Insufficient resources**: Ensure at least 2GB RAM and 2 CPU cores available

### Logs

Check system status:

```bash
# All pods status
kubectl get pods -n kube-system

# Node information
kubectl describe nodes

# CoreDNS logs
kubectl logs -n kube-system deployment/coredns
```

## Next Steps

- Deploy full OpenYurt components (yurt-manager, yurthub)
