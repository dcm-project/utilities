# Test Plan: FLPATH-3280 — Implement ACM Cluster SP API

**Ticket:** [FLPATH-3280](https://redhat.atlassian.net/browse/FLPATH-3280)
**Status:** ON_QA
**Assignee:** Gabriel Farache
**Parent:** DCM: Create OCP service provider
**Repo:** [dcm-project/acm-cluster-service-provider](https://github.com/dcm-project/acm-cluster-service-provider)

## Summary

The ACM Cluster SP implements a CRUD API for OpenShift cluster lifecycle management via ACM HyperShift. It supports creating, listing, getting, and deleting clusters on KubeVirt and Baremetal platforms. The API registers with DCM as `serviceType: cluster` with `operations: CREATE, DELETE, READ`.

## Ticket Scope

These tickets verify the **API layer** — that endpoints exist, respond correctly, validate input, and register with DCM. Full cluster provisioning (requiring ACM Hub + HyperShift) is tracked separately in [FLPATH-3378](https://redhat.atlassian.net/browse/FLPATH-3378).

### Deployment Status

As of 2026-03-31: The ACM Cluster SP has no container image or compose profile yet. Once containerized and added to the compose stack, the test cases below become executable.

### Test Tiers

Verified against actual implementation in PRs [#5](https://github.com/dcm-project/acm-cluster-service-provider/pull/5), [#6](https://github.com/dcm-project/acm-cluster-service-provider/pull/6), [#8](https://github.com/dcm-project/acm-cluster-service-provider/pull/8):

| Tier | Requires | Test Cases | Notes |
|------|----------|------------|-------|
| **Handler validation only** | SP binary + DCM stack, any cluster | TC-04, TC-05, TC-12 | Fail at handler validation before reaching ClusterService |
| **Registration** | SP binary + DCM stack, any cluster | TC-01 (partial) | Registers even when unhealthy (see below), but `kubernetesSupportedVersions` metadata requires ClusterImageSet CRDs |
| **CRUD with K8s access** | SP binary + DCM stack + **HyperShift CRDs on cluster** | TC-07, TC-09, TC-11 | Get/List/Delete call through to ClusterService which hits K8s API for HostedCluster resources — returns **500** (not 404/empty) without CRDs |
| **Full lifecycle** (FLPATH-3378) | ACM Hub + HyperShift + KubeVirt/BM infra + ClusterImageSets | TC-02, TC-03, TC-06, TC-08, TC-10, TC-13 |

#### Code-verified behavior notes

- **Registration does NOT gate on `status: "healthy"`** (PR #5): The readiness probe in `internal/apiserver/server.go` only checks `resp.StatusCode == 200`. Since the health handler *always* returns HTTP 200 (even with `status: "unhealthy"`), registration fires as soon as the process is up. TC-01 registration will succeed on any cluster, but `kubernetesSupportedVersions` metadata will be empty/fail without ClusterImageSet CRDs (version discovery in `internal/registration/registration.go` lists `ClusterImageSet` GVK).
- **CRUD routes (Get/List/Delete) depend on HostedCluster GVK** (PR #8): `ClusterService` methods use typed `HostedCluster` objects via controller-runtime. Without HyperShift CRDs registered on the API server, these calls fail with `InternalError` → handler maps to **HTTP 500** with generic `"an internal error occurred"` body. TC-07 (expect 404), TC-09 (expect empty list), TC-11 (expect 404) will instead get 500 on a cluster without HyperShift.
- **Handler validation is cluster-independent** (PR #6): `validateCreateRequest` checks field presence/format in Go code before calling ClusterService. TC-04, TC-05, TC-12 work against any cluster.

### Upstream Test Coverage (in repo)

The repo has comprehensive test plans in `.ai/test-plans/`:

- **Integration:** `.ai/test-plans/acm-cluster-sp.integration-tests.md` — 29 integration test cases
- **Unit:** `.ai/test-plans/acm-cluster-sp.unit-tests.md`
- **Spec:** `.ai/specs/acm-cluster-sp.spec.md`
- **165 requirements mapped** across unit + integration layers

Key PRs:
- [PR #2](https://github.com/dcm-project/acm-cluster-service-provider/pull/2) — spec and test plans (linked from Jira)
- [PR #6](https://github.com/dcm-project/acm-cluster-service-provider/pull/6) — API handler (Topic 4)
- [PR #8](https://github.com/dcm-project/acm-cluster-service-provider/pull/8) — KubeVirt + BareMetal cluster service

**What the repo tests already cover (no need to duplicate at E2E):**
- Handler validation and RFC 7807 errors (TC-HDL-CRT-UT-001–018, TC-HDL-GET-UT-001–005, etc.)
- Status mapping from HyperShift conditions (TC-STS-UT-001–012, shared component)
- Registration payload and retry logic (TC-REG-UT-001–006)
- KubeVirt CRD construction (TC-KV-UT-001–020)
- Baremetal CRD construction (TC-BM-UT-001–012)
- Conflict detection (duplicate ID by label, duplicate metadata.name)

### References

- [acm-cluster-sp enhancement](https://github.com/dcm-project/enhancements/blob/main/enhancements/acm-cluster-sp/acm-cluster-sp.md)
- OpenAPI spec: `api/v1alpha1/openapi.yaml` in repo

## Prerequisites (handler validation tier)

- DCM stack deployed (`./scripts/deploy-dcm.sh`)
- ACM Cluster SP running (binary or container)
- Any Kubernetes/OCP cluster for connectivity

## Prerequisites (CRUD with K8s access tier)

- All of the above, plus:
- HyperShift CRDs installed on the cluster (CRDs only — full HyperShift operator not required)

## Prerequisites (full lifecycle tier — FLPATH-3378)

- ACM Hub cluster with HyperShift enabled
- KubeVirt or Baremetal infrastructure available
- `ClusterImageSet` resources present on the Hub

## API Endpoints Under Test

| Method | Path | Operation |
|--------|------|-----------|
| POST | `/api/v1alpha1/clusters` | Create a cluster |
| GET | `/api/v1alpha1/clusters` | List clusters |
| GET | `/api/v1alpha1/clusters/{clusterId}` | Get a cluster |
| DELETE | `/api/v1alpha1/clusters/{clusterId}` | Delete a cluster |
| GET | `/api/v1alpha1/clusters/health` | Health (covered by FLPATH-3281) |

## Test Cases

### Registration

#### TC-01: SP registers with DCM on startup

**Note:** Registration fires as soon as the health endpoint returns HTTP 200, which happens immediately regardless of `status` value. On a cluster without HyperShift CRDs, health returns `"unhealthy"` but registration still proceeds. Version discovery (`kubernetesSupportedVersions`) will fail/be empty without `ClusterImageSet` CRDs.

| Step | Action | Expected |
|------|--------|----------|
| 1 | Start the SP with `DCM_REGISTRATION_URL` pointing at running SPRM | SP starts, health returns HTTP 200 (possibly `status: "unhealthy"`) |
| 2 | Query DCM provider list | Provider with `name: acm-cluster-sp`, `serviceType: cluster` exists |
| 3 | Check registered operations | `CREATE, DELETE, READ` |
| 4 | Check registered endpoint | Matches `SP_ENDPOINT` config |
| 5 | Check metadata | `supportedPlatforms`, `supportedProvisioningTypes` present; `kubernetesSupportedVersions` may be empty without ClusterImageSet CRDs |

#### TC-02: SP re-registers when Kubernetes versions change

| Step | Action | Expected |
|------|--------|----------|
| 1 | SP is running and registered | Initial version list recorded |
| 2 | Add a new `ClusterImageSet` to the Hub | — |
| 3 | Wait for `SP_VERSION_CHECK_INTERVAL` (default 5m) | SP re-registers |
| 4 | Query DCM provider metadata | Updated `kubernetesSupportedVersions` includes new version |

### Create Cluster

#### TC-03: Create a KubeVirt cluster with valid spec

**Preconditions:** `SP_ENABLED_PLATFORMS` includes `kubevirt`.

| Step | Action | Expected |
|------|--------|----------|
| 1 | POST `/api/v1alpha1/clusters` with valid KubeVirt cluster spec | HTTP 200/201 |
| 2 | Check response body | Contains `id`, `spec`, initial status fields |
| 3 | Verify on Hub cluster | `HostedCluster` and `NodePool` CR created in correct namespace |

#### TC-04: Create returns error for invalid spec

| Step | Action | Expected |
|------|--------|----------|
| 1 | POST `/api/v1alpha1/clusters` with empty body | HTTP 400 with RFC 7807 error |
| 2 | POST with missing required fields | HTTP 400 with descriptive error |

#### TC-05: Create returns error for unsupported platform

| Step | Action | Expected |
|------|--------|----------|
| 1 | POST with platform type not in `SP_ENABLED_PLATFORMS` | HTTP 400 or 422 |

### Get Cluster

#### TC-06: Get an existing cluster by ID

| Step | Action | Expected |
|------|--------|----------|
| 1 | Create a cluster, note the returned `id` | — |
| 2 | GET `/api/v1alpha1/clusters/{id}` | HTTP 200 |
| 3 | Validate response matches create response | Same `id`, `spec` |

#### TC-07: Get a non-existent cluster

| Step | Action | Expected |
|------|--------|----------|
| 1 | GET `/api/v1alpha1/clusters/nonexistent-id` | HTTP 404 |

### List Clusters

#### TC-08: List returns all managed clusters

| Step | Action | Expected |
|------|--------|----------|
| 1 | Create 2 clusters | — |
| 2 | GET `/api/v1alpha1/clusters` | HTTP 200 |
| 3 | Check response | Both clusters present in results |

#### TC-09: List returns empty when no clusters exist

| Step | Action | Expected |
|------|--------|----------|
| 1 | Ensure no managed clusters | — |
| 2 | GET `/api/v1alpha1/clusters` | HTTP 200 with empty results array |

### Delete Cluster

#### TC-10: Delete an existing cluster

| Step | Action | Expected |
|------|--------|----------|
| 1 | Create a cluster, note `id` | — |
| 2 | DELETE `/api/v1alpha1/clusters/{id}` | HTTP 200 or 204 |
| 3 | GET the same `id` | HTTP 404 (eventually, after cleanup) |
| 4 | Verify on Hub cluster | `HostedCluster` and `NodePool` removed |

#### TC-11: Delete a non-existent cluster

| Step | Action | Expected |
|------|--------|----------|
| 1 | DELETE `/api/v1alpha1/clusters/nonexistent-id` | HTTP 404 |

### OpenAPI Validation

#### TC-12: Request validation against OpenAPI spec

| Step | Action | Expected |
|------|--------|----------|
| 1 | POST with extra unknown fields | Rejected or ignored per spec |
| 2 | POST with wrong field types | HTTP 400 |
| 3 | GET with invalid path parameter | HTTP 400 or 404 |

### E2E via DCM Gateway

#### TC-13: Create cluster through full DCM flow

**Preconditions:** Full DCM stack + ACM SP deployed.

| Step | Action | Expected |
|------|--------|----------|
| 1 | Create a CatalogItem for cluster type | — |
| 2 | Create a CatalogItemInstance | Placement Manager selects ACM SP |
| 3 | Verify cluster creation via SP | HostedCluster created on Hub |
| 4 | Check status propagation | DCM reflects cluster status |

## Upstream Test Coverage (in repo)

**Run:** `make test` in the repo.

See `.ai/test-plans/acm-cluster-sp.integration-tests.md` for the full mapping. Key files:

- `internal/handler/handler_test.go` — CRUD handler tests (TC-HDL-xxx-UT)
- `internal/cluster/kubevirt/kubevirt_test.go` — KubeVirt CRD construction (TC-KV-UT)
- `internal/cluster/baremetal/baremetal_test.go` — Baremetal CRD (TC-BM-UT)
- `internal/service/status/mapper_test.go` — Shared status mapping (TC-STS-UT)
- `internal/health/health_test.go` — Health checks (TC-HLT-UT)
- `test/integration/` — envtest-based full round-trips (TC-INT)

### E2E vs Unit/Integration Boundary

| Concern | Tested by repo (fake client) | E2E adds value |
|---------|------------------------------|----------------|
| Input validation / 400 errors | Yes (TC-HDL-CRT-UT) | Minimal |
| HostedCluster CR creation | Yes (fake K8s client) | **Yes** — real ACM admission, webhooks |
| NodePool provisioning | Yes (fake client) | **Yes** — real HyperShift reconciliation |
| ClusterImageSet discovery | Yes (TC-KV-UT-004) | **Yes** — real CIS resources on Hub |
| Status from HyperShift conditions | Yes (TC-STS-UT, shared) | **Yes** — real condition progression |
| Registration with live SPRM | No (mock httptest) | **Yes** — full compose network flow |

## Key Configuration

| Variable | Default | Purpose |
|----------|---------|---------|
| `DCM_REGISTRATION_URL` | *(required)* | URL of the DCM service-provider-manager |
| `SP_ENDPOINT` | *(required)* | This SP's externally-reachable URL |
| `SP_NAME` | `acm-cluster-sp` | Provider name for registration |
| `SP_ENABLED_PLATFORMS` | `kubevirt,baremetal` | Supported provisioning platforms |
| `SP_VERSION_CHECK_INTERVAL` | `5m` | How often to re-check K8s versions |
