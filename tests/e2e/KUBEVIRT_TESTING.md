# KubeVirt Service Provider E2E Testing

This document describes the E2E test suite for the KubeVirt Service Provider (FLPATH-2897).

## Branches

| Branch | Contents |
|--------|----------|
| `kubevirt-provider-tests-titan90` | **This branch** — default green suite (ready for CI / day-to-day runs) |
| `kubevirt-provider-tests-deferred` | Disruptive + intentionally skipped cases (`DCM_DISRUPTIVE=1`, SP/fixture gaps) |

Deferred coverage (TC-02/03/05/08/17/20/28a/b/d/e, TC-30, bad storage-class error path) lives only on `kubevirt-provider-tests-deferred`. See that branch’s `KUBEVIRT_TESTING.md` for the full deferred table.

## Overview

The KubeVirt SP test suite validates VM lifecycle management through the DCM platform, including:
- Direct KubeVirt SP API tests (health, CRUD, validation, labels, concurrency)
- Full platform integration tests (catalog → policy → placement → VM provisioning → delete)
- VM status monitoring and NATS event verification

## Test Files

| File | Purpose | Labels |
|------|---------|--------|
| `sp_kubevirt_helpers_test.go` | Helpers, VM builders, cluster/label utilities | — |
| `sp_kubevirt_api_test.go` | Direct SP API (health, CRUD, validation, concurrency, storage) | `sp`, `kubevirt` |
| `sp_kubevirt_status_test.go` | NATS status events + GET status | `sp`, `kubevirt`, `nats` |
| `core_platform_kubevirt_test.go` | Catalog → Running → delete lifecycle | `core`, `platform`, `kubevirt` |

## Prerequisites

- OpenShift/K8s with KubeVirt/CNV and at least one StorageClass
- DCM stack + KubeVirt SP (`./scripts/deploy-dcm.sh --kubevirt-service-provider ...`)
- NATS for status tests
- `oc`/`kubectl` access for cluster assertions

## Running Tests

```bash
export KUBECONFIG=/path/to/kubeconfig
export DCM_GATEWAY_URL=http://localhost:8080/api/v1alpha1
export DCM_KUBEVIRT_SP_URL=http://localhost:8081/api/v1alpha1
export DCM_NATS_URL=nats://localhost:4222
export KUBERNETES_NAMESPACE=vms

# Clean leftovers first
./scripts/cleanup-kubevirt-e2e.sh

make test-kubevirt-sp
```

## Coverage (FLPATH-2897) — green suite

| TC | Status | Notes |
|----|--------|-------|
| TC-01 | Implemented | Provider `service_type`, `schema_version`, `endpoint` |
| TC-04 | Implemented | Health healthy |
| TC-06 | Implemented | Create + cluster labels |
| TC-07 | Implemented | Custom `?id=` |
| TC-09 | Implemented | Empty/missing/invalid memory |
| TC-10 | Implemented | Duplicate id → 409 |
| TC-11 | Implemented | Domain resources present on cluster VM |
| TC-12 | Implemented | GET existing |
| TC-13 | Implemented | GET missing (404 preferred; 500 tolerated) |
| TC-14 | Implemented | GET status phase |
| TC-15 | Implemented | List contains 3 created VMs |
| TC-16 | Implemented | Unlabeled cluster VM excluded |
| TC-18 | Implemented | DELETE + cluster gone |
| TC-19 | Implemented | DELETE missing |
| TC-21 | Implemented | Transitions + GET status |
| TC-22 | Implemented | CloudEvent schema; filter by VM id |
| TC-23 | Implemented | Catalog → Running + cluster labels |
| TC-24 | Implemented | Delete instance → STI gone + VM gone |
| TC-25 | Implemented | Malformed JSON / content-type |
| TC-26 | Implemented | DCM labels on VM + template |
| TC-27 | Implemented | Instance id label round-trip |
| TC-28c | Implemented | Tiny memory / admission path |
| TC-29 | Implemented | Invalid memory unit |
| TC-31 | Implemented | 5 parallel creates |
| TC-32 | Implemented | External halt/start via runStrategy |
| TC-33 | Implemented | Provisions against available StorageClass |

## Notes

- Cluster VMs use `GenerateName: dcm-`; look up by label `dcm.project/dcm-instance-id=<id>`.
- Catalog fields must use `storage.disks` as an array (control-plane nested_map bug with `disks[0]`).
- STI status casing is `Running` (not `RUNNING`).
- Cleanup: `./scripts/cleanup-kubevirt-e2e.sh`
- Disruptive / Skip-stub cases: checkout `kubevirt-provider-tests-deferred`
