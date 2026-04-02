#!/usr/bin/env bash
set -euo pipefail

# DCM E2E Test Harness
# Orchestrates: deploy stack → resolve CLI → run Ginkgo tests → teardown.
# Delegates stack lifecycle to scripts/deploy-dcm.sh.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SCRIPT_DIR
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
readonly REPO_ROOT
readonly DEPLOY_SCRIPT="${REPO_ROOT}/scripts/deploy-dcm.sh"
readonly TEST_DIR="${SCRIPT_DIR}/e2e"
readonly CLI_BIN_DIR="${REPO_ROOT}/bin"
readonly CLI_GITHUB_REPO="dcm-project/cli"

# --- Usage ----------------------------------------------------------------- #

usage() {
    cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Run the DCM E2E test suite. By default, deploys the stack, runs all tests,
and tears down afterward.

Options:
  --skip-deploy                Skip stack deployment (assumes stack is running)
  --skip-teardown              Leave the stack running after tests
  --skip-cli                   Skip CLI binary resolution (CLI tests will be skipped)
  --dcm-cli-path PATH          Path to pre-built dcm binary (skips resolution)
  --gateway-url URL            Override DCM_GATEWAY_URL (default: http://localhost:9080/api/v1alpha1)
  --label-filter EXPR          Ginkgo label filter (e.g. "smoke", "cli")
  --junit-report FILE          Write JUnit XML report to FILE
  --help                       Show this help message

Deploy passthrough flags (forwarded to deploy-dcm.sh):
  --api-gateway-branch REF     Branch to clone
  --api-gateway-dir PATH       Directory to clone into
  --api-gateway-repo URL       Git repo for api-gateway
  --cleanup-on-failure         Tear down on deployment failure

CLI binary resolution order:
  1. --dcm-cli-path flag or DCM_CLI_PATH env var
  2. dcm in \$PATH
  3. Previously downloaded binary in bin/dcm
  4. Auto-download latest release from GitHub (requires gh CLI)

Examples:
  $(basename "$0")
  $(basename "$0") --skip-deploy
  $(basename "$0") --skip-deploy --label-filter smoke
  $(basename "$0") --dcm-cli-path ~/git/dcm/cli/bin/dcm
  $(basename "$0") --skip-cli --label-filter '!cli'
  $(basename "$0") --api-gateway-branch feature-x --skip-teardown
EOF
}

# --- Logging --------------------------------------------------------------- #

log()  { echo "==> $*"; }
info() { echo "    $*"; }
err()  { echo "ERROR: $*" >&2; }

# --- CLI binary resolution ------------------------------------------------- #

download_dcm_cli() {
    local detected_os detected_arch

    if ! command -v gh &>/dev/null; then
        err "gh CLI not found — cannot auto-download DCM CLI"
        err "Install gh (https://cli.github.com) or provide --dcm-cli-path"
        return 1
    fi

    detected_os="$(uname -s | tr '[:upper:]' '[:lower:]')"
    detected_arch="$(uname -m)"
    case "${detected_arch}" in
        x86_64)  detected_arch="amd64" ;;
        aarch64) detected_arch="arm64" ;;
    esac

    mkdir -p "${CLI_BIN_DIR}"
    log "Downloading latest DCM CLI for ${detected_os}/${detected_arch}"
    gh release download --repo "${CLI_GITHUB_REPO}" --pattern "cli_*_${detected_os}_${detected_arch}.tar.gz" --dir "${CLI_BIN_DIR}" --clobber
    tar -xzf "${CLI_BIN_DIR}"/cli_*_"${detected_os}"_"${detected_arch}".tar.gz -C "${CLI_BIN_DIR}" dcm
    rm -f "${CLI_BIN_DIR}"/cli_*_"${detected_os}"_"${detected_arch}".tar.gz
    chmod +x "${CLI_BIN_DIR}/dcm"
    info "Downloaded to ${CLI_BIN_DIR}/dcm"
}

resolve_dcm_cli() {
    # 1. Explicit path (flag or env var).
    if [[ -n "${DCM_CLI_PATH}" ]]; then
        if [[ ! -x "${DCM_CLI_PATH}" ]]; then
            err "DCM CLI not found or not executable: ${DCM_CLI_PATH}"
            return 1
        fi
        info "Using DCM CLI: ${DCM_CLI_PATH}"
        return 0
    fi

    # 2. Already in PATH.
    if command -v dcm &>/dev/null; then
        DCM_CLI_PATH="$(command -v dcm)"
        info "Found DCM CLI in PATH: ${DCM_CLI_PATH}"
        return 0
    fi

    # 3. Previously downloaded to bin/.
    if [[ -x "${CLI_BIN_DIR}/dcm" ]]; then
        DCM_CLI_PATH="${CLI_BIN_DIR}/dcm"
        info "Found DCM CLI in bin/: ${DCM_CLI_PATH}"
        return 0
    fi

    # 4. Auto-download from GitHub releases.
    log "DCM CLI not found — attempting download from GitHub"
    if download_dcm_cli; then
        DCM_CLI_PATH="${CLI_BIN_DIR}/dcm"
        return 0
    fi

    err "Could not resolve DCM CLI binary — CLI tests will be skipped"
    return 1
}

# --- Argument parsing ------------------------------------------------------ #

SKIP_DEPLOY=false
SKIP_TEARDOWN=false
SKIP_CLI=false
DCM_CLI_PATH="${DCM_CLI_PATH:-}"
GATEWAY_URL=""
LABEL_FILTER=""
JUNIT_REPORT=""
DEPLOY_ARGS=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        --skip-deploy)
            SKIP_DEPLOY=true
            shift ;;
        --skip-teardown)
            SKIP_TEARDOWN=true
            shift ;;
        --skip-cli)
            SKIP_CLI=true
            shift ;;
        --dcm-cli-path)
            DCM_CLI_PATH="$2"
            shift 2 ;;
        --gateway-url)
            GATEWAY_URL="$2"
            shift 2 ;;
        --label-filter)
            LABEL_FILTER="$2"
            shift 2 ;;
        --junit-report)
            JUNIT_REPORT="$2"
            shift 2 ;;
        --api-gateway-branch|--api-gateway-dir|--api-gateway-repo)
            DEPLOY_ARGS+=("$1" "$2")
            shift 2 ;;
        --cleanup-on-failure)
            DEPLOY_ARGS+=("$1")
            shift ;;
        --help)
            usage
            exit 0 ;;
        *)
            err "Unknown option: $1"
            usage
            exit 1 ;;
    esac
done

# --- Main ------------------------------------------------------------------ #

if ! command -v go &>/dev/null; then
    err "Go toolchain not found — install Go before running tests"
    exit 1
fi

# Deploy the stack.
if [[ "${SKIP_DEPLOY}" == "false" ]]; then
    log "Deploying DCM stack"
    "${DEPLOY_SCRIPT}" "${DEPLOY_ARGS[@]+"${DEPLOY_ARGS[@]}"}"
else
    log "Skipping deployment (--skip-deploy)"
fi

# Resolve CLI binary.
if [[ "${SKIP_CLI}" == "false" ]]; then
    if resolve_dcm_cli; then
        export DCM_CLI_PATH
        info "DCM_CLI_PATH=${DCM_CLI_PATH}"
    fi
else
    log "Skipping CLI resolution (--skip-cli)"
fi

# Export gateway URL if provided.
if [[ -n "${GATEWAY_URL}" ]]; then
    export DCM_GATEWAY_URL="${GATEWAY_URL}"
    info "DCM_GATEWAY_URL=${GATEWAY_URL}"
fi

# Build ginkgo arguments.
GINKGO_ARGS=(-r -v --tags=e2e)
if [[ -n "${LABEL_FILTER}" ]]; then
    GINKGO_ARGS+=(--label-filter="${LABEL_FILTER}")
fi
if [[ -n "${JUNIT_REPORT}" ]]; then
    GINKGO_ARGS+=(--junit-report="${JUNIT_REPORT}")
fi

# Run the tests, capturing the exit code.
log "Running E2E tests"
TEST_EXIT=0
(cd "${TEST_DIR}" && go run github.com/onsi/ginkgo/v2/ginkgo "${GINKGO_ARGS[@]}" .) || TEST_EXIT=$?

if [[ "${TEST_EXIT}" -eq 0 ]]; then
    log "Tests passed"
else
    err "Tests failed (exit code: ${TEST_EXIT})"
fi

# Teardown the stack.
if [[ "${SKIP_TEARDOWN}" == "false" ]]; then
    log "Tearing down DCM stack"
    if ! "${DEPLOY_SCRIPT}" --tear-down; then
        err "Teardown failed (non-fatal) — containers may still be running"
        err "Manual cleanup: ${DEPLOY_SCRIPT} --tear-down"
    fi
else
    log "Skipping teardown (--skip-teardown)"
fi

exit "${TEST_EXIT}"
