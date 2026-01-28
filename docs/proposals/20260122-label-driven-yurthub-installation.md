# Label-Driven Automated YurtHub Installation and Uninstallation

## Overview

This feature implements automated installation and uninstallation of YurtHub on Kubernetes nodes based on node labels, addressing [feature request #2494](https://github.com/openyurtio/openyurt/issues/2494).

## Motivation

Currently, YurtHub can only be installed via `yurtadm` during the initial node joining process. This creates several limitations:

- **Inflexibility**: Cannot easily convert existing Kubernetes worker nodes to OpenYurt edge nodes without rejoining
- **Barrier to adoption**: Users managing clusters with third-party tools cannot easily enable edge capabilities
- **Operational overhead**: Manual installation doesn't scale well in large edge deployments

This feature enables **declarative, Kubernetes-native** management of YurtHub through node labels.

## Architecture

### Components

1. **YurtHub Installer Controller** (`pkg/yurtmanager/controller/yurthubinstaller/`)
   - Watches for node label changes
   - Triggers installation/uninstallation via Kubernetes Jobs
   - Manages node annotations to track installation status

2. **Node-Servant Jobs**
   - Reuses existing `node-servant` tooling
   - Executes installation/uninstallation on target nodes
   - Runs as privileged Jobs with host access

### Key Labels and Annotations

#### Labels
- `openyurt.io/is-edge-worker=true` - Triggers YurtHub installation on the node

#### Annotations
- `openyurt.io/yurthub-installed=true` - Indicates YurtHub is installed
- `openyurt.io/yurthub-installation-in-progress=true` - Installation/uninstallation is running
- `openyurt.io/yurthub-version` - (Optional) Specifies YurtHub version to install

## Usage

### Prerequisites

1. OpenYurt control plane components installed
2. YurtHub installer controller enabled in yurt-manager
3. Node-servant image available

### Basic Usage

#### Install YurtHub on a Node

Simply add the edge worker label to any existing Kubernetes node:

```bash
kubectl label node <node-name> openyurt.io/is-edge-worker=true
```

The controller will:
1. Detect the label change
2. Create a Job to install YurtHub on the node
3. Update node annotations when installation completes

#### Uninstall YurtHub from a Node

Remove the edge worker label:

```bash
kubectl label node <node-name> openyurt.io/is-edge-worker-
```

The controller will:
1. Detect the label removal
2. Create a Job to uninstall YurtHub
3. Clean up node annotations

#### Specify Custom YurtHub Version

```bash
kubectl annotate node <node-name> openyurt.io/yurthub-version=v1.4.0
kubectl label node <node-name> openyurt.io/is-edge-worker=true
```

### Controller Configuration

The YurtHub installer controller can be configured via yurt-manager flags:

```bash
--enable-yurthub-installer=true               # Enable the controller (default: false)
--concurrent-yurthub-installer-workers=3      # Number of concurrent workers (default: 3)
--node-servant-image=openyurt/node-servant:latest  # Image for installation jobs
--yurthub-version=latest                      # Default YurtHub version
```

### Helm Chart Configuration

When installing yurt-manager via Helm:

```yaml
# values.yaml
yurthubInstaller:
  enabled: true
  nodeServantImage: "openyurt/node-servant:latest"
  yurtHubVersion: "latest"
  concurrentWorkers: 3
```

## Implementation Details

### Controller Logic

1. **Reconciliation Trigger**: Node label or annotation changes, Job status updates
2. **Installation Flow**:
   - Check if node has `openyurt.io/is-edge-worker=true` label
   - If not installed, create installation Job
   - Mark node with `in-progress` annotation
   - Monitor Job completion
   - Update node with `installed` annotation on success

3. **Uninstallation Flow**:
   - Check if label was removed but node still has `installed` annotation
   - Create uninstallation Job
   - Monitor Job completion
   - Remove annotations on success

### Job Templates

Uses existing `node-servant` job templates:
- **Installation**: `node-servant convert` command
- **Uninstallation**: `node-servant revert` command

### Bootstrap Token Management

The controller creates bootstrap tokens (stored as Secrets) for each node to enable YurtHub authentication. In production, this should integrate with Kubernetes Bootstrap Token API.

## Security Considerations

1. **RBAC**: Controller requires permissions to:
   - Manage nodes (read/update)
   - Create/manage Jobs and Secrets
   - Create Events

2. **Job Permissions**: Installation Jobs run as privileged with host access (required for systemd management)

3. **Bootstrap Tokens**: Tokens are node-specific and stored as Secrets in kube-system namespace

## Monitoring and Troubleshooting

### Check Installation Status

```bash
# View node annotations
kubectl get node <node-name> -o jsonpath='{.metadata.annotations}'

# Check installation jobs
kubectl get jobs -n kube-system | grep node-servant-convert

# View job logs
kubectl logs -n kube-system job/node-servant-convert-<node-name>
```

### Events

The controller emits events on nodes:
- `InstallJobCreated`: Installation job created
- `UninstallJobCreated`: Uninstallation job created
- `InstallationComplete`: Installation succeeded
- `UninstallationComplete`: Uninstallation succeeded
- `InstallationFailed`: Installation failed

```bash
kubectl describe node <node-name>
```

### Common Issues

1. **Job Fails to Start**
   - Check node-servant image availability
   - Verify RBAC permissions

2. **Installation Stuck in Progress**
   - Check Job logs for errors
   - Verify node can pull images
   - Ensure node has systemd

3. **Bootstrap Token Errors**
   - Verify Secret creation in kube-system namespace
   - Check controller logs

## Testing

### Manual Testing

```bash
# 1. Label a node
kubectl label node test-node openyurt.io/is-edge-worker=true

# 2. Watch for job creation
kubectl get jobs -n kube-system -w

# 3. Check installation status
kubectl get node test-node -o yaml

# 4. Verify YurtHub is running on the node
ssh test-node "systemctl status yurthub"

# 5. Remove label to uninstall
kubectl label node test-node openyurt.io/is-edge-worker-
```

## Future Enhancements

1. **Integration with Bootstrap Token API**: Replace simplified token management with proper Kubernetes Bootstrap Token API
2. **Custom ConfigMaps**: Allow users to specify custom YurtHub configurations
3. **Health Checks**: Periodic verification that YurtHub is running on labeled nodes
4. **Batch Operations**: Support for bulk node labeling with rate limiting
5. **Webhooks**: Validating webhooks to prevent misconfigurations

## Files Added

```
pkg/yurtmanager/controller/yurthubinstaller/
├── config/
│   └── config.go                          # Controller configuration
├── constants.go                           # Labels, annotations, constants
├── yurthub_installer_controller.go        # Main controller logic
└── enqueue_handlers.go                    # Event handlers

cmd/yurt-manager/app/options/
└── yurthubinstaller.go                    # CLI options

cmd/yurt-manager/names/
└── controller_names.go                    # (modified) Controller name registration
```

## Files Modified

- `pkg/yurtmanager/controller/base/controller.go` - Controller registration
- `pkg/yurtmanager/controller/apis/config/types.go` - Configuration types
- `cmd/yurt-manager/app/options/options.go` - CLI options integration
- `cmd/yurt-manager/names/controller_names.go` - Controller name constants

## References

- [Feature Request #2494](https://github.com/openyurtio/openyurt/issues/2494)
- [Node-Servant Documentation](../pkg/node-servant/)
- [YurtHub Documentation](../pkg/yurthub/)
