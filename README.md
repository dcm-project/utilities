# DCM Utilities

Common scripts and tooling shared across the [DCM](https://github.com/dcm-project) ecosystem. This repository currently provides the E2E deploy script for bringing up the full DCM stack locally. An E2E test suite will be added in the future.

## Contents

| Path | Description |
|------|-------------|
| `scripts/deploy-dcm.sh` | Deploy, health-check, and tear down the full DCM stack via podman-compose |
| `scripts/dcm-versions.json` | Example output of container version resolution |

## E2E Deploy Script

`scripts/deploy-dcm.sh` automates the full DCM stack lifecycle for E2E testing:

1. Clones the [api-gateway](https://github.com/dcm-project/api-gateway) repo (which owns the `compose.yaml`)
2. Starts all services with `podman-compose up`
3. Polls health endpoints until every service responds 2xx
4. Resolves running container images to git commit SHAs via the Quay.io API

### Prerequisites

- `git`, `podman`, `podman-compose`, `curl`, `jq`
- `oc` (only when enabling the KubeVirt service provider)

### Quick Start

```bash
# 1. Deploy the full DCM stack
./scripts/deploy-dcm.sh

# 2. Deploy with the KubeVirt service provider
./scripts/deploy-dcm.sh --kubevirt-service-provider --kubeconfig ~/.kube/config

# 3. Tear down when done
./scripts/deploy-dcm.sh --tear-down
```

Run `./scripts/deploy-dcm.sh --help` for all flags and environment variable overrides.

## Development

### Linting

Shell scripts are linted with [ShellCheck](https://www.shellcheck.net/). CI runs ShellCheck automatically on PRs against changed `*.sh` files.

```bash
shellcheck scripts/*.sh
```

## License

Apache 2.0 — see [LICENSE](LICENSE).
