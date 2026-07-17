# OpenYurt Claude Code Skills

This directory contains executable AI skills designed for [Claude Code](https://claude.ai/code). These skills allow Claude to autonomously execute deployment and configuration tasks inside an existing Kubernetes cluster.

## Available Skills

### `/openyurt-deploy`
An interactive workflow for converting an existing Kubernetes cluster to an edge-native cluster using OpenYurt. 
This skill guides the AI assistant to:
- Verify preflight dependencies (including `crictl` for the conversion jobs).
- Deploy `yurt-manager` via Helm.
- Create Edge NodePools.
- Handle label-driven node conversion via the `YurtNodeConversionController`.
- Verify conversion via `isConvertedStable` logic.

### `/openyurt-raven`
Configures Raven for cross-region edge networking and L3/L7 tunnels across NodePools.

## Architecture
The skills are structured modularly for maintainability:
- `SKILL.md` acts as the entrypoint and restricts capabilities via YAML Frontmatter.
- `references/` contain the actual instructional logic, split into happy paths, troubleshooting decision trees, and rollback procedures.

## Security Constraints
These skills utilize YAML Frontmatter to restrict the capabilities of the AI assistant. For example, `allowed-tools` is strictly scoped to `Bash(kubectl *)`, `Bash(helm *)`, and `Bash(ssh *)`. The model cannot invoke these skills spontaneously without explicit user initiation (`disable-model-invocation: true`).
