# Compose network teardown

Helpers for tearing down compose stacks when external containers are attached to compose
networks (common with Kind + compose local dev).

## Script

`network-teardown.sh` has two subcommands (run **around** `compose down`, not in one shot):

| Subcommand | When | Purpose |
|------------|------|---------|
| `disconnect` | **Before** `compose down` | Detach all containers from named network(s) |
| `remove` | **After** `compose down` | Delete network(s) (best-effort) |

For Kind, also run `scripts/kind/kind-disconnect.sh` before `disconnect` — it targets the Kind
node by name when `kubectl` context is `kind-*`.

## Typical sequence

```bash
COMPOSE_NETWORK=control-plane_default bash ../utilities/scripts/kind/kind-disconnect.sh
COMPOSE_NETWORKS="deploy_default control-plane_default" \
	bash scripts/compose/network-teardown.sh disconnect

docker compose down -v --remove-orphans

COMPOSE_NETWORKS="deploy_default control-plane_default" \
	bash scripts/compose/network-teardown.sh remove
```

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `COMPOSE_NETWORKS` | common network names | Space-separated list of networks |
| `COMPOSE_NETWORK` | — | Single network (used when `COMPOSE_NETWORKS` unset) |
| `CONTAINER_ENGINE` | auto-detect | `podman` or `docker` |

## Consumer Makefile example

```makefile
UTILITIES_DIR ?= ../utilities
COMPOSE_NETWORKS ?= deploy_default $(COMPOSE_NETWORK)

disconnect-compose-networks:
	COMPOSE_NETWORKS="$(COMPOSE_NETWORKS)" CONTAINER_ENGINE=$(CONTAINER_ENGINE) \
		bash $(UTILITIES_DIR)/scripts/compose/network-teardown.sh disconnect

remove-compose-networks:
	COMPOSE_NETWORKS="$(COMPOSE_NETWORKS)" CONTAINER_ENGINE=$(CONTAINER_ENGINE) \
		bash $(UTILITIES_DIR)/scripts/compose/network-teardown.sh remove

compose-down: kind-disconnect disconnect-compose-networks
	$(COMPOSE) -f deploy/compose.yaml down -v --remove-orphans
	$(MAKE) remove-compose-networks
```
