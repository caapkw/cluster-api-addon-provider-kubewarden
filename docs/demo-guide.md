# CAAPKW Demo - Quick Reference

## Prerequisites

Ensure you have the development environment set up:

```bash
./scripts/local-dev-setup.sh
```

This creates:
- Kind management cluster (`caapkw-mgmt`) 
- Cluster API + CAPD installed
- CAAPKW CRDs installed
- CAPD workload cluster (`caapkw-workload`) in Running state
- CNI installed (Calico)
- Kubeconfig for workload cluster at `./caapkw-workload.kubeconfig`

## Running the Demo

Execute the interactive demo script:

```bash
./scripts/demo.sh
```

The demo walks through 7 steps:

### Step 1: Environment Check
- Verifies management and workload clusters are ready
- Shows cluster information

### Step 2: Deploy CAAPKW Controller
- Installs cert-manager (for webhook TLS certificates)
- Builds controller image
- Loads image into Kind management cluster
- Deploys controller to `caapkw-system` namespace
- Webhooks are fully functional

### Step 3: Create KubewardenAddon
- Creates a `KubewardenAddon` resource in management cluster
- Demonstrates ClusterResourceSet selector matching
- Shows controller reconciliation

### Step 4: Verify Kubewarden Installation
- Checks workload cluster for Kubewarden components
- Shows Policy Server deployment
- Verifies webhook service

### Step 5: Create KubewardenPolicy
- Deploys a policy to block privileged pods
- Creates `KubewardenPolicy` resource in management cluster
- Shows automatic propagation to workload cluster

### Step 6: Test Policy Enforcement
- Attempts to create a privileged pod (should be blocked)
- Attempts to create a non-privileged pod (should succeed)
- Demonstrates policy in action

### Step 7: Cleanup
- Removes demo resources
- Leaves environment running for further testing

## Manual Testing

### Check Controller Status

```bash
kubectl get pods -n caapkw-system
kubectl logs -n caapkw-system deployment/caapkw-controller-manager
```

### Check Webhooks

```bash
kubectl get validatingwebhookconfigurations
kubectl get mutatingwebhookconfigurations
```

### Create Custom Resources

```bash
# Create a KubewardenAddon
kubectl apply -f config/samples/addon_v1alpha1_kubewardenaddon.yaml

# Check status
kubectl get kubewardenaddon -o yaml

# Verify in workload cluster
kubectl --kubeconfig=./caapkw-workload.kubeconfig get pods -n kubewarden
```

### Test Policy

```bash
# Create a KubewardenPolicy
cat <<EOF | kubectl apply -f -
apiVersion: addon.cluster.x-k8s.io/v1alpha1
kind: KubewardenPolicy
metadata:
  name: no-privileged-pods
  namespace: default
spec:
  clusterResourceSetName: kubewarden-policies
  policySpec:
    module: registry://ghcr.io/kubewarden/policies/pod-privileged:v1.0.8
    rules:
      - apiGroups: [""]
        apiVersions: ["v1"]
        resources: ["pods"]
        operations: ["CREATE", "UPDATE"]
    mutating: false
    settings:
      defaultAllowPrivilegedPods: false
EOF

# Check in workload cluster
kubectl --kubeconfig=./caapkw-workload.kubeconfig get clusteradmissionpolicies
```

## Cleanup

### Remove Demo Resources Only

```bash
kubectl delete kubewardenpolicy --all
kubectl delete kubewardenaddon --all
```

### Tear Down Everything

```bash
kind delete cluster --name caapkw-mgmt
```

This removes:
- Management cluster
- Workload cluster (Docker containers)
- All resources and data

## Troubleshooting

### Controller Not Starting

Check for image loading:
```bash
docker exec -it caapkw-mgmt-control-plane crictl images | grep caapkw
```

Check cert-manager:
```bash
kubectl get pods -n cert-manager
kubectl get certificates -n caapkw-system
```

### Webhooks Not Working

Check webhook configurations:
```bash
kubectl get validatingwebhookconfigurations -o yaml | grep -A5 caBundle
kubectl get mutatingwebhookconfigurations -o yaml | grep -A5 caBundle
```

The `caBundle` field should be populated by cert-manager.

Check controller logs:
```bash
kubectl logs -n caapkw-system deployment/caapkw-controller-manager -f
```

### Resources Not Propagating

Check ClusterResourceSet:
```bash
kubectl get clusterresourcesets -A
kubectl describe clusterresourceset <name>
```

Check cluster labels:
```bash
kubectl get clusters -A --show-labels
```

### Policy Not Enforcing

Check in workload cluster:
```bash
kubectl --kubeconfig=./caapkw-workload.kubeconfig get clusteradmissionpolicies
kubectl --kubeconfig=./caapkw-workload.kubeconfig get pods -n kubewarden
kubectl --kubeconfig=./caapkw-workload.kubeconfig logs -n kubewarden deployment/policy-server-default
```

## Development Workflow

1. Make code changes
2. Rebuild and reload:
   ```bash
   make docker-build IMG=caapkw-controller:dev
   kind load docker-image caapkw-controller:dev --name caapkw-mgmt
   kubectl rollout restart -n caapkw-system deployment/caapkw-controller-manager
   ```
3. Check logs: `kubectl logs -n caapkw-system deployment/caapkw-controller-manager -f`
4. Test changes with demo script or manual resources

## Additional Resources

- [Development Guide](./development.md)
- [Webhook Certificates](./webhook-certificates.md)
- [Quick Start Guide](./quick-start.md)
