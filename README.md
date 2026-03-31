# DCM Utilities

Common scripts and tooling shared across the [DCM](https://github.com/dcm-project) ecosystem. This repository currently provides the E2E deploy script for bringing up the full DCM stack locally. An E2E test suite will be added in the future.

## Contents

| Path | Description |
|------|-------------|
| `scripts/deploy-dcm.sh` | Deploy, health-check, and tear down the full DCM stack via podman-compose |
| `scripts/dcm-versions.json` | Example output of container version resolution |
| `test-plans/` | E2E test plans and results for DCM service providers |

### `dcm-versions.json`

Both deploy mode and `--running-versions` produce a `dcm-versions.json` mapping each container image to its digest and the git commit SHA that built it (resolved via the Quay.io tag API). Third-party images show `null` for `git_sha`.

```json
{
  "quay.io/dcm-project/catalog-manager:latest": {
    "image_digest": "sha256:1cdf5482f586ce513724074c0a132b718672d2be5cbae600a47e94324078b01e",
    "git_sha": "2388248"
  },
  "docker.io/library/postgres:16-alpine": {
    "image_digest": "sha256:b7587f3cb74f4f4b2a4f9d67f052edbf95eb93f4fec7c5ada3792546caaf7383",
    "git_sha": null
  }
}
```

## E2E Deploy Script

`scripts/deploy-dcm.sh` automates the full DCM stack lifecycle for E2E testing:

1. Clones the [api-gateway](https://github.com/dcm-project/api-gateway) repo (which owns the `compose.yaml`)
2. Starts all services with `podman-compose up`
3. Polls health endpoints until every service responds 2xx
4. Resolves running container images to git commit SHAs via the Quay.io API

### Prerequisites

- `git`, `podman`, `podman-compose`, `curl`, `jq`
- `oc` (for KubeVirt provider; also used for `oc login` auth)
- `oc` or `kubectl` (for k8s container provider — either works)

### Quick Start

```bash
# 1. Deploy the full DCM stack (no providers)
./scripts/deploy-dcm.sh

# 2. Deploy with the k8s container service provider (auto-detects cluster)
./scripts/deploy-dcm.sh --k8s-container-service-provider

# 3. Deploy with KubeVirt + explicit kubeconfig
./scripts/deploy-dcm.sh --kubevirt-service-provider --kubeconfig ~/.kube/config

# 4. Deploy all providers, logging in via oc
./scripts/deploy-dcm.sh --all-service-providers \
    --cluster-api https://api.cluster.example.com --cluster-password secret

# 5. Tear down when done
./scripts/deploy-dcm.sh --tear-down
```

Run `./scripts/deploy-dcm.sh --help` for all flags and environment variable overrides.

## Cursor Integration

This repo includes configuration for [Cursor](https://cursor.sh) and [Claude Code](https://claude.ai/code):

| Path | Purpose |
|------|---------|
| `CLAUDE.md` | Consolidated project context (works in any AI tool) |
| `.cursor/rules/` | Auto-loaded context rules for Cursor |
| `.cursor/prompts/` | Task-specific prompt templates (use `@<name>` in Cursor) |
| `.cursor/agents/` | Specialized agent definitions |

Available prompts: `@deploy-dcm`, `@tear-down`, `@check-versions`, `@troubleshoot-deploy`, `@maintain-pr-summary`.

## Development

### Linting

Shell scripts are linted with [ShellCheck](https://www.shellcheck.net/). CI runs ShellCheck automatically on PRs against changed `*.sh` files.

```bash
shellcheck scripts/*.sh
```

## License

Apache 2.0 — see [LICENSE](LICENSE).
