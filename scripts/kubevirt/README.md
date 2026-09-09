# KubeVirt install

Install KubeVirt on the cluster selected by `kubectl` context when it is not already present.

## Skip conditions (idempotent)

The script exits successfully without changes when:

- **OpenShift / CNV:** any `KubeVirt` CR exists (`kubectl get kv -A` / `oc get kv -A`), e.g. CNV in
  `openshift-cnv` with `PHASE=Deployed`
- **Vanilla Kubernetes:** `kubevirt` CR exists in namespace `kubevirt`

Do **not** run the upstream KubeVirt installer on OpenShift when CNV is already deployed.

## Usage

```bash
kubectl config use-context kind-dcm-local   # or openshift cluster
bash scripts/kubevirt/install-kubevirt.sh
```

On OpenShift with CNV, only verify:

```bash
oc get kv -A
```

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `KUBE_CONTEXT` | `kubectl` current context | Target cluster |
| `KUBEVIRT_VERSION` | `v1.5.0` | KubeVirt release tag (vanilla K8s / Kind only) |

Run before starting a workload that needs the vm SP (embedded or external).
