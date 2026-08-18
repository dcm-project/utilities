# Test Plan: FLPATH-2761 — DCM Tech Demo / POC

| Field | Value |
|---|---|
| **Ticket** | [FLPATH-2761](https://redhat.atlassian.net/browse/FLPATH-2761) |
| **Author** | Thomas Stetson |
| **Version** | 1.3 |
| **Last Updated** | 2026-08-17 |
| **Status** | Draft |

## Description

The DCM Tech Demo / POC epic delivers a GitHub-hosted website containing technical direction (ADRs/enhancements), user documentation, and links to demo recordings covering all major features. The website serves as the primary vehicle for potential customers to understand DCM's concepts, architecture, and operational value.

This test plan validates the **documentation accuracy, completeness, and reproducibility** of the website and its tutorials against the live DCM stack.

### References

- [FLPATH-2761](https://redhat.atlassian.net/browse/FLPATH-2761) — DCM: Tech Demo / POC (epic)
- [FLPATH-2920](https://redhat.atlassian.net/browse/FLPATH-2920) — Use GitHub Pages for our website (Closed)
- [FLPATH-2954](https://redhat.atlassian.net/browse/FLPATH-2954) — User documentation (Closed)
- [FLPATH-3012](https://redhat.atlassian.net/browse/FLPATH-3012) — Link enhancements to our website (Closed)
- [FLPATH-3067](https://redhat.atlassian.net/browse/FLPATH-3067) — Add link verification and linting to website CI pipeline (Closed)
- [FLPATH-3115](https://redhat.atlassian.net/browse/FLPATH-3115) — Introduction to DCM (Closed)
- Website: [dcm-project.github.io](https://dcm-project.github.io)
- Repo: [dcm-project/dcm-project.github.io](https://github.com/dcm-project/dcm-project.github.io)

### Acceptance Criteria (from epic)

1. GitHub-hosted website containing technical direction (ADRs) and user documentation
2. GitHub-hosted website containing links to demo recordings containing all major features

---

## Environment and Global Setup

### Environment Requirements

- A workstation with a browser, `curl`, `jq`, `podman`, `podman-compose`, and Go installed
- Network access to `dcm-project.github.io` and `github.com/dcm-project`
- (For tutorial reproduction) Ports 8080, 7007, 5432, 4222 free
- (Optional) A Kubernetes cluster with KubeVirt for provider tutorials

### Global Setup

**Step 1: Verify the website is live**

```bash
curl -s -o /dev/null -w '%{http_code}' https://dcm-project.github.io
```

**Expected:** `200`.

**Step 2: Clone the control-plane for tutorial reproduction**

```bash
git clone https://github.com/dcm-project/control-plane.git
cd control-plane/deploy
```

**Step 3: Start the DCM stack locally**

```bash
podman-compose up -d
```

**Expected:** All services healthy within 60 seconds.

**Step 4: Build the CLI**

```bash
git clone https://github.com/dcm-project/cli.git
cd cli && make build
sudo cp bin/dcm /usr/local/bin/
```

**Expected:** `dcm version` returns version information.

---

## Upstream CI Coverage

The website repo ([dcm-project/dcm-project.github.io](https://github.com/dcm-project/dcm-project.github.io)) has the following CI workflows:

| Workflow | What it checks | Trigger |
|----------|---------------|---------|
| **Check formatting** | Prettier on `content/docs/**`, `content/blog/**` | PRs to `main` |
| **Check spelling** | cspell dictionary validation | PRs to `main` |
| **Deploy Hugo site to Pages** | Hugo build (catches broken shortcodes, missing modules) | Push to `main` |

**Notable gap:** There is no dedicated external link checker (e.g., lychee, htmlproofer). Hugo's build validates internal module references and shortcodes but does not crawl external URLs for 404s. See "Link Validation Recommendation" in Risk Observations for assessment.

---

## Test Tiers

| Tier | Requires | Test Cases | Notes |
|------|----------|------------|-------|
| **Content review** | Browser + internet | TC-01, TC-05, TC-06, TC-08, TC-09 | Documentation accuracy review |
| **Tutorial reproduction** | Local DCM stack | TC-02, TC-03, TC-04 | Follow documented steps verbatim |
| **CI validation** | GitHub repo access | TC-07 | Verify automated checks |

---

## Test Cases

### TC-01: Website accessibility and navigation

**Priority:** P2 (important)
**Type:** Functional
**Method:** Manual
**Requires:** Browser, internet access

#### Description

All top-level navigation links on the website should resolve without errors. The site structure matches what's documented in the child tickets.

#### Steps

**Step 1: Load the homepage**

```bash
curl -s -o /dev/null -w '%{http_code}' https://dcm-project.github.io
```

**Expected:** HTTP 200.

**Step 2: Navigate all top-level links**

```bash
for path in / /docs/ /docs/getting-started/ /docs/user-guide/ /docs/enhancements/; do
  echo "$path: $(curl -s -o /dev/null -w '%{http_code}' https://dcm-project.github.io$path)"
done
```

**Expected:** All return 200.

**Step 3: Navigate Getting Started sub-pages**

```bash
for page in architecture local-setup create-small-vm-catalog-item create-placement-policy create-small-vm-instance register-another-provider troubleshooting; do
  echo "$page: $(curl -s -o /dev/null -w '%{http_code}' https://dcm-project.github.io/docs/getting-started/$page/)"
done
```

**Expected:** All return 200.

**Step 4: Navigate User Guide sub-pages**

```bash
for page in cli-configuration providers service-types catalog-items policies catalog-item-instances service-provider-resources ui; do
  echo "$page: $(curl -s -o /dev/null -w '%{http_code}' https://dcm-project.github.io/docs/user-guide/$page/)"
done
```

**Expected:** All return 200.

**Step 5: Navigate Blog section**

```bash
curl -s -o /dev/null -w '%{http_code}' https://dcm-project.github.io/blog/
echo "sovereignty demo: $(curl -s -o /dev/null -w '%{http_code}' https://dcm-project.github.io/blog/sovereignty-rehydrate-demo/)"
```

**Expected:** All return 200.

#### Cleanup

None.

---

### TC-02: Getting Started — Local Setup is reproducible

**Priority:** P1 (critical)
**Type:** Functional
**Method:** Manual
**Requires:** Clean workstation, podman, Go

#### Description

Following the "Local Setup" page instructions verbatim should result in a running DCM stack with a healthy API and functional CLI.

#### Prerequisites

- No existing DCM containers running
- Ports 8080, 7007, 5432, 4222 free

#### Steps

**Step 1: Follow documented clone and start commands**

```bash
DEPLOY_DIR=$(pwd)/control-plane
git clone https://github.com/dcm-project/control-plane.git "$DEPLOY_DIR"
cd "$DEPLOY_DIR/deploy"
podman-compose up -d
```

**Expected:** All containers start. `podman-compose ps` shows services running.

**Step 2: Verify health endpoint as documented**

```bash
curl http://localhost:8080/api/v1alpha1/health
```

**Expected:** `{"status":"ok","path":"/api/v1alpha1/health","checks":{"database":"ok","nats":"ok"}}` (or at minimum `status: "ok"`).

**Step 3: Verify DCM UI is accessible**

```bash
curl -s -o /dev/null -w '%{http_code}' http://localhost:7007
```

**Expected:** HTTP 200.

**Step 4: Build CLI as documented**

```bash
git clone https://github.com/dcm-project/cli.git /tmp/cli-build
cd /tmp/cli-build
make build
./bin/dcm version
```

**Expected:** Binary builds without errors. `dcm version` outputs version info.

**Step 5: List providers (no KubeVirt SP expected by default)**

```bash
curl http://localhost:8080/api/v1alpha1/providers
```

**Expected:** `{"providers":[]}` or providers list (matches documentation note about empty state).

#### Cleanup

```bash
cd "$DEPLOY_DIR/deploy" && podman-compose down -v
```

---

### TC-03: Getting Started tutorials produce documented output

**Priority:** P1 (critical)
**Type:** Functional
**Method:** Manual
**Requires:** TC-02 stack running, CLI available

#### Description

Step through each Getting Started tutorial page and verify the commands produce output consistent with what's documented. This validates that the API contracts haven't drifted from the docs.

#### Prerequisites

- DCM stack from TC-02 running
- `dcm` CLI built and in PATH
- (Optional) KubeVirt cluster for provider registration

#### Steps

**Step 1: Create Small VM Catalog Item (per docs)**

Follow commands from `https://dcm-project.github.io/docs/getting-started/create-catalog-item/`.

**Expected:** Catalog item created with HTTP 201. Response body matches documented fields (display_name, spec, service_type).

**Step 2: Create Placement Policy (per docs)**

Follow commands from `https://dcm-project.github.io/docs/getting-started/create-policy/`.

**Expected:** Policy created with HTTP 201. Response body includes id, display_name, rego_code.

**Step 3: Create Instance (per docs)**

Follow commands from `https://dcm-project.github.io/docs/getting-started/create-instance/`.

**Expected:** Instance created with HTTP 201 (or documented error if no provider is registered). Error message matches what's documented in Troubleshooting.

**Step 4: Register Another Provider (per docs)**

Follow commands from `https://dcm-project.github.io/docs/getting-started/register-provider/`.

**Expected:** Provider registered. Response matches documented shape.

**Step 5: Troubleshooting page commands work**

Follow diagnostic commands from `https://dcm-project.github.io/docs/getting-started/troubleshooting/`.

**Expected:** Commands execute without errors. Output format matches examples (container logs, status checks).

#### Cleanup

Delete resources created during steps, then `podman-compose down -v`.

---

### TC-04: UI User Guide accuracy against live UI

**Priority:** P1 (critical)
**Type:** Functional
**Method:** Manual
**Requires:** TC-02 stack running, browser

#### Description

Open the DCM UI and verify each tab matches its documentation: column names, available actions, and behavior patterns.

#### Prerequisites

- DCM stack running with UI at `http://localhost:7007`

#### Steps

**Step 1: Navigate to the DCM page**

Open `http://localhost:7007` and navigate to Administration → Data Center (or `/dcm`).

**Expected:** Six tabs visible: Providers, Policies, Service Types, Catalog Items, Instances, Resources.

**Step 2: Verify Providers tab columns**

**Expected:** Columns match documentation: Display name, Name, Endpoint, Service type, Operations, Status, Actions. Actions include Edit and Delete.

**Step 3: Verify Policies tab columns and actions**

**Expected:** Columns: Display name, Type, Priority, Enabled, Description, Actions. Actions: enable/disable toggle, edit, delete. Create button available.

**Step 4: Verify Service Types tab (read-only)**

**Expected:** Columns: Service type, API version, Path, Created. No create/edit/delete actions.

**Step 5: Verify Catalog Items tab**

**Expected:** Columns: Display name, API version, Service type, Fields, Created, Actions. Create opens a side drawer (not modal).

**Step 6: Verify Instances tab**

**Expected:** Columns: Display name, Catalog item, Resource ID, API version, Created, Actions. Actions include Rehydrate and Delete.

**Step 7: Verify Resources tab (read-only)**

**Expected:** Columns: ID, Service type, Provider, Status, Created. No create/edit/delete actions.

**Step 8: Test common patterns**

- Search field filters rows in real time
- Pagination (5/10/25 per page) works
- Destructive actions show confirmation dialogs

**Expected:** All behaviors match documentation.

#### Cleanup

None.

---

### TC-05: Enhancements section linked and accessible

**Priority:** P2 (important)
**Type:** Functional
**Method:** Manual
**Requires:** Browser, internet access

#### Description

The Enhancements section should link to the enhancements repository and individual enhancement documents should be viewable.

#### Steps

**Step 1: Navigate to enhancements documentation page**

```bash
curl -s -o /dev/null -w '%{http_code}' https://dcm-project.github.io/docs/enhancements/
```

**Expected:** HTTP 200.

**Step 2: Verify link to enhancements repository**

```bash
curl -s https://dcm-project.github.io/docs/enhancements/ | grep -o 'https://github.com/dcm-project/enhancements[^"]*'
```

**Expected:** Contains a link to `https://github.com/dcm-project/enhancements`.

**Step 3: Verify enhancements repo has content**

```bash
gh api repos/dcm-project/enhancements/contents/enhancements --jq '.[].name' | head -5
```

**Expected:** At least one enhancement directory listed.

#### Cleanup

None.

---

### TC-06: Architecture documentation accuracy

**Priority:** P1 (critical)
**Type:** Functional
**Method:** Manual (review)
**Requires:** Browser, running DCM stack

#### Description

The architecture page describes the current system topology. Verify component names, ports, relationships, and the request flow diagram match reality.

#### Steps

**Step 1: Verify the architecture page loads**

```bash
curl -s -o /dev/null -w '%{http_code}' https://dcm-project.github.io/docs/getting-started/architecture/
```

**Expected:** HTTP 200.

**Step 2: Verify documented port matches reality**

```bash
curl -s http://localhost:8080/api/v1alpha1/health | jq .status
```

**Expected:** `"ok"` — confirming port 8080 is correct.

**Step 3: Review component table for accuracy**

Documented components:
- Control Plane (single process, port 8080) ✓
- Catalog Manager (inside control plane) ✓
- Policy Manager (inside control plane, embedded OPA) ✓
- Placement Manager (inside control plane) ✓
- Service Provider Manager (inside control plane) ✓
- PostgreSQL (persistent storage) ✓
- NATS (message bus for SP communication) ✓
- Service Providers (external systems) ✓

**Expected:** All components exist and are described accurately per current implementation.

**Step 4: Verify request flow matches actual behavior**

Create a catalog item instance and observe that placement and provisioning occur as described in the sequence diagram.

**Expected:** The flow matches: User → Control Plane → Catalog Manager → Placement Manager → Policy Manager → SP Manager → Service Provider.

#### Cleanup

None.

---

### TC-07: CI quality gates are active and passing

**Priority:** P2 (important)
**Type:** Functional
**Method:** Manual (GitHub inspection)
**Requires:** GitHub access

#### Description

FLPATH-3067 added CI pipelines for content quality. Verify the formatting and spelling workflows are still active, triggering on PRs, and passing. Note: despite the ticket title mentioning "link verification," the implemented CI checks formatting (Prettier) and spelling (cspell) — there is no dedicated external link checker.

#### Steps

**Step 1: Verify workflows exist and are active**

```bash
gh api repos/dcm-project/dcm-project.github.io/actions/workflows --jq '.workflows[] | {name, state}'
```

**Expected:**
- `Check formatting` — `active`
- `Check spelling` — `active`
- `Deploy Hugo site to Pages` — `active`

**Step 2: Verify recent runs are passing**

```bash
gh api repos/dcm-project/dcm-project.github.io/actions/runs --jq '.workflow_runs[:5] | .[] | {name: .name, conclusion: .conclusion, created_at: .created_at}'
```

**Expected:** Recent runs show `conclusion: "success"`.

**Step 3: Verify quality checks trigger on PRs**

```bash
gh api repos/dcm-project/dcm-project.github.io/contents/.github/workflows/check-format.yaml --jq '.content' | base64 -d | grep -A3 "^on:"
```

**Expected:** Triggers include `pull_request` on `branches: [main]`.

**Step 4: Verify Hugo build catches structural issues**

The Hugo build step uses `--gc --minify` and will fail if:
- Internal shortcode references are broken
- Hugo modules fail to resolve
- Template syntax errors exist

**Expected:** This provides implicit internal link validation for Hugo-managed cross-references, but does **not** validate external URLs or raw HTML `<a>` links.

#### Cleanup

None.

---

### TC-08: Demo content exists and is accessible (AC #2)

**Priority:** P1 (critical)
**Type:** Functional
**Method:** Manual (review)
**Requires:** Browser, internet access

#### Description

The epic's second acceptance criterion requires "links to demo recordings containing all major features." The website delivers this via a blog post ("Sovereignty in action: a DCM demo") with an interactive walkthrough hosted on Red Hat's interactive demo platform. Verify these are accessible and cover the claimed features.

#### Current state (as of analysis)

The blog section contains one demo post:
- **Blog post:** `content/blog/sovereignty-rehydrate-demo/index.md`
- **Interactive walkthrough:** [interact.redhat.com/share/yFwS2KKEs4Zvjc3USx5j](https://interact.redhat.com/share/yFwS2KKEs4Zvjc3USx5j)
- **Features covered:** Policy creation (sovereignty/Rego), catalog item deployment, multi-provider placement, datacenter failover, rehydration

#### Steps

**Step 1: Verify blog post is accessible**

```bash
curl -s -o /dev/null -w '%{http_code}' https://dcm-project.github.io/blog/sovereignty-rehydrate-demo/
```

**Expected:** HTTP 200.

**Step 2: Verify interactive walkthrough link is live**

```bash
curl -s -o /dev/null -w '%{http_code}' https://interact.redhat.com/share/yFwS2KKEs4Zvjc3USx5j
```

**Expected:** HTTP 200 (or 301/302 redirect to the demo). Not gated behind authentication.

**Step 3: Open the interactive walkthrough in a browser**

Navigate to the link and click through the demo.

**Expected:** Demo loads, is interactive, and walks through:
- Viewing providers across regions
- Creating a sovereignty Rego policy
- Deploying Pet Clinic to a specific region
- Simulating datacenter failure
- Rehydrating the instance (stays within policy-compliant region)

**Step 4: Assess feature coverage against AC #2**

| Major Feature | Covered by demo? |
|---------------|-----------------|
| Policy-based placement | Yes (sovereignty policy) |
| Multi-provider/multi-region | Yes (3 DCs, 2 regions) |
| Catalog item deployment | Yes (Pet Clinic) |
| Rehydration / disaster recovery | Yes (core scenario) |
| Service provider registration | Partially (providers pre-exist in demo) |
| CLI usage | Mentioned (rehydrate command) but walkthrough is UI-focused |

**Expected:** Core placement and rehydration features are well covered. Service provider CRUD and CLI-only workflows are not demoed — assess whether this gap matters for AC #2 ("all major features").

#### Cleanup

None.

**Assessment:** AC #2 is **partially met**. The interactive walkthrough covers the primary differentiated feature (policy-driven sovereignty + rehydration) comprehensively, but does not demo basic CRUD operations like provider registration or catalog item creation as standalone features. Whether this is sufficient depends on interpretation of "all major features."

---

### TC-09: CLI User Guide accuracy

**Priority:** P2 (important)
**Type:** Functional
**Method:** Manual
**Requires:** CLI built, DCM stack running

#### Description

Verify that CLI commands documented in the User Guide match actual CLI behavior — flag names, subcommand structure, output formats, and examples.

#### Prerequisites

- `dcm` CLI built and in PATH
- DCM stack running

#### Steps

**Step 1: Verify CLI help matches documented subcommands**

```bash
dcm --help
```

**Expected:** Top-level command groups include `catalog`, `sp`, `policy`, and `version`.

**Step 2: Verify sp provider commands**

```bash
dcm sp provider list
dcm sp provider --help
```

**Expected:** Output format and flags match documentation at `/docs/user-guide/cli/providers/`.

**Step 3: Verify catalog item commands**

```bash
dcm catalog item list
dcm catalog item --help
```

**Expected:** Matches documentation at `/docs/user-guide/cli/catalog-items/`.

**Step 4: Verify policy commands**

```bash
dcm policy list
dcm policy --help
```

**Expected:** Matches documentation at `/docs/user-guide/cli/policies/`.

**Step 5: Verify catalog instance commands**

```bash
dcm catalog instance list
dcm catalog instance --help
```

**Expected:** Matches documentation at `/docs/user-guide/cli/instances/`.

**Step 6: Verify configuration documentation**

```bash
dcm --help | grep -i url
```

**Expected:** Global flags (`--control-plane-url`, `-o`/`--output`) match documentation at `/docs/user-guide/cli/configuration/`.

#### Cleanup

None.

---

## Not Testable / Out of Scope

| Scenario | Why |
|----------|-----|
| KubeVirt provider tutorials (end-to-end) | Requires a K8s cluster with KubeVirt — deferred unless cluster available |
| Demo recording content quality | Subjective; can verify existence and accessibility but not production quality |
| Website performance / SEO | Not in acceptance criteria |
| Hugo template correctness | Covered by FLPATH-2920 (Closed); CI linting validates build |

---

## Coverage Summary

| AC | Test Cases | Status |
|----|-----------|--------|
| 1a. Website with technical direction (ADRs) | TC-05 | Enhancements linked from site |
| 1b. Website with user documentation | TC-01, TC-02, TC-03, TC-04, TC-06, TC-09 | Full documentation review |
| 2. Links to demo recordings of all major features | TC-08 | **Partially met** — sovereignty/rehydration demo exists; basic CRUD not demoed separately |
| — CI quality gate (FLPATH-3067) | TC-07 | Formatting + spelling active; no external link checker |

---

## Risk Observations

1. **Epic still "In Progress" with all children Closed**: Likely indicates additional demo content or a status update is pending. The sovereignty demo blog post was added 2026-05-25, after the child stories closed.
2. **Demo covers one scenario well but not "all major features"**: The interactive walkthrough demonstrates the sovereignty/rehydration flow comprehensively but doesn't cover basic CRUD operations (provider registration, catalog item creation, policy management) as standalone demos. Interpretation of AC #2 determines if this is a gap.
3. **Documentation may drift from implementation**: The control-plane evolves rapidly. Tutorial commands that worked when docs were written may produce different output or errors now (e.g., new required fields, changed response formats).
4. **UI documentation is extensive but not automated**: The UI User Guide describes column names and action behaviors in detail. Any UI refactor could silently invalidate documentation without detection.
5. **No link to `deploy-dcm.sh` utility script**: The website documents `podman-compose up -d` directly. Users not using the utilities repo won't benefit from health polling, version pinning, or service provider setup.
6. **Interactive walkthrough depends on third-party hosting**: The demo lives at `interact.redhat.com` — if that platform is deprecated or the share link expires, AC #2 regresses without any CI signal.

### Link Validation Recommendation

**Do we need an external link checker?**

The website has ~30 content pages with external links to GitHub repos, the OPA docs, Red Hat interactive platform, KubeVirt docs, and Go/Podman download pages. The current CI (Prettier + cspell + Hugo build) catches:
- Broken Hugo shortcodes and internal module references (build fails)
- Formatting drift (Prettier)
- Spelling errors (cspell)

It does **not** catch:
- External URLs that have gone 404 (e.g., renamed GitHub repos, moved docs)
- Broken anchor fragments (`#section-that-was-renamed`)
- The interactive demo link going stale

**Assessment:** For a 33-commit, low-churn documentation site, a dedicated external link checker is **nice-to-have but not critical**. The cost/benefit:

| Factor | For | Against |
|--------|-----|---------|
| Site size | Small (~30 pages) | Manual spot-checks are feasible |
| External link count | Moderate (GitHub, OPA, Red Hat, kubevirt.io) | Most are stable, long-lived URLs |
| Churn rate | ~2-3 commits/month | Link rot risk is low at this velocity |
| CI complexity | lychee-action is ~5 lines of YAML | One more workflow to maintain |
| False positives | Rate-limited hosts (GitHub, Quay) can flake | Needs `fail: false` + cache tuning |

**Recommendation:** If adding link validation, use **[lychee](https://github.com/lycheeverse/lychee-action)** — it's the de facto standard for Hugo/GitHub Pages sites in 2026, having largely replaced the older Ruby-based htmlproofer. Key advantages:
- Rust-based, async — fast even on large sites
- Built-in caching (`.lycheecache`) avoids redundant requests and rate limits
- `fail: false` mode prevents flaky external hosts from blocking merges
- Official GitHub Action (`lycheeverse/lychee-action@v2`) with minimal config

However, given the site's low churn and small link surface, **manual validation during test execution (TC-01, TC-05, TC-08) is sufficient** for now. A lychee workflow becomes worthwhile if the site grows past ~50 pages or external links start breaking unnoticed.

---

## Test Execution Results — 2026-08-06

**Environment:** macOS, podman 5.x, DCM stack deployed from `main` via `deploy-dcm.sh`, CLI `main` (commit 7316690, built 2026-07-21).

| TC | Result | Notes |
|----|--------|-------|
| **TC-01** | **PASS** | All 21 URLs return HTTP 200 (homepage, 6 top-level, 7 Getting Started, 8 User Guide, blog index, sovereignty demo) |
| **TC-02** | **PASS** | Stack deployed, health OK (`database: ok, nats: ok`), UI on :7007 returns 200, CLI returns version info, providers list empty as expected |
| **TC-03** | **PARTIAL FAIL** | Policy creation works. **Catalog item creation fails** — API requires `spec.resources` field not shown in tutorial YAML (HTTP 400). Instance creation also fails for the same reason (`resource` property missing in user_values). Troubleshooting commands work correctly. |
| **TC-04** | **SKIPPED** | Requires manual browser inspection |
| **TC-05** | **PASS** | Enhancements page returns 200, links to `github.com/dcm-project/enhancements`, repo contains 26 enhancement directories |
| **TC-06** | **PASS** | Architecture page accurate: port 8080 confirmed, control plane monolith running, PostgreSQL + NATS + DCM-UI all present, API path structure matches documented flow |
| **TC-07** | **PASS** | 3 active workflows (Check formatting, Check spelling, Deploy Hugo site to Pages). All recent runs passing (latest: 2026-07-03). Both quality checks trigger on PRs to main. |
| **TC-08** | **PASS** | Blog post at `/blog/sovereignty-rehydrate-demo/` returns 200. Interactive walkthrough at `interact.redhat.com` returns 200 (publicly accessible). |
| **TC-09** | **PASS** (with note) | Latest CLI (`main` build) matches docs: `--control-plane-url` flag, nested commands (`dcm sp provider`, `dcm catalog item`, etc.), default port 8080. All list commands work against live stack. **Note:** Previously-downloaded v0.2.0 binary had old flag name `--api-gateway-url` (default :9080) — users with stale binaries will hit this. |

### Defects Found

| Severity | Description | Affected TCs |
|----------|-------------|-------------|
| **Critical** | End-to-end Getting Started journey is broken. The multi-resource catalog item schema (control-plane PR #11, merged Jul 3) requires `spec.resources[]` and `user_values[].resource`, but: (1) tutorial YAML uses the old flat schema, (2) the CLI (all versions including latest `main` build) silently drops the `resources` field because its `go.mod` still pins pre-schema-change types, (3) no tagged CLI release exists that supports the new schema. Only raw `curl` can create catalog items against the current API. | TC-03, TC-09 |
| **Low** | `make download-cli` pulls from `main` (no version tag). Downloaded binary version string is a commit hash, not a semver. Users following release docs may have stale v0.2.0. | TC-09 |
| **Info** | Keycloak container exits with code 134 on ARM64 (image platform mismatch warning). Does not affect core functionality — control plane, NATS, postgres, and UI all run fine. | TC-02 |

### Filed Defect

The critical bug discovered during test execution was filed as
[FLPATH-4770](https://redhat.atlassian.net/browse/FLPATH-4770) — "Getting
Started user journey broken end-to-end: CLI cannot create catalog items against
current control-plane."

**Root cause (3 layers):**
1. Website tutorial YAML uses the pre-July flat schema (missing `spec.resources`)
2. CLI's `go.mod` pins a stale `control-plane` dependency — its
   `CreateCatalogItemJSONRequestBody` type lacks the `Resources` field, so
   YAML→JSON round-trip silently drops it
3. No published CLI artifact can create a catalog item against the current API

**Fix (in progress on local branches `FLPATH-4770-fix-user-journey`):**
- CLI: `go get github.com/dcm-project/control-plane@main && go mod tidy`
- Website: Update both getting-started YAML files to multi-resource schema

**Preventive CI:** See [FLPATH-4794](https://redhat.atlassian.net/browse/FLPATH-4794)
and the Preventive CI Recommendations section below.

**Timeline:**
- Jun 17: CLI pins to control-plane `e4374fc` (pre-schema)
- Jul 3: Control-plane PR #11 introduces required `spec.resources` field
- Jul 21: CLI PR #28 updates test fixtures and table display only — does not bump `go.mod`
- Aug 6: Issue discovered during E2E test plan execution (this plan)
- Aug 17: Fix implemented locally, FLPATH-4770 filed, FLPATH-4794 filed for preventive CI

Full details (reproduction steps, exact YAML diffs, verification commands) are
in [FLPATH-4770](https://redhat.atlassian.net/browse/FLPATH-4770).

---

## Preventive CI Recommendations

The FLPATH-4770 failure exposed a structural gap: no automated check validates
that documented YAML examples remain compatible with the current API schema or
that the CLI can serialize those examples correctly. The following CI gates
would prevent recurrence.

### 1. Doc Example Schema Validation (website repo)

**Where:** `dcm-project/dcm-project.github.io` — new PR workflow

**What:** Extract YAML code blocks from `content/docs/getting-started/` and
validate them against the control-plane's published OpenAPI spec. Fails the PR
if any documented example violates required fields, type constraints, or
structural rules.

**Why:** Catches the exact scenario where a control-plane schema change
(FLPATH-4384) ships without updating the tutorial documentation.

**Effort:** Low — a single workflow step using a JSON Schema validator
(e.g., `ajv-cli` or a Go-based OpenAPI validator). No running services needed.

**Related tickets:** None exist. This is a new preventive measure.

### 2. CLI-to-API Contract Test (control-plane or CLI repo)

**Where:** `dcm-project/control-plane` subsystem CI (preferred — infrastructure
already exists) or `dcm-project/cli` CI.

**What:** Build the CLI from source, exercise the documented Getting Started
journey against a real control-plane instance (using the existing
`catalog-subsystem` compose stack with auth disabled), and verify 2xx responses.

**Why:** Catches the scenario where the CLI's `go.mod` dependency falls behind
the API, causing silently dropped fields. Also validates that the CLI's
YAML → JSON serialization path produces payloads the API accepts.

**Existing related work:**
- [FLPATH-3355](https://redhat.atlassian.net/browse/FLPATH-3355) — "Develop
  contract tests between api-gateway and providers" (Backlog, gateway↔SP scope)
- [control-plane#40](https://github.com/dcm-project/control-plane/issues/40) —
  "Org-wide: no real cross-repo contract validation exists" (Open, broader
  systemic issue)
- [FLPATH-4759](https://redhat.atlassian.net/browse/FLPATH-4759) — "kind +
  control-plane + osac-sp + mock-provider E2E" (New, first real-service
  contract test attempt)
- [FLPATH-4638](https://redhat.atlassian.net/browse/FLPATH-4638) — "Write CLI
  catalog item and catalog instance E2E tests" (New, adds test cases but not
  CI contract enforcement)

**Note:** None of these existing tickets specifically addressed the CLI↔API
contract boundary. [FLPATH-4794](https://redhat.atlassian.net/browse/FLPATH-4794)
was created to track this work. Detailed test plan:
`test-plans/FLPATH-4794-cli-api-contract-testing.md`.

**Effort:** Medium — reuses the existing subsystem compose stack; main work is
the workflow YAML and a `testdata/docs/` fixture directory.

### 3. Dependency Freshness Alert (CLI repo)

**Where:** `dcm-project/cli` — scheduled weekly workflow

**What:** Compare the pinned `control-plane` version in `go.mod` against
`@main` and emit a warning (or open an issue) if the dependency is more than
2 weeks stale.

**Why:** Early warning before drift becomes a breaking change. Low noise
(weekly cadence, warning only).

**Effort:** Trivial — 10-line workflow using `go list -m`.

---

## Version History

| Version | Date | Changes |
|---|---|---|
| 1.3 | 2026-08-17 | Added Preventive CI Recommendations section with three proposed gates to prevent FLPATH-4770 recurrence |
| 1.2 | 2026-08-06 | Added test execution results section with findings from automated run |
| 1.1 | 2026-08-06 | Revised after repo inspection: corrected page URLs, updated TC-07 to reflect actual CI (formatting+spelling, no link checker), updated TC-08 with actual demo content (sovereignty blog post + interactive walkthrough), added link validation recommendation, added Upstream CI Coverage section |
| 1.0 | 2026-08-06 | Initial test plan: 9 test cases covering website navigation, tutorial reproducibility, UI accuracy, architecture review, CI validation, demo recordings, and CLI docs |

---

## Sanitization Notice

This document is intended for sharing. The following rules apply:

- No credentials, tokens, API keys, or passwords — use placeholders
- No internal hostnames or IPs — use `localhost` for local deployments
- No PII
- Open-source tool and project names are fine as-is
