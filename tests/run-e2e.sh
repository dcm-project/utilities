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
  --auth-enabled               Enable RHBK/OIDC bearer authentication for API and CLI tests
  --auth-issuer-url URL        OIDC issuer URL (also accepted as --keycloak-url)
  --auth-target TARGET         Authentication target: rhdh or compose
  --auth-namespace NS          RHDH namespace used to discover auth settings
  --auth-ca-file PATH          CA bundle used to connect to RHBK
  --auth-advanced              Prepare and run TC-42 through TC-46
  --gateway-url URL            Override DCM_GATEWAY_URL (default: http://localhost:8080/api/v1alpha1)
  --label-filter EXPR          Ginkgo label filter (e.g. "smoke", "cli")
  --junit-report FILE          Write JUnit XML report to FILE
  --help                       Show this help message

Deploy passthrough flags (forwarded to deploy-dcm.sh):
  --control-plane-branch REF     Branch to clone
  --control-plane-dir PATH       Directory to clone into
  --control-plane-repo URL       Git repo for control-plane
  --cleanup-on-failure         Tear down on deployment failure
  --gitops                      Enable the dcm-gitops reconciliation container

Service provider flags (forwarded to deploy-dcm.sh):
  --all-service-providers           Enable all SPs
  --k8s-container-service-provider  Enable the k8s container SP
  --k8s-storage-service-provider    Enable the k8s storage SP
  --kubevirt-service-provider       Enable the kubevirt SP
  --acm-cluster-service-provider    Enable the ACM cluster SP
  --deploy-acm                      Deploy ACM on the cluster (opt-in, heavy)
  --deploy-mce                      Deploy MCE on the cluster (opt-in, heavy)
  --kubeconfig PATH                 Path to kubeconfig file
  --k8s-container-namespace NS      Namespace for container workloads
  --k8s-storage-namespace NS        Namespace for storage PVCs
  --acm-cluster-namespace NS        Namespace for ACM clusters
  --kubevirt-vm-namespace NS        Namespace for kubevirt VMs
  --cluster-api URL                 OpenShift API URL for oc login
  --cluster-username USER           Username for oc login
  --cluster-password PASS           Password for oc login

Environment variables:
  DCM_CONTAINER_SP_URL     Container SP direct URL (default: http://localhost:8082/api/v1alpha1)
  DCM_STORAGE_SP_URL       Storage SP direct URL (default: http://localhost:8089/api/v1alpha1)
  DCM_ACM_CLUSTER_SP_URL   ACM Cluster SP direct URL (default: http://localhost:8083/api/v1alpha1)
  DCM_KUBEVIRT_SP_URL      KubeVirt SP direct URL (default: http://localhost:8081/api/v1alpha1)
  DCM_NATS_URL             NATS URL for event tests (default: nats://localhost:4222)
  DCM_GATEWAY_URL          Control plane API URL (default: http://localhost:8080/api/v1alpha1)
  DCM_AUTH_ENABLED         Enable OIDC bearer authentication (default: false)
  DCM_AUTH_ISSUER_URL      OIDC issuer URL (required when authentication is enabled)
  DCM_AUTH_TOKEN_ISSUER_URL Optional token endpoint base URL when the issuer is only resolvable inside Compose
  DCM_AUTH_CLIENT_ID       OIDC client ID (default: dcm-proxy)
  DCM_AUTH_CLIENT_SECRET   OIDC client secret
  DCM_AUTH_USERNAME        OIDC user name for password-grant tokens
  DCM_AUTH_PASSWORD        OIDC password for password-grant tokens
  DCM_AUTH_TOKEN           Optional static bearer token (avoids password grant)
  DCM_AUTH_CA_FILE         Optional CA bundle for the OIDC issuer
  DCM_AUTH_RESTART_COMMAND Command used by TC-42 to restart the auth provider
  DCM_AUTH_PROXY_URL       RHDH DCM proxy URL for TC-43
  DCM_AUTH_PROXY_SESSION_TOKEN  Valid RHDH session token for TC-43
  DCM_AUTH_ADMIN_URL       RHBK realm admin API URL for TC-45
  DCM_AUTH_ADMIN_TOKEN     RHBK admin bearer token for TC-45
  DCM_AUTH_JWKS_URL        Host-reachable JWKS URL when discovery uses an internal Compose hostname
  DCM_AUTH_DCM_LOG_COMMAND  Command returning DCM logs to inspect for TC-46
  DCM_AUTH_RHDH_LOG_COMMAND Command returning RHDH logs to inspect for TC-46

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
  $(basename "$0") --control-plane-branch feature-x --skip-teardown
  $(basename "$0") --k8s-container-service-provider --cluster-api https://api.example.com:6443
  $(basename "$0") --skip-deploy --label-filter "sp && container"
EOF
}

# --- Logging --------------------------------------------------------------- #

log()  { echo "==> $*"; }
info() { echo "    $*"; }
err()  { echo "ERROR: $*" >&2; }

# --- CLI binary resolution ------------------------------------------------- #

download_dcm_cli() {
    local version="${1:-main}"
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
    log "Downloading DCM CLI (${version}) for ${detected_os}/${detected_arch}"
    gh release download "${version}" --repo "${CLI_GITHUB_REPO}" --pattern "cli_*_${detected_os}_${detected_arch}.tar.gz" --dir "${CLI_BIN_DIR}" --clobber
    tar -xzf "${CLI_BIN_DIR}"/cli_*_"${detected_os}"_"${detected_arch}".tar.gz -C "${CLI_BIN_DIR}" dcm
    rm -f "${CLI_BIN_DIR}"/cli_*_"${detected_os}"_"${detected_arch}".tar.gz
    chmod +x "${CLI_BIN_DIR}/dcm"
    info "Downloaded to ${CLI_BIN_DIR}/dcm"
}

log_cli_version() {
    local cli_version_output
    cli_version_output="$("${DCM_CLI_PATH}" version 2>&1)" || return 0
    local ver commit
    ver="$(echo "${cli_version_output}" | awk '/^dcm version/{sub(/^dcm version /,""); print}')"
    commit="$(echo "${cli_version_output}" | awk '/commit:/{sub(/^ *commit: */,""); print}')"
    info "DCM CLI ${ver} (commit ${commit})"
}

resolve_dcm_cli() {
    # 1. Explicit path (flag or env var).
    if [[ -n "${DCM_CLI_PATH}" ]]; then
        if [[ ! -x "${DCM_CLI_PATH}" ]]; then
            err "DCM CLI not found or not executable: ${DCM_CLI_PATH}"
            return 1
        fi
        info "Using DCM CLI: ${DCM_CLI_PATH}"
        log_cli_version
        return 0
    fi

    # 2. Remove any stale binary, then download fresh.
    rm -f "${CLI_BIN_DIR}/dcm"
    if download_dcm_cli "${CLI_VERSION}"; then
        DCM_CLI_PATH="${CLI_BIN_DIR}/dcm"
        log_cli_version
        return 0
    fi

    err "Could not download DCM CLI — CLI tests will be skipped"
    return 1
}

# --- Argument parsing ------------------------------------------------------ #

SKIP_DEPLOY=false
SKIP_TEARDOWN=false
SKIP_CLI=false
DCM_CLI_PATH="${DCM_CLI_PATH:-}"
CLI_VERSION="${CLI_VERSION:-main}"
GATEWAY_URL=""
AUTH_ENABLED="${DCM_AUTH_ENABLED:-false}"
AUTH_ISSUER_URL="${DCM_AUTH_ISSUER_URL:-}"
AUTH_TARGET="${DCM_AUTH_TARGET:-}"
AUTH_NAMESPACE="${DCM_AUTH_NAMESPACE:-${RHDH_NAMESPACE:-rhdh-operator}}"
AUTH_CA_FILE="${DCM_AUTH_CA_FILE:-}"
AUTH_ADVANCED="${DCM_AUTH_ADVANCED:-false}"
CONTROL_PLANE_DIR="${CONTROL_PLANE_TMP_DIR:-/tmp/dcm-e2e}"
LABEL_FILTER=""
JUNIT_REPORT=""
DEPLOY_ARGS=()
ENABLE_CONTAINER_SP=false
ENABLE_ACM_CLUSTER_SP=false
ENABLE_KUBEVIRT_SP=false
KUBEVIRT_VM_NS_ARG=""

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
        --auth-enabled)
            AUTH_ENABLED=true
            shift ;;
        --auth-issuer-url|--keycloak-url)
            AUTH_ENABLED=true
            AUTH_ISSUER_URL="$2"
            shift 2 ;;
        --auth-target)
            AUTH_TARGET="$2"
            shift 2 ;;
        --auth-namespace)
            AUTH_NAMESPACE="$2"
            shift 2 ;;
        --auth-ca-file)
            AUTH_CA_FILE="$2"
            shift 2 ;;
        --auth-advanced)
            AUTH_ADVANCED=true
            shift ;;
        --cli-version)
            CLI_VERSION="$2"
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
        --control-plane-dir)
            CONTROL_PLANE_DIR="$2"
            DEPLOY_ARGS+=("$1" "$2")
            shift 2 ;;
        --control-plane-branch|--control-plane-repo)
            DEPLOY_ARGS+=("$1" "$2")
            shift 2 ;;
        --cleanup-on-failure)
            DEPLOY_ARGS+=("$1")
            shift ;;
        --gitops)
            DEPLOY_ARGS+=("$1")
            shift ;;
        --k8s-container-service-provider)
            ENABLE_CONTAINER_SP=true
            DEPLOY_ARGS+=("$1")
            shift ;;
        --all-service-providers)
            ENABLE_CONTAINER_SP=true
            ENABLE_ACM_CLUSTER_SP=true
            ENABLE_KUBEVIRT_SP=true
            DEPLOY_ARGS+=("$1")
            shift ;;
        --acm-cluster-service-provider)
            ENABLE_ACM_CLUSTER_SP=true
            DEPLOY_ARGS+=("$1")
            shift ;;
        --kubevirt-service-provider)
            ENABLE_KUBEVIRT_SP=true
            DEPLOY_ARGS+=("$1")
            shift ;;
        --deploy-acm|--deploy-mce)
            DEPLOY_ARGS+=("$1")
            shift ;;
        --compose-file|--kubeconfig|--k8s-container-namespace|--acm-cluster-namespace|--cluster-api|--cluster-username|--cluster-password|--acm-cluster-sp-repo|--acm-cluster-sp-branch)
            DEPLOY_ARGS+=("$1" "$2")
            shift 2 ;;
        --kubevirt-vm-namespace)
            KUBEVIRT_VM_NS_ARG="$2"
            DEPLOY_ARGS+=("$1" "$2")
            shift 2 ;;
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

prepare_advanced_auth() {
    local issuer_base realm route_host user_response refresh_token admin_password
    local admin_token
    local -a ca_args=()

    [[ "${AUTH_ADVANCED}" == true ]] || return 0
    command -v curl >/dev/null || { err "curl is required for advanced auth tests"; return 1; }
    command -v jq >/dev/null || { err "jq is required for advanced auth tests"; return 1; }
    if [[ -n "${AUTH_CA_FILE}" ]]; then
        ca_args=(--cacert "${AUTH_CA_FILE}")
    fi

    if [[ "${AUTH_TARGET}" == rhdh ]]; then
        command -v oc >/dev/null || { err "oc is required for --auth-target rhdh"; return 1; }
        route_host="$(oc -n "${AUTH_NAMESPACE}" get route -o jsonpath='{.items[0].spec.host}')"
        issuer_base="${AUTH_ISSUER_URL%/realms/*}"
        realm="${AUTH_ISSUER_URL##*/realms/}"
        user_response="$(curl --fail --silent --show-error \
            "${ca_args[@]}" \
            -X POST "${AUTH_ISSUER_URL}/protocol/openid-connect/token" \
            -d grant_type=password -d client_id="${DCM_AUTH_CLIENT_ID}" \
            --data-urlencode client_secret="${DCM_AUTH_CLIENT_SECRET}" \
            -d username="${DCM_AUTH_USERNAME}" -d password="${DCM_AUTH_PASSWORD}" -d scope=openid)"
        refresh_token="$(printf '%s' "${user_response}" | jq -r .refresh_token)"
        DCM_AUTH_PROXY_SESSION_TOKEN="$(curl --fail --silent --show-error \
            "${ca_args[@]}" \
            "https://${route_host}/api/auth/oidc/refresh?optional&scope=openid%20profile%20email&env=production" \
            -H 'x-requested-with: XMLHttpRequest' \
            --cookie "oidc-refresh-token=${refresh_token}" | jq -r .backstageIdentity.token)"
        admin_password="$(oc -n "${RHBK_NAMESPACE:-rhbk}" get secret rhbk-admin -o jsonpath='{.data.password}' | base64 -d)"
        admin_token="$(curl --fail --silent --show-error "${ca_args[@]}" \
            -X POST "${issuer_base}/realms/master/protocol/openid-connect/token" \
            -d grant_type=password -d client_id=admin-cli -d username=admin \
            --data-urlencode password="${admin_password}" | jq -r .access_token)"
        export DCM_AUTH_PROXY_URL="https://${route_host}/api/dcm/proxy"
        export DCM_AUTH_ADMIN_URL="${issuer_base}/admin/realms/${realm}"
        export DCM_AUTH_ADMIN_TOKEN="${admin_token}"
        export DCM_AUTH_RESTART_COMMAND="oc -n ${RHBK_NAMESPACE:-rhbk} rollout restart statefulset/rhbk"
        export DCM_AUTH_DCM_LOG_COMMAND="podman logs dcm-e2e_control-plane_1"
        export DCM_AUTH_RHDH_LOG_COMMAND="oc -n ${AUTH_NAMESPACE} logs -l app.kubernetes.io/name=backstage --all-containers=true"
        export DCM_AUTH_PROXY_SESSION_TOKEN
        return 0
    fi

    if [[ "${AUTH_TARGET}" == compose ]]; then
        issuer_base="${DCM_AUTH_TOKEN_ISSUER_URL:-${AUTH_ISSUER_URL}}"
        export DCM_AUTH_JWKS_URL="${DCM_AUTH_JWKS_URL:-${issuer_base}/protocol/openid-connect/certs}"
        issuer_base="${issuer_base%/realms/*}"
        export DCM_AUTH_ADMIN_URL="${DCM_AUTH_ADMIN_URL:-${issuer_base}/admin/realms/${AUTH_REALM:-dcm}}"
        if [[ -z "${DCM_AUTH_ADMIN_TOKEN:-}" && -n "${DCM_AUTH_ADMIN_USERNAME:-}" && -n "${DCM_AUTH_ADMIN_PASSWORD:-}" ]]; then
            admin_token="$(curl --fail --silent --show-error \
                -X POST "${issuer_base}/realms/master/protocol/openid-connect/token" \
                -d grant_type=password -d client_id=admin-cli \
                --data-urlencode username="${DCM_AUTH_ADMIN_USERNAME}" \
                --data-urlencode password="${DCM_AUTH_ADMIN_PASSWORD}" | jq -r .access_token)"
            export DCM_AUTH_ADMIN_TOKEN="${admin_token}"
        fi
        export DCM_AUTH_RESTART_COMMAND="${DCM_AUTH_RESTART_COMMAND:-podman restart dcm-e2e_keycloak_1}"
        export DCM_AUTH_DCM_LOG_COMMAND="${DCM_AUTH_DCM_LOG_COMMAND:-podman logs dcm-e2e_control-plane_1}"
        return 0
    fi

    err "--auth-advanced requires --auth-target rhdh or compose"
    return 1
}

read_deploy_env_value() {
    local key="$1" env_file="$2"
    sed -n "s/^${key}=//p" "${env_file}" | tail -n 1
}

prepare_compose_auth() {
    local env_file="${CONTROL_PLANE_DIR}/deploy/.env"

    [[ "${AUTH_ENABLED}" == true && "${AUTH_TARGET}" == compose ]] || return 0
    if [[ ! -f "${env_file}" ]]; then
        err "Compose auth environment not found: ${env_file}"
        return 1
    fi

    export DCM_AUTH_CLIENT_ID="${DCM_AUTH_CLIENT_ID:-dcm-proxy}"
    export DCM_AUTH_CLIENT_SECRET="${DCM_AUTH_CLIENT_SECRET:-$(read_deploy_env_value AUTH_PROXY_SECRET "${env_file}")}"
    export DCM_AUTH_USERNAME="${DCM_AUTH_USERNAME:-dcm-admin}"
    export DCM_AUTH_PASSWORD="${DCM_AUTH_PASSWORD:-$(read_deploy_env_value DCM_DEV_USER_PASSWORD "${env_file}")}"
    export DCM_AUTH_ADMIN_USERNAME="${DCM_AUTH_ADMIN_USERNAME:-$(read_deploy_env_value KEYCLOAK_ADMIN "${env_file}")}"
    export DCM_AUTH_ADMIN_PASSWORD="${DCM_AUTH_ADMIN_PASSWORD:-$(read_deploy_env_value KEYCLOAK_ADMIN_PASSWORD "${env_file}")}"
}

if ! command -v go &>/dev/null; then
    err "Go toolchain not found — install Go before running tests"
    exit 1
fi

if [[ "${AUTH_ENABLED}" == "true" ]]; then
    if [[ "${AUTH_TARGET}" == rhdh && -z "${AUTH_ISSUER_URL}" ]]; then
        AUTH_ISSUER_URL="$(oc -n "${AUTH_NAMESPACE}" get configmap rhbk-dcm-auth -o jsonpath='{.data.issuer-url}')"
    fi
    if [[ -z "${AUTH_ISSUER_URL}" ]]; then
        err "--auth-enabled requires --auth-issuer-url or DCM_AUTH_ISSUER_URL"
        exit 1
    fi
    if [[ "${AUTH_TARGET}" == rhdh ]]; then
        export DCM_AUTH_CLIENT_ID="${DCM_AUTH_CLIENT_ID:-rhdh-auth}"
        DCM_AUTH_CLIENT_SECRET="$(oc -n "${AUTH_NAMESPACE}" get secret rhdh-auth-secrets -o jsonpath='{.data.KEYCLOAK_CLIENT_SECRET}' | base64 -d)"
        export DCM_AUTH_CLIENT_SECRET
        export DCM_AUTH_USERNAME="${DCM_AUTH_USERNAME:-testuser1}"
        export DCM_AUTH_PASSWORD="${DCM_AUTH_PASSWORD:-testuser1}"
    fi
    if [[ -n "${AUTH_CA_FILE}" ]]; then
        export DCM_AUTH_CA_FILE="${AUTH_CA_FILE}"
    fi
    export DCM_AUTH_ENABLED=true
    export DCM_AUTH_ISSUER_URL="${AUTH_ISSUER_URL}"
    export DCM_AUTH_AUDIENCE="${DCM_AUTH_AUDIENCE:-dcm-api}"
    export AUTH_ISSUER_URL="${AUTH_ISSUER_URL}"
    DEPLOY_ARGS+=(--auth-enabled)
    info "DCM authentication enabled for E2E requests"
else
    export DCM_AUTH_ENABLED=false
    info "DCM authentication disabled for E2E requests"
fi

# Deploy the stack.
if [[ "${SKIP_DEPLOY}" == "false" ]]; then
    log "Deploying DCM stack"
    "${DEPLOY_SCRIPT}" "${DEPLOY_ARGS[@]+"${DEPLOY_ARGS[@]}"}"
else
    log "Skipping deployment (--skip-deploy)"
fi

prepare_compose_auth

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

# Export SP URLs when providers are enabled.
if [[ "${ENABLE_CONTAINER_SP}" == "true" ]] || [[ "${ENABLE_ACM_CLUSTER_SP}" == "true" ]]; then
    export DCM_NATS_URL="${DCM_NATS_URL:-nats://localhost:4222}"
    info "DCM_NATS_URL=${DCM_NATS_URL}"
fi
if [[ "${ENABLE_CONTAINER_SP}" == "true" ]]; then
    export DCM_CONTAINER_SP_URL="${DCM_CONTAINER_SP_URL:-http://localhost:8082/api/v1alpha1}"
    info "DCM_CONTAINER_SP_URL=${DCM_CONTAINER_SP_URL}"
fi
if [[ "${ENABLE_ACM_CLUSTER_SP}" == "true" ]]; then
    export DCM_ACM_CLUSTER_SP_URL="${DCM_ACM_CLUSTER_SP_URL:-http://localhost:8083/api/v1alpha1}"
    info "DCM_ACM_CLUSTER_SP_URL=${DCM_ACM_CLUSTER_SP_URL}"
fi
if [[ "${ENABLE_KUBEVIRT_SP}" == "true" ]]; then
    export DCM_KUBEVIRT_SP_URL="${DCM_KUBEVIRT_SP_URL:-http://localhost:8081/api/v1alpha1}"
    info "DCM_KUBEVIRT_SP_URL=${DCM_KUBEVIRT_SP_URL}"
    # Keep Ginkgo cluster lookups in the same NS the SP uses (compose KUBERNETES_NAMESPACE).
    if [[ -z "${KUBERNETES_NAMESPACE:-}" ]]; then
        export KUBERNETES_NAMESPACE="${KUBEVIRT_VM_NS_ARG:-${KUBEVIRT_VM_NAMESPACE:-vms}}"
    fi
    export KUBEVIRT_VM_NAMESPACE="${KUBEVIRT_VM_NAMESPACE:-${KUBERNETES_NAMESPACE}}"
    info "KUBERNETES_NAMESPACE=${KUBERNETES_NAMESPACE}"
fi

run_ginkgo_suite() {
    local label_filter="$1" report_file="$2"
    local -a arguments=(-r -v --tags=e2e)
    if [[ -n "${label_filter}" ]]; then
        arguments+=(--label-filter="${label_filter}")
    fi
    if [[ -n "${report_file}" ]]; then
        arguments+=(--junit-report="${report_file}")
    fi
    (cd "${TEST_DIR}" && go run github.com/onsi/ginkgo/v2/ginkgo "${arguments[@]}" .)
}

# Run the regular suite first. Advanced auth tests run as a final ordered phase
# so restart, key rotation, and log inspection cannot disrupt unrelated specs.
log "Running E2E tests"
TEST_EXIT=0
REGULAR_FILTER="${LABEL_FILTER}"
if [[ "${AUTH_ADVANCED}" == true ]]; then
    if [[ -n "${REGULAR_FILTER}" ]]; then
        REGULAR_FILTER="(${REGULAR_FILTER}) && !advanced-auth"
    else
        REGULAR_FILTER="!advanced-auth"
    fi
fi
run_ginkgo_suite "${REGULAR_FILTER}" "${JUNIT_REPORT}" || TEST_EXIT=$?

if [[ "${AUTH_ADVANCED}" == true ]]; then
    log "Running advanced authentication tests"
    ADVANCED_EXIT=0
    ADVANCED_REPORT=""
    if [[ -n "${JUNIT_REPORT}" ]]; then
        ADVANCED_REPORT="${JUNIT_REPORT%.xml}-auth-advanced.xml"
        info "Advanced auth JUnit report: ${ADVANCED_REPORT}"
    fi
    prepare_advanced_auth || ADVANCED_EXIT=$?
    if [[ "${ADVANCED_EXIT}" -eq 0 ]]; then
        run_ginkgo_suite "advanced-auth" "${ADVANCED_REPORT}" || ADVANCED_EXIT=$?
    fi
    if [[ "${ADVANCED_EXIT}" -ne 0 ]]; then
        TEST_EXIT="${ADVANCED_EXIT}"
    fi
fi

if [[ "${TEST_EXIT}" -eq 0 ]]; then
    log "Tests passed"
else
    err "Tests failed (exit code: ${TEST_EXIT})"
fi

# Teardown the stack.
if [[ "${SKIP_TEARDOWN}" == "false" ]]; then
    log "Tearing down DCM stack"
    if ! "${DEPLOY_SCRIPT}" --tear-down "${DEPLOY_ARGS[@]+"${DEPLOY_ARGS[@]}"}"; then
        err "Teardown failed (non-fatal) — containers may still be running"
        err "Manual cleanup: ${DEPLOY_SCRIPT} --tear-down ${DEPLOY_ARGS[*]}"
    fi
else
    log "Skipping teardown (--skip-teardown)"
fi

exit "${TEST_EXIT}"
