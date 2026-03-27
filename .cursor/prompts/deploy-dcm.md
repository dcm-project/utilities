# Deploy the DCM Stack

Deploy the full DCM stack for E2E testing using `scripts/deploy-dcm.sh`.

## Prerequisites

1. **Required tools**: `git`, `podman`, `podman-compose`, `curl`, `jq`
2. **Optional**: `oc` (only for KubeVirt service provider)

## Commands

### Default Deploy
```bash
./scripts/deploy-dcm.sh
```

### Deploy from a Different Branch
```bash
./scripts/deploy-dcm.sh --api-gateway-branch feature-x
```

### Deploy from a Fork
```bash
./scripts/deploy-dcm.sh --api-gateway-repo https://github.com/myfork/api-gateway.git
```

### Deploy to a Custom Directory
```bash
./scripts/deploy-dcm.sh --api-gateway-dir /tmp/my-dcm-deploy
```

### Deploy with Auto-Cleanup on Failure
```bash
./scripts/deploy-dcm.sh --cleanup-on-failure
```

### Deploy with KubeVirt Service Provider
```bash
./scripts/deploy-dcm.sh --kubevirt-service-provider --kubeconfig ~/.kube/config
```

### Deploy with All Service Providers
```bash
./scripts/deploy-dcm.sh --all-service-providers --kubeconfig ~/.kube/config
```

## Environment Variable Overrides

| Variable | Flag equivalent |
|----------|----------------|
| `API_GATEWAY_REPO` | `--api-gateway-repo` |
| `API_GATEWAY_BRANCH` | `--api-gateway-branch` |
| `API_GATEWAY_TMP_DIR` | `--api-gateway-dir` |
| `KUBECONFIG` | `--kubeconfig` |
| `KUBEVIRT_VM_NAMESPACE` | `--kubevirt-vm-namespace` |

Flags take precedence over environment variables.

## What Happens

1. Clones api-gateway (owns `compose.yaml`)
2. Runs `podman-compose up -d`
3. Verifies all containers are running
4. Polls `/api/v1alpha1/health/*` endpoints (90s timeout)
5. Resolves container images to git commit SHAs via Quay.io API
6. Writes `dcm-versions.json`

## Output

- Stack available at `http://localhost:9080`
- Version info written to `dcm-versions.json`
