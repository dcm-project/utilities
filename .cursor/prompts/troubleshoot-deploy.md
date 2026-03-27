# Troubleshoot DCM Deployment

Diagnose and fix issues with the DCM stack deployment.

## Required Information

Please provide:
1. **Error output**: Paste the deploy script output or error message
2. **Deploy directory**: Which directory was used? (default: `/tmp/dcm-e2e`)
3. **Flags used**: Any non-default flags (e.g., `--kubevirt-service-provider`)

## Common Failure Patterns

### "Missing required tools"
Install the missing tools listed in the error:
```bash
# macOS
brew install podman podman-compose curl jq

# Fedora/RHEL
sudo dnf install podman podman-compose curl jq
```

### "Cannot connect to cluster" (KubeVirt mode)
```bash
# Verify kubeconfig
oc --kubeconfig ~/.kube/config cluster-info

# Check CNV is installed
oc get crd virtualmachines.kubevirt.io
```

### Containers fail to start
```bash
# Check container status
podman-compose -f /tmp/dcm-e2e/compose.yaml ps

# View logs for a failing service
podman-compose -f /tmp/dcm-e2e/compose.yaml logs --tail=50 <service-name>

# Check for port conflicts
podman ps --format '{{.Ports}}' | grep 9080
```

### Health check timeouts
```bash
# Manual health check
curl -v http://localhost:9080/api/v1alpha1/health/providers

# All health endpoints
for ep in providers catalog policies placement; do
  echo -n "$ep: "
  curl -s -o /dev/null -w "%{http_code}" "http://localhost:9080/api/v1alpha1/health/$ep"
  echo
done

# Check gateway logs
podman-compose -f /tmp/dcm-e2e/compose.yaml logs --tail=50 api-gateway
```

### Compose file not found
```bash
# Verify clone worked
ls -la /tmp/dcm-e2e/compose.yaml

# Re-deploy (cleans and re-clones)
./scripts/deploy-dcm.sh
```

### Port already in use
```bash
# Find what's using port 9080
lsof -i :9080

# Tear down and redeploy
./scripts/deploy-dcm.sh --tear-down
./scripts/deploy-dcm.sh
```

## Diagnostic Commands

```bash
# All container status
podman-compose -f /tmp/dcm-e2e/compose.yaml ps

# Recent container logs (all services)
podman-compose -f /tmp/dcm-e2e/compose.yaml logs --tail=20

# Specific service logs
podman-compose -f /tmp/dcm-e2e/compose.yaml logs --tail=50 <service>

# Container resource usage
podman stats --no-stream

# Network inspection
podman network inspect dcm-e2e_default
```

## Nuclear Option: Full Reset

```bash
# Tear down everything
./scripts/deploy-dcm.sh --tear-down

# Also clean podman state if needed
podman system prune -f

# Re-deploy
./scripts/deploy-dcm.sh
```
