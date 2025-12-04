# CAAPKW Quick Testing Reference

## 🚀 One-Command Setup

```bash
./scripts/local-dev-setup.sh
```

## 🎯 Quick Test Flow

### Terminal 1: Run Controller
```bash
make run
```

### Terminal 2: Deploy & Test
```bash
# 1. Deploy Kubewarden addon
kubectl apply -f config/samples/addon_v1alpha1_kubewardenaddon.yaml

# 2. Watch addon status
kubectl get kubewardenaddon -w

# 3. Deploy policy (wait for addon to be ready)
kubectl apply -f config/samples/addon_v1alpha1_kubewardenpolicy.yaml

# 4. Watch policy status
kubectl get kubewardenpolicy -w

# 5. Check workload cluster
export KUBECONFIG=$(pwd)/caapkw-workload.kubeconfig
kubectl get clusteradmissionpolicy
```

## 🧪 Automated Testing

```bash
./scripts/test-policy-deployment.sh
```

## 📊 Useful Commands

### Management Cluster (Kind)
```bash
# View all resources
kubectl get kubewardenaddon,kubewardenpolicy,clusters -A

# Watch policy deployment
kubectl get kubewardenpolicy -w

# Describe resources
kubectl describe kubewardenpolicy <name>
kubectl describe cluster caapkw-workload

# Check cluster annotations
kubectl get cluster caapkw-workload -o jsonpath='{.metadata.annotations}'
```

### Workload Cluster
```bash
export KUBECONFIG=$(pwd)/caapkw-workload.kubeconfig

# Check Kubewarden components
kubectl get all -n kubewarden
kubectl get policyserver -n kubewarden
kubectl get clusteradmissionpolicy
kubectl get admissionpolicy -A

# Test policy enforcement
kubectl run test-pod --image=nginx
kubectl run privileged-pod --image=nginx --privileged=true  # Should fail
```

## 🔧 Development

```bash
# Regenerate after API changes
make generate manifests

# Run tests
make test

# Build
go build ./...

# Run with debug logging
go run ./cmd/main.go --zap-log-level=debug
```

## 🧹 Cleanup

```bash
./scripts/local-dev-cleanup.sh
```

## 🐛 Debug Tips

1. **Controller not reconciling?**
   - Check controller logs for errors
   - Verify cluster is Ready: `kubectl get cluster -A`

2. **Policy not deploying?**
   - Check Kubewarden installed: `kubectl get cluster <name> -o yaml | grep caapkw.kubewarden.io/installed`
   - Verify cluster matches selector: `kubectl get cluster --show-labels`

3. **Policy deployed but not active?**
   ```bash
   export KUBECONFIG=$(pwd)/caapkw-workload.kubeconfig
   kubectl describe policyserver default -n kubewarden
   kubectl describe clusteradmissionpolicy <name>
   ```

4. **Need fresh start?**
   ```bash
   ./scripts/local-dev-cleanup.sh
   ./scripts/local-dev-setup.sh
   ```

## 📝 Example Workflows

### Test Different Policy Types
```bash
# ClusterAdmissionPolicy (cluster-wide)
kubectl apply -f config/samples/addon_v1alpha1_kubewardenpolicy.yaml

# AdmissionPolicy (namespace-scoped)  
kubectl apply -f config/samples/addon_v1alpha1_kubewardenpolicy_advanced.yaml
```

### Test Cluster Selection
```bash
# Label clusters
kubectl label cluster caapkw-workload environment=staging

# Policy targeting staging
cat <<EOF | kubectl apply -f -
apiVersion: addon.cluster.x-k8s.io/v1alpha1
kind: KubewardenPolicy
metadata:
  name: staging-only-policy
spec:
  clusterSelector:
    matchLabels:
      environment: staging
  module: registry://ghcr.io/kubewarden/policies/pod-privileged:v1.0.8
  # ... rest of spec
EOF
```

### Test Policy Updates
```bash
# Deploy policy
kubectl apply -f my-policy.yaml

# Edit policy
vim my-policy.yaml

# Apply changes
kubectl apply -f my-policy.yaml

# Verify in workload cluster
export KUBECONFIG=$(pwd)/caapkw-workload.kubeconfig
kubectl get clusteradmissionpolicy <name> -o yaml
```

## ⏱️ Expected Times

- Setup: ~5-10 minutes
- Kubewarden installation: ~2-3 minutes
- Policy deployment: ~30-60 seconds
- Policy activation: ~30-60 seconds

## 📚 Resources

- [Local Development Guide](./docs/local-development.md) - Full guide
- [KubewardenPolicy Guide](./docs/kubewardenpolicy-guide.md) - Policy documentation
- [Kubewarden Docs](https://docs.kubewarden.io/) - Official docs
- [Policy Hub](https://artifacthub.io/packages/search?kind=13) - Browse policies
