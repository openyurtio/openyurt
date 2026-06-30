# OpenYurt Claude Code Skills

This directory contains [Claude Code](https://claude.ai/code) Skills — slash commands that help you deploy and configure OpenYurt interactively without needing to read through the full documentation.

## Available Skills

| Command | Description |
|---------|-------------|
| `/openyurt-deploy` | Deploy OpenYurt on an existing Kubernetes cluster |
| `/openyurt-raven` | Configure Raven for cross-region networking |

## Usage

Open this repository in Claude Code and type the slash command. Claude will read the skill file and execute the described steps interactively on your behalf.

```
/openyurt-deploy
/openyurt-raven
```

## How It Works

Skills are Markdown files stored in `.claude/commands/`. When you invoke a slash command, Claude Code reads the corresponding file and follows the instructions step by step, running commands, checking outputs, and handling errors — all while keeping you informed.

## Prerequisites

- [kubectl](https://kubernetes.io/docs/tasks/tools/) configured to reach your target cluster
- [Helm](https://helm.sh/docs/intro/install/) ≥ v3
- Kubernetes ≥ 1.24
- OpenYurt deployed (required for `/openyurt-raven`)

## Related Resources

- [OpenYurt Documentation](https://openyurt.io/docs)
- [Helm Charts](../../charts/)
- [Raven Repository](https://github.com/openyurtio/raven)
