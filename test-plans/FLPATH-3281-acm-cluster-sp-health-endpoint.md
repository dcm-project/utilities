# Test Plan: FLPATH-3281 — Implement Health Endpoint for ACM Cluster SP

**Ticket:** [FLPATH-3281](https://redhat.atlassian.net/browse/FLPATH-3281)
**Status:** ON_QA
**Assignee:** Gabriel Farache
**Parent:** DCM: Create OCP service provider
**Repo:** [dcm-project/acm-cluster-service-provider](https://github.com/dcm-project/acm-cluster-service-provider)

## Summary

The ACM Cluster Service Provider exposes a health endpoint at `GET /api/v1alpha1/clusters/health` that verifies connectivity to the ACM Hub Kubernetes API, HyperShift CRDs, and platform-specific dependencies (KubeVirt VMI CRDs, Baremetal Agent CRDs). The HTTP server uses this endpoint as a readiness probe — registration with DCM only starts after health returns 200.

## Ticket Scope

These tickets verify the **API layer** — that the health endpoint exists, responds correctly, validates CRD availability, and gates registration with DCM. Full cluster provisioning requiring ACM Hub + HyperShift is tracked separately in [FLPATH-3378](https://redhat.atlassian.net/browse/FLPATH-3378).

### Deployment Status

As of 2026-03-31: The ACM Cluster SP has no container image or compose profile yet. Once containerized and added to the compose stack, the API-layer tests below are immediately executable. An OCP cluster **without** HyperShift CRDs is actually useful — it validates the "unhealthy" paths.

### Test Tiers

Verified against actual implementation in PR [#5](https://github.com/dcm-project/acm-cluster-service-provider/pull/5):

| Tier | Requires | Test Cases | Notes |
|------|----------|------------|-------|
| **API layer** (this ticket's scope) | SP binary + DCM stack + any K8s/OCP cluster | TC-02, TC-03, TC-04, TC-05, TC-06, TC-07, TC-08, TC-09 | All testable on any cluster |
| **Healthy path** (requires FLPATH-3378) | ACM Hub + HyperShift + platform CRDs | TC-01 | Only case needing all CRDs present |

**Note:** On our current OCP cluster (no HyperShift), the health endpoint should correctly return `"unhealthy"` — this is **valid and expected verification** that the health check works. TC-01 (healthy status) is the only case that requires full ACM components.

#### Code-verified behavior notes

- **Health endpoint ALWAYS returns HTTP 200** (PR #5): `GetHealth` in `internal/handler/handler.go` unconditionally returns `GetHealth200JSONResponse(result)`. Failed dependency checks are expressed as `status: "unhealthy"` in the JSON body, not as HTTP error codes.
- **Registration is NOT gated on `status: "healthy"`** (PR #5): The readiness probe in `internal/apiserver/server.go` (`waitForReady`) polls the health endpoint and only checks `resp.StatusCode == 200`. Since the handler always returns 200, **registration fires immediately** even when health status is `"unhealthy"`. This affects TC-08.
- **CRD checks use `List(limit=1)`** (PR #5): `checkCRDAvailable` calls `List` on the K8s API server with `client.Limit(1)` for each GVK. HyperShift `HostedClusterList` is checked first (critical); platform CRDs (KubeVirt VMI, Baremetal Agent) are checked conditionally based on `SP_ENABLED_PLATFORMS`.

### Upstream Test Coverage (in repo)

- **Unit:** `internal/health/health_unit_test.go` — mocked K8s client, healthy/unhealthy states (TC-HLT-UT-001–005)
- **Integration:** `internal/apiserver/health_integration_test.go` — HTTP route integration (TC-HLT-IT-001)
- **Spec:** `.ai/specs/acm-cluster-sp.spec.md` (REQ-HLT-010–120)

Key PR: [PR #5](https://github.com/dcm-project/acm-cluster-service-provider/pull/5) — health endpoint implementation

**What the repo tests already cover:**
- Health status determination logic (TC-HLT-UT-001–005, mocked CRD checks)
- HTTP route registration and response format (TC-HLT-IT-001)
- Timeout handling (TC-HLT-UT-003)

### References

- [service-provider-health-check enhancement](https://github.com/dcm-project/enhancements/blob/main/enhancements/service-provider-health-check/service-provider-health-check.md)
- ACM Cluster SP OpenAPI spec: `api/v1alpha1/openapi.yaml` in repo

## Prerequisites (API-layer tier)

- DCM stack deployed (`./scripts/deploy-dcm.sh`)
- ACM Cluster SP running (binary or container) — **does NOT require ACM Hub**
- Any Kubernetes/OCP cluster for connectivity
- The health endpoint will correctly report `"unhealthy"` on clusters without HyperShift CRDs — this is expected and verifiable

## Prerequisites (healthy path — FLPATH-3378)

- ACM Hub cluster with HyperShift enabled
- KubeVirt and/or Baremetal agent CRDs installed (depending on `SP_ENABLED_PLATFORMS`)

## Test Cases

### TC-01: Health endpoint returns 200 when all dependencies are available

**Preconditions:** SP is running, Hub cluster has HyperShift CRDs + enabled platform CRDs installed.

| Step | Action | Expected |
|------|--------|----------|
| 1 | `curl -s http://<sp-host>:8080/api/v1alpha1/clusters/health` | HTTP 200 |
| 2 | Parse JSON response | `status` = `"healthy"` |
| 3 | Validate response schema | Contains `type`, `status`, `path`, `version`, `uptime` |
| 4 | Check `type` field | `"acm-cluster-service-provider.dcm.io/health"` |
| 5 | Check `path` field | `"health"` |
| 6 | Check `uptime` field | Integer >= 0 |
| 7 | Check `version` field | Non-empty string |

### TC-02: Health response schema matches OpenAPI spec

**Preconditions:** SP is running (healthy or unhealthy — schema is the same either way).

| Step | Action | Expected |
|------|--------|----------|
| 1 | Fetch `/api/v1alpha1/clusters/health` | HTTP 200 |
| 2 | Validate JSON against `Health` schema in `api/v1alpha1/openapi.yaml` | All required fields present: `type` (string), `status` (string), `path` (string), `version` (string), `uptime` (integer) |
| 3 | Verify field values | `type` = `"acm-cluster-service-provider.dcm.io/health"`, `path` = `"health"`, `uptime` >= 0 |

### TC-03: Health returns unhealthy when HyperShift CRD is missing

**Preconditions:** SP is running against a cluster WITHOUT `hypershift.openshift.io` CRDs.

| Step | Action | Expected |
|------|--------|----------|
| 1 | `curl -s http://<sp-host>:8080/api/v1alpha1/clusters/health` | HTTP 200 (endpoint responds) |
| 2 | Parse JSON | `status` = `"unhealthy"` |
| 3 | Other fields (`type`, `path`, `version`, `uptime`) | Still populated correctly |

### TC-04: Health returns unhealthy when KubeVirt CRD is missing (kubevirt platform enabled)

**Preconditions:** `SP_ENABLED_PLATFORMS` includes `kubevirt`, but `kubevirt.io` CRDs are not installed on the cluster.

| Step | Action | Expected |
|------|--------|----------|
| 1 | Fetch health endpoint | HTTP 200 |
| 2 | Parse JSON | `status` = `"unhealthy"` |

### TC-05: Health returns unhealthy when Agent CRD is missing (baremetal platform enabled)

**Preconditions:** `SP_ENABLED_PLATFORMS` includes `baremetal`, but `agent-install.openshift.io` CRDs are not installed.

| Step | Action | Expected |
|------|--------|----------|
| 1 | Fetch health endpoint | HTTP 200 |
| 2 | Parse JSON | `status` = `"unhealthy"` |

### TC-06: Health check timeout is respected

**Preconditions:** Cluster API is slow or network-constrained. `SP_HEALTH_CHECK_TIMEOUT` is set to a low value (e.g., `1s`).

| Step | Action | Expected |
|------|--------|----------|
| 1 | Fetch health endpoint | Responds within ~1s even if cluster is slow |
| 2 | Parse JSON | `status` = `"unhealthy"` (timeout triggered) |

### TC-07: Uptime increments over time

| Step | Action | Expected |
|------|--------|----------|
| 1 | Fetch health, record `uptime` as T1 | Integer >= 0 |
| 2 | Wait 10 seconds | — |
| 3 | Fetch health, record `uptime` as T2 | T2 >= T1 + 9 (within tolerance) |

### TC-08: Readiness gate — registration starts after health endpoint is reachable

**Preconditions:** SP starts fresh; DCM service-provider-manager is running.

**Important (verified from PR #5 code):** The readiness probe only checks `resp.StatusCode == 200`, NOT the JSON `status` field. Since the health handler always returns HTTP 200, registration fires as soon as the process is up — even with `status: "unhealthy"`. The gate prevents registration during startup (before the HTTP server binds), not during unhealthy states.

| Step | Action | Expected |
|------|--------|----------|
| 1 | Start the SP | SP starts HTTP server |
| 2 | Health endpoint returns HTTP 200 (with `status: "unhealthy"` on cluster without HyperShift CRDs) | Readiness probe succeeds |
| 3 | Check DCM provider list shortly after startup | SP is registered with `serviceType: cluster` (even though health status is `"unhealthy"`) |
| 4 | Verify registration payload | `operations: CREATE, DELETE, READ`, endpoint matches config |

### TC-09: Health endpoint via gateway (E2E)

**Preconditions:** Full DCM stack deployed with ACM SP, traffic routed through api-gateway.

| Step | Action | Expected |
|------|--------|----------|
| 1 | `curl http://localhost:9080/api/v1alpha1/health/providers` | Includes ACM Cluster SP health |

## Upstream Test Coverage (in repo)

**Run:** `make test` in the repo.

- `internal/health/health_unit_test.go` — TC-HLT-UT-001–005 (mocked K8s client)
- `internal/apiserver/health_integration_test.go` — TC-HLT-IT-001 (HTTP integration)

### E2E vs Unit/Integration Boundary

| Concern | Tested by repo (fake client) | E2E adds value |
|---------|------------------------------|----------------|
| Health status logic | Yes (mocked CRD discovery) | Minimal |
| Real CRD availability checks | No (mocked) | **Yes** — actual cluster CRD state |
| Readiness gate → registration | No (mocked SPRM) | **Yes** — live SPRM timing |
| Uptime accuracy | Yes (unit test) | Marginal |
| Timeout with real cluster latency | No (mocked timeout) | **Yes** — real network conditions |

## Environment Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `SP_HEALTH_CHECK_TIMEOUT` | `5s` | Timeout for individual health checks |
| `SP_ENABLED_PLATFORMS` | `kubevirt,baremetal` | Platforms to check CRDs for |
| `SP_SERVER_ADDRESS` | `:8080` | Listen address |
