# OpenYurt Claude Code Skills

This directory contains executable AI skills designed for [Claude Code](https://claude.ai/code). These skills allow Claude to autonomously execute deployment, configuration, and troubleshooting tasks inside an existing Kubernetes cluster running OpenYurt.

## Available Skills

### `/openyurt-deploy`
An interactive workflow for converting an existing Kubernetes cluster to an edge-native cluster using OpenYurt.
This skill guides the AI assistant to:
- Run autonomous preflight validation (including SSH-based `crictl` detection and CRI socket path verification for both kubeadm and k3s).
- Deploy `yurt-manager` via Helm with configurable parameters.
- Create Edge and Cloud NodePool CRs (`apps.openyurt.io/v1beta2`).
- Handle label-driven node conversion via the `YurtNodeConversionController`.
- Execute an automated bash polling loop to verify the `isConvertedStable` state (checking `is-edge-worker=true`, `YurtConversionFailed=False/Converted`, and node uncordoned).
- Enable edge autonomy via the correct annotation path.
- Run a comprehensive final verification checklist.
- Diagnose failures via a structured decision tree covering CRI socket errors, webhook rejections, yurt-manager RBAC issues, and more.
- Perform clean rollback and uninstall, including cleanup of stale Jobs and YurtHub static pod manifests on the node filesystem.

### `/openyurt-raven`
Configures Raven for cross-region edge networking and L3/L7 tunnels across NodePools.
This skill guides the AI assistant to:
- Generate VPN PSK and deploy `raven-agent` DaemonSet via Helm.
- Create Gateway CRs with proper `v1beta1` schema and `LabelSelector` shape (respecting the `SetDefaultsGateway` mutating webhook behavior).
- Support fine-grained per-Gateway overrides via `raven.openyurt.io/enable-tunnel` and `raven.openyurt.io/enable-proxy` annotations.
- Deploy ephemeral test pods and run an autonomous cross-NodePool L3 connectivity test.
- Diagnose failures via a structured troubleshooting tree covering libreswan, PSK mismatch, webhook overwrite, and firewall rules.

## Architecture
The skills are structured modularly for maintainability:
- `SKILL.md` acts as the entrypoint. It defines `allowed-tools` restrictions and `when_to_use` triggers via YAML Frontmatter.
- `references/` contain the actual instructional logic, split into:
  - **Happy-path flows** (step-by-step deployment/configuration)
  - **Troubleshooting decision trees** (structured diagnosis with executable bash scripts)
  - **Rollback/uninstall procedures** (automated revert polling and cleanup)

```
.claude/skills/
├── README.md
├── openyurt-deploy/
│   ├── SKILL.md
│   └── references/
│       ├── openyurt-deploy-flow.md
│       ├── openyurt-deploy-troubleshooting.md
│       └── openyurt-deploy-uninstall.md
└── openyurt-raven/
    ├── SKILL.md
    └── references/
        ├── openyurt-raven-flow.md
        └── openyurt-raven-troubleshooting.md
```

## Security Constraints
These skills utilize YAML Frontmatter to restrict the capabilities of the AI assistant:
- **`allowed-tools`**: Strictly scoped to `Bash(kubectl *)`, `Bash(helm *)`, and other explicitly required tools. Claude cannot execute arbitrary shell commands.
- **`disable-model-invocation: true`**: The skills cannot be triggered by ambient conversation. They only run when the user explicitly invokes the slash command, because they modify cluster state.
