#!/usr/bin/env bash
set -euo pipefail

# DCM E2E Deploy Script
# Clones the api-gateway repo, brings up the full DCM stack via podman-compose,
# and verifies all services are healthy.

readonly DEFAULT_API_GATEWAY_REPO="https://github.com/dcm-project/api-gateway.git"
readonly DEFAULT_API_GATEWAY_BRANCH="main"
readonly DEFAULT_API_GATEWAY_TMP_DIR="/tmp/dcm-e2e"
readonly GATEWAY_PORT="9080"
readonly HEALTH_TIMEOUT_SECONDS=90
readonly HEALTH_POLL_INTERVAL=5

readonly HEALTH_ENDPOINTS=(
    "/api/v1alpha1/health/providers"
    "/api/v1alpha1/health/catalog"
    "/api/v1alpha1/health/policies"
    "/api/v1alpha1/health/placement"
)

# Supported providers and their compose profile names
readonly PROVIDER_KUBEVIRT="kubevirt"

# --- Usage ----------------------------------------------------------------- #

usage() {
    cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Deploy the full DCM stack for E2E testing. The api-gateway repo contains the
compose.yaml that orchestrates all DCM services (managers, gateway, infra).

Options:
  --api-gateway-repo URL         Git repo for api-gateway (default: ${DEFAULT_API_GATEWAY_REPO})
  --api-gateway-branch REF       Branch to clone (default: ${DEFAULT_API_GATEWAY_BRANCH})
  --api-gateway-dir PATH         Directory to clone api-gateway into (default: ${DEFAULT_API_GATEWAY_TMP_DIR})
  --all-service-providers        Enable all available service providers
  --kubevirt-service-provider    Enable the kubevirt service provider
  --kubeconfig PATH              Path to kubeconfig file (required for --kubevirt-service-provider)
  --kubevirt-vm-namespace NS     Kubevirt namespace for VMs (default: vms)
  --cleanup-on-failure           Tear down the stack automatically if deployment fails (default: leave for debugging)
  --running-versions             Print versions of all running containers and write dcm-versions.json
  --tear-down                    Stop the stack, remove volumes, and clean the deploy directory
  --help                         Show this help message

Environment variables (flags take precedence):
  API_GATEWAY_REPO          Same as --api-gateway-repo
  API_GATEWAY_BRANCH        Same as --api-gateway-branch
  API_GATEWAY_TMP_DIR       Same as --api-gateway-dir
  KUBECONFIG                Same as --kubeconfig
  KUBEVIRT_VM_NAMESPACE     Same as --kubevirt-vm-namespace (default: vms)

Examples:
  $(basename "$0")
  $(basename "$0") --api-gateway-branch feature-x
  $(basename "$0") --api-gateway-repo https://github.com/myfork/api-gateway.git
  $(basename "$0") --kubevirt-service-provider
  $(basename "$0") --all-service-providers
  $(basename "$0") --tear-down
  $(basename "$0") --running-versions
EOF
}

# --- Logging --------------------------------------------------------------- #

log()  { echo "==> $*"; }
info() { echo "    $*"; }
err()  { echo "ERROR: $*" >&2; }

# --- Prerequisite helpers -------------------------------------------------- #

# Guard against catastrophic rm -rf on system paths
validate_deploy_dir() {
    local dir="$1"

    case "${dir}" in
        /|/bin|/boot|/dev|/etc|/home|/lib*|/opt|/proc|/root|/run|/sbin|/srv|/sys|/tmp|/usr|/var)
            err "Refusing to use system path as deploy directory: ${dir}"
            return 1 ;;
    esac

    if [[ "${dir}" == "/" ]] || [[ -z "${dir}" ]]; then
        err "Deploy directory path is empty or root — aborting"
        return 1
    fi
}

check_required_tools() {
    local missing=()
    for tool in "$@"; do
        if ! command -v "${tool}" &>/dev/null; then
            missing+=("${tool}")
        fi
    done

    if [[ ${#missing[@]} -gt 0 ]]; then
        err "Missing required tools: ${missing[*]}"
        err "Install them before running this script."
        return 1
    fi
}

# --- Tear-down ------------------------------------------------------------- #

tear_down() {
    local deploy_dir="$1"
    shift
    local compose_profiles=("$@")

    log "Tearing down DCM stack"

    if [[ -d "${deploy_dir}" ]]; then
        info "Stopping containers and removing volumes..."
        # Use all profiles so every service (including optional ones) is stopped
        podman-compose -f "${deploy_dir}/compose.yaml" ${compose_profiles[@]+"${compose_profiles[@]}"} down -v 2>/dev/null || true

        # podman-compose down can leave orphans when containers have dependency
        # chains (e.g. healthcheck depends_on). Force-remove any stragglers.
        local project_name
        project_name=$(basename "${deploy_dir}")
        local remaining
        remaining=$(podman ps -a --filter "name=${project_name}_" --format '{{.ID}}' 2>/dev/null || true)
        if [[ -n "${remaining}" ]]; then
            info "Force-removing remaining containers..."
            echo "${remaining}" | xargs -r podman rm -f 2>/dev/null || true
        fi

        # Clean up pod and network that compose may have created
        podman pod ls --filter "name=${project_name}" --format '{{.ID}}' 2>/dev/null | xargs -r podman pod rm -f 2>/dev/null || true
        podman network rm -f "${project_name}_default" 2>/dev/null || true

        info "Removing deploy directory: ${deploy_dir}"
        rm -rf "${deploy_dir}"
    fi

    log "Tear-down complete"
}

# --- Provider validation -------------------------------------------------- #

validate_kubevirt_provider() {
    local kubeconfig="$1"
    local vm_namespace="$2"

    log "Validating kubevirt provider prerequisites"

    if [[ -z "${kubeconfig}" ]]; then
        err "KUBECONFIG or --kubeconfig is required when kubevirt provider is enabled"
        return 1
    fi

    if [[ ! -f "${kubeconfig}" ]]; then
        err "Kubeconfig file not found: ${kubeconfig}"
        return 1
    fi

    info "Verifying cluster connectivity..."
    if ! oc --kubeconfig="${kubeconfig}" cluster-info &>/dev/null; then
        err "Cannot connect to cluster using kubeconfig: ${kubeconfig}"
        err "Verify the kubeconfig is valid and the cluster is reachable"
        return 1
    fi
    info "Cluster is reachable"

    info "Checking for OpenShift Virtualization (kubevirt.io CRDs)..."
    if ! oc --kubeconfig="${kubeconfig}" get crd virtualmachines.kubevirt.io &>/dev/null; then
        err "kubevirt.io CRDs not found — OpenShift Virtualization (CNV) must be installed"
        return 1
    fi
    info "OpenShift Virtualization is installed"

    info "Ensuring namespace '${vm_namespace}' exists..."
    if ! oc --kubeconfig="${kubeconfig}" get namespace "${vm_namespace}" &>/dev/null; then
        info "Creating namespace '${vm_namespace}'..."
        oc --kubeconfig="${kubeconfig}" create namespace "${vm_namespace}"
    fi
    info "Namespace '${vm_namespace}' is ready"
}

# --- Health verification --------------------------------------------------- #

verify_health() {
    local compose_file="$1"
    shift
    local compose_profiles=("$@")

    log "Verifying service health"

    # Confirm all compose services have running containers
    info "Checking container readiness..."
    local expected_services running_services
    expected_services=$(podman-compose -f "${compose_file}" ${compose_profiles[@]+"${compose_profiles[@]}"} config --services 2>/dev/null | sort)
    # podman-compose ps doesn't support --format {{.Service}}, so extract service
    # names from the NAMES column (format: <project>_<service>_<instance>)
    running_services=$(podman-compose -f "${compose_file}" ${compose_profiles[@]+"${compose_profiles[@]}"} ps 2>/dev/null | awk 'NR>1 {print $NF}' | sed 's/.*_\(.*\)_[0-9]*/\1/' | sort)

    local container_failures=()
    while IFS= read -r service; do
        [[ -z "${service}" ]] && continue
        if ! echo "${running_services}" | grep -qx "${service}"; then
            container_failures+=("${service}")
        fi
    done <<< "${expected_services}"

    if [[ ${#container_failures[@]} -gt 0 ]]; then
        err "The following services are not running: ${container_failures[*]}"
        err "Check logs with: podman-compose -f ${compose_file} logs <service>"
        return 1
    fi
    info "All containers running"

    # Poll gateway health endpoints
    info "Polling health endpoints (timeout: ${HEALTH_TIMEOUT_SECONDS}s)..."

    local gateway_url="http://localhost:${GATEWAY_PORT}"
    local health_failures=()

    for endpoint in "${HEALTH_ENDPOINTS[@]}"; do
        local healthy=false
        local attempt_elapsed=0

        while [[ ${attempt_elapsed} -lt ${HEALTH_TIMEOUT_SECONDS} ]]; do
            local http_code
            http_code=$(curl -s --connect-timeout 5 --max-time 10 -o /dev/null -w "%{http_code}" "${gateway_url}${endpoint}" 2>/dev/null || echo "000")
            if [[ "${http_code}" =~ ^2[0-9]{2}$ ]]; then
                healthy=true
                break
            fi
            sleep "${HEALTH_POLL_INTERVAL}"
            attempt_elapsed=$((attempt_elapsed + HEALTH_POLL_INTERVAL))
        done

        if [[ "${healthy}" == true ]]; then
            info "  PASS  ${endpoint}"
        else
            info "  FAIL  ${endpoint} (last HTTP ${http_code})"
            health_failures+=("${endpoint}")
        fi
    done

    echo
    if [[ ${#health_failures[@]} -gt 0 ]]; then
        err "Health check failed for: ${health_failures[*]}"
        err "Check logs with: podman-compose -f ${compose_file} logs"
        return 1
    fi
}

# --- Running versions ------------------------------------------------------ #

# Queries the Quay.io API to find the git commit SHA that produced an image.
# Matches the image's manifest digest against tags named "sha-<commit>".
resolve_git_sha() {
    local repo_name="$1"
    local image_digest="$2"

    local api_url="https://quay.io/api/v1/repository/dcm-project/${repo_name}/tag/?onlyActiveTags=true&limit=100"
    local api_response
    api_response=$(curl -s --connect-timeout 5 --max-time 10 "${api_url}" 2>/dev/null || echo "")

    if [[ -z "${api_response}" ]]; then
        info "  WARN  Quay API unreachable for ${repo_name}"
        return 1
    fi

    local matched_sha
    matched_sha=$(echo "${api_response}" | jq -r --arg digest "${image_digest}" '.tags[] | select(.manifest_digest == $digest) | .name' 2>/dev/null | grep -E '^sha-[a-f0-9]+$' | head -1)

    if [[ -z "${matched_sha}" ]]; then
        info "  WARN  Could not resolve git SHA for ${repo_name} (digest: ${image_digest:0:19}...)"
        return 1
    fi

    echo "${matched_sha#sha-}"
}

get_running_versions() {
    local compose_file="$1"
    shift
    local compose_profiles=("$@")

    if [[ ! -f "${compose_file}" ]]; then
        err "Compose file not found: ${compose_file}"
        err "Is the DCM stack deployed? Use --api-gateway-dir to specify the deploy directory."
        return 1
    fi

    log "Collecting running container versions"

    local container_ids
    container_ids=$(podman-compose -f "${compose_file}" ${compose_profiles[@]+"${compose_profiles[@]}"} ps -q 2>/dev/null)

    if [[ -z "${container_ids}" ]]; then
        err "No running containers found"
        return 1
    fi

    local entries=()

    while IFS= read -r container_id; do
        [[ -z "${container_id}" ]] && continue

        local image_name image_digest
        read -r image_name image_digest < <(podman inspect --format '{{.ImageName}} {{.ImageDigest}}' "${container_id}" 2>/dev/null || echo "unknown unknown")

        local git_sha="null"
        if [[ "${image_name}" == quay.io/dcm-project/* ]]; then
            local repo_name resolved_sha
            repo_name="${image_name#quay.io/dcm-project/}"
            repo_name="${repo_name%%:*}"

            if resolved_sha=$(resolve_git_sha "${repo_name}" "${image_digest}"); then
                git_sha="\"${resolved_sha}\""
            fi
        fi

        entries+=("$(jq -n --arg image "${image_name}" --arg digest "${image_digest}" --argjson git_sha "${git_sha}" '{($image): {image_digest: $digest, git_sha: $git_sha}}')")
    done <<< "${container_ids}"

    local output_file="${PWD}/dcm-versions.json"

    echo
    log "Container versions"
    printf '%s\n' "${entries[@]}" | jq -s 'add' | tee "${output_file}"
    echo
    info "Versions written to ${output_file}"
}

# --- Argument parsing ------------------------------------------------------ #

API_GATEWAY_REPO="${API_GATEWAY_REPO:-${DEFAULT_API_GATEWAY_REPO}}"
API_GATEWAY_BRANCH="${API_GATEWAY_BRANCH:-${DEFAULT_API_GATEWAY_BRANCH}}"
API_GATEWAY_TMP_DIR="${API_GATEWAY_TMP_DIR:-${DEFAULT_API_GATEWAY_TMP_DIR}}"
TEAR_DOWN=false
RUNNING_VERSIONS=false
CLEANUP_ON_FAILURE=false
ENABLE_KUBEVIRT=false
DCM_KUBECONFIG="${KUBECONFIG:-}"
DCM_VM_NAMESPACE="${KUBEVIRT_VM_NAMESPACE:-vms}"

require_arg() {
    if [[ -z "${2:-}" ]] || [[ "$2" == --* ]]; then
        err "Option $1 requires a value"
        usage; exit 1
    fi
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --api-gateway-repo)
            require_arg "$1" "${2:-}"
            API_GATEWAY_REPO="${2:-}"; shift 2 ;;
        --api-gateway-branch)
            require_arg "$1" "${2:-}"
            API_GATEWAY_BRANCH="${2:-}"; shift 2 ;;
        --api-gateway-dir)
            require_arg "$1" "${2:-}"
            API_GATEWAY_TMP_DIR="${2:-}"; shift 2 ;;
        --all-service-providers)
            ENABLE_KUBEVIRT=true; shift ;;
        --kubevirt-service-provider)
            ENABLE_KUBEVIRT=true; shift ;;
        --kubeconfig)
            require_arg "$1" "${2:-}"
            DCM_KUBECONFIG="${2:-}"; shift 2 ;;
        --kubevirt-vm-namespace)
            require_arg "$1" "${2:-}"
            DCM_VM_NAMESPACE="${2:-}"; shift 2 ;;
        --cleanup-on-failure)
            CLEANUP_ON_FAILURE=true; shift ;;
        --running-versions)
            RUNNING_VERSIONS=true; shift ;;
        --tear-down)
            TEAR_DOWN=true; shift ;;
        --help)
            usage; exit 0 ;;
        *)
            err "Unknown option: $1"
            usage; exit 1 ;;
    esac
done

validate_deploy_dir "${API_GATEWAY_TMP_DIR}" || exit 1

# Build compose profile args based on enabled providers
COMPOSE_PROFILES=()
if [[ "${ENABLE_KUBEVIRT}" == true ]]; then
    COMPOSE_PROFILES+=("--profile" "${PROVIDER_KUBEVIRT}")
fi

# --- Running versions (standalone) ----------------------------------------- #

if [[ "${RUNNING_VERSIONS}" == true ]]; then
    check_required_tools podman podman-compose curl jq || exit 1
    get_running_versions "${API_GATEWAY_TMP_DIR}/compose.yaml" ${COMPOSE_PROFILES[@]+"${COMPOSE_PROFILES[@]}"} || exit 1
    exit 0
fi

if [[ "${TEAR_DOWN}" == true ]]; then
    tear_down "${API_GATEWAY_TMP_DIR}" ${COMPOSE_PROFILES[@]+"${COMPOSE_PROFILES[@]}"}
    exit 0
fi

# --- Prerequisite validation ----------------------------------------------- #

log "Checking prerequisites"

REQUIRED_TOOLS=(git podman podman-compose curl jq)
if [[ "${ENABLE_KUBEVIRT}" == true ]]; then
    REQUIRED_TOOLS+=(oc)
fi

check_required_tools "${REQUIRED_TOOLS[@]}" || exit 1
info "All prerequisites found: ${REQUIRED_TOOLS[*]}"

if [[ "${ENABLE_KUBEVIRT}" == true ]]; then
    validate_kubevirt_provider "${DCM_KUBECONFIG}" "${DCM_VM_NAMESPACE}" || exit 1

    # Export as KUBERNETES_* for compose.yaml substitution
    export KUBERNETES_KUBECONFIG="${DCM_KUBECONFIG}"
    export KUBERNETES_NAMESPACE="${DCM_VM_NAMESPACE}"
fi

# --- Clone ----------------------------------------------------------------- #

log "Preparing deploy directory: ${API_GATEWAY_TMP_DIR}"

if [[ -d "${API_GATEWAY_TMP_DIR}" ]]; then
    info "Cleaning existing deploy directory..."
    podman-compose -f "${API_GATEWAY_TMP_DIR}/compose.yaml" ${COMPOSE_PROFILES[@]+"${COMPOSE_PROFILES[@]}"} down -v 2>/dev/null || true
    rm -rf "${API_GATEWAY_TMP_DIR}"
fi

log "Cloning api-gateway (repo=${API_GATEWAY_REPO}, branch=${API_GATEWAY_BRANCH})"
git clone --branch "${API_GATEWAY_BRANCH}" --single-branch --depth 1 "${API_GATEWAY_REPO}" "${API_GATEWAY_TMP_DIR}"

# --- Deploy ---------------------------------------------------------------- #

if [[ "${CLEANUP_ON_FAILURE}" == true ]]; then
    trap 'err "Deploy failed — cleaning up"; tear_down "${API_GATEWAY_TMP_DIR}" ${COMPOSE_PROFILES[@]+"${COMPOSE_PROFILES[@]}"}' ERR
fi

log "Starting DCM stack"
if [[ "${ENABLE_KUBEVIRT}" == true ]]; then
    info "Enabled providers: kubevirt"
fi
podman-compose -f "${API_GATEWAY_TMP_DIR}/compose.yaml" ${COMPOSE_PROFILES[@]+"${COMPOSE_PROFILES[@]}"} up -d

echo
log "Container status"
podman-compose -f "${API_GATEWAY_TMP_DIR}/compose.yaml" ${COMPOSE_PROFILES[@]+"${COMPOSE_PROFILES[@]}"} ps

verify_health "${API_GATEWAY_TMP_DIR}/compose.yaml" ${COMPOSE_PROFILES[@]+"${COMPOSE_PROFILES[@]}"} || exit 1

get_running_versions "${API_GATEWAY_TMP_DIR}/compose.yaml" ${COMPOSE_PROFILES[@]+"${COMPOSE_PROFILES[@]}"} || info "Version collection failed (non-fatal)"

GATEWAY_URL="http://localhost:${GATEWAY_PORT}"
log "DCM stack is up and healthy at ${GATEWAY_URL}"
if [[ "${API_GATEWAY_TMP_DIR}" != "${DEFAULT_API_GATEWAY_TMP_DIR}" ]]; then
    info "To tear down: $(basename "$0") --api-gateway-dir ${API_GATEWAY_TMP_DIR} --tear-down"
else
    info "To tear down: $(basename "$0") --tear-down"
fi
