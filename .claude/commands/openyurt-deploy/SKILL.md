---
description: Deploy OpenYurt onto an existing Kubernetes cluster and convert edge nodes.
when_to_use: Use when the user wants to deploy OpenYurt, create NodePools, or convert existing worker nodes to edge nodes via the YurtNodeConversionController.
disable-model-invocation: true
allowed-tools: >
  Bash(kubectl *)
  Bash(helm *)
  Bash(ssh *)
  Read
  Grep
---

# OpenYurt Deployment Skill

This skill guides you through deploying OpenYurt on an existing Kubernetes cluster and converting worker nodes to edge nodes. 

## Instructions
Do not execute all steps blindly. Read the reference documents below to understand the flow, troubleshooting steps, and rollback procedures.

1. Read `references/openyurt-deploy-flow.md` to execute the happy-path deployment.
2. If any step fails (especially the Node Conversion phase), immediately stop and read `references/openyurt-deploy-troubleshooting.md` to diagnose the failure.
3. If the user requests to uninstall OpenYurt, read `references/openyurt-deploy-uninstall.md`.
