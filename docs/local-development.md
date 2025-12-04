# Local Development Guide

This guide walks you through setting up a local development environment for testing CAAPKW and the new KubewardenPolicy CRD.

## Prerequisites

Install the following tools:

- [Docker](https://docs.docker.com/get-docker/) - Container runtime
- [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation) - Kubernetes in Docker
- [kubectl](https://kubernetes.io/docs/tasks/tools/) - Kubernetes CLI
- [clusterctl](https://cluster-api.sigs.k8s.io/user/quick-start.html#install-clusterctl) - Cluster API CLI
- [Go 1.22+](https://go.dev/doc/install) - For building the controller

## Quick Start

### 1. Setup Local Environment

Run the automated setup script:

```bash
./scripts/local-dev-setup.sh
```

This script will:
- Create a Kind management cluster
- Initialize Cluster API with the Docker provider
- Install CAAPKW CRDs
- Create a CAPI workload cluster
- Generate kubeconfig for the workload cluster

**Time:** ~5-10 minutes

### 2. Run the Controller

In a new terminal, start the controller locally:

```bash
make run
```

This runs the controller with your local code changes and connects to the management cluster.

### 3. Test KubewardenAddon

Apply the sample addon to install Kubewarden on the workload cluster:

```bash
kubectl apply -f config/samples/addon_v1alpha1_kubewardenaddon.yaml
```

Watch the addon status:

```bash
kubectl get kubewardenaddon -w
```

You should see:
1. The addon resource created
2. Matching clusters populated
3. Kubewarden installed on the workload cluster (check cluster annotations)

### 4. Test KubewardenPolicy

Once Kubewarden is installed, deploy a test policy:

```bash
kubectl apply -f config/samples/addon_v1alpha1_kubewardenpolicy.yaml
```

Watch the policy status:

```bash
kubectl get kubewardenpolicy -w
```

### 5. Run Automated Tests

Use the test script for a complete end-to-end test:

```bash
./scripts/test-policy-deployment.sh
```

This will:
- Verify Kubewarden is installed
- Deploy a test policy
- Monitor deployment status
- Verify the policy in the workload cluster
- Test policy enforcement
- Optionally clean up test resources

## Manual Testing Workflow

### Inspect Workload Cluster

Access the workload cluster:

```bash
export KUBECONFIG=$(pwd)/caapkw-workload.kubeconfig
kubectl get pods -A
```

Check Kubewarden components:

```bash
# Check Kubewarden namespace
kubectl get ns kubewarden

# Check PolicyServer
kubectl get policyserver -n kubewarden

# Check installed policies
kubectl get clusteradmissionpolicy -A
kubectl get admissionpolicy -A

# Check webhooks
kubectl get validatingwebhookconfiguration -l kubewarden
kubectl get mutatingwebhookconfiguration -l kubewarden
```

### Test Policy Enforcement

Create test pods in the workload cluster:

```bash
export KUBECONFIG=$(pwd)/caapkw-workload.kubeconfig

# This should succeed
kubectl run nginx --image=nginx

# This should be denied (if you have the privileged-pods policy)
kubectl run privileged-nginx --image=nginx --privileged=true
```

### View Controller Logs

The controller logs will show:
- Reconciliation events
- Policy deployments
- Error messages (if any)

Look for logs like:
```
INFO    Reconciling KubewardenPolicy
INFO    Creating ClusterAdmissionPolicy    cluster=caapkw-workload
INFO    Policy successfully deployed and active
```

## Development Workflow

### Making Code Changes

1. Edit the code in your editor
2. Stop the running controller (Ctrl+C)
3. Rebuild and restart:
   ```bash
   make run
   ```

### Testing Changes

After code changes:

```bash
# Run unit tests
make test

# Build to check for compilation errors
go build ./...

# Regenerate manifests if you changed APIs
make generate manifests
```

### Debugging

Enable verbose logging:

```bash
# Run controller with debug logging
go run ./cmd/main.go --zap-log-level=debug
```

Check resource status:

```bash
# Management cluster
kubectl describe kubewardenaddon <name>
kubectl describe kubewardenpolicy <name>
kubectl describe cluster <name>

# Workload cluster
export KUBECONFIG=$(pwd)/caapkw-workload.kubeconfig
kubectl describe clusteradmissionpolicy <name>
kubectl describe policyserver -n kubewarden
```

## Testing Different Scenarios

### Test with Multiple Policies

Create different policy types:

```bash
# Cluster-wide policy
kubectl apply -f config/samples/addon_v1alpha1_kubewardenpolicy.yaml

# Namespace-scoped policy
kubectl apply -f config/samples/addon_v1alpha1_kubewardenpolicy_advanced.yaml
```

### Test Cluster Selection

Label your workload cluster:

```bash
kubectl label cluster caapkw-workload environment=staging
kubectl label cluster caapkw-workload tier=backend
```

Create policies targeting specific labels:

```yaml
spec:
  clusterSelector:
    matchLabels:
      environment: staging
```

### Test Policy Updates

1. Deploy a policy
2. Edit the policy (e.g., change module version or settings)
3. Apply the changes
4. Verify the policy is updated in the workload cluster

### Test Policy Deletion

```bash
# Delete policy from management cluster
kubectl delete kubewardenpolicy <name>

# Verify it's removed from workload cluster
export KUBECONFIG=$(pwd)/caapkw-workload.kubeconfig
kubectl get clusteradmissionpolicy <name>  # Should not exist
```

## Common Issues and Solutions

### Issue: Controller can't connect to workload cluster

**Solution:** Ensure the cluster is ready and kubeconfig secret exists:
```bash
kubectl get cluster -A
kubectl get secret -n default | grep kubeconfig
```

### Issue: Policy not deploying

**Solution:** Check that Kubewarden is installed:
```bash
kubectl get cluster <name> -o jsonpath='{.metadata.annotations}'
# Look for: caapkw.kubewarden.io/installed: "true"
```

### Issue: Policy deployed but not active

**Solution:** Check PolicyServer status in workload cluster:
```bash
export KUBECONFIG=$(pwd)/caapkw-workload.kubeconfig
kubectl get policyserver -n kubewarden
kubectl describe policyserver default -n kubewarden
```

### Issue: Changes not taking effect

**Solution:** Restart the controller and check for errors:
```bash
# Stop controller (Ctrl+C)
# Rebuild
make generate manifests
# Restart
make run
```

## Cleanup

Remove all local resources:

```bash
./scripts/local-dev-cleanup.sh
```

This will:
- Delete the management cluster (and nested workload cluster)
- Remove kubeconfig files
- Clean up local state

## Advanced Testing

### Test with Multiple Workload Clusters

Create additional workload clusters:

```bash
# Create second cluster with different labels
WORKLOAD_CLUSTER_NAME=caapkw-workload-2 ./scripts/local-dev-setup.sh
kubectl label cluster caapkw-workload-2 environment=production
```

Deploy policies targeting different clusters:

```yaml
# Policy for staging
spec:
  clusterSelector:
    matchLabels:
      environment: staging

---
# Policy for production
spec:
  clusterSelector:
    matchLabels:
      environment: production
```

### Test Policy Server Configuration

Modify the addon to use custom PolicyServer settings:

```yaml
spec:
  policyServerConfig:
    replicas: 2
    resources:
      cpu: 200m
      memory: 256Mi
```

### Integration with CI/CD

Run automated tests in CI:

```bash
# In your CI pipeline
./scripts/local-dev-setup.sh
make test
./scripts/test-policy-deployment.sh
```

## Next Steps

- Read the [KubewardenPolicy User Guide](./kubewardenpolicy-guide.md) for detailed policy configuration
- Explore [Kubewarden policies](https://artifacthub.io/packages/search?kind=13) to test different policy types
- Check the [Development Guide](./development.md) for contribution guidelines

## Tips

1. **Keep the controller running** - Run it in a separate terminal for faster iteration
2. **Use watch commands** - Monitor resources in real-time: `kubectl get kubewardenpolicy -w`
3. **Check both clusters** - Verify changes in both management and workload clusters
4. **Use describe commands** - They show detailed status and events
5. **Enable debug logging** - Helps troubleshoot reconciliation issues
