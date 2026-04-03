# Test Plan: FLPATH-3017 — Implement Container SP Status Update with Informer

**Ticket:** [FLPATH-3017](https://redhat.atlassian.net/browse/FLPATH-3017)
**Status:** ON_QA
**Assignee:** Gabriel Farache
**Parent:** DCM: container provider
**Repo:** [dcm-project/k8s-container-service-provider](https://github.com/dcm-project/k8s-container-service-provider)

## Summary

The K8s Container SP uses Kubernetes informers (SharedInformerFactory) to watch Deployments and Pods labeled with DCM labels. When status changes, it reconciles the state into a `ContainerStatus` (PENDING, RUNNING, FAILED, UNKNOWN, DELETED), debounces rapid updates, and publishes CloudEvents over NATS to the DCM control plane.

## Scope

This plan covers **E2E tests against a real cluster + live NATS** — verifying status propagation that unit/integration tests with fake informers cannot fully replicate.

**E2E focus areas:** Real Pod lifecycle transitions on a cluster, actual NATS message delivery, CloudEvent format over the wire, and cross-container correlation with live workloads.

### Upstream Test Coverage (in repo)

The repo has comprehensive monitoring tests in `.ai/test-plans/k8s-container-sp-integration.test-plan.md`:

- **Informer integration:** TC-I027–TC-I042 (fake K8s client with real informers)
- **Debounce integration:** TC-I043–TC-I046 (timing tests with fake clock)
- **NATS publish integration:** TC-I047–TC-I050 (mock NATS server)
- **Reconciliation unit:** TC-U040–TC-U055 (deterministic status mapping from Pod/Deployment state)
- **Resilience integration:** TC-I051–TC-I056 (NATS reconnection, informer re-list)

Key PRs:
- [PR #15](https://github.com/dcm-project/k8s-container-service-provider/pull/15) — status monitoring subsystem (merged)
- [PR #17](https://github.com/dcm-project/k8s-container-service-provider/pull/17) — CloudEvents `subject` attribute (open)

**Status mapping design:** The reconciliation logic maps k8s Pod phase to DCM status. `ImagePullBackOff` is a waiting reason within the "Pending" phase and maps to `PENDING`, not `FAILED`. This is by design — see TC-U044 in the unit test plan. `FAILED` requires the Pod to reach a terminal failure phase (e.g., `CrashLoopBackOff` after successful pull, or OOMKilled).

### References

- [service-provider-status-report-implementation enhancement](https://github.com/dcm-project/enhancements/blob/main/enhancements/service-provider-status-report-implementation/service-provider-status-report-implementation.md) (Pattern A: Event-Driven Streaming)

## Prerequisites

- Kubernetes cluster accessible
- NATS server running (part of DCM compose stack)
- K8s Container SP deployed and registered with DCM
- DCM stack deployed (`./scripts/deploy-dcm.sh` — includes NATS on port 4222)

## Architecture

```
K8s API Server
  ├── Deployment events ─┐
  └── Pod events ────────┤
                         ▼
              StatusMonitor (informers)
                  │
                  ▼ reconcileAndSubmit
              ReconcileStatus
                  │
                  ▼ debounce (500ms default)
              NATSPublisher.Publish
                  │
                  ▼ CloudEvent on subject "dcm.container"
              NATS → DCM Control Plane
```

## Test Cases

### Informer Setup

#### TC-01: Monitor watches only DCM-labeled resources

**Preconditions:** SP running with `SP_K8S_NAMESPACE` configured.

| Step | Action | Expected |
|------|--------|----------|
| 1 | Create a Deployment WITHOUT DCM labels in the watched namespace | No event published to NATS |
| 2 | Create a Deployment WITH labels `dcm.project/managed-by=dcm`, `dcm.project/dcm-service-type=container`, `dcm.project/dcm-instance-id=test-1` | Event published to NATS |

#### TC-02: Monitor uses correct label selector

| Step | Action | Expected |
|------|--------|----------|
| 1 | Check informer configuration | Selector = `dcm.project/managed-by=dcm,dcm.project/dcm-service-type=container` |
| 2 | Resources without `dcm-service-type=container` | Ignored by informer |

### Status Reconciliation

#### TC-03: New Deployment → PENDING status

| Step | Action | Expected |
|------|--------|----------|
| 1 | Create a DCM-labeled container via the SP API | — |
| 2 | Before any pods are scheduled | CloudEvent published with status `PENDING` |

#### TC-04: Pod running → RUNNING status

| Step | Action | Expected |
|------|--------|----------|
| 1 | Wait for the container's Pod to reach `Running` phase | — |
| 2 | Check NATS messages | CloudEvent with status `RUNNING` |

#### TC-05: Pod failure → FAILED status

**Note:** `ImagePullBackOff` maps to `PENDING` (not `FAILED`) because the k8s Pod phase is still "Pending". To trigger `FAILED`, use an image that pulls successfully but crashes (e.g., `docker.io/library/busybox:latest` with `command: ["false"]`).

| Step | Action | Expected |
|------|--------|----------|
| 1 | Create a container with an image that crashes (e.g., busybox with exit 1) | — |
| 2 | Wait for Pod to enter `CrashLoopBackOff` | CloudEvent with status `FAILED` |

#### TC-06: Deployment scaled to zero → status reflects no pods

| Step | Action | Expected |
|------|--------|----------|
| 1 | Scale DCM-managed Deployment to 0 replicas | — |
| 2 | Check NATS messages | Status reflects scaled-down state |

#### TC-07: Delete container → DELETED status

| Step | Action | Expected |
|------|--------|----------|
| 1 | Delete a container via the SP API | — |
| 2 | Check NATS messages | CloudEvent with status `DELETED` |

### CloudEvent Format

#### TC-08: Published event conforms to CloudEvents 1.0

| Step | Action | Expected |
|------|--------|----------|
| 1 | Subscribe to NATS subject `dcm.container` | — |
| 2 | Trigger a status change | Receive a message |
| 3 | Validate CloudEvent structure | `specversion: 1.0`, `type: dcm.status.container`, `source: dcm/providers/<providerName>` |
| 4 | Validate data payload | Contains `id` (instance ID), `status`, `message` |

#### TC-09: Event source matches provider name

| Step | Action | Expected |
|------|--------|----------|
| 1 | SP configured with `SP_NAME=my-container-sp` | — |
| 2 | Trigger status change | CloudEvent `source` = `dcm/providers/my-container-sp` |

### Debouncing

#### TC-10: Rapid status changes are debounced

| Step | Action | Expected |
|------|--------|----------|
| 1 | Trigger multiple rapid status changes for same instance (e.g., create + Pod scheduling + Pod running within 500ms) | — |
| 2 | Count NATS messages for that instance | Fewer messages than raw events (debounced) |
| 3 | Final message reflects latest state | Status is `RUNNING` (not intermediate states) |

### Resilience

#### TC-11: Informer re-lists after connection drop

| Step | Action | Expected |
|------|--------|----------|
| 1 | SP is monitoring resources | — |
| 2 | Temporarily disrupt K8s API connectivity | Informer detects disconnect |
| 3 | Restore connectivity | Informer re-lists, reconciles any missed changes |

#### TC-12: NATS publish retries on failure

| Step | Action | Expected |
|------|--------|----------|
| 1 | SP is monitoring a resource | — |
| 2 | Temporarily stop NATS | Publish fails |
| 3 | Restart NATS | SP retries and successfully publishes pending events |

### Instance ID Indexing

#### TC-13: Status updates correlate to correct instance ID

| Step | Action | Expected |
|------|--------|----------|
| 1 | Create two containers with different instance IDs | — |
| 2 | Trigger status change on container A only | — |
| 3 | Check NATS messages | Only container A's ID appears in the event |

## Upstream Test Coverage (in repo)

**Run:** `make test` in the repo.

See `.ai/test-plans/k8s-container-sp-integration.test-plan.md` for full TC mapping. Key files:

- `internal/monitoring/monitor.go` + `monitoring_test.go` — informer setup
- `internal/monitoring/reconcile.go` + `reconcile_unit_test.go` + `reconcile_integration_test.go` — status mapping
- `internal/monitoring/publisher.go` + `publish_integration_test.go` — NATS CloudEvent publishing
- `internal/monitoring/debouncer.go` + `debounce_integration_test.go` — debounce timing
- `internal/monitoring/resilience_integration_test.go` — NATS reconnection, informer re-list

### E2E vs Unit/Integration Boundary

| Concern | Tested by repo (fake client) | E2E adds value |
|---------|------------------------------|----------------|
| Status reconciliation logic | Yes (TC-U040–TC-U055, deterministic) | Minimal — mapping is pure function |
| Debounce timing | Yes (TC-I043–TC-I046, fake clock) | **Yes** — real timing with k8s events |
| CloudEvent format | Yes (TC-I047, mock NATS) | **Yes** — actual NATS delivery, wire format |
| Label filtering | Yes (TC-I027–TC-I030, fake client) | **Yes** — real k8s admission + API server |
| NATS reconnection | Yes (TC-I051–TC-I056) | Marginal — real NATS adds confidence |
| Real Pod lifecycle (PENDING→RUNNING) | No (fake client can't schedule) | **Yes** — primary E2E value |

## Key Configuration

| Variable | Default | Purpose |
|----------|---------|---------|
| `SP_K8S_NAMESPACE` | *(required)* | Namespace to watch for DCM-managed resources |
| `SP_NATS_URL` | `nats://localhost:4222` | NATS server URL |
| `SP_MONITOR_DEBOUNCE_MS` | `500` | Debounce interval in milliseconds |
| `SP_NAME` | *(required)* | Provider name (used in CloudEvent source) |
