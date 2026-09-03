#!/usr/bin/env bash
# Compose network teardown: disconnect containers before compose-down, remove networks after.
set -euo pipefail

usage() {
	cat <<'EOF'
Usage: network-teardown.sh <disconnect|remove>

  disconnect  Detach all containers from compose network(s) (run before compose down)
  remove      Delete compose network(s) (run after compose down, best-effort)

Environment:
  COMPOSE_NETWORKS   Space-separated network names (default: common DCM dev networks)
  COMPOSE_NETWORK    Single network when COMPOSE_NETWORKS is unset
  CONTAINER_ENGINE   podman or docker (auto-detected when unset)
EOF
}

resolve_networks() {
	if [[ -n "${COMPOSE_NETWORKS:-}" ]]; then
		NETWORKS="${COMPOSE_NETWORKS}"
	elif [[ -n "${COMPOSE_NETWORK:-}" ]]; then
		NETWORKS="${COMPOSE_NETWORK}"
	else
		NETWORKS="environment-agent_default control-plane_default deploy_default dcm-e2e_default"
	fi
}

pick_engine() {
	if [[ -n "${CONTAINER_ENGINE:-}" ]]; then
		if command -v "${CONTAINER_ENGINE}" >/dev/null 2>&1; then
			return 0
		fi
		echo "Error: CONTAINER_ENGINE=${CONTAINER_ENGINE} not found" >&2
		return 1
	fi
	for candidate in podman docker; do
		if command -v "${candidate}" >/dev/null 2>&1; then
			CONTAINER_ENGINE="${candidate}"
			return 0
		fi
	done
	echo "Error: no container engine found (podman or docker)" >&2
	return 1
}

pick_engine_optional() {
	if pick_engine; then
		return 0
	fi
	return 1
}

cmd_disconnect() {
	resolve_networks
	pick_engine || exit 1

	for network in ${NETWORKS}; do
		if [[ "${CONTAINER_ENGINE}" == podman ]]; then
			if ! podman network exists "${network}" 2>/dev/null; then
				continue
			fi
			while IFS= read -r container; do
				[[ -z "${container}" ]] && continue
				echo "Disconnecting ${container} from ${network}"
				podman network disconnect -f "${network}" "${container}" 2>/dev/null || true
			done < <(podman ps -a --filter "network=${network}" -q 2>/dev/null)
		elif [[ "${CONTAINER_ENGINE}" == docker ]]; then
			if ! docker network inspect "${network}" >/dev/null 2>&1; then
				continue
			fi
			while IFS= read -r container; do
				[[ -z "${container}" ]] && continue
				echo "Disconnecting ${container} from ${network}"
				docker network disconnect "${network}" "${container}" --force 2>/dev/null || true
			done < <(docker ps -a --filter "network=${network}" -q 2>/dev/null)
		fi
	done
}

cmd_remove() {
	resolve_networks
	if ! pick_engine_optional; then
		exit 0
	fi

	for network in ${NETWORKS}; do
		if [[ "${CONTAINER_ENGINE}" == podman ]]; then
			if podman network exists "${network}" 2>/dev/null; then
				echo "Removing network ${network}"
				podman network rm -f "${network}" 2>/dev/null || true
			fi
		elif [[ "${CONTAINER_ENGINE}" == docker ]]; then
			if docker network inspect "${network}" >/dev/null 2>&1; then
				echo "Removing network ${network}"
				docker network rm "${network}" 2>/dev/null || true
			fi
		fi
	done
}

main() {
	local cmd="${1:-}"
	case "${cmd}" in
		disconnect) cmd_disconnect ;;
		remove) cmd_remove ;;
		-h | --help | help | "") usage; [[ -n "${cmd}" ]] || exit 1 ;;
		*)
			echo "Error: unknown command '${cmd}'" >&2
			usage >&2
			exit 1
			;;
	esac
}

main "$@"
