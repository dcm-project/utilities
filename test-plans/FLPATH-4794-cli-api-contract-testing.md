# Test Plan: FLPATH-4794 — CLI-to-API Contract Testing

| Field | Value |
|---|---|
| **Ticket** | [FLPATH-4794](https://redhat.atlassian.net/browse/FLPATH-4794) |
| **Author** | Thomas Stetson |
| **Version** | 1.2 |
| **Last Updated** | 2026-09-03 |
| **Status** | Draft |

## Description

This test plan defines a contract testing strategy between the DCM CLI
(`dcm-project/cli`) and the control-plane API (`dcm-project/control-plane`).
The goal is to detect serialization/schema drift automatically — preventing
the class of bug identified in
[FLPATH-4770](https://redhat.atlassian.net/browse/FLPATH-4770), where the CLI
silently dropped required API fields because its `go.mod` dependency was stale.

Contract tests validate that:
1. The CLI can parse documented YAML examples without data loss
2. The resulting JSON payload satisfies the API's required-field constraints
3. The API accepts the CLI's request and returns a success response

### References

- [FLPATH-4770](https://redhat.atlassian.net/browse/FLPATH-4770) — Getting
  Started user journey broken end-to-end (the motivating bug)
- [FLPATH-3355](https://redhat.atlassian.net/browse/FLPATH-3355) — Develop
  contract tests between api-gateway and providers (Backlog; gateway↔SP scope)
- [control-plane#40](https://github.com/dcm-project/control-plane/issues/40) —
  Org-wide: no real cross-repo contract validation exists
- [FLPATH-4759](https://redhat.atlassian.net/browse/FLPATH-4759) — kind +
  control-plane + osac-sp E2E (first real-service contract test)
- [FLPATH-4638](https://redhat.atlassian.net/browse/FLPATH-4638) — Write CLI
  catalog item and catalog instance E2E tests
- [FLPATH-4617](https://redhat.atlassian.net/browse/FLPATH-4617) — Update CLI
  test fixtures to use multi-resource catalog item schema

### Acceptance Criteria

(Numbered to match [FLPATH-4794](https://redhat.atlassian.net/browse/FLPATH-4794))

1. CLI repo CI includes a static contract test that fails if documented YAML
   examples cannot round-trip through `parseInputFileAs` without data loss
   → **TC-01, TC-02**
2. A subsystem-level test validates the CLI binary can create catalog items and
   instances against a real control-plane (reusing existing compose
   infrastructure) → **TC-03, TC-04**
3. A scheduled workflow alerts when the CLI `control-plane` dependency is more
   than 2 weeks stale → **TC-05**

---

## Environment and Global Setup

### Environment Requirements

| Component | Required for |
|-----------|-------------|
| Go (version from `go.mod`) | Building CLI from source |
| Docker or Podman | Running the subsystem compose stack |
| `curl`, `jq` | Health checks and verification |
| Network ports 28080, 5432, 28081 | Subsystem compose stack (control-plane, Postgres, WireMock) |

### Deployment Configuration

The contract tests reuse the **existing control-plane `catalog-subsystem`
compose stack** (`test/subsystem/catalog/docker-compose.yaml`), which provides:

- Postgres (port 5432) — real database
- Control-plane (port 28080) — real binary with auth disabled, NATS disabled
- WireMock (port 28081) — placement-manager mock (starts with no mappings)

**Important:** WireMock boots empty. The Ginkgo subsystem tests call
`stubPMCreateResource()` in `BeforeEach` to register mappings via its admin
API. CLI contract tests must do the same via `curl` before creating instances.

This is the lightest possible real-API configuration (~15s startup, no
external cluster needed).

### Global Setup

```bash
# Clone control-plane (hosts the compose stack)
git clone --depth=1 https://github.com/dcm-project/control-plane.git /tmp/cp
cd /tmp/cp

# Start subsystem stack
make catalog-subsystem-test-up

# Verify health
curl -sf http://localhost:28080/api/v1alpha1/health

# Stub placement-manager in WireMock (required for instance creation)
curl -s -X POST http://localhost:28081/__admin/mappings \
  -H "Content-Type: application/json" \
  -d '{
    "request": {"method": "POST", "urlPath": "/api/v1alpha1/runs"},
    "response": {
      "status": 202,
      "headers": {"Content-Type": "application/json"},
      "jsonBody": {
        "run_id": "contract-run-1",
        "catalog_item_instance_id": "ignored",
        "resources": [{"id": "r-1", "name": "main", "path": "resources/r-1", "spec": {}}]
      }
    }
  }'
```

---

## Test Tiers

| Tier | Requires | Scope | Notes |
|------|----------|-------|-------|
| Static (CLI repo) | Go toolchain only | YAML→JSON round-trip, no network | Fastest; catches serialization bugs |
| Subsystem (control-plane repo) | Compose stack | CLI binary → real API | Catches schema + validation drift |
| E2E (utilities repo) | Full DCM stack | CLI → gateway → control-plane | Already planned in FLPATH-4638 |

This plan focuses on **Tiers 1 and 2** — the lightweight, CI-native checks
that would have caught FLPATH-4770 without requiring the full DCM stack.

---

## Upstream Test Coverage

### What exists today

| Repo | Test type | What it validates |
|------|-----------|-------------------|
| `cli` | Unit (`internal/commands/*_test.go`) | CLI flag parsing, output formatting, HTTP mock interactions |
| `cli` | Snapshot tests (`go-snaps`) | CLI output matches recorded snapshots |
| `control-plane` | Subsystem (`test/subsystem/catalog/`) | CRUD operations via generated Go client against real API |
| `utilities` | E2E (`tests/e2e/cli_*.go`) | CLI binary against live full stack |

### What's missing (this plan fills)

- No test validates CLI's `parseInputFileAs` path with the **actual documented
  YAML** from the website
- No test exercises CLI against a **real control-plane** as part of CLI repo CI
- No alerting mechanism for stale `go.mod` dependencies

---

## Test Cases

### TC-01: CLI catalog item create parses multi-resource YAML correctly

**Priority:** P1 (critical)
**Type:** Functional
**Method:** Automated (static — CLI repo unit test)
**Labels:** `contract`, `static`
**Requires:** None

#### Description

Validates that the CLI's YAML→JSON conversion preserves the `spec.resources`
field structure. This is the exact path that failed in FLPATH-4770: the CLI's
Go types lacked the `Resources` field, so `json.Unmarshal` silently dropped it.

#### Prerequisites

- CLI repo checked out with current `go.mod`
- Test fixture `testdata/website/small-vm.yaml` (copy of website tutorial YAML)

#### Steps

**Step 1: Parse the documented catalog item YAML through CLI's conversion path**

The CLI calls `parseInputFileAs[catalogapi.CreateCatalogItemJSONRequestBody]`
which is the exact type that lost the `Resources` field in FLPATH-4770.

```go
// In internal/commands/contract_test.go
import catalogapi "github.com/dcm-project/control-plane/api/catalog/v1alpha1"

func TestDocCatalogItemPreservesResources(t *testing.T) {
    ci, err := parseInputFileAs[catalogapi.CreateCatalogItemJSONRequestBody](
        "testdata/website/small-vm.yaml",
    )
    require.NoError(t, err)

    // The critical assertion: resources field is populated
    require.NotEmpty(t, ci.Spec.Resources, "spec.resources must not be empty")
    assert.Equal(t, "main", ci.Spec.Resources[0].Name)
    assert.Equal(t, "vm", ci.Spec.Resources[0].ServiceType)
    assert.NotNil(t, ci.Spec.Resources[0].Fields)

    // Verify JSON round-trip (what actually goes on the wire)
    payload, _ := json.Marshal(ci)
    var raw map[string]any
    json.Unmarshal(payload, &raw)
    spec := raw["spec"].(map[string]any)
    _, hasResources := spec["resources"]
    assert.True(t, hasResources, "JSON payload must contain 'resources' key")
}
```

**Expected:** Test passes — `spec.resources[0].name == "main"`,
`spec.resources[0].service_type == "vm"`, JSON payload contains `resources` key.

#### Cleanup

None.

---

### TC-02: CLI catalog instance create preserves resource field in user_values

**Priority:** P1 (critical)
**Type:** Functional
**Method:** Automated (static — CLI repo unit test)
**Labels:** `contract`, `static`
**Requires:** None

#### Description

Validates that `user_values[].resource` is preserved through the CLI's
YAML→JSON conversion. This field was added as part of the multi-resource schema
and is now required by the API.

#### Prerequisites

- Test fixture `testdata/website/my-vm.yaml` (copy of website tutorial YAML)

#### Steps

**Step 1: Parse the documented instance YAML**

The CLI calls
`parseInputFileAs[catalogapi.CreateCatalogItemInstanceJSONRequestBody]`.

```go
func TestDocInstancePreservesResourceField(t *testing.T) {
    inst, err := parseInputFileAs[catalogapi.CreateCatalogItemInstanceJSONRequestBody](
        "testdata/website/my-vm.yaml",
    )
    require.NoError(t, err)

    require.NotEmpty(t, inst.Spec.UserValues)
    for i, uv := range inst.Spec.UserValues {
        assert.NotEmpty(t, uv.Resource,
            "user_values[%d].resource must not be empty", i)
    }
    assert.Equal(t, "main", inst.Spec.UserValues[0].Resource)
}
```

**Expected:** All `user_values` entries have `resource == "main"`.

#### Cleanup

None.

---

### TC-03: CLI catalog item and instance lifecycle against real control-plane

**Priority:** P1 (critical)
**Type:** Functional
**Method:** Automated (subsystem — control-plane compose stack)
**Labels:** `contract`, `subsystem`
**Requires:** Catalog-subsystem compose stack running

#### Description

Builds the CLI from source and exercises the full Getting Started journey
(create catalog item → create instance) against a real control-plane. This
would have caught FLPATH-4770 because the API returns HTTP 400 when
`spec.resources` is missing or `user_values[].resource` is absent.

The test uses `small-vm` as the catalog item ID to match the fixture YAML's
`catalog_item_id` reference, ensuring the documented examples work as-is
without modification.

#### Prerequisites

- Catalog-subsystem compose stack running (port 28080)
- WireMock configured to accept placement-manager requests (default in compose)
- CLI built from source (`go build -o /tmp/dcm ./cmd/dcm`)
- Fixture YAMLs in `testdata/website/` (byte-for-byte copies of website examples)

#### Steps

**Step 1: Create a catalog item using the documented YAML**

```bash
/tmp/dcm catalog item create \
  --from-file testdata/website/small-vm.yaml \
  --id small-vm \
  --control-plane-url http://localhost:28080
```

**Expected:** Exit code 0. Table output showing `UID=small-vm`,
`SERVICE TYPE=vm`.

**Step 2: Verify via raw API that resources are stored correctly**

```bash
curl -s http://localhost:28080/api/v1alpha1/catalog-items/small-vm | \
  jq '.spec.resources[0].name'
```

**Expected:** Output is `"main"`.

**Step 3: Create an instance using the documented YAML**

The fixture `my-vm.yaml` references `catalog_item_id: small-vm`, which must
match the item created in Step 1.

```bash
/tmp/dcm catalog instance create \
  --from-file testdata/website/my-vm.yaml \
  --id my-dev-vm \
  --control-plane-url http://localhost:28080
```

**Expected:** Exit code 0. Table output showing `UID=my-dev-vm`.

**Step 4: Verify via raw API that user_values have resource field**

```bash
curl -s http://localhost:28080/api/v1alpha1/catalog-item-instances/my-dev-vm | \
  jq '.spec.user_values[0].resource'
```

**Expected:** Output is `"main"`.

#### Cleanup

Instance must be deleted before the catalog item (API enforces referential
integrity — returns 409 if instances still reference the item).

```bash
curl -s -X DELETE http://localhost:28080/api/v1alpha1/catalog-item-instances/my-dev-vm
/tmp/dcm catalog item delete small-vm \
  --control-plane-url http://localhost:28080
```

---

### TC-04: CLI fails gracefully when API rejects malformed payload

**Priority:** P2 (important)
**Type:** Negative
**Method:** Automated (subsystem)
**Labels:** `contract`, `negative`
**Requires:** Compose stack running

#### Description

Verifies that when the CLI sends a payload the API rejects (e.g., missing
`spec.resources`), the error is surfaced to the user with actionable detail.

#### Prerequisites

- A YAML file using the **old** flat schema (no `resources` wrapper)

#### Steps

**Step 1: Create a YAML file with the old schema**

```bash
cat > /tmp/old-schema.yaml << 'EOF'
api_version: v1alpha1
display_name: "Old Schema"
spec:
  service_type: vm
  fields:
    - path: vcpu.count
      editable: true
      default: 2
EOF
```

**Step 2: Attempt to create a catalog item with the old-schema YAML**

```bash
/tmp/dcm catalog item create \
  --from-file /tmp/old-schema.yaml \
  --id should-fail \
  --control-plane-url http://localhost:28080
```

**Expected:** Non-zero exit code. Error message includes the API's problem
detail (e.g., `"property \"resources\" is missing"`). The CLI should NOT
silently succeed with a partial payload.

#### Cleanup

None (creation should fail).

---

### TC-05: Dependency freshness check detects stale control-plane pin

**Priority:** P2 (important)
**Type:** Functional
**Method:** Automated (CI workflow — CLI repo)
**Labels:** `contract`, `ci`
**Requires:** None

#### Description

A scheduled workflow that compares the CLI's pinned `control-plane` version
against `@main` and alerts if more than 2 weeks behind.

#### Steps

**Step 1: Query current and latest versions**

```bash
CURRENT=$(grep 'dcm-project/control-plane' go.mod | awk '{print $2}')
LATEST=$(go list -m -json github.com/dcm-project/control-plane@main 2>/dev/null | jq -r .Version)
echo "Current: $CURRENT"
echo "Latest:  $LATEST"
```

**Step 2: Extract timestamps and compute age**

Pseudo-versions use format `v0.0.0-YYYYMMDDHHMMSS-commithash`. Tagged versions
(e.g., `v0.1.0`) do not embed a timestamp and should be resolved via `go list`
to get the commit time.

```bash
extract_epoch() {
  local version="$1"
  local ts
  ts=$(echo "$version" | grep -oE '[0-9]{14}')
  if [ -z "$ts" ]; then
    # Tagged version — resolve commit time via go list
    ts=$(go list -m -json "github.com/dcm-project/control-plane@${version}" 2>/dev/null \
      | jq -r '.Time // empty' | cut -dT -f1 | tr -d '-')
    ts="${ts}000000"
  fi
  if [ -z "$ts" ]; then
    echo "0"
    return
  fi
  # Convert YYYYMMDDHHMMSS to epoch seconds
  date -jf "%Y%m%d%H%M%S" "$ts" "+%s" 2>/dev/null || \
    date -d "${ts:0:8} ${ts:8:2}:${ts:10:2}:${ts:12:2}" "+%s" 2>/dev/null || \
    echo "0"
}

current_epoch=$(extract_epoch "$CURRENT")
latest_epoch=$(extract_epoch "$LATEST")
```

**Step 3: Alert if stale**

```bash
if [ "$current_epoch" -eq 0 ] || [ "$latest_epoch" -eq 0 ]; then
  echo "::warning::Could not parse dependency version timestamps"
  exit 0
fi

age_days=$(( (latest_epoch - current_epoch) / 86400 ))
echo "Dependency age: ${age_days} days behind main"

if [ "$age_days" -gt 14 ]; then
  echo "::warning::control-plane dependency is ${age_days} days stale (have ${CURRENT}, latest ${LATEST})"
  # Optionally open an issue:
  # gh issue create --title "CLI: control-plane dep is ${age_days} days stale" \
  #   --body "Current: ${CURRENT}\nLatest: ${LATEST}" --label "dependency"
  exit 1
fi

echo "Dependency is fresh (${age_days} days)"
```

**Expected:** If the dependency is fresh (≤14 days behind `@main`), the step
passes silently. If stale (>14 days), the workflow emits a `::warning`
annotation and exits non-zero, failing the scheduled check.

#### Cleanup

None.

---

## Not Testable at Contract Level

| Scenario | Reason | Where tested instead |
|----------|--------|---------------------|
| Full provisioning flow (SP assigns resources) | Requires real SP + cluster | E2E suite (`core_platform_test.go`) |
| Authentication/OIDC token refresh | Auth disabled in subsystem stack | Auth subsystem tests + E2E auth tests |
| CLI output formatting/table rendering | UI concern, not a contract | CLI unit tests with snapshots |
| Network failures / retries | Infrastructure-level | Not currently tested anywhere |

---

## Implementation Notes

### Proposed file locations

| Artifact | Location | Status |
|----------|----------|--------|
| Static contract tests (TC-U154, TC-U155) | `cli/internal/commands/contract_test.go` | Implemented |
| Doc fixture YAMLs | `cli/testdata/website/small-vm.yaml`, `cli/testdata/website/my-vm.yaml` | Implemented |
| Fixture drift check | `cli/hack/check-website-fixtures.sh` | Implemented |
| Fixture drift workflow | `cli/.github/workflows/check-website-fixtures.yaml` | Implemented |
| Subsystem workflow | `control-plane/.github/workflows/cli-contract.yaml` | Not started |
| Dep freshness workflow | `cli/.github/workflows/dep-freshness.yaml` | Not started |
| Doc validation workflow | `dcm-project.github.io/.github/workflows/validate-examples.yaml` | Not started |

### Existing infrastructure to reuse

- `control-plane/test/subsystem/catalog/docker-compose.yaml` — compose stack
  with Postgres + control-plane + WireMock
- `shared-workflows/.github/workflows/black-box.yaml` — reusable CI pattern
  for compose-up → test → compose-down
- CLI's `internal/commands/helpers.go` — `parseInputFileAs[T]` is the exact
  function under test

### Fixture maintenance

The `testdata/website/` YAML files should be **byte-for-byte copies** of the YAML
code blocks in the website's getting-started pages. CI verifies drift via:

```bash
# Local check (also: make check-fixtures in cli repo)
hack/check-website-fixtures.sh

# Refresh fixtures from upstream website main (after website PR merges)
hack/check-website-fixtures.sh --update
```

The `check-website-fixtures` workflow runs on PRs and weekly. It fails until the
companion website PR (FLPATH-4770) merges — fixtures intentionally carry the
fixed schema ahead of upstream `main`.

---

## Coverage Summary

| Jira AC | Static (TC-01/02) | Subsystem (TC-03/04) | CI Alert (TC-05) |
|--------------------|--------------------|--------------------------|-------------------|
| 1. Static round-trip without data loss | TC-01, TC-02 | — | — |
| 2. CLI binary → real API acceptance | — | TC-03, TC-04 | — |
| 3. Staleness detection | — | — | TC-05 |

---

## Risk Observations

1. **Fixture drift** — If `testdata/website/` files get out of sync with the
   website, the contract test gives false confidence. Mitigate with a
   cross-repo sync check (see Fixture Maintenance above).

2. **Subsystem stack divergence** — The `catalog-subsystem` compose stack may
   evolve independently. If it changes ports or removes WireMock, the contract
   test workflow needs updating. Low risk (stable infrastructure since Feb 2026).

3. **CLI flag changes** — If the CLI renames `--control-plane-url` or
   `--from-file`, subsystem tests break. Mitigate by running CLI with
   `--help` as a smoke check before the journey tests.

4. **Generated client vs. CLI path** — The subsystem tests use a generated
   Go client; the contract tests exercise the CLI's own serialization path.
   Both are needed — they test different code paths to the same API.

5. **Cross-repo coordination** — Implementation spans three repos (CLI,
   control-plane, website). Changes should be coordinated to land in sequence:
   static tests first (CLI), then subsystem integration (control-plane),
   then doc validation (website).

---

## Version History

| Version | Date | Changes |
|---|---|---|
| 1.2 | 2026-09-03 | Updated implementation notes: `testdata/website/`, `check-website-fixtures` script/workflow, TC-U154/TC-U155 status |
| 1.1 | 2026-08-18 | Merged TC-03/04 into single lifecycle test (fixture ID alignment), renumbered TC-05→04 and TC-06→05, completed TC-05 freshness procedure, fixed CLI subcommand references |
| 1.0 | 2026-08-17 | Initial test plan: 5 test cases covering static serialization, subsystem integration, negative scenarios, and dependency freshness |

---

## Sanitization Notice

This document is intended for sharing. The following rules apply:

- No credentials, tokens, API keys, or passwords — use placeholders
- No internal hostnames or IPs — use `localhost` for local deployments
- No PII
- Open-source tool and project names are fine as-is
