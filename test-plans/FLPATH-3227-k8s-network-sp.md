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

This plan defines **automated utilities E2E** (Ginkgo) for the K8s Network SP —
deploy wiring, agent registration, catalog, and thin happy-path checks against a
real cluster and DCM stack.

**Execution mode:** Ginkgo tests in `tests/e2e/network_sp_api_test.go`, run via
`tests/run-e2e.sh` (see [Test implementation](#test-implementation)). Each E2E-*
case below maps to one or more `It` blocks. Manual curl/kubectl steps are
reference only for debugging — not the deliverable.

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

**SP `main` today:** config, health, registration; network CRUD handlers return
500 not-implemented. CRUD is implemented on
[k8s-network-service-provider#2](https://github.com/dcm-project/k8s-network-service-provider/pull/2)
(not merged to `main` / Quay image not published yet). Utilities E2E Phase B
(E2E-04+) is gated on that merge.

### Integration backlog (SP repo — not duplicated here)

Add or execute in the SP integration plan, not in utilities E2E:

- TC-I020–I028, I030–I037, I050–I052, I080–I082 — full CRUD and type inference
- TC-I060–I076 — NATS and informer
- TC-I090–I092 — error scenarios
- **TC-I029** *(proposed)* — second Service/LB with same `node_ports` → 409
- **TC-I029b** *(proposed)* — create with selector, no matching pods → READY, empty endpoints
- **TC-I029c** *(proposed)* — invalid `?id=` → 400

## Prerequisites

- Go 1.23+, Ginkgo/Gomega (same as existing utilities E2E)
- OpenShift or Kubernetes cluster; kubeconfig on deploy host
- Cluster CLI for cluster assertions: `CLUSTER_CLI` = `oc` or `kubectl` (default:
  first on `PATH`)
- DCM stack with network SP — deployed by `run-e2e.sh` or `deploy-dcm.sh`
  (utilities compose override until control-plane adds a profile)
- Namespace for network Services (default or dedicated, e.g. `dcm-network-test`)
- Host ports **8080** (DCM control-plane) and **8090** (network SP) free before
  deploy

> **CI note:** On Ecosystem Jenkins `flightpath-dcm-deploy`, the control-plane
> host port is remapped **8080 → 9080** (FLPATH-4421). Set
> `DCM_GATEWAY_URL=http://localhost:9080/api/v1alpha1` in CI. Network SP host
> port **8090** is unchanged.

### Run automated tests

```bash
# Full lifecycle: deploy, run network SP tests, tear down
./tests/run-e2e.sh \
  --k8s-network-service-provider \
  --kubeconfig ~/.kube/config \
  --label-filter 'sp && network'

# Stack already up
./tests/run-e2e.sh --skip-deploy --skip-teardown --label-filter 'sp && network'
```

Deploy-only (debug):

```bash
./scripts/deploy-dcm.sh \
  --k8s-network-service-provider \
  --kubeconfig ~/.kube/config \
  --k8s-network-namespace dcm-network-test
```

**Phase B gate (E2E-04+):** SP CRUD on `main` and pullable Quay image
([SP PR #2](https://github.com/dcm-project/k8s-network-service-provider/pull/2)
merged). Until then, Phase A tests run green; Phase B tests use
`Label("crud")` and skip or fail fast with a clear precondition message.

## Port map

| Service | Host URL |
|---------|----------|
| DCM control-plane API | `http://localhost:8080/api/v1alpha1/` |
| K8s Network SP (direct) | `http://localhost:8090/api/v1alpha1/networks` |
| Network SP health | `http://localhost:8090/api/v1alpha1/networks/health` |

If host 8080 is occupied, use compose override `9080:8080` and set
`DCM_GATEWAY_URL=http://localhost:9080/api/v1alpha1`.

> Lab compose uses **HTTP only** by design (same as other SP utilities smoke);
> TLS termination is out of scope for FLPATH-4865.

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

Ginkgo labels (mirror container SP: `Label("sp", "container")`):

| Label | Meaning |
|-------|---------|
| `sp`, `network` | All network SP tests |
| `lab-default` | Any cluster with kubeconfig |
| `crud` | Requires SP CRUD on `main` (Phase B) |
| `no-lb-controller` | No cloud LB / MetalLB — E2E-06 only |
| `cluster` | Needs `$CLUSTER_CLI` cluster assertions |
| `contract` | RFC 9457 API contract (E2E-08) |

Legacy tags `@lab-default` / `@no-lb-controller` in tables below map to these labels.

### Phase 1 — Deploy and DCM wiring (P0)

#### E2E-01: Deploy stack with network SP `lab-default`

**Ginkgo:** `BeforeSuite` / deploy precondition + `Context("health")` in
`network_sp_api_test.go` (mirror `sp_container_api_test.go` health context).

| Step | Action | Expected |
|------|--------|----------|
| 0 | Verify host ports free (see [Port map](#port-map)); e.g. `ss -tln \| grep -E ':8080\|:8090'` returns nothing on the ports you will use | No conflicting listener (avoids env pollution / false pass) |
| 1 | `deploy-dcm.sh --k8s-network-service-provider --kubeconfig <path>` | Exit 0 |
| 2 | `podman ps` | `k8s-network-service-provider` running |
| 3 | `curl -sf localhost:8080/api/v1alpha1/health` (or `9080` per CI note) | DCM healthy |
| 4 | `curl -sf localhost:8090/api/v1alpha1/networks/health` | SP healthy (`status: healthy`) |

#### E2E-02: Network agent registered in DCM `lab-default`

| Step | Action | Expected |
|------|--------|----------|
| 1 | `GET /api/v1alpha1/agents` (page if needed) | HTTP 200 |
| 2 | Find agent where `service_types` contains `network` | Agent present (e.g. `k8s-network-sp`) |
| 3 | Check agent endpoint / metadata | Reachable network SP base URL; registration reflects `network` capability |

Reference implementation: `sp_container_api_test.go` registration context;
`discoverAgentByServiceType("network", "")` in `api_helpers_test.go`.

#### E2E-03: Catalog network service type `lab-default`

| Step | Action | Expected |
|------|--------|----------|
| 1 | `GET /api/v1alpha1/service-types` | HTTP 200, `results` array |
| 2 | Filter `results` for `service_type: network` | Entry found |
| 3 | Inspect schema for that entry | `provider_hints.kubernetes` supports `selector`, `cluster_ip`, `node_ports` |

Reference implementation: `core_platform_test.go` “verifies the container service
type exists”.

### Phase 2 — SP API smoke (P0 — `crud` label, requires SP PR #2 on `main`)

#### E2E-04: Create and get ClusterIP `lab-default`, `crud`, `cluster`

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
| 4 | GET response | `kubernetes.type: ClusterIP`; `spec.ports[0]` round-trips request: `protocol: TCP`, `port: 80`, `target_port: 8080` |

#### E2E-05: Delete smoke `lab-default`, `crud`, `cluster`

| Step | Action | Expected |
|------|--------|----------|
| 1 | DELETE network created in E2E-04 | HTTP 204 |
| 2 | `$CLUSTER_CLI get svc -n <ns>` | Service removed |
| 3 | GET same id | HTTP 404 |

### Phase 3 — Lab-specific (P1)

#### E2E-06: LoadBalancer stays PENDING without LB controller `no-lb-controller`, `crud`, `cluster`

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

#### E2E-07: Provision via catalog instance `lab-default`, `crud`, `cluster`

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

#### E2E-08: Validation error returns RFC 9457 problem body `lab-default`, `crud`, `contract`

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

**Automated:** `AfterEach` / `AfterSuite` in `network_sp_api_test.go` — delete
network resources created in the suite (track IDs like `sp_container_api_test.go`);
optional cluster cleanup via `$CLUSTER_CLI delete svc -n <ns> -l
dcm.project/managed-by=dcm`. Full stack teardown via `run-e2e.sh` (default) or
`./scripts/deploy-dcm.sh --tear-down`.

## FLPATH-4865 exit criteria

**Automated** — `make test-e2e` / `run-e2e.sh` with `--label-filter 'sp && network'`.

| Level | Required tests | Notes |
|-------|----------------|-------|
| **Phase A (QE minimum)** | E2E-01–E2E-03 green in CI | Deploy, agent, catalog — no SP CRUD required |
| **Phase B (QE + epic)** | E2E-01–E2E-07 green | After [SP PR #2](https://github.com/dcm-project/k8s-network-service-provider/pull/2) on `main` + Quay image |
| **Phase C** | E2E-08 green | RFC 9457 contract |
| **With monitoring** | SP integration TC-I060–I076 green | Dev-owned; not gating utilities E2E |
| **Deferred** | UI (FLPATH-4762), E2E-06 on cloud LB clusters | Skip via `no-lb-controller` label |

## Key configuration

| Variable | Default | Purpose |
|----------|---------|---------|
| `DCM_REGISTRATION_URL` | *(required)* | DCM SP registrar URL |
| `SP_ENDPOINT` | `http://k8s-network-service-provider:8080` | Registered with DCM |
| `SP_NAME` | `k8s-network-sp` | Provider name |
| `SP_K8S_NAMESPACE` | `default` | Namespace for Services |
| `SP_K8S_KUBECONFIG` | `/kubeconfig` | Cluster credentials |
| `SP_NATS_URL` | `nats://nats:4222` | Status events |
| `DCM_NETWORK_SP_URL` | `http://localhost:8090/api/v1alpha1` | Network SP direct URL (Ginkgo) |

## Test implementation (utilities)

| Item | Path / action |
|------|----------------|
| **Test file** | `tests/e2e/network_sp_api_test.go` — `Describe("Network SP API", Label("sp", "network"), ...)` |
| **Helpers** | Extend `tests/e2e/sp_helpers_test.go`: `initNetworkSP()`, `requireNetworkSP()`, `doNetworkSPRequest()` (mirror container SP) |
| **Control-plane** | Reuse `doRequest`, `discoverAgentByServiceType("network", ...)`, `expectRFC9457Problem` from existing helpers |
| **Cluster asserts** | Reuse `initKubectl()` / `$CLUSTER_CLI` pattern from `api_helpers_test.go` |
| **Run** | `./tests/run-e2e.sh --k8s-network-service-provider --label-filter 'sp && network'` |
| **Makefile** | `make test-sp` with network label (when wired) |
| **CI / Jenkins** | `flightpath-dcm-nightly` + `--k8s-network-service-provider` |
| **Rollout** | Phase A (E2E-01–03) land first; Phase B (E2E-04+) enable when SP PR #2 merges |
| **control-plane** | Add compose profile; shrink utilities override to port publish only |

### Phase A vs B (implementation order)

| Phase | E2E IDs | Depends on |
|-------|---------|------------|
| **A** | E2E-01–03 | This PR (deploy wiring on 8090) |
| **B** | E2E-04–08 | SP CRUD on `main`, Quay image published |

## Dev implementation backlog (SP repo — not utilities)

| Area | Test case IDs |
|------|----------------|
| Unit — spec building, validation, handlers | TC-U010–U078, TC-U090–U092 |
| Unit — CloudEvents, debounce | TC-U040–U044, TC-U100–U107 |
| Integration — CRUD, inference | TC-I020–I037, I050–I052, I080–I082 |
| Integration — proposed gaps | TC-I029 (nodePort conflict), I029b (empty endpoints), I029c (invalid id) |
| Integration — NATS / informer | TC-I060–I076, I090–I092 |
