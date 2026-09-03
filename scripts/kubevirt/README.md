# KubeVirt install

Install KubeVirt on the cluster selected by `kubectl` context. Works with Kind, OpenShift,
or any Kubernetes cluster — not tied to Kind networking.

## Usage

```bash
kubectl config use-context kind-dcm-local   # or any cluster context
bash scripts/kubevirt/install-kubevirt.sh
```

## Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `KUBE_CONTEXT` | `kubectl` current context | Target cluster |
| `KUBEVIRT_VERSION` | `v1.5.0` | KubeVirt release tag |

Run before starting a workload that needs the vm SP (embedded or external).
