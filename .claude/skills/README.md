# OpenYurt Claude Code skills

Project skills for [Claude Code](https://code.claude.com/docs/en/skills). Claude Code
picks them up automatically when it is started inside this repository.

| Skill | Invoke | Scope |
|---|---|---|
| [openyurt-deploy](openyurt-deploy/SKILL.md) | `/openyurt-deploy [node ...]` | Install yurt-manager v1.7.0 on an existing kubeadm cluster, convert/revert worker nodes with the label-driven `YurtNodeConversionController`, enable autonomy, recover failed conversions, uninstall |

Planned, not included yet: `openyurt-raven` (cross-pool networking), `yurtadm join`
flows, versioned references for releases after v1.7.0.

## Using `/openyurt-deploy`

Requirements on the machine that runs Claude Code: `kubectl`, `helm` v3 or v4, `bash`
(3.2 or later), and either `git` with an OpenYurt clone that has the `v1.7.0` tag or
`curl` to download the release tarball. Cluster requirements are listed in
[SKILL.md](openyurt-deploy/SKILL.md#supported-scope); `preflight-node.sh` and
`check-install.sh` verify them.

The skill:

- runs only when you type `/openyurt-deploy`, because `disable-model-invocation: true`;
  Claude never starts it on its own;
- asks you to confirm the target kube context once and passes it explicitly to every
  `kubectl` / `helm` / script call;
- shows a change card and waits for your "yes" before each cluster change.
  `allowed-tools` pre-approves only the read-only scripts. Whether other commands
  prompt depends on your own permission settings, so the skill does not rely on prompts.

## Layout and conventions

```
openyurt-deploy/
├── SKILL.md          entry point: scope, rules, navigation (kept short)
├── references/       loaded on demand
│   ├── openyurt-deploy-flow.md            happy path
│   ├── openyurt-deploy-troubleshooting.md symptom -> cause -> fix
│   ├── openyurt-deploy-recovery.md        state machine, failed rounds, host cleanup
│   └── openyurt-deploy-uninstall.md
└── scripts/          executed, not loaded; deterministic logic lives here
    ├── lib.sh                    shared constants mirrored from the v1.7.0 source
    ├── prepare-charts.sh         charts + yurthub-cfg manifest from the v1.7.0 tag
    ├── preflight-node.sh         host checks through a read-only Job
    ├── check-install.sh          reuse decision, Node webhook dry-run probe
    ├── wait-node-conversion.sh   round identification by Job UID
    ├── cleanup-node.sh           out-of-band host revert (mutating)
    └── collect-diagnostics.sh    per-node diagnostics and cluster-wide classification
```

Rules for changes:

- **Source is the reference.** Every behaviour the skill relies on is pinned to a
  release tag and cites the file it comes from. When supporting a new release, re-check
  `pkg/yurtmanager/controller/yurtnodeconversion`, `pkg/node-servant`,
  `pkg/yurtmanager/webhook/node` and the charts at that tag. Do not copy from master.
- **Scripts:**
  - must work with macOS bash 3.2: no associative arrays, no `mapfile`;
  - depend only on `kubectl` (plus `helm` where stated), and do not need `jq`;
  - take a mandatory `--context`;
  - print `KEY=VALUE` lines ending with `RESULT=`, and use documented exit codes;
  - read-only scripts must stay read-only; their header says whether they change anything.
- `check-install.sh --self-test` covers the controller/webhook flag semantics. Run it
  after touching that logic. `bash -n` and `shellcheck` all scripts.
