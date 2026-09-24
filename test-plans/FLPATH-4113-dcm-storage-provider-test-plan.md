# FLPATH-4113 — Test Plan: DCM k8s Storage Service Provider

**Epic:** [FLPATH-4113](https://redhat.atlassian.net/browse/FLPATH-4113) — DCM storage provider  
**QA story:** [FLPATH-4434](https://redhat.atlassian.net/browse/FLPATH-4434)  
**Repo:** `github.com/dcm-project/k8s-storage-service-provider`

> Unit and integration tests (`make test`, `make test-catalog`) are handled in-repo.
> This plan covers manual end-to-end scenarios only.
>
> The current deployment uses the DCM Environment Agent. The Kubernetes storage
> provider is embedded in the agent and is registered as the `storage` service
> type. The agent appends `/api/v1alpha1/agents` to `DCM_REGISTRATION_URL`.

---

## Environment

```bash
AGENT_NAME=dcm-test-agent
AGENT_ENVIRONMENT=dcm-test
AGENT_COST=medium
AGENT_EMBEDDED_SPS=storage
AGENT_SERVER_ADDRESS=:8080
AGENT_KUBECONFIG=~/.kube/config
AGENT_MESSAGING_URL=nats://localhost:4222
DCM_REGISTRATION_URL=http://localhost:8081
SP_K8S_NAMESPACE=default
SP_K8S_EXTERNAL_SVC_TYPE=NodePort
```

Start control-plane: `BIND_ADDRESS=127.0.0.1:8081 make run` (dcm-project/control-plane)  
Start environment-agent with the variables above and `make run` (dcm-project/environment-agent).

---

## ✅ TC-01 — Embedded storage health endpoint returns healthy

**Purpose:** Verify the health endpoint reports healthy when k8s API is reachable.

**Steps:**
1. Start the Environment Agent with `AGENT_EMBEDDED_SPS=storage` and a valid
   `AGENT_KUBECONFIG` pointing to a reachable cluster
2. `curl http://localhost:8080/api/v1alpha1/health`
3. `curl http://localhost:8080/api/v1alpha1/providers` and inspect the
   embedded `storage` provider status

**Expected:** HTTP 200, body contains `"status":"healthy"`

**Result:** PASS. On `ocp-edge123`, `GET /api/v1alpha1/health` returned HTTP
200 with `{"path":"health","status":"healthy"}`. The embedded `storage`
provider was also reported with status `Ready`, confirming that the agent can
reach the Kubernetes API. The former `/api/v1alpha1/volumes/health` path is not
implemented by the current Environment Agent and returns HTTP 400.

---

## ❌ TC-02 — Embedded storage health endpoint returns unhealthy when k8s unreachable

**Purpose:** Verify the health endpoint detects k8s API unreachability.

**Steps:**
1. Start the Environment Agent with `AGENT_EMBEDDED_SPS=storage` and
   `AGENT_KUBECONFIG` pointing to an unreachable cluster (e.g. wrong server URL)
2. `curl http://localhost:8080/api/v1alpha1/health`
3. `curl http://localhost:8080/api/v1alpha1/providers` and inspect the
   embedded `storage` provider status

**Expected:** The storage provider status indicates `Unhealthy` when the
Kubernetes API is unreachable. If this test is intended to validate the
agent liveness endpoint itself, a non-200 response or unhealthy body is
expected instead.

**Result:** FAIL as originally specified. With a valid kubeconfig pointing to
an unreachable API (`127.0.0.1:9`), `/api/v1alpha1/health` returned HTTP 200
with `{"status":"healthy"}`, because it is the agent liveness endpoint. The
embedded `storage` provider correctly transitioned to `Unhealthy`, and the
agent log recorded the `Ready` → `Unhealthy` transition. The isolated test
agent was removed afterward; the deployed agent remained healthy. Filed bug:
[FLPATH-4920](https://redhat.atlassian.net/browse/FLPATH-4920).

---

## ✅ TC-03 — Environment Agent registers with DCM control-plane on startup

**Purpose:** Verify the Environment Agent registers itself and advertises the
embedded storage provider after startup.

**Steps:**
1. Start the control plane and Environment Agent with the registration settings
   above.
2. Query the deployed control plane:
   `curl http://127.0.0.1:9080/api/v1alpha1/agents`

**Expected:** The response lists the agent with:
- `name` matching `AGENT_NAME`
- `environment` matching `AGENT_ENVIRONMENT`
- `health_status` equal to `ready`
- `service_types` containing `storage`

The agent’s storage API is available at its configured HTTP address under
`/api/v1alpha1/volumes`.

**Result:** PASS on `ocp-edge123`. The control plane returned agent
`dcm-test-agent` with environment `dcm-test`, `health_status: ready`, and
`service_types: ["container", "storage"]`. The agent health endpoint returned
HTTP 200 and both embedded providers, including `storage`, reported `Ready`.

---

## ✅ TC-04 — Storage service type visible in DCM catalog

**Purpose:** Verify the `storage` service type is seeded in the control-plane catalog.

**Steps:**
1. Start the control-plane
2. `curl http://127.0.0.1:8081/api/v1alpha1/service-types`

**Expected:** Response includes an entry with `"serviceType":"storage"` and `"capacity":""` in spec

**Result:** PASS on `ocp-edge123`. The control plane returned the `storage`
service type with `spec: {"capacity":"","volume_name":""}`. The control
plane container was running during the check.

---

## ✅ TC-05 — Create PVC through a catalog item instance

**Purpose:** Verify that creating a storage catalog item instance invokes
placement, routes the request to the embedded storage provider, and creates a
Kubernetes PVC.

**Steps:**
1. Ensure the Environment Agent is registered and ready with
   `AGENT_EMBEDDED_SPS=storage` and `SP_K8S_NAMESPACE=default`.
2. Ensure the global routing policy selects `dcm-test-agent`.
3. Create a `Basic PVC` catalog item with storage defaults:
   `storage_class: nfs-dynamic`, `access_mode: ReadWriteOnce`, and capacity
   `1Gi`.
4. Create a catalog item instance through the control-plane API:
   ```bash
   curl -X POST 'http://localhost:9080/api/v1alpha1/catalog-item-instances' \
     -H 'Content-Type: application/json' \
     -d \
     '{
       "api_version":"v1alpha1",
       "display_name":"test-pvc-3",
       "spec":{
         "catalog_item_id":"e860d45e-8656-4444-842d-d110334f8228",
         "user_values":[
           {"path":"metadata.name","value":"test-pvc-3","resource":"volume"},
           {"path":"capacity","value":"1Gi","resource":"volume"}
         ]
       }
     }'
   ```
5. Expect HTTP 201 and record the returned catalog item instance UID and run ID.
6. List catalog item instances:
   `curl http://localhost:9080/api/v1alpha1/catalog-item-instances`.
7. List storage instances through SPRM:
   `curl 'http://localhost:9080/api/v1alpha1/service-type-instances?service_type=storage'`.
8. `kubectl get pvc test-pvc-3 -n default` — check the PVC was created.
9. `kubectl describe pvc test-pvc-3 -n default`.

**Expected:** The catalog item instance POST returns HTTP 201. The returned
run creates a storage service instance assigned to `dcm-test-agent`, and
`test-pvc-3` exists in namespace `default` with `storageClassName:
nfs-dynamic`, `ReadWriteOnce`, and `Bound` or `Pending` status.

**Result:** PASS on `ocp-edge123` (2026-09-08). The catalog item `Basic PVC`
was created with UID
`e860d45e-8656-4444-842d-d110334f8228`. The catalog item instance POST for
`test-pvc-3` returned HTTP 201 with UID
`40d45eff-33f8-480f-af85-fae32c91eb2f` and run ID
`684cbae9-e926-430f-9e0c-1ff0125bec4c`. The resulting storage instance was
created with ID `74f9ecb2-684f-4b7a-8cc0-eb4802cb2a70`, status `running`, and
`agent_name: dcm-test-agent`. The embedded storage provider created PVC
`74f9ecb2-684f-4b7a-8cc0-eb4802cb2a70`, which is `Bound` with capacity `1Gi`,
access mode `RWO`, and StorageClass `nfs-dynamic`.

The provider names the Kubernetes PVC with the service-instance UUID rather
than the catalog display name `test-pvc-3`; the PVC is labeled with the DCM
instance ID and storage service type.

The earlier direct SPRM POST for `test-vol` remains a separate negative result:
the deployed handler returned HTTP 400 because it passed an empty
`agent_name`. The catalog-item flow is the supported placement-based path.

---

## ✅ TC-06 — List and Get storage instances via SPRM

**Purpose:** Verify storage instances created through the catalog flow can be
listed and retrieved through the DCM control-plane SPRM API.

**Prerequisites:** TC-05 completed (`test-pvc-3` instance and PVC exist).

**Steps:**
1. List storage instances:
   `curl 'http://localhost:9080/api/v1alpha1/service-type-instances?service_type=storage'`
2. Get storage instance:
   `curl http://localhost:9080/api/v1alpha1/service-type-instances/74f9ecb2-684f-4b7a-8cc0-eb4802cb2a70`

**Expected:**
- List returns an `instances` array including `test-pvc-3`
- Get returns the instance assigned to `dcm-test-agent` with status `running`
- The instance reports capacity `1Gi` and StorageClass `nfs-dynamic`

**Result:** PASS on `ocp-edge123` (2026-09-08). The filtered storage list
returned the fresh `test-pvc-3` instance. GET for storage instance
`74f9ecb2-684f-4b7a-8cc0-eb4802cb2a70` returned HTTP 200 with status
`running`, agent `dcm-test-agent`, capacity `1Gi`, and StorageClass
`nfs-dynamic`. The Environment Agent’s legacy `/api/v1alpha1/volumes` routes
remain unavailable and are not used by the supported SPRM flow.

---

## ✅ TC-07 — Delete storage instance via SPRM

**Purpose:** Verify deleting a storage instance through the DCM control plane
removes the provider-managed PVC from Kubernetes.

**Prerequisites:** TC-05 completed (storage instance
`74f9ecb2-684f-4b7a-8cc0-eb4802cb2a70` and its PVC exist).

**Steps:**
1. `curl -X DELETE http://localhost:9080/api/v1alpha1/service-type-instances/74f9ecb2-684f-4b7a-8cc0-eb4802cb2a70`
2. Verify GET for the instance returns 404.
3. `kubectl get pvc 74f9ecb2-684f-4b7a-8cc0-eb4802cb2a70 -n default`

**Expected:** HTTP 204; the instance GET returns 404; PVC is no longer
present in the namespace.

**Result:** PASS on `ocp-edge123` (2026-09-08). DELETE returned HTTP 204 for
storage instance `74f9ecb2-684f-4b7a-8cc0-eb4802cb2a70`. A follow-up GET
returned HTTP 404, and the provider-managed PVC with the same name was no
longer present. The supported deletion path is SPRM; the legacy embedded
`/api/v1alpha1/volumes` delete route remains unavailable.

---

## ✅ TC-08 — Kubernetes provider hints applied through catalog flow

**Purpose:** Verify that the catalog item’s Kubernetes provider hints are
forwarded to the embedded storage provider and reflected on the created PVC.

**Steps:**
1. Use the `Basic PVC` catalog item created in TC-05. Its storage fields
   define `provider_hints.kubernetes.storage_class: nfs-dynamic` and
   `provider_hints.kubernetes.access_mode: ReadWriteOnce`.
2. Create a new catalog item instance, for example `test-pvc-4`, supplying
   `metadata.name` and `capacity: 1Gi` as in TC-05.
3. List the resulting storage instance through SPRM and identify its ID.
4. Inspect the provider-managed PVC:
   `kubectl describe pvc <service-instance-id> -n default`.

**Expected:** The storage instance is assigned to `dcm-test-agent`, and the
PVC is created with `storageClassName: nfs-dynamic`, access mode `RWO`, and
capacity `1Gi`, matching the catalog item’s Kubernetes provider hints.

**Result:** PASS on `ocp-edge123` (2026-09-08). The catalog item instance
`test-pvc-4` returned HTTP 201 with UID
`f932f668-6074-4f23-9171-7851b242b4e9` and run ID
`089d7963-7a98-40aa-b10b-6e7f899d14b2`. The resulting storage instance was
`d8ea0d5e-763b-4ff4-854d-3ac7a25ced3a`, assigned to `dcm-test-agent`, with
status `running`. Its provider-managed PVC was `Bound` with capacity `1Gi`,
StorageClass `nfs-dynamic`, and access mode `RWO`, matching the catalog
item’s Kubernetes provider hints.

---

## ✅ TC-09 — Embedded storage publishes PVC status changes to NATS

**Purpose:** Verify the embedded storage provider detects PVC status transitions
and publishes events to `dcm.storage`.

**Prerequisites:** NATS server running and accessible; Environment Agent
configured with embedded storage and NATS access.

**Steps:**
1. Subscribe to NATS subject `dcm.storage` (or wildcard `>` for diagnostics).
2. Create a catalog item instance through the supported flow (TC-05).
3. Wait for the provider-managed PVC to bind.
4. Observe NATS subscriber output

**Expected:**
- A message is published to `dcm.storage` when the PVC becomes `Bound`.
- The message contains the service-instance ID and status details.

**Result:** PASS on `ocp-edge123` (2026-09-08). Creating catalog instance
`test-pvc-6` produced a storage instance with ID
`532e1e10-9441-4c3c-89f3-65bf77f6f996` and a Bound PVC using
`nfs-dynamic`. The NATS capture received a CloudEvent on `dcm.storage`:
`type: dcm.status.storage`, `source: dcm/providers/storage`, with
`data.status: RUNNING` and a message identifying the bound PV. The capture
also showed the preceding `dcm.request.create` and
`dcm.agent.creation-acknowledged` events.

---

## ✅ TC-10 — NATS events use CloudEvents format

**Purpose:** Verify NATS messages conform to CloudEvents spec.

**Prerequisites:** TC-09 setup (NATS subscriber active, PVC transitions happening).

**Steps:**
1. Capture a NATS message from `dcm.storage` during TC-09
2. Inspect message structure

**Expected:** Message contains required CloudEvents fields:
- `specversion` (e.g. `"1.0"`)
- `type` (e.g. `"dcm.storage.pvc.status"`)
- `source`
- `id`
- `data` with PVC status details

**Result:** PASS on `ocp-edge123` (2026-09-08). During creation of catalog
instance `test-pvc-7`, the `dcm.storage` subscriber received a valid
CloudEvent:

```json
{
  "specversion":"1.0",
  "id":"7b9f2867-1edd-444e-80da-d001f350ff8f",
  "source":"dcm/providers/storage",
  "type":"dcm.status.storage",
  "subject":"dcm.storage",
  "datacontenttype":"application/json",
  "data":{
    "id":"91a2b92c-c9fd-4e1c-b2cb-d147278013cf",
    "status":"RUNNING",
    "message":"PVC is bound to volume pvc-a48a4dd1-fe1a-4c22-ab0c-5217ce708669"
  }
}
```

## Results summary

| TC | Area | Result |
|---|---|---|
| TC-01 | Embedded storage health when Kubernetes is reachable | ✅ PASSED |
| TC-02 | Embedded storage health when Kubernetes is unreachable | ❌ FAILED — liveness endpoint stayed healthy; provider became Unhealthy; [FLPATH-4920](https://redhat.atlassian.net/browse/FLPATH-4920) |
| TC-03 | Environment Agent registration | ✅ PASSED |
| TC-04 | Storage service type in the DCM catalog | ✅ PASSED |
| TC-05 | Create PVC through catalog item instance | ✅ PASSED |
| TC-06 | List and get storage instances via SPRM | ✅ PASSED |
| TC-07 | Delete storage instance via SPRM | ✅ PASSED |
| TC-08 | Kubernetes provider hints through catalog flow | ✅ PASSED |
| TC-09 | PVC status events published to NATS | ✅ PASSED |
| TC-10 | NATS CloudEvents format | ✅ PASSED |

**Overall:** 9 of 10 test cases passed. TC-02 failed as originally specified
because `/api/v1alpha1/health` is the agent liveness endpoint and remained
healthy while the embedded storage provider correctly reported `Unhealthy`.
The issue is tracked in [FLPATH-4920](https://redhat.atlassian.net/browse/FLPATH-4920).
