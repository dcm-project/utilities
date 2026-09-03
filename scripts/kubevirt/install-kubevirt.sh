#!/usr/bin/env bash
# Install KubeVirt on the cluster selected by kubectl context (any cluster, not Kind-only).
set -euo pipefail

if ! command -v kubectl >/dev/null 2>&1; then
	echo "Error: kubectl is required" >&2
	exit 1
fi

KUBE_CONTEXT="${KUBE_CONTEXT:-$(kubectl config current-context 2>/dev/null || true)}"
if [[ -z "${KUBE_CONTEXT}" ]]; then
	echo "Error: no kubectl current-context; set KUBE_CONTEXT or kubectl config use-context" >&2
	exit 1
fi

KUBEVIRT_VERSION="${KUBEVIRT_VERSION:-v1.5.0}"

echo "Installing KubeVirt ${KUBEVIRT_VERSION} on context ${KUBE_CONTEXT}"
kubectl --context "${KUBE_CONTEXT}" apply -f "https://github.com/kubevirt/kubevirt/releases/download/${KUBEVIRT_VERSION}/kubevirt-operator.yaml"

echo "Waiting for KubeVirt CRDs to become established..."
kubectl --context "${KUBE_CONTEXT}" wait --for=condition=Established crd/kubevirts.kubevirt.io --timeout=300s

kubectl --context "${KUBE_CONTEXT}" apply -f "https://github.com/kubevirt/kubevirt/releases/download/${KUBEVIRT_VERSION}/kubevirt-cr.yaml"

echo "Waiting for KubeVirt to become available..."
kubectl --context "${KUBE_CONTEXT}" -n kubevirt wait kv kubevirt --for=condition=Available --timeout=300s
echo "KubeVirt is ready."
