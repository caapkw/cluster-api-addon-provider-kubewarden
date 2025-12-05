# KubewardenPolicy User Guide

## Overview

The `KubewardenPolicy` CRD allows you to define and deploy Kubewarden policies to your CAPI workload clusters from the management cluster. This provides centralized policy management and enforcement across your infrastructure.

## Prerequisites

- CAAPKW installed on your management cluster
- `KubewardenAddon` deployed and active on target workload clusters
- Workload clusters selected by appropriate labels

## Policy Types

Kubewarden supports two types of policies:

### ClusterAdmissionPolicy (Cluster-wide)

ClusterAdmissionPolicy resources are cluster-scoped and can validate/mutate resources across all namespaces in the cluster.

**Use cases:**
- Enforcing cluster-wide security policies
- Validating resource configurations across all namespaces
- Mutating resources globally

### AdmissionPolicy (Namespace-scoped)

AdmissionPolicy resources are namespace-scoped and only validate/mutate resources in a specific namespace.

**Requirements:**
- Kubernetes 1.21.0+ in workload clusters
- Must specify `targetNamespace`

**Use cases:**
- Namespace-specific policy enforcement
- Tenant-specific policies
- Fine-grained access control

## Basic Usage

### Example 1: Prevent Privileged Containers (Cluster-wide)

```yaml
apiVersion: addon.cluster.x-k8s.io/v1alpha1
kind: KubewardenPolicy
metadata:
  name: no-privileged-pods
  namespace: default
spec:
  # Select target clusters
  clusterSelector:
    matchLabels:
      environment: production
  
  # Policy type
  policyType: ClusterAdmissionPolicy
  
  # Policy module (from OCI registry)
  module: registry://ghcr.io/kubewarden/policies/pod-privileged:v1.0.8
  
  # Resources and operations
  rules:
    - apiGroups: [""]
      apiVersions: ["v1"]
      resources: ["pods"]
      operations:
        - CREATE
        - UPDATE
  
  # Policy behavior
  mutating: false
  failurePolicy: Fail
```

### Example 2: Enforce Pod Security Standards with Settings

```yaml
apiVersion: addon.cluster.x-k8s.io/v1alpha1
kind: KubewardenPolicy
metadata:
  name: psp-capabilities
  namespace: default
spec:
  clusterSelector:
    matchLabels:
      environment: production
  
  policyType: ClusterAdmissionPolicy
  module: registry://ghcr.io/kubewarden/policies/pod-privileged:v1.0.8
  
  rules:
    - apiGroups: [""]
      apiVersions: ["v1"]
      resources: ["pods"]
      operations:
        - CREATE
        - UPDATE
  
  # This policy can mutate pod specs
  mutating: true
  
  # Policy-specific configuration
  settings:
    allowed_capabilities:
      - CHOWN
      - NET_BIND_SERVICE
    required_drop_capabilities:
      - NET_ADMIN
      - SYS_ADMIN
  
  failurePolicy: Fail
```

### Example 3: Namespace-scoped Policy

```yaml
apiVersion: addon.cluster.x-k8s.io/v1alpha1
kind: KubewardenPolicy
metadata:
  name: verify-signatures-production
  namespace: default
spec:
  clusterSelector:
    matchLabels:
      environment: production
  
  # Use namespace-scoped policy
  policyType: AdmissionPolicy
  targetNamespace: production-apps
  
  module: registry://ghcr.io/kubewarden/policies/verify-image-signatures:v0.2.1
  
  rules:
    - apiGroups: [""]
      apiVersions: ["v1"]
      resources: ["pods"]
      operations:
        - CREATE
  
  mutating: false
  
  settings:
    signatures:
      - image: "ghcr.io/my-org/*"
        pubKeys:
          - |
            -----BEGIN PUBLIC KEY-----
            MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE...
            -----END PUBLIC KEY-----
  
  failurePolicy: Fail
```

## Spec Fields Reference

### Required Fields

| Field | Type | Description |
|-------|------|-------------|
| `clusterSelector` | LabelSelector | Selects target clusters where the policy will be deployed |
| `module` | string | Location of the WASM policy module (registry://, https://, file://) |
| `rules` | []PolicyRule | Kubernetes resources and operations this policy applies to |

### Optional Fields

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `policyType` | string | `ClusterAdmissionPolicy` | Type of policy to create |
| `policyName` | string | (resource name) | Name of the policy in workload cluster |
| `targetNamespace` | string | `default` | Namespace for AdmissionPolicy (ignored for ClusterAdmissionPolicy) |
| `policyServer` | string | `default` | PolicyServer that will serve this policy |
| `mutating` | bool | `false` | Whether the policy can mutate requests |
| `settings` | object | - | Policy-specific configuration |
| `failurePolicy` | string | `Fail` | How to handle policy errors (`Fail` or `Ignore`) |
| `matchConditions` | []MatchCondition | - | CEL expressions for advanced filtering |

### PolicyRule Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `apiGroups` | []string | No | API groups (e.g., `[""]`, `["apps"]`) |
| `apiVersions` | []string | Yes | API versions (e.g., `["v1"]`) |
| `resources` | []string | Yes | Resource types (e.g., `["pods"]`, `["deployments"]`) |
| `operations` | []string | Yes | Operations (`CREATE`, `UPDATE`, `DELETE`, `CONNECT`) |
| `scope` | string | No | Scope filter (`*`, `Cluster`, `Namespaced`) |

## Cluster Selection

Use label selectors to target specific clusters:

```yaml
spec:
  clusterSelector:
    matchLabels:
      environment: production
      region: us-west
```

Or use match expressions for more complex selection:

```yaml
spec:
  clusterSelector:
    matchExpressions:
      - key: environment
        operator: In
        values: [production, staging]
      - key: tier
        operator: NotIn
        values: [test]
```

## Policy Settings

The `settings` field is policy-specific and varies by policy. Refer to the policy documentation for available settings:

- Browse policies on [ArtifactHub](https://artifacthub.io/packages/search?kind=13)
- View [Kubewarden policy catalog](https://github.com/topics/kubewarden-policy)

Example settings for common policies:

### Pod Privileged Policy
```yaml
settings: {}  # No settings needed
```

### Allowed FSGroups
```yaml
settings:
  ranges:
    - min: 1000
      max: 2000
```

### User Group Policy
```yaml
settings:
  run_as_user:
    rule: MustRunAs
    ranges:
      - min: 1000
        max: 65535
  run_as_group:
    rule: MayRunAs
    ranges:
      - min: 1000
        max: 65535
```

## Status Monitoring

Check the status of your policy deployment:

```bash
kubectl get kubewardenpolicy -A
```

Output:
```
NAMESPACE   NAME                POLICY TYPE              MODULE                                                   READY   AGE
default     no-privileged-pods  ClusterAdmissionPolicy   registry://ghcr.io/kubewarden/policies/pod-privileged   true    5m
```

Detailed status:
```bash
kubectl describe kubewardenpolicy no-privileged-pods
```

The status includes:
- Overall ready state
- List of matching clusters
- Per-cluster deployment status
- Policy activation status
- Error messages (if any)

## Best Practices

### 1. Start with Monitor Mode

Test policies in monitor mode before enforcing them:

**Note:** Currently CAAPKW doesn't expose the `mode` field. You can manually patch policies in workload clusters or this will be added in a future version.

### 2. Use Meaningful Names

Use descriptive names that indicate the policy purpose:
- ✅ `enforce-resource-limits`
- ✅ `verify-production-images`
- ❌ `policy1`
- ❌ `test`

### 3. Organize by Environment

Use different policies for different environments:

```yaml
# Production - strict enforcement
clusterSelector:
  matchLabels:
    environment: production
failurePolicy: Fail
```

```yaml
# Staging - warn but allow
clusterSelector:
  matchLabels:
    environment: staging
failurePolicy: Ignore
```

### 4. Version Your Policies

Always specify policy versions in the module URL:
- ✅ `registry://ghcr.io/kubewarden/policies/pod-privileged:v1.0.8`
- ❌ `registry://ghcr.io/kubewarden/policies/pod-privileged:latest`

### 5. Test Before Deploying

Test policies in a non-production cluster first:

1. Create a test cluster with label `environment: test`
2. Deploy policy to test cluster
3. Verify policy works as expected
4. Update cluster selector to include production

## Troubleshooting

### Policy Not Deployed

**Symptoms:** Policy shows `ready: false` in status

**Possible causes:**
1. Kubewarden not installed on target cluster
   - Check for annotation: `caapkw.kubewarden.io/installed: "true"`
   - Deploy `KubewardenAddon` first
2. Cluster not ready
   - Verify cluster control plane is ready
3. Invalid policy configuration
   - Check policy validation webhook messages

### Policy Not Active

**Symptoms:** Policy deployed but `active: false` in status

**Possible causes:**
1. PolicyServer not ready
   - Check PolicyServer status in workload cluster
2. Policy validation failed
   - Check policy status in workload cluster
3. Policy rollout in progress
   - Wait for PolicyServer rollout to complete

### Requests Not Being Validated

**Possible causes:**
1. Policy rules don't match the resources
   - Verify `rules` configuration
2. Wrong policy type for namespace
   - Use ClusterAdmissionPolicy for cluster-wide
   - Use AdmissionPolicy for namespace-specific
3. ObjectSelector not matching
   - Check if objects have required labels

### Debug Commands

```bash
# Check policy status
kubectl get kubewardenpolicy -A

# Get detailed policy information
kubectl describe kubewardenpolicy <name>

# Check matching clusters
kubectl get clusters -l environment=production

# Check Kubewarden installation on workload cluster
kubectl --context=workload-cluster get policyserver -n kubewarden

# Check policy in workload cluster
kubectl --context=workload-cluster get clusteradmissionpolicy
kubectl --context=workload-cluster get admissionpolicy -A

# Check webhook configurations
kubectl --context=workload-cluster get validatingwebhookconfiguration -l kubewarden
kubectl --context=workload-cluster get mutatingwebhookconfiguration -l kubewarden
```

## Examples Repository

For more examples, see:
- [config/samples/addon_v1alpha1_kubewardenpolicy.yaml](../config/samples/addon_v1alpha1_kubewardenpolicy.yaml)
- [config/samples/addon_v1alpha1_kubewardenpolicy_advanced.yaml](../config/samples/addon_v1alpha1_kubewardenpolicy_advanced.yaml)

## Additional Resources

- [Kubewarden Documentation](https://docs.kubewarden.io/)
- [Kubewarden Quick Start](https://docs.kubewarden.io/quick-start)
- [Policy Hub on ArtifactHub](https://artifacthub.io/packages/search?kind=13)
- [Writing Kubewarden Policies](https://docs.kubewarden.io/tutorials/writing-policies)
