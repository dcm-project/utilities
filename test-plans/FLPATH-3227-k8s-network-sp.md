# Test Plan: FLPATH-3227 — DCM Network Provider (K8s Network SP)

**Epic:** [FLPATH-3227](https://redhat.atlassian.net/browse/FLPATH-3227) — DCM network provider
**QE Task:** [FLPATH-4865](https://redhat.atlassian.net/browse/FLPATH-4865) — TESTING: DCM Network provider
**Status:** New
**Assignee:** Vlad Kolodny
**Repo (SP under test):** [dcm-project/k8s-network-service-provider](https://github.com/dcm-project/k8s-network-service-provider)
**Enhancement:** [k8s-network-sp.md](https://github.com/dcm-project/enhancements/blob/main/enhancements/k8s-network-sp/k8s-network-sp.md) ([PR #65](https://github.com/dcm-project/enhancements/pull/65))

## Summary

The K8s Network SP implements CREATE, READ, and DELETE for the DCM `network` service
type. It maps requests to Kubernetes `Service` resources (ClusterIP, NodePort,
LoadBalancer). Service type is inferred from `routing_level` and
`provider_hints.kubernetes.node_ports`.

**v1 non-goals:** UPDATE/day-2 ops, Ingress, NetworkPolicy, DNS, service mesh.

## Scope of this document

This plan covers **manual QE smoke** against a real cluster and DCM stack —
deploy wiring, agent registration, catalog, and thin happy-path checks.

**Execution mode:** **manual-only** until `tests/e2e/network_sp_*_test.go` and
`run-e2e.sh` label integration land (see [Future automation](#future-automation)).
Do not treat exit criteria below as automated CI gates until those files exist.

It does **not** duplicate the SP repo test matrix. Full CRUD, validation,
conflicts, pagination, NATS, and informer behavior are owned by the SP repo
plans (see [Test layer ownership](#test-layer-ownership)).

### API conventions (match existing utilities E2E)

- **Agents (not providers):** DCM migrated `/providers` → `/agents`. Use
  `GET /api/v1alpha1/agents` and filter `service_types` for `network` (same
  pattern as `discoverAgentByServiceType` in `tests/e2e/api_helpers_test.go` and
  `sp_container_api_test.go`).
- **Service types:** Use `GET /api/v1alpha1/service-types`, filter `results`
  for `service_type: network` (same pattern as `core_platform_test.go`). Do not
  assume `GET /service-types/network` unless verified on the deployed
  control-plane version.

**Related (out of scope here):** [FLPATH-4762](https://redhat.atlassian.net/browse/FLPATH-4762) UI
integration (ON_QA) — optional follow-up smoke.

## Test layer ownership

Each concern has **one primary owner**. Do not re-implement the same scenario in
another layer unless the row below says otherwise.

| Concern | Unit (SP repo, fake K8s) | Integration (SP repo, real cluster) | E2E (utilities, this plan) |
|---------|--------------------------|-------------------------------------|----------------------------|
| Config / defaults | **Owner** — TC-U001–U004 | — | — |
| Request validation matrix | **Owner** — TC-U020–U026 | — | Smoke only — E2E-08 |
| Service spec building / inference logic | **Owner** — TC-U010–U016 | — | — |
| Status mapping (PENDING/READY/DELETED) | **Owner** — TC-U030–U035 | Verify on cluster — TC-I031–I032 | — |
| Handler error mapping (409, 404, etc.) | **Owner** — TC-U060–U078 | **Owner** — real K8s — TC-I020–I052 | — |
| Duplicate Service **name** → 409 | TC-U061 | **Owner** — TC-I025 | — |
| Duplicate **nodePort** across Services → 409 | TC-U075 (mock) | **Owner** — see integration backlog | — |
| Selector with no matching pods (empty endpoints) | — | **Owner** — integration backlog | — |
| Registration retry / 4xx stop | **Owner** — TC-U110–U118 | TC-I010–I011 (real control-plane) | E2E-02 |
| Health handler / CheckHealth | **Owner** — TC-U080–U083 | TC-I011 | E2E-01, E2E-04 |
| CloudEvents + informer | **Owner** — TC-U040–U044, U100–U107 | **Owner** — TC-I060–I076 | — |
| NATS down, CRUD still works | — | **Owner** — TC-I090 | — |
| K8s API unavailable | — | **Owner** — TC-I091 | — |
| deploy-dcm.sh + host port 8090 | — | — | **Owner** — E2E-01 |
| Catalog `network` service type | — | — | **Owner** — E2E-03 |
| CatalogItem → Instance → K8s Service | — | — | **Owner** — E2E-07 |
| LoadBalancer PENDING (no LB controller) | — | **Owner** — TC-I024, TC-I031 | E2E-06 smoke (`@no-lb-controller`) |
| LoadBalancer READY (MetalLB / cloud LB) | — | **Owner** — integration + TC-I061 | — (`@requires-metallb` or cloud) |

### Upstream plans (source of truth for non-E2E)

| Layer | Location | PR CI |
|-------|----------|-------|
| Unit | `k8s-network-service-provider/.ai/test-plans/k8s-network-sp-unit.test-plan.md` | Yes (`make test`, ~25% implemented today) |
| Integration | `k8s-network-service-provider/.ai/test-plans/k8s-network-sp-integration.test-plan.md` | No (Kind + NATS; not in Makefile yet) |
| Spec | `k8s-network-service-provider/.ai/specs/k8s-network-sp.spec.md` | — |

**PR CI today:** config, health, registration (httptest), server lifecycle; network
CRUD handlers return 500 not-implemented.

### Integration backlog (SP repo — not duplicated here)

Add or execute in the SP integration plan, not in utilities E2E:

- TC-I020–I028, I030–I037, I050–I052, I080–I082 — full CRUD and type inference
- TC-I060–I076 — NATS and informer
- TC-I090–I092 — error scenarios
- **TC-I029** *(proposed)* — second Service/LB with same `node_ports` → 409
- **TC-I029b** *(proposed)* — create with selector, no matching pods → READY, empty endpoints
- **TC-I029c** *(proposed)* — invalid `?id=` → 400

## Prerequisites

- OpenShift or Kubernetes cluster; kubeconfig on deploy host
- Cluster CLI: set `CLUSTER_CLI` to `oc` (OCP) or `kubectl` (upstream K8s).
  Default: first available on `PATH` (`oc` preferred when both exist)
- DCM stack via `deploy-dcm.sh` with network SP (utilities override until
  control-plane `compose.yaml` adds a profile)
- Namespace for network Services (default or dedicated, e.g. `dcm-network-test`)
- Host ports **8080** (DCM control-plane) and **8090** (network SP) free before
  deploy — no leftover stack or other listener on those ports

> **CI note:** On Ecosystem Jenkins `flightpath-dcm-deploy`, the control-plane
> host port is remapped **8080 → 9080** to avoid conflicts (FLPATH-4421). Use
> `http://localhost:9080` for control-plane API calls in CI; local deploy uses
> `8080`. Network SP host port **8090** is unchanged.

```bash
CLUSTER_CLI="${CLUSTER_CLI:-$(command -v oc || command -v kubectl)}"

./scripts/deploy-dcm.sh \
  --k8s-network-service-provider \
  --kubeconfig ~/.kube/config \
  --k8s-network-namespace dcm-network-test
```

**Entry gate for Phases 2+:** SP image implements network CRUD (FLPATH-4796 merged;
POST/GET/DELETE not returning 500). Verify with a single POST before running
E2E-04+.

## Port map

| Service | Host URL |
|---------|----------|
| DCM control-plane API | `http://localhost:8080/api/v1alpha1/` |
| K8s Network SP (direct) | `http://localhost:8090/api/v1alpha1/networks` |
| Network SP health | `http://localhost:8090/api/v1alpha1/networks/health` |

If host 8080 is occupied, use compose override `9080:8080` and set
`DCM_GATEWAY_URL=http://localhost:9080/api/v1alpha1`.

## Service type inference (reference)

Used by integration tests and lab notes; not re-tested exhaustively in E2E.

| `routing_level` | `node_ports` | K8s Service type | Expected DCM status |
|-----------------|--------------|------------------|---------------------|
| omitted | No | ClusterIP | READY |
| omitted | Yes | NodePort | READY |
| `network` | No | LoadBalancer | PENDING (no LB) / READY (with MetalLB) |
| `network` | Yes | LoadBalancer | PENDING (no LB) / READY (with MetalLB) |
| `application` | — | — | Error (Ingress not supported v1) |

## E2E test cases (utilities)

Environment tags:

- `@lab-default` — runs on any cluster with kubeconfig
- `@no-lb-controller` — cluster has **no** cloud LB integration and no MetalLB;
  required for E2E-06 (LB stays PENDING). Skip on clouds where LoadBalancer
  Services get an external IP automatically.
- `@requires-metallb` — MetalLB installed; for SP integration LB→READY tests only

### Phase 1 — Deploy and DCM wiring (P0)

#### E2E-01: Deploy stack with network SP `@lab-default`

| Step | Action | Expected |
|------|--------|----------|
| 0 | Verify host ports free (see [Port map](#port-map)); e.g. `ss -tln \| grep -E ':8080\|:8090'` returns nothing on the ports you will use | No conflicting listener (avoids env pollution / false pass) |
| 1 | `deploy-dcm.sh --k8s-network-service-provider --kubeconfig <path>` | Exit 0 |
| 2 | `podman ps` | `k8s-network-service-provider` running |
| 3 | `curl -sf localhost:8080/api/v1alpha1/health` (or `9080` per CI note) | DCM healthy |
| 4 | `curl -sf localhost:8090/api/v1alpha1/networks/health` | SP healthy (`status: healthy`) |

#### E2E-02: Network agent registered in DCM `@lab-default`

| Step | Action | Expected |
|------|--------|----------|
| 1 | `GET /api/v1alpha1/agents` (page if needed) | HTTP 200 |
| 2 | Find agent where `service_types` contains `network` | Agent present (e.g. `k8s-network-sp`) |
| 3 | Check agent endpoint / metadata | Reachable network SP base URL; registration reflects `network` capability |

Reference implementation: `sp_container_api_test.go` registration context;
`discoverAgentByServiceType("network", "")` in `api_helpers_test.go`.

#### E2E-03: Catalog network service type `@lab-default`

| Step | Action | Expected |
|------|--------|----------|
| 1 | `GET /api/v1alpha1/service-types` | HTTP 200, `results` array |
| 2 | Filter `results` for `service_type: network` | Entry found |
| 3 | Inspect schema for that entry | `provider_hints.kubernetes` supports `selector`, `cluster_ip`, `node_ports` |

Reference implementation: `core_platform_test.go` “verifies the container service
type exists”.

### Phase 2 — SP API smoke (P0 — requires CRUD implemented)

#### E2E-04: Create and get ClusterIP `@lab-default`

Minimal POST to `http://localhost:8090/api/v1alpha1/networks`:

```json
{
  "spec": {
    "service_type": "network",
    "metadata": { "name": "e2e-clusterip-smoke" },
    "ports": [{ "name": "http", "protocol": "TCP", "port": 80, "target_port": 8080 }]
  }
}
```

| Step | Action | Expected |
|------|--------|----------|
| 1 | POST | HTTP 201 |
| 2 | Poll `GET .../networks/{id}` until `status: READY` or timeout (e.g. 60s) | `status: READY` (ClusterIP should be immediate; polling avoids flake) |
| 3 | `$CLUSTER_CLI get svc e2e-clusterip-smoke -n <ns>` | `TYPE=ClusterIP`, DCM labels present |
| 4 | GET response | `kubernetes.type: ClusterIP` |

#### E2E-05: Delete smoke `@lab-default`

| Step | Action | Expected |
|------|--------|----------|
| 1 | DELETE network created in E2E-04 | HTTP 204 |
| 2 | `$CLUSTER_CLI get svc -n <ns>` | Service removed |
| 3 | GET same id | HTTP 404 |

### Phase 3 — Lab-specific (P1)

#### E2E-06: LoadBalancer stays PENDING without LB controller `@no-lb-controller`

**Precondition:** No cloud LoadBalancer controller and no MetalLB (bare Kind,
typical lab OCP without MetalLB). **Skip** on EKS/GKE/ROSA etc. where external
IPs are assigned automatically.

POST with `routing_level: "network"`, no `node_ports`, unique `metadata.name`.

| Step | Action | Expected |
|------|--------|----------|
| 1 | POST | HTTP 201 |
| 2 | Poll GET; initial `status` may be `PENDING` | `status: PENDING` while `status.loadBalancer.ingress` is empty |
| 3 | `$CLUSTER_CLI get svc -n <ns>` | `TYPE=LoadBalancer`, no external IP / empty ingress |
| 4 | Poll GET for 2 min | Still `PENDING` (**pass** — not a defect on this cluster class) |

**If external IP appears:** cluster has an LB controller — skip this TC or record
`READY` as expected for that environment (document in run notes).

LoadBalancer → READY with MetalLB is **integration-only** (SP repo,
`@requires-metallb`).

### Phase 4 — Full DCM flow (P1)

#### E2E-07: Provision via catalog instance `@lab-default`

Mirrors `core_platform_test.go`: discover agent → catalog item → routing policy →
catalog item instance → verify placement and cluster object.

| Step | Action | Expected |
|------|--------|----------|
| 1 | `GET /api/v1alpha1/agents`; find agent where `service_types` contains `network` | Agent name recorded (e.g. `k8s-network-sp`) — same as E2E-02 |
| 2 | `POST /api/v1alpha1/catalog-items` with `service_type: network` and editable fields for `metadata.name`, `ports` | HTTP 201; `uid` saved |
| 3 | `POST /api/v1alpha1/policies` — GLOBAL policy with Rego `selected_agent` set to the network agent from step 1 | HTTP 201; policy `id` saved |
| 4 | `POST /api/v1alpha1/catalog-item-instances` referencing the catalog item and user values (unique `metadata.name`, port 80/TCP) | HTTP 201; `uid` and `run_id` present |
| 5 | Poll `GET /api/v1alpha1/service-type-instances/{resource_id}` until `status: RUNNING` (or timeout) | Instance reaches RUNNING |
| 6 | `GET` same instance | `agent_name` matches network agent from step 1 |
| 7 | `$CLUSTER_CLI get svc <metadata.name> -n <ns>` | Service exists with DCM labels |
| 8 | Compare instance / STI status with SP network status | Reflects SP status (READY for ClusterIP; PENDING for LB without controller) |

Example catalog item payload (adjust field paths to match deployed schema from E2E-03):

```json
{
  "api_version": "v1alpha1",
  "display_name": "e2e-network-catalog",
  "spec": {
    "resources": [{
      "name": "main",
      "service_type": "network",
      "fields": [
        {"path": "metadata.name", "display_name": "Service Name", "editable": true, "default": "e2e-network-inst"},
        {"path": "ports[0].name", "editable": false, "default": "http"},
        {"path": "ports[0].protocol", "editable": false, "default": "TCP"},
        {"path": "ports[0].port", "editable": false, "default": 80},
        {"path": "ports[0].target_port", "editable": false, "default": 8080}
      ]
    }]
  }
}
```

Example routing policy (package name must be unique per run):

```json
{
  "display_name": "e2e-network-policy",
  "policy_type": "GLOBAL",
  "priority": 100,
  "description": "E2E: route to network agent",
  "rego_code": "package e2e_network\n\nmain := {\"selected_agent\": \"k8s-network-sp\"}"
}
```

Reference implementation: `core_platform_test.go` — “discovers the container agent”,
“creates a routing policy”, “creates a catalog item instance”, “reaches RUNNING status”.

### Phase 5 — API contract smoke (P2)

#### E2E-08: Validation error returns RFC 9457 problem body `@lab-default`

| Step | Action | Expected |
|------|--------|----------|
| 1 | POST `/api/v1alpha1/networks` with `{}` | HTTP 400 |
| 2 | `Content-Type` | `application/problem+json` |
| 3 | Body | RFC 9457 fields: `type`, `title`, `status` (see `problem_detail_test.go`, `sp_container_api_test.go` contract context) |

RFC 9457 obsoletes RFC 7807; media type is unchanged. Full validation matrix —
**SP unit plan TC-U020–U026 only**.

## Monitoring (deferred)

NATS CloudEvents and informer behavior — **SP integration plan TC-I060–I076**.
Execute when monitoring lands in the SP; do not add parallel TCs in utilities.

**Gate:** FLPATH-4796 + monitoring stories closed; integration suite green.

## Cleanup

After each manual run or E2E job:

```bash
CLUSTER_CLI="${CLUSTER_CLI:-$(command -v oc || command -v kubectl)}"

# Delete DCM-managed Services in test namespace
$CLUSTER_CLI delete svc -n dcm-network-test -l dcm.project/managed-by=dcm

# Or tear down full stack
./scripts/deploy-dcm.sh --tear-down
```

## FLPATH-4865 exit criteria

**Manual execution** until Ginkgo automation exists (see Future automation).

| Level | Required tests | Notes |
|-------|----------------|-------|
| **Minimum (QE task)** | E2E-01–E2E-05 pass (manual checklist) | Blocked until CRUD not 500 |
| **Epic sign-off** | E2E-01–E2E-07 pass (manual) | Includes catalog instance flow |
| **Automation gate (future)** | Same TCs in `network_sp_*_test.go` green in CI | Not required for initial FLPATH-4865 close |
| **With monitoring** | SP integration TC-I060–I076 green | Dev-owned; not gating QE initially |
| **Deferred** | UI (FLPATH-4762), E2E-06 on cloud LB clusters | Document in ticket |

## Key configuration

| Variable | Default | Purpose |
|----------|---------|---------|
| `DCM_REGISTRATION_URL` | *(required)* | DCM SP registrar URL |
| `SP_ENDPOINT` | `http://k8s-network-service-provider:8080` | Registered with DCM |
| `SP_NAME` | `k8s-network-sp` | Provider name |
| `SP_K8S_NAMESPACE` | `default` | Namespace for Services |
| `SP_K8S_KUBECONFIG` | `/kubeconfig` | Cluster credentials |
| `SP_NATS_URL` | `nats://nats:4222` | Status events |

## Future automation (utilities)

Prerequisite before exit criteria can be automated:

| Item | Path / action |
|------|----------------|
| Ginkgo E2E | `tests/e2e/network_sp_*_test.go` — use `/agents`, `GET /service-types` filter, `discoverAgentByServiceType("network", ...)`, RFC 9457 via `problem_detail_test.go` |
| Helpers | `requireNetworkSP()` mirroring `requireContainerSP()` |
| run-e2e.sh | `--label-filter 'network'` (or `sp,network`) |
| Jenkins nightly | `flightpath-dcm-nightly` + `--k8s-network-service-provider` |
| control-plane | Add compose profile; shrink utilities override to port publish only |

## Dev implementation backlog (SP repo — not utilities)

| Area | Test case IDs |
|------|----------------|
| Unit — spec building, validation, handlers | TC-U010–U078, TC-U090–U092 |
| Unit — CloudEvents, debounce | TC-U040–U044, TC-U100–U107 |
| Integration — CRUD, inference | TC-I020–I037, I050–I052, I080–I082 |
| Integration — proposed gaps | TC-I029 (nodePort conflict), I029b (empty endpoints), I029c (invalid id) |
| Integration — NATS / informer | TC-I060–I076, I090–I092 |
