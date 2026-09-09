#!/usr/bin/env bash
# Install KubeVirt on the cluster selected by kubectl context (Kind, vanilla K8s).
# Skips when KubeVirt is already present (including OpenShift CNV in openshift-cnv).
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

kubectl_ctx() {
	kubectl --context "${KUBE_CONTEXT}" "$@"
}

# OpenShift / CNV: KubeVirt CR lives in openshift-cnv (or another operator namespace).
if kubectl_ctx get kv -A --no-headers 2>/dev/null | grep -q .; then
	echo "KubeVirt already installed (skipping install):"
	kubectl_ctx get kv -A
	exit 0
fi

# Vanilla Kubernetes: operator namespace kubevirt with CR name kubevirt.
if kubectl_ctx get kv kubevirt -n kubevirt >/dev/null 2>&1; then
	echo "KubeVirt already installed in namespace kubevirt (skipping install)"
	kubectl_ctx get kv -n kubevirt
	exit 0
fi

KUBEVIRT_VERSION="${KUBEVIRT_VERSION:-v1.5.0}"

echo "Installing KubeVirt ${KUBEVIRT_VERSION} on context ${KUBE_CONTEXT}"
kubectl_ctx apply -f "https://github.com/kubevirt/kubevirt/releases/download/${KUBEVIRT_VERSION}/kubevirt-operator.yaml"

echo "Waiting for KubeVirt CRDs to become established..."
kubectl_ctx wait --for=condition=Established crd/kubevirts.kubevirt.io --timeout=300s

kubectl_ctx apply -f "https://github.com/kubevirt/kubevirt/releases/download/${KUBEVIRT_VERSION}/kubevirt-cr.yaml"

echo "Waiting for KubeVirt to become available..."
kubectl_ctx -n kubevirt wait kv kubevirt --for=condition=Available --timeout=300s
echo "KubeVirt is ready."
