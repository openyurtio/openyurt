---
description: Configure Raven for cross-region edge networking in OpenYurt.
when_to_use: Use when the user wants to configure VPNs, cross-NodePool networking, or L3/L7 tunnels via Raven.
disable-model-invocation: true
allowed-tools: >
  Bash(kubectl *)
  Bash(helm *)
  Bash(openssl *)
  Read
  Grep
---

# OpenYurt Raven Skill

This skill guides you through deploying Raven for cross-NodePool networking in OpenYurt.

## Instructions
Do not execute steps blindly. Read the reference document below to understand the configuration flow.

1. Read `references/openyurt-raven-flow.md` to execute the Raven configuration.
2. If `yurt-manager` is not installed, inform the user they must run `/openyurt-deploy` first.
