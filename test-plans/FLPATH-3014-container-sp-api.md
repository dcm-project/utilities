# Test Plan: FLPATH-3014 — Implement Container SP API

**Ticket:** [FLPATH-3014](https://redhat.atlassian.net/browse/FLPATH-3014)
**Status:** ON_QA
**Assignee:** Gabriel Farache
**Parent:** DCM: container provider
**Repo:** [dcm-project/k8s-container-service-provider](https://github.com/dcm-project/k8s-container-service-provider)
**Blocks:** FLPATH-3016 (Bootstrap Container SP), FLPATH-2924 (ADR container provider), FLPATH-3015 (Health Endpoint for Container SP)

## Summary

The K8s Container SP implements a CRUD API for managing containers as Kubernetes Deployments (with Services for networking). It supports creating, listing, getting, and deleting containers. The API registers with DCM as `serviceType: container` with `operations: CREATE, DELETE, READ`. Resources are managed via DCM labels (`dcm.project/managed-by`, `dcm.project/dcm-instance-id`, `dcm.project/dcm-service-type`).

## Scope

This plan covers **E2E tests against a real cluster** — verifying behavior that unit/integration tests with fake clients cannot cover. For input validation, handler logic, and K8s store edge cases, see the repo's own test plans.

**E2E focus areas:** Real K8s Deployment/Service lifecycle, label verification on cluster, cross-service registration flow, pagination against real data, and API responses with live Pod status.

### Upstream Test Coverage (in repo)

The repo has comprehensive unit + integration test plans in `.ai/test-plans/`:

- **Integration:** `.ai/test-plans/k8s-container-sp-integration.test-plan.md` — 114+ test cases (Ginkgo, fake K8s client)
- **Unit:** `.ai/test-plans/k8s-container-sp-unit.test-plan.md`
- **Spec:** `.ai/specs/k8s-container-sp.spec.md`

Key PRs:
- [PR #5](https://github.com/dcm-project/k8s-container-service-provider/pull/5) — requirements and test plans
- [PR #9](https://github.com/dcm-project/k8s-container-service-provider/pull/9) — API handler implementation
- [PR #14](https://github.com/dcm-project/k8s-container-service-provider/pull/14) — API alignment fixes

**What the repo tests already cover (no need to duplicate at E2E):**
- OpenAPI request validation (TC-I008, TC-I009 — table-driven with fake server)
- Handler error mapping and RFC 7807 format (TC-U001–TC-U056)
- Pagination boundary conditions (TC-U057, TC-I008)
- Graceful shutdown / SIGTERM handling (TC-I004, TC-I005)
- Registration retry + backoff logic (TC-I010–TC-I016)

### References

- [k8s-container-sp enhancement](https://github.com/dcm-project/enhancements/blob/main/enhancements/k8s-container-sp/k8s-container-sp.md)
- OpenAPI spec: `api/v1alpha1/openapi.yaml` in repo

## Prerequisites

- Kubernetes cluster accessible
- NATS server running (for status monitoring)
- DCM stack deployed (`./scripts/deploy-dcm.sh`)
- K8s Container SP running and registered with DCM

## API Endpoints Under Test

| Method | Path | Operation |
|--------|------|-----------|
| POST | `/api/v1alpha1/containers` | Create a container |
| GET | `/api/v1alpha1/containers` | List containers |
| GET | `/api/v1alpha1/containers/{container_id}` | Get a container |
| DELETE | `/api/v1alpha1/containers/{container_id}` | Delete a container |
| GET | `/api/v1alpha1/containers/health` | Health check |

## Test Cases

### Registration

#### TC-01: SP registers with DCM on startup

| Step | Action | Expected |
|------|--------|----------|
| 1 | Start SP with `DCM_REGISTRATION_URL` pointing at SPRM | SP starts, health passes |
| 2 | Query DCM provider list | Provider with `serviceType: container` exists |
| 3 | Check registered operations | `CREATE, DELETE, READ` |
| 4 | Check registered endpoint | Contains `/api/v1alpha1/containers` (from `PostPath()`) |
| 5 | Check `schemaVersion` | `v1alpha1` |

#### TC-02: Registration retries on failure

| Step | Action | Expected |
|------|--------|----------|
| 1 | Start SP with SPRM unavailable | SP starts, health passes, registration retries |
| 2 | Start SPRM | SP eventually registers successfully (exponential backoff) |

#### TC-03: Registration stops retrying on 4xx

| Step | Action | Expected |
|------|--------|----------|
| 1 | SP sends registration with invalid payload | SPRM returns 4xx |
| 2 | Check SP logs | Non-retryable error logged, no infinite retry loop |

### Health Endpoint

#### TC-04: Health returns healthy

| Step | Action | Expected |
|------|--------|----------|
| 1 | `curl http://<sp>:8080/api/v1alpha1/containers/health` | HTTP 200 |
| 2 | Parse response | `status: "healthy"`, `type: "k8s-container-service-provider.dcm.io/health"` |
| 3 | Check `uptime` | Integer >= 0 |
| 4 | Check `version` | Non-empty string |

### Create Container

#### TC-05: Create a container with valid spec

| Step | Action | Expected |
|------|--------|----------|
| 1 | POST `/api/v1alpha1/containers` with valid container spec (image, ports, resources) | HTTP 200/201 |
| 2 | Check response body | Contains `id`, container details |
| 3 | Verify on K8s cluster | Deployment created with DCM labels |
| 4 | Check labels on Deployment | `dcm.project/managed-by=dcm`, `dcm.project/dcm-service-type=container`, `dcm.project/dcm-instance-id=<id>` |

#### TC-06: Create with custom ID (via query parameter)

| Step | Action | Expected |
|------|--------|----------|
| 1 | POST `/api/v1alpha1/containers?id=my-custom-id` | HTTP 200/201 |
| 2 | Check response `id` | `my-custom-id` |
| 3 | GET `/api/v1alpha1/containers/my-custom-id` | HTTP 200 |

#### TC-07: Create with auto-generated ID (no query parameter)

| Step | Action | Expected |
|------|--------|----------|
| 1 | POST `/api/v1alpha1/containers` (no `id` query) | HTTP 200/201 |
| 2 | Check response `id` | Valid UUID |

#### TC-08: Create returns error for invalid spec

| Step | Action | Expected |
|------|--------|----------|
| 1 | POST with empty body | HTTP 400 |
| 2 | POST with missing required fields (e.g., no image) | HTTP 400 with descriptive error |
| 3 | POST with invalid field types | HTTP 400 (OpenAPI validation) |

#### TC-09: Create provisions a Service when ports are specified

| Step | Action | Expected |
|------|--------|----------|
| 1 | POST with container spec including port definitions | HTTP 200/201 |
| 2 | Verify on K8s | Both Deployment and Service created |
| 3 | Check Service selectors | Match DCM labels on the Deployment |

### Get Container

#### TC-10: Get an existing container

| Step | Action | Expected |
|------|--------|----------|
| 1 | Create a container, note `id` | — |
| 2 | GET `/api/v1alpha1/containers/{id}` | HTTP 200 |
| 3 | Response includes status from Pod | Pod phase reflected (PENDING, RUNNING, etc.) |

#### TC-11: Get a non-existent container

| Step | Action | Expected |
|------|--------|----------|
| 1 | GET `/api/v1alpha1/containers/does-not-exist` | HTTP 404 |

### List Containers

#### TC-12: List returns all managed containers

| Step | Action | Expected |
|------|--------|----------|
| 1 | Create 3 containers | — |
| 2 | GET `/api/v1alpha1/containers` | HTTP 200 |
| 3 | Check results | All 3 containers present |

#### TC-13: List supports pagination

| Step | Action | Expected |
|------|--------|----------|
| 1 | Create 5+ containers | — |
| 2 | GET `/api/v1alpha1/containers?max_page_size=2` | HTTP 200, results contain 2 items + `next_page_token` |
| 3 | GET with `page_token` | Next page of results |

#### TC-14: List only returns DCM-managed containers

| Step | Action | Expected |
|------|--------|----------|
| 1 | Create a Deployment manually (no DCM labels) in the same namespace | — |
| 2 | GET `/api/v1alpha1/containers` | Manual Deployment NOT in results |

### Delete Container

#### TC-15: Delete an existing container

| Step | Action | Expected |
|------|--------|----------|
| 1 | Create a container, note `id` | — |
| 2 | DELETE `/api/v1alpha1/containers/{id}` | HTTP 204 |
| 3 | Verify on K8s | Deployment (and Service if created) removed |
| 4 | GET the same `id` | HTTP 404 |

#### TC-16: Delete a non-existent container

| Step | Action | Expected |
|------|--------|----------|
| 1 | DELETE `/api/v1alpha1/containers/does-not-exist` | HTTP 404 |

### OpenAPI Validation

#### TC-17: Requests are validated against OpenAPI spec

| Step | Action | Expected |
|------|--------|----------|
| 1 | POST with unknown fields | Handled per spec (rejected or ignored) |
| 2 | POST with wrong content type | HTTP 400/415 |

### E2E via DCM Gateway

#### TC-18: Create container through full DCM flow

**Preconditions:** Full DCM stack + Container SP deployed and registered.

| Step | Action | Expected |
|------|--------|----------|
| 1 | Create a CatalogItem for container type | — |
| 2 | Create a CatalogItemInstance | Placement Manager routes to Container SP |
| 3 | Verify container creation | Deployment created on K8s |
| 4 | Check status propagation via NATS | CloudEvent published with container status |

## Upstream Test Coverage (in repo)

**Run:** `make test` in the repo (32+ tests as of 2026-03-27).

See `.ai/test-plans/k8s-container-sp-integration.test-plan.md` for the full TC-I001–TC-I114 mapping. Key files:

- `internal/handlers/container/handler_unit_test.go` — CRUD handler tests with mocked store
- `internal/kubernetes/store_unit_test.go` — K8s store operations
- `internal/kubernetes/store_create_test.go`, `store_delete.go`, `store_get.go` — per-operation tests
- `internal/kubernetes/store_service_test.go` — Service creation/deletion
- `internal/apiserver/server_integration_test.go` — HTTP integration tests
- `internal/apiserver/server_validation_test.go` — OpenAPI validation tests

### E2E vs Unit/Integration Boundary

| Concern | Tested by repo (fake client) | E2E adds value |
|---------|------------------------------|----------------|
| Input validation / 400 errors | Yes (TC-I008, TC-U057) | Minimal — confirms middleware wiring |
| K8s Deployment creation | Yes (fake client) | **Yes** — real scheduling, image pull, Pod lifecycle |
| Service creation with ports | Yes (fake client) | **Yes** — real ClusterIP assignment |
| DCM label verification | Yes (fake client) | **Yes** — labels survive real K8s admission |
| Pagination | Yes (TC-U057, TC-I008) | Marginal — confirms real list ordering |
| Registration with live SPRM | No (mock httptest) | **Yes** — full compose network flow |

## Key Configuration

| Variable | Default | Purpose |
|----------|---------|---------|
| `DCM_REGISTRATION_URL` | *(required)* | URL of DCM service-provider-manager |
| `SP_ENDPOINT` | *(required)* | This SP's externally-reachable URL |
| `SP_NAME` | *(required)* | Provider name for registration |
| `SP_K8S_NAMESPACE` | *(required)* | Namespace for container Deployments |
| `SP_K8S_KUBECONFIG` | *(optional)* | Path to kubeconfig (uses in-cluster if unset) |
| `SP_NATS_URL` | `nats://localhost:4222` | NATS server for status events |
