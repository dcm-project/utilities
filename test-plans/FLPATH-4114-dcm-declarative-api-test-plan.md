# Test Plan: FLPATH-4114 — DCM Declarative API, orchestration and GitOps

**Epic:** [FLPATH-4114](https://redhat.atlassian.net/browse/FLPATH-4114)  
**QA story:** [FLPATH-4712](https://redhat.atlassian.net/browse/FLPATH-4712)  
**Component:** `dcm`

> This plan covers manual end-to-end validation of the declarative catalog,
> multi-resource orchestration, policy placement, runtime outputs, GitOps
> reconciliation, rehydration, secret redaction, and DAG retry behavior.
> Unit, subsystem, and repository-level E2E tests remain evidence from the
> relevant DCM repositories and are not replaced by this plan.

## Implementation references

- [control-plane#11](https://github.com/dcm-project/control-plane/pull/11) — multi-resource catalog items and instances
- [control-plane#39](https://github.com/dcm-project/control-plane/pull/39) — run-based multi-resource placement
- [control-plane#41](https://github.com/dcm-project/control-plane/pull/41) — runtime output fields
- [control-plane#53](https://github.com/dcm-project/control-plane/pull/53) — status-driven DAG progression and CEL binding
- [control-plane#36](https://github.com/dcm-project/control-plane/pull/36) — GitOps controller
- [control-plane#62](https://github.com/dcm-project/control-plane/pull/62) — DAG callback retry/CAS hardening
- [cli#8](https://github.com/dcm-project/cli/pull/8) — catalog item/instance CLI commands
- [cli#18](https://github.com/dcm-project/cli/pull/18) — catalog-instance rehydration command
- [cli#28](https://github.com/dcm-project/cli/pull/28) — multi-resource catalog fixtures and display
- [utilities#28](https://github.com/dcm-project/utilities/pull/28), [utilities#29](https://github.com/dcm-project/utilities/pull/29), and [utilities#36](https://github.com/dcm-project/utilities/pull/36) — E2E fixture and run-ID compatibility updates

## Environment

Target environment: `ocp-edge123`, DCM test stack `dcm-test`.

- DCM control plane: `http://<host>:9080`
- API prefix: `/api/v1alpha1`
- Environment Agent: `http://<host>:8082`
- NATS: host port `4222`
- PostgreSQL: host port `5432`
- Keycloak: host port `8180`
- Kubernetes kubeconfig: `/home/kni/clusterconfigs/auth/kubeconfig_dcm-test`
- Kubernetes namespace for provider resources: `default`
- DCM containers: control-plane, dcm-ui, environment-agent, NATS, PostgreSQL, Keycloak

Suggested variables:

```sh
export GW=http://<host>:9080/api/v1alpha1
export AGENT=http://<host>:8082/api/v1alpha1
export KUBECONFIG=/home/kni/clusterconfigs/auth/kubeconfig_dcm-test
```

## Fixtures

- [catalog-item.yaml](catalog-item.yaml) defines `two-tier-web`:
  - `backend`: service type `container`
  - `frontend`: service type `container`, requiring `backend`
  - editable names and image references; fixed CPU and memory limits
- [catalog-item-instance.yaml](catalog-item-instance.yaml) creates
  `two-container-test-1` and overrides both resource names and images.
- [policy.yaml](policy.yaml) is a global policy that selects `local-agent` for
  `container` resources.

The fixture now uses `container` for both resources, so the TC-02 container
placement policy matches both DAG nodes. The `frontend -> backend` dependency
still provides the ordering boundary for orchestration.

## Base preconditions

1. Confirm all DCM containers are running and PostgreSQL/Keycloak report healthy.
2. Confirm all six `dcm-test` Kubernetes nodes are `Ready`.
3. Confirm the Environment Agent is registered with `health_status: ready` and
   advertises `container` and `storage` service types.
4. Confirm the required `container` and `storage` service types are present.
5. Use unique names for each run and clean up created instances/resources after
   each test unless the result requires preserving evidence.

## Setup gates

The catalog item and placement policy are created by the test plan; they are
not assumed to exist before execution:

1. Run **TC-01** to create the FLPATH-4114 multi-resource catalog item. Record
   the returned catalog item UID as `CATALOG_ITEM_ID` and verify the stored
   schema.
2. Run **TC-02** to create the FLPATH-4114 placement policy. Record the
   returned policy ID as `POLICY_ID` and verify the persisted Rego rule.
3. Run **TC-03** to verify the catalog item remains readable after creation.
4. Do not run **TC-04–TC-14** until TC-01 and TC-02 pass and their IDs are
   recorded. Catalog item instance creation must use `CATALOG_ITEM_ID`; policy
   checks must use `POLICY_ID`.
5. TCs 15–20 are maintained in the
   [follow-up lifecycle test plan](FLPATH-4114-DCM-declarative-API-follow-up-test.md)
   and should run only after the parent-plan setup gates pass.

For every test, record catalog item IDs, policy IDs, catalog instance UIDs, run
IDs, and service-type-instance IDs. If TC-01 or TC-02 was already executed in
the environment, GET and reuse the existing object only after confirming that
its schema/policy matches the fixture; do not create an uncontrolled duplicate.

## Test cases

### ✅ TC-01 — Create the FLPATH-4114 multi-resource catalog item PASSED

**Purpose:** Create the catalog item fixture required by the remaining
multi-resource tests and verify that DCM persists its declarative schema.

**Steps:**

1. Convert [catalog-item.yaml](catalog-item.yaml) to JSON and POST it with
   `Content-Type: application/json` to `/api/v1alpha1/catalog-items`.
2. Record the returned catalog item UID as `CATALOG_ITEM_ID`.
3. GET `/api/v1alpha1/catalog-items/$CATALOG_ITEM_ID`.
4. List `/api/v1alpha1/catalog-items` and verify the item is visible.

**Expected:** HTTP 201. The response contains a stable UID. The stored item
contains both `backend` and `frontend`, their service types and fields, and
the `frontend -> backend` dependency. Use the returned UID rather than the
fixture label `two-tier-web` for subsequent instance creation.

**Result:** PASSED on `ocp-edge123` (2026-09-08; rerun after fixture update).

- The initial request correctly failed with HTTP 400 because the API rejects
  `Content-Type: application/yaml`.
- Retried with the fixture converted to JSON and received HTTP 201.
- Created catalog item UID:
  `d49112e5-9616-46e6-b0bf-c14d0435c5de`.
- GET and list verification confirmed display name
  `Two-tier web (multi-container)`, resources `backend`/`frontend`, service
  types `container`/`container`, and `frontend.requires_resources: [backend]`.
- Rerun catalog item UID:
  `0c354672-2bd7-4ef7-8662-6ae98a058754`.
- The earlier catalog item UID
  `d49112e5-9616-46e6-b0bf-c14d0435c5de` remains the previous
  `container`/`storage` version and was not modified.

### ✅ TC-02 — Create the FLPATH-4114 placement policy PASSED

**Purpose:** Create the policy fixture and verify its persisted placement rule.

**Steps:**

1. Confirm the target agent name available from `/api/v1alpha1/agents`.
2. If the test environment uses `dcm-test-agent` rather than `local-agent`,
   update the test copy of `policy.yaml` or create an equivalent policy for
   `dcm-test-agent`; do not silently claim that `local-agent` exists.
3. POST [policy.yaml](policy.yaml) to `/api/v1alpha1/policies`.
4. Record the returned policy ID as `POLICY_ID`.
5. GET `/api/v1alpha1/policies/$POLICY_ID` and list policies.

**Expected:** HTTP 201. The policy is enabled, global, has priority `99`, and
contains the expected Rego rule selecting the target agent when
`input.spec.service_type == "container"`. The policy ID is recorded for
later cleanup.

**Result:** PASSED on `ocp-edge123` (2026-09-08).

- The live Environment Agent is `dcm-test-agent`; the fixture's
  `local-agent` value was replaced with `dcm-test-agent` before submission.
- Policy ID: `13666e34-5200-4f89-936a-e68bed3c488b`.
- GET and list verification confirmed display name
  `Route container to local-agent`, enabled `GLOBAL` policy, priority `99`,
  and Rego selection of `dcm-test-agent` for `container` resources.

### ✅ TC-03 — Catalog item schema is preserved after creation PASSED

**Purpose:** Verify the catalog item created by TC-01 is persisted and can be
read back without losing its multi-resource structure.

**Steps:**

1. GET `/api/v1alpha1/catalog-items/$CATALOG_ITEM_ID`.
2. List `/api/v1alpha1/catalog-items`.
3. Compare the stored object with [catalog-item.yaml](catalog-item.yaml).

**Expected:** HTTP 200; the item contains both `backend` and `frontend`,
including their service types, fields, defaults, editability, and the
`frontend -> backend` dependency. The catalog list contains exactly the
created item identity used by the test.

**Result:** PASSED on `ocp-edge123` (2026-09-08).

- GET returned catalog item UID
  `d49112e5-9616-46e6-b0bf-c14d0435c5de` and display name
  `Two-tier web (multi-container)`.
- Validation confirmed exactly two resources: `backend`/`container` and
  `frontend`/`storage`.
- Validation confirmed `frontend.requires_resources: [backend]`.
- List verification found the same catalog item UID.

### ✅ TC-04 — Catalog item instance applies per-resource user values PASSED

**Purpose:** Verify instance overrides are associated with the correct resource.

**Steps:**

1. Create an instance from `catalog-item-instance.yaml`, using the catalog UID
   returned by TC-01.
2. Inspect the response and GET the resulting catalog item instance.

**Expected:** HTTP 201; the instance contains four user overrides, each scoped
to the intended resource. Backend and frontend names/images remain distinct.

**Result:** PASSED on `ocp-edge123` (2026-09-08).

- Catalog item instance UID:
  `afe2ddd5-e030-484c-8318-0a79becea907`.
- Run ID: `c364934f-9f94-41aa-8d16-32b3e6b26586`.
- The instance references catalog item
  `d49112e5-9616-46e6-b0bf-c14d0435c5de` and display name
  `two-container-test-1`.
- GET/list validation confirmed four user values: two scoped to `backend` and
  two scoped to `frontend`, with distinct names and image references.

### ✅ TC-05 — Invalid resource and field references are rejected PASSED

**Purpose:** Verify request validation prevents malformed composite requests.

**Steps:**

1. Submit an instance with an unknown `user_values[].resource`.
2. Submit an instance with an unknown field path.
3. Submit an instance missing `spec.catalog_item_id`. The catalog item has
   defaults for its resource fields, so a missing catalog-item ID is used as
   the unambiguous schema-negative case.

**Expected:** The API returns a documented 4xx response for each invalid
request, identifies the invalid resource/path, and creates no partial run or
service-type instances.

**Result:** PASSED on `ocp-edge123` (2026-09-08).

- Unknown resource returned HTTP 400: `user value resource not found in
  catalog item: unknown`.
- Unknown field path returned HTTP 400: `resource backend: user value path not
  found in catalog item fields: does.not.exist`.
- Missing `spec.catalog_item_id` returned HTTP 400 with the schema validation
  error identifying the missing property.
- No `tc05-*` catalog item instances were present after the requests.

### ✅ TC-06 — Dependency graph and run execution are ordered correctly PASSED (rerun)

**Purpose:** Verify the DAG provisions dependencies before dependent resources.

**Steps:**

1. Use a two-resource catalog item whose placement policy matches both service
   types.
2. Create an instance and record its `run_id`.
3. Observe catalog instance status and service-type-instance events.
4. Inspect timestamps and dependency references for both resources.

**Expected:** One run represents the composite request. `backend` reaches its
required running/output state before `frontend` is provisioned. The request
does not create duplicate resources when status events are redelivered.

**Result:** The original run failed on `ocp-edge123` (2026-09-08) because the
fixture used a storage frontend with an incompatible storage-resource shape. No
Jira bug was filed. The rerun below uses the updated `container`/`container`
fixture and passed.

- Catalog item instance UID:
  `75443e8f-c542-4103-a319-ca0b9aa81727`.
- Run ID: `9bedeac6-913c-40ab-b6e3-1daf811553db`.
- Backend service-type instance
  `e37e5b4c-6668-43be-b276-665f01cd8485` reached `running`.
- Only after the backend reached `running` did the control plane create the
  dependent frontend service-type instance
  `2f2f72f5-3dd8-4591-a145-bf31f7f78717`, demonstrating the expected DAG
  ordering.
- The frontend storage instance reached `failed`. Environment-agent logs show
  the embedded storage provider rejected the create request with HTTP 400.
  The generated storage spec contains `image.reference` and empty `capacity`
  and `volume_name`, while the storage provider fixture expects storage-specific
  fields. The composite request therefore did not complete successfully.

**Rerun result:** PASSED on `ocp-edge123` (2026-09-08) with catalog item
`0c354672-2bd7-4ef7-8662-6ae98a058754`, whose `backend` and `frontend` resources
both use service type `container`.

- Catalog item instance UID:
  `8c85dc1a-1749-4f9e-9ad6-465d02a2a5d3`.
- Run ID: `9086fb83-4ea0-4dea-b510-754009feab5f`.
- Backend service-type instance
  `8122d1bd-eb15-48d7-864e-177b813c4632` reached `running` at
  `2026-09-08T19:30:13.460Z`.
- Only after the backend reached `running` did the control plane create the
  dependent frontend service-type instance
  `80256d5a-f966-4afa-9a0c-9d781d827bc8`.
- Frontend reached `running` at `2026-09-08T19:30:20.494Z`; both resources were
  recorded as `RUNNING` for the same run.

### ✅ TC-07 — Policy placement selects the expected agent per service type PASSED

**Purpose:** Verify policy evaluation is performed for each resource in the DAG.

**Steps:**

1. Apply `policy.yaml` and verify it is enabled and global.
2. Submit the three-resource fixture without a storage policy.
3. Inspect placement decisions and service-type-instance status.
4. Add a storage policy selecting an agent that advertises `storage`.
5. Repeat the request.

**Expected:** The container backend and frontend select `local-agent`. The
storage resource does not incorrectly inherit the container-only policy. After
adding a valid storage policy, all three resources receive compatible placement
decisions and the request can progress.

**Result:** PASSED on `ocp-edge123` (2026-09-08) after disabling the broad
`route-all-to-local-agent` policy for isolation. The container-only policy
remained enabled. No Jira bug filed.

- Catalog item UID: `7e81cc3c-56a8-4cb6-984e-500e6c5ef164`.
- With only the container policy enabled, a fresh three-resource instance
  returned HTTP `424 Failed Dependency`; the storage resource was not placed.
  This confirms the container-only policy did not inherit to `storage`.
- A storage-only policy, `Route storage to local-agent`, was then created with
  ID `61710a52-2cae-4f45-823d-8b406c3d0952`, priority `98`, and selected
  `dcm-test-agent` when `input.spec.service_type == "storage"`.
- The post-policy instance UID was
  `0666906b-2e3e-43d3-91cb-b0d7f6c69744`; run ID
  `fd6ad8d8-5652-4578-be40-a8c17bd510c9`.
- Backend, frontend, and storage were each assigned to `dcm-test-agent` and
  reached `running`. The DAG progressed in order: backend (level 0), frontend
  (level 1), then storage (level 2).
- The storage instance reported `PVC is bound to volume
  pvc-3c125d79-80b0-4cc2-9dd9-93ba051c9755`.

### ❌ TC-08 — Failure prevents invalid dependent provisioning FAILED

**Purpose:** Verify a failed dependency stops or correctly marks downstream
resources.

**Steps:**

1. Configure the backend provider to fail or make it unavailable.
2. Create the composite instance.
3. Observe the run, backend, and frontend statuses.

**Expected:** The backend transitions to a documented failure state. The
frontend is not provisioned with an unresolved or stale dependency. The overall
application status reflects the failure and remains queryable.

**Result:** FAILED on `ocp-edge123` (2026-09-09). Jira bug
[FLPATH-4851](https://redhat.atlassian.net/browse/FLPATH-4851) filed. The current
embedded container provider did not expose a terminal failure for the failure
trigger.

- Catalog item instance UID:
  `19cd945d-2dd9-4de4-909d-a902fdb20110`.
- Run ID: `27a17ea1-064f-4357-be5a-a50a06b78688`.
- The backend used the intentionally invalid image
  `docker.io/library/image-does-not-exist:tc08` and created service-type
  instance `fd23a8f2-4fa8-438b-a64f-7d3e85626f8c`.
- The container provider acknowledged the create request, but the backend
  remained `pending` for more than 10 minutes. No Kubernetes workload or image
  pull event was created, and no terminal failure status was emitted.
- The dependent frontend was not provisioned, but this is insufficient to pass
  TC-08 because the backend never reached the expected failure state.

### ✅ TC-09 — GitOps repository reconciliation creates and removes instances PASSED

**Purpose:** Verify the proactive GitOps API creates and removes managed DCM
instances from repository manifests.

**Steps:**

1. Register a test Git repository through the GitOps API.
2. Commit valid catalog item instance manifests.
3. Wait for reconciliation and inspect the managed instances and runs.
4. Remove the manifests from Git and verify the managed instances and provider
   resources are deleted.

**Expected:** Valid manifests create the requested DCM objects. Removing the
manifests deletes the managed objects and their provider resources. Repeated
polling is idempotent.

**Result:** PASSED on `ocp-edge123` (2026-09-23).

- The test used the published reconciler image
  `quay.io/dcm-project/dcm-gitops@sha256:3c217ab4e805955e5ba0ff9cdb87f39eecacbb8a8979180a1a32a737e42274b8`
  (`0494fe7`).
- Commit `9d1d5cd` created `gitops-two-tier-storage` with run ID
  `46488280-3264-45a3-820f-c94708115eae`.
- Commit `1172284` created `gitops-two-tier-storage-new` with run ID
  `f99c6a37-805b-4333-b3b3-91165342baf7`.
- Removing both manifests in commit `d2e11a9` caused the controller to report
  `deleted: 2`, publish delete events for both provider resources, and leave no
  matching service-type instances or PVCs.
- The fixture was restored to valid commit `baf0848`.

### ❌ TC-09A — GitOps updates existing instances FAILED

**Purpose:** Verify that changes to supported parameters in an existing
GitOps-managed catalog item instance are reconciled.

**Steps:**

1. Reconcile a valid instance manifest.
2. Change an existing resource name and commit the manifest.
3. Change an existing storage capacity and commit the manifest.
4. Wait for reconciliation and inspect the instance spec and GitOps commit
   labels.

**Expected:** Supported changes to an existing instance are applied, or a
clear unsupported-update error is reported.

**Result:** FAILED on `ocp-edge123` (2026-09-23). Jira bug
[FLPATH-4918](https://redhat.atlassian.net/browse/FLPATH-4918) filed.

- Update commit `9f80ab7` changed the resource name, but the controller logged
  `No lifecycle changes detected` and retained the old name.
- Update commit `bd4ba62` changed capacity from `1Gi` to `2Gi`, but the
  instance remained at `1Gi` and retained the previous GitOps commit label.
- The fixture was restored to valid commit `03bf8cb`.

### ❌ TC-09B — GitOps reports invalid manifests FAILED

**Purpose:** Verify that semantically invalid manifests are reported without
corrupting the last valid managed state.

**Steps:**

1. Reconcile a valid catalog item instance manifest.
2. Commit a manifest containing an unsupported field path, such as
   `does.not.exist`.
3. Inspect repository sync state, reconciliation logs, and the last valid
   managed instance.

**Expected:** The repository remains available, the invalid manifest receives
an explicit reconciliation error or failure status, and the last valid state
is preserved.

**Result:** FAILED on `ocp-edge123` (2026-09-23).

- Invalid commit `5195619` was fetched, but the repository remained `SYNCED`.
- No reconciliation error or failure status was reported.
- The previous valid instance remained intact.
- Jira bug filed: [FLPATH-4919](https://redhat.atlassian.net/browse/FLPATH-4919).

### ❌ TC-10 — Rehydration preserves intent and changes provider resource identity FAILED

**Purpose:** Verify composite resources can be rehydrated while preserving the
catalog instance intent and user values.

**Steps:**

1. Create and run a composite catalog item instance.
2. Record catalog instance UID, user values, resource IDs, and provider.
3. Trigger rehydration while the original provider is available.
4. Repeat with the original provider unavailable and a compatible alternate
   provider available.
5. Inspect the resulting resources and status history.

**Expected:** The catalog instance UID and intent/user values are preserved.
Rehydration creates the required replacement resources, updates provider
resource IDs as applicable, and does not leave duplicate active resources.

**Result:** FAILED on `ocp-edge123` (2026-09-09).

- Jira bug filed: [FLPATH-4852](https://redhat.atlassian.net/browse/FLPATH-4852).

- Created composite catalog item instance
  `54436fc4-fd3d-4667-90f5-78832591f875` using catalog item
  `0c354672-2bd7-4ef7-8662-6ae98a058754`.
- Run ID: `bc1c5c86-c3fc-456d-a438-381513c7f777`.
- Initial provider resources were running on `dcm-test-agent`:
  `711be7f2-1474-4b04-8594-9646876bd608` (backend) and
  `7d08960d-50a0-4726-8eb0-1587c0b151d4` (frontend).
- `POST /api/v1alpha1/catalog-item-instances/54436fc4-fd3d-4667-90f5-78832591f875:rehydrate`
  returned HTTP 500: `rehydrate currently supports only single-resource runs,
  got 2 resources`.
- The instance and original resources remained unchanged; no replacement
  resource was created.

### ✅ TC-11 — Duplicate and concurrent callbacks are safe PASSED

**Purpose:** Verify retry/CAS protection prevents duplicate progression,
provisioning, or deletion.

**Steps:**

1. Create a multi-resource run.
2. Redeliver or concurrently trigger the same resource status callback.
3. Repeat around dependent-resource progression and deletion.
4. Inspect run history, service-type instances, provider calls, and final state.

**Expected:** Transient callback failures are retried. CAS conflicts are
handled safely. Each logical resource has one effective provisioning/deletion
transition, and the run eventually reaches the correct terminal state.

**Result:** PASSED on `ocp-edge123` (2026-09-09).

- Created composite catalog item instance
  `437d35d3-d3cd-47c3-85a8-2a945d1da114` with run ID
  `ba677585-a6ca-4426-91a6-97f7f421a906`.
- Initial resources were:
  `52cb044c-60d4-479a-a429-d447c3932bc9` (backend) and
  `6e8a10cc-3a08-46fa-9028-acc152a8d726` (frontend).
- Published two concurrent identical `RUNNING` CloudEvents for the backend
  on `dcm.container`; both were consumed, but no additional resource was
  created and the frontend retained its single resource ID.
- Deleted the catalog item instance and published two concurrent identical
  `DELETED` CloudEvents for the backend. Both stale callbacks were safely
  ignored after the resource had been removed.
- Final verification found no remaining provider resources for the isolated
  instance.
- Deviation: a transient provider failure and retry cycle was not injected;
  duplicate/concurrent callback handling was validated.

### ✅ TC-12 — Cleanup removes all test resources PASSED

**Purpose:** Verify deletion of a composite instance cleans up its provider
resources and orchestration state.

**Steps:**

1. Delete a completed composite catalog item instance.
2. Poll until the run and child service-type instances reach their documented
   deleted/terminal state.
3. Check provider resources, DCM records, NATS activity, and GitOps managed
   state if applicable.

**Expected:** All child resources are deleted exactly once, no orphaned
provider resource remains, and the API returns the documented post-delete
response.

**Result:** PASSED on `ocp-edge123` (2026-09-09).

- Created and completed isolated composite catalog item instance
  `7aad5a1c-c71a-44fb-8f1d-026f5e833e3b` with run ID
  `20a3c8ba-6752-4b0f-bca9-a47620eb25b5`.
- Child resources were:
  `6040f3ab-07b5-452d-be6a-1e2fb0ee5365` (backend) and
  `1d049f21-27cf-4c54-8faf-1c281d48e3fc` (frontend).
- DELETE `/api/v1alpha1/catalog-item-instances/7aad5a1c-c71a-44fb-8f1d-026f5e833e3b`
  returned HTTP 204.
- Subsequent catalog-instance GET returned HTTP 404.
- Both child service-type instances and their Kubernetes pods were removed;
  no orphaned resources remained.
- Unrelated `local-backend-1` remained present and running.

### ✅ TC-13 — DCM CLI creates, lists, gets, and deletes a catalog item PASSED

**Purpose:** Verify the declarative catalog-item lifecycle through the DCM CLI.

**Prerequisites:** The CLI is installed, configured for the DCM control plane,
authenticated if required, and supports `-o json`.

**Steps:**

1. Create a CLI-specific catalog item ID to avoid colliding with TC-01:
   `flpath-4114-two-tier-web-cli`.
2. Run:
   `dcm catalog item create --from-file catalog-item.yaml --id flpath-4114-two-tier-web-cli -o json`.
3. Record the returned UID and compare it with `CATALOG_ITEM_ID` only to
   confirm that it is a separate object.
4. Run `dcm catalog item list -o json` and verify the item is present.
5. Run `dcm catalog item get flpath-4114-two-tier-web-cli -o json` and verify
   both resources, service types, fields, and the dependency.
6. Run `dcm catalog item delete flpath-4114-two-tier-web-cli`.
7. Confirm a subsequent GET returns the documented not-found response.

**Expected:** CLI create/list/get/delete operations succeed. JSON output
preserves the multi-resource declarative schema and deletion removes only the
CLI-specific catalog item.

**Result:** PASSED on `ocp-edge123` (2026-09-24), rerun with the fix for
[FLPATH-4853](https://redhat.atlassian.net/browse/FLPATH-4853).

- Used the published CLI build from `dcm-project/cli` commit `3d1b79a`
  (`dcm version 3d1b79a`, built `2026-09-22T14:26:57Z`) with
  `--control-plane-url http://127.0.0.1:8080`.
- Create succeeded for catalog item UID
  `flpath-4114-two-tier-web-cli-20260924`.
- List and get verification preserved two resources: `backend` and `frontend`,
  both with service type `container` and six fields each.
- The `frontend -> backend` dependency was preserved.
- Delete succeeded, and the subsequent GET returned HTTP 404:
  `catalog item not found`.

### ✅ TC-14 — DCM CLI creates, lists, gets, and deletes a policy PASSED

**Purpose:** Verify policy creation and inspection through the DCM CLI.

**Prerequisites:** Use an agent name that exists in the current environment.
The supplied `policy.yaml` names `local-agent`; on `ocp-edge123`, use an
equivalent test copy targeting `dcm-test-agent` unless `local-agent` has been
registered.

**Steps:**

1. Create a CLI-specific policy ID:
   `flpath-4114-container-placement-cli`.
2. Run:
   `dcm policy create --from-file policy-cli.yaml --id flpath-4114-container-placement-cli -o json`.
3. Record the returned policy ID as `CLI_POLICY_ID`.
4. Run `dcm policy list -o json` and verify the policy is enabled and global.
5. Run `dcm policy get "$CLI_POLICY_ID" -o json` and verify priority `97` and
   the container service-type Rego condition.
6. Delete the policy using the CLI and confirm it no longer appears in list.

**Expected:** CLI policy CRUD succeeds and retains the expected policy type,
priority `97`, enabled state, and Rego code. No unrelated policy is changed.

**Result:** PASSED on `ocp-edge123` (2026-09-09), with a controlled fixture
deviation.

- The initial priority `99` create attempt returned HTTP 409 because the
  existing container-placement policy already uses priority `99` for a GLOBAL
  policy.
- The isolated fixture was changed to unused priority `97`; the existing
  policy was not modified.
- CLI create succeeded with ID
  `flpath-4114-container-placement-cli`, enabled `true`, policy type `GLOBAL`,
  priority `97`, and the expected container Rego condition selecting
  `dcm-test-agent`.
- CLI list and get returned the created policy.
- CLI delete succeeded; subsequent get returned HTTP 404.

The remaining lifecycle and cleanup cases (TC-15–TC-20) were moved to the [follow-up lifecycle test plan](FLPATH-4114-DCM-declarative-API-follow-up-test.md).

## Results summary

| TC | Area | Result |
|---|---|---|
| TC-01 | Create multi-resource catalog item | ✅ PASSED |
| TC-02 | Create placement policy | ✅ PASSED |
| TC-03 | Multi-resource catalog definition | ✅ PASSED |
| TC-04 | Per-resource instance overrides | ✅ PASSED |
| TC-05 | Validation and negative requests | ✅ PASSED |
| TC-06 | Dependency DAG and run execution | ✅ PASSED (rerun; original storage fixture failed) |
| TC-07 | Policy placement | ✅ PASSED |
| TC-08 | Dependency failure handling | ❌ FAILED — [FLPATH-4851](https://redhat.atlassian.net/browse/FLPATH-4851) |
| TC-09 | GitOps create/remove reconciliation | ✅ PASSED |
| TC-09A | GitOps updates to existing instances | ❌ FAILED — [FLPATH-4918](https://redhat.atlassian.net/browse/FLPATH-4918) |
| TC-09B | GitOps invalid-manifest reporting | ❌ FAILED — [FLPATH-4919](https://redhat.atlassian.net/browse/FLPATH-4919) |
| TC-10 | Composite rehydration | ❌ FAILED — [FLPATH-4852](https://redhat.atlassian.net/browse/FLPATH-4852) |
| TC-11 | Retry/CAS and duplicate callbacks | ✅ PASSED |
| TC-12 | Composite cleanup | ✅ PASSED |
| TC-13 | CLI catalog item lifecycle | ✅ PASSED — [FLPATH-4853](https://redhat.atlassian.net/browse/FLPATH-4853) verified |
| TC-14 | CLI policy lifecycle | ✅ PASSED |

**Overall:** Partially passed. The declarative API, orchestration, policy,
cleanup, CLI catalog-item and policy, and GitOps create/remove coverage passed.
Existing
GitOps-managed instance updates and invalid-manifest reporting failed, with
bugs [FLPATH-4918](https://redhat.atlassian.net/browse/FLPATH-4918) and
[FLPATH-4919](https://redhat.atlassian.net/browse/FLPATH-4919). The original
TC-06 storage-fixture failure is retained as historical evidence; the updated
container/container fixture passed.

