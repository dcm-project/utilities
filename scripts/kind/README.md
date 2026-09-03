# Kind + Compose helpers

Shared scripts for connecting a [Kind](https://kind.sigs.k8s.io/) cluster to a Docker/Podman
compose network so workloads (environment-agent) can reach
the Kubernetes API at `https://kubernetes:6443`.

Used by [control-plane](https://github.com/dcm-project/control-plane) and
[environment-agent](https://github.com/dcm-project/environment-agent) local deploy flows.

## Prerequisites

- `kubectl` with current context `kind-<cluster-name>`
- Kind and compose on the same container runtime (Docker or Podman)
- Compose stack running before `kind-connect` (so the target network exists)

## Create a cluster

```bash
kind create cluster --name dcm-local --config scripts/kind/kind-local.yaml
kubectl config use-context kind-dcm-local
```

`kind-local.yaml` maps NodePorts `30081` and `30422` to localhost for in-cluster agent/NATS
verification on Docker Desktop and similar hosts. Omit `--config` for compose-only workflows.

## Scripts

| Script | Purpose |
|--------|---------|
| `install-kubevirt.sh` | Install KubeVirt on the current Kind cluster |
| `kubeconfig-for-compose.sh` | Write kubeconfig with API URL `https://kubernetes:6443` |
| `kind-connect.sh` | Join Kind node to a compose network with alias `kubernetes` |
| `kind-disconnect.sh` | Disconnect Kind from compose networks before `compose down` |
| `kind-env.sh` | Shared helpers (sourced by the scripts above) |

Run from any repo; set paths via environment variables (see below).

## Environment variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `COMPOSE_NETWORK` | yes (`kind-connect`) | — | Compose network name (e.g. `control-plane_default`) |
| `KUBECONFIG_OUT` | no | `${DEPLOY_ROOT}/deploy/.kube/config` or `./deploy/.kube/config` | Output kubeconfig path |
| `DEPLOY_ROOT` | no | — | Repo root when `KUBECONFIG_OUT` is unset |
| `COMPOSE_NETWORKS` | no | common network names | Space-separated list for `kind-disconnect` |
| `KIND_NETWORK_ALIAS` | no | `kubernetes` | Network alias (must match API server cert SAN) |
| `CONTAINER_ENGINE` | no | auto-detect | `docker` or `podman` |
| `KUBEVIRT_VERSION` | no | `v1.5.0` | KubeVirt release tag |

## Examples

**Control-plane + environment-agent** (from control-plane repo root):

```bash
make compose-up
bash ../utilities/scripts/kind/install-kubevirt.sh
DEPLOY_ROOT="$(pwd)" bash ../utilities/scripts/kind/kubeconfig-for-compose.sh
COMPOSE_NETWORK=control-plane_default bash ../utilities/scripts/kind/kind-connect.sh
# deploy/.env: AGENT_KUBECONFIG_HOST=deploy/.kube/config
make compose-up-with-agent
```

**Standalone environment-agent** (from environment-agent repo root):

```bash
bash ../utilities/scripts/kind/install-kubevirt.sh
DEPLOY_ROOT="$(pwd)" bash ../utilities/scripts/kind/kubeconfig-for-compose.sh
make compose-up
COMPOSE_NETWORK=environment-agent_default bash ../utilities/scripts/kind/kind-connect.sh
```

## Consumption from Makefiles

Consumer repos typically set `KIND_SCRIPTS_DIR ?= ../utilities/scripts/kind` and wrap calls with
repo-specific defaults:

```makefile
kubeconfig-for-compose:
	DEPLOY_ROOT="$(CURDIR)" bash $(KIND_SCRIPTS_DIR)/kubeconfig-for-compose.sh

kind-connect:
	COMPOSE_NETWORK=$(COMPOSE_NETWORK) bash $(KIND_SCRIPTS_DIR)/kind-connect.sh
```

## Troubleshooting

**Connection refused from agent to cluster**

- Confirm Kind is on the compose network: inspect the Kind node container networks.
- Regenerate kubeconfig and restart compose.
- Confirm `kubectl config current-context` is `kind-<cluster-name>`.

**TLS / certificate errors**

The API server certificate must include `kubernetes` as a SAN. `kind-connect` uses that alias
by default.
