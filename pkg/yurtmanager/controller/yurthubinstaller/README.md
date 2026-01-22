# YurtHub Installer Controller

## Overview

The YurtHub Installer Controller provides label-driven automated installation and uninstallation of YurtHub on Kubernetes edge nodes. This enables declarative, Kubernetes-native management of edge node capabilities without requiring `yurtadm` or manual intervention.

## Quick Start

### Enable the Controller

Add to yurt-manager deployment:
```bash
--enable-yurthub-installer=true
--node-servant-image=openyurt/node-servant:latest
```

### Install YurtHub on a Node

```bash
kubectl label node <node-name> openyurt.io/is-edge-worker=true
```

### Uninstall YurtHub from a Node

```bash
kubectl label node <node-name> openyurt.io/is-edge-worker-
```

## How It Works

1. **Watch**: Controller watches for node label changes
2. **Trigger**: When `openyurt.io/is-edge-worker=true` is detected, creates installation Job
3. **Execute**: Job runs `node-servant convert` on the target node
4. **Track**: Updates node annotations to reflect installation status
5. **Complete**: YurtHub runs as systemd service on the node

Uninstallation follows the same pattern with `node-servant revert`.

## Configuration

### Controller Options

| Flag | Default | Description |
|------|---------|-------------|
| `--enable-yurthub-installer` | `false` | Enable the controller |
| `--concurrent-yurthub-installer-workers` | `3` | Number of concurrent workers |
| `--node-servant-image` | `openyurt/node-servant:latest` | Installation job image |
| `--yurthub-version` | `latest` | Default YurtHub version |

### Node Labels

- `openyurt.io/is-edge-worker=true` - Triggers YurtHub installation

### Node Annotations

- `openyurt.io/yurthub-installed=true` - Indicates successful installation
- `openyurt.io/yurthub-installation-in-progress=true` - Installation/uninstallation running
- `openyurt.io/yurthub-version` - (Optional) Custom YurtHub version

## Architecture

```
┌─────────────┐      ┌──────────────────────┐      ┌─────────────┐
│   kubectl   │─────▶│  YurtHub Installer   │─────▶│  Job (Pod)  │
│ label node  │      │     Controller       │      │ on target   │
└─────────────┘      └──────────────────────┘      │    node     │
                              │                     └─────────────┘
                              │                            │
                              ▼                            ▼
                     ┌─────────────────┐          ┌──────────────┐
                     │ Node Annotations│          │   YurtHub    │
                     │  - installed    │          │  (systemd)   │
                     │  - in-progress  │          └──────────────┘
                     └─────────────────┘
```

## Files

```
yurthubinstaller/
├── config/
│   └── config.go                          # Configuration types
├── constants.go                           # Labels and annotations
├── yurthub_installer_controller.go        # Main controller
├── yurthub_installer_controller_test.go   # Unit tests
├── enqueue_handlers.go                    # Event handlers
└── README.md                              # This file
```

## Events

The controller emits the following events on nodes:

- `InstallJobCreated`: Installation job created
- `UninstallJobCreated`: Uninstallation job created
- `InstallationComplete`: Installation succeeded
- `UninstallationComplete`: Uninstallation succeeded
- `InstallationFailed`: Installation/uninstallation failed

## Monitoring

### Check Installation Status

```bash
# View node annotations
kubectl get node <node-name> -o jsonpath='{.metadata.annotations}'

# Check installation jobs
kubectl get jobs -n kube-system | grep node-servant

# View events
kubectl describe node <node-name> | grep -A10 Events
```

### Troubleshooting

**Installation stuck in progress:**
```bash
# Check job status
kubectl get job -n kube-system -l node=<node-name>

# View job logs
kubectl logs -n kube-system job/node-servant-convert-<node-name>
```

**Job fails:**
- Verify node-servant image is available
- Check RBAC permissions
- Ensure node has systemd

## Security

### RBAC Requirements

Controller needs permissions for:
- Nodes: read, update
- Jobs: full CRUD
- Secrets: full CRUD (for bootstrap tokens)
- Events: create, patch

### Bootstrap Tokens

The controller creates per-node bootstrap tokens stored as Secrets in `kube-system` namespace. These enable YurtHub to authenticate with the Kubernetes API server.

## Testing

Run unit tests:
```bash
go test ./pkg/yurtmanager/controller/yurthubinstaller/...
```

Manual test:
```bash
# 1. Enable controller
# 2. Label a node
kubectl label node test-node openyurt.io/is-edge-worker=true

# 3. Watch progress
kubectl get jobs -n kube-system -w

# 4. Verify installation
ssh test-node "systemctl status yurthub"
```

## Limitations

1. **Bootstrap Tokens**: Current implementation uses simplified token management. Production should integrate with K8s Bootstrap Token API.
2. **Single Version**: All nodes get the same YurtHub version (unless overridden via annotation).
3. **No Health Monitoring**: Controller doesn't verify YurtHub continues running after installation.

## Future Work

- [ ] Kubernetes Bootstrap Token API integration
- [ ] Custom YurtHub configuration via ConfigMaps
- [ ] Periodic health checks
- [ ] Validating webhooks
- [ ] Installation metrics

## References

- [Feature Request #2494](https://github.com/openyurtio/openyurt/issues/2494)
- [Proposal Document](../../../docs/proposals/20260122-label-driven-yurthub-installation.md)
- [Node-Servant Package](../../node-servant/)
