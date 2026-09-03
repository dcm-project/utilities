# Kind + Compose helpers

Scripts for connecting a [Kind](https://kind.sigs.k8s.io/) cluster to a Docker/Podman
compose network so in-container workloads can reach the Kubernetes API at
`https://kubernetes:6443`.

Used by [control-plane](https://github.com/dcm-project/control-plane) and
[environment-agent](https://github.com/dcm-project/environment-agent) local deploy flows.

For KubeVirt installation (any cluster), see [../kubevirt/README.md](../kubevirt/README.md).
For compose network teardown (not Kind-specific), see [../compose/README.md](../compose/README.md).

## Prerequisites

- `kubectl` with current context `kind-<cluster-name>`
- Kind and compose on the **same** container runtime (Docker or Podman)
- Compose stack running before `kind-connect` (so the target network exists)

## Create a cluster

```bash
kind create cluster --name dcm-local
kubectl config use-context kind-dcm-local
```

**For In-cluster agent** (agent Pod on Kind): see the
[environment-agent in-cluster guide](https://github.com/dcm-project/environment-agent/blob/main/deploy/docs/in-cluster.md)
and pass `--config scripts/kind/kind-local.yaml` so NodePorts `30081` (agent) and `30422` (NATS)
map to localhost.

```bash
kind create cluster --name dcm-local --config scripts/kind/kind-local.yaml
kubectl config use-context kind-dcm-local
```

## Scripts

| Script | Purpose |
|--------|---------|
| `kubeconfig-for-compose.sh` | Write kubeconfig with API URL `https://kubernetes:6443` |
| `kind-connect.sh` | Join Kind node to a compose network with alias `kubernetes` |
| `kind-disconnect.sh` | Disconnect Kind node from compose network(s) |
| `kind-env.sh` | Shared helpers (sourced by the scripts above) |

## Environment variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `COMPOSE_NETWORK` | yes (`kind-connect`) | — | Compose network name (e.g. `control-plane_default`) |
| `KUBECONFIG_OUT` | no | `${DEPLOY_ROOT}/deploy/.kube/config` or `./deploy/.kube/config` | Output kubeconfig path |
| `DEPLOY_ROOT` | no | — | Repo root when `KUBECONFIG_OUT` is unset |
| `COMPOSE_NETWORKS` | no | common network names | Space-separated list for `kind-disconnect` |
| `KIND_NETWORK_ALIAS` | no | `kubernetes` | Network alias (must match API server cert SAN) |
| `CONTAINER_ENGINE` | no | auto-detect | `docker` or `podman` |

## Examples

**Control-plane + environment-agent** (from control-plane repo root):

```bash
make compose-up
bash ../utilities/scripts/kubevirt/install-kubevirt.sh   # when vm SP is enabled
DEPLOY_ROOT="$(pwd)" bash ../utilities/scripts/kind/kubeconfig-for-compose.sh
COMPOSE_NETWORK=control-plane_default bash ../utilities/scripts/kind/kind-connect.sh
make compose-up-with-agent
```

**Teardown:**

```bash
COMPOSE_NETWORK=control-plane_default bash ../utilities/scripts/kind/kind-disconnect.sh
COMPOSE_NETWORKS="deploy_default control-plane_default" \
	bash ../utilities/scripts/compose/network-teardown.sh disconnect
make compose-down   # compose down + network-teardown remove — see compose/README.md
```

## Consumption from Makefiles

```makefile
UTILITIES_DIR ?= ../utilities
KIND_SCRIPTS_DIR ?= $(UTILITIES_DIR)/scripts/kind

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
