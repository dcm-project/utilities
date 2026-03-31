# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Repo Is

DCM Utilities — a shared repository for common scripts and tooling used across the [dcm-project](https://github.com/dcm-project) ecosystem. Currently houses the E2E deploy script; will also contain the E2E test suite.

This repo contains **bash scripts, not Go code**. There is no build step or compiled output. It also contains E2E test plans and results under `test-plans/`.

## Cursor Integration

This repo includes `.cursor/` with rules, prompts, and agents for Cursor IDE. When using Cursor, context is loaded automatically from `.cursor/rules/` and task-specific prompts are available via `@<prompt-name>`. See `.cursor/prompts/README.md` for the full list.

## Important: Keep Docs Up to Date

When making changes in a PR, always check whether `CLAUDE.md`, `README.md`, and relevant `.cursor/` files need updating to reflect the change. This includes new flags, changed behavior, new scripts, or modified conventions. Update all affected files as part of the same PR.

## Linting

```bash
shellcheck scripts/*.sh
```

CI runs ShellCheck on changed `*.sh` files via `.github/workflows/lint.yaml` (only on PRs/pushes to `main`, only on changed files). Always validate locally before pushing.

## Key Script: `scripts/deploy-dcm.sh`

Deploys the full DCM stack for E2E testing by cloning api-gateway (which owns `compose.yaml`), running `podman-compose up`, and polling health endpoints until all services respond 2xx.

**Flow:** clone api-gateway → `podman-compose up -d` → verify containers running → poll `/api/v1alpha1/health/*` endpoints → collect container versions from Quay.io API → write `dcm-versions.json`.

**Modes:** The script has three mutually exclusive modes:
- **Deploy** (default): full clone + bring-up + health check. Pass `--cleanup-on-failure` to auto-teardown on error (default leaves partial state for debugging).
- `--running-versions`: query already-running containers, resolve git SHAs via Quay.io API, write `dcm-versions.json`
- `--tear-down`: stop containers, remove volumes, delete deploy directory

**Service provider profiles:** Optional compose profiles add extra services:
- `--kubevirt-service-provider` — requires OCP cluster with CNV installed
- `--k8s-container-service-provider` — works with any Kubernetes cluster
- `--all-service-providers` — enables all providers

**Cluster authentication:** When any provider is enabled, the script resolves cluster access in priority order: explicit `--kubeconfig`, existing `oc`/`kubectl` session, or `oc login` via `--cluster-api` + `--cluster-password`.

Run `./scripts/deploy-dcm.sh --help` for all flags and environment variable overrides.

### Script Structure

The script is organized into sections separated by comment banners. Key functions:

| Function | Purpose |
|----------|---------|
| `validate_deploy_dir` | Guards against `rm -rf` on system paths |
| `check_required_tools` | Verifies `git`, `podman`, `curl`, `jq`, etc. are installed |
| `tear_down` | Stops containers, removes volumes, deletes deploy dir |
| `resolve_kubeconfig` | Resolves cluster credentials (kubeconfig file, existing session, or `oc login`) |
| `validate_kubevirt_provider` | Checks cluster connectivity and CNV CRDs via `oc` |
| `validate_k8s_container_provider` | Checks cluster connectivity via `oc` or `kubectl` |
| `verify_health` | Confirms all compose services are running, then polls health endpoints with timeout |
| `resolve_git_sha` | Queries Quay.io tag API to map image digest → git commit SHA |
| `get_running_versions` | Iterates running containers, calls `resolve_git_sha`, writes JSON |

Argument parsing happens inline (not in a function) via a `while/case` loop after the function definitions.

## Shell Conventions

- Scripts use `set -euo pipefail` and `bash` (not POSIX sh).
- Constants are `readonly` at the top of the file.
- Logging helpers: `log()` for section headers (`==>`), `info()` for indented details, `err()` for stderr.
- Argument parsing uses a `while/case` loop with `require_arg` validation; flags take precedence over environment variables of the same name.
- Compose profiles are passed via array expansion: `${COMPOSE_PROFILES[@]+"${COMPOSE_PROFILES[@]}"}` (safe for empty arrays under `set -u`).

## `test-plans/`

E2E test plans and results for DCM service providers. Each file is named by Jira ticket (e.g. `FLPATH-3014-container-sp-api.md`). Test results are in `e2e-test-results-<date>.md`.

Test plans include:
- Scope and tier breakdowns (what's testable at each infrastructure level)
- Cross-references to upstream repo test plans (`.ai/test-plans/`) to avoid duplicating unit/integration coverage
- Code-verified behavior notes from actual PR implementations

## `dcm-versions.json`

Artifact produced by the deploy script (both deploy mode and `--running-versions`). Maps container image names to their digest and the git commit SHA that produced the image (resolved via Quay.io tag API). The file is gitignored at the repo root; the copy under `scripts/` is the authoritative output location.
