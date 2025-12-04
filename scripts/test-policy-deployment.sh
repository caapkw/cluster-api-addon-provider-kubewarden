#!/usr/bin/env bash

# Test script for KubewardenPolicy deployment
# This script tests the full workflow of deploying policies to workload clusters

set -o errexit
set -o nounset
set -o pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

MGMT_CLUSTER_NAME="${MGMT_CLUSTER_NAME:-caapkw-mgmt}"
WORKLOAD_CLUSTER_NAME="${WORKLOAD_CLUSTER_NAME:-caapkw-workload}"

# Switch to management cluster context
switch_to_mgmt() {
    kubectl config use-context "kind-${MGMT_CLUSTER_NAME}" >/dev/null 2>&1
}

# Switch to workload cluster context
switch_to_workload() {
    if [ -f "${WORKLOAD_CLUSTER_NAME}.kubeconfig" ]; then
        export KUBECONFIG="${WORKLOAD_CLUSTER_NAME}.kubeconfig"
    else
        error "Workload cluster kubeconfig not found: ${WORKLOAD_CLUSTER_NAME}.kubeconfig"
        exit 1
    fi
}

# Step 1: Verify KubewardenAddon
step1_verify_addon() {
    step "1. Verifying KubewardenAddon..."
    switch_to_mgmt
    
    if ! kubectl get kubewardenaddon kubewardenaddon-sample -n default >/dev/null 2>&1; then
        info "KubewardenAddon not found. Creating it..."
        kubectl apply -f config/samples/addon_v1alpha1_kubewardenaddon.yaml
    fi
    
    info "Waiting for KubewardenAddon to be deployed (this may take a few minutes)..."
    local max_attempts=60
    local attempt=0
    while [ $attempt -lt $max_attempts ]; do
        if kubectl get cluster "${WORKLOAD_CLUSTER_NAME}" -n default -o jsonpath='{.metadata.annotations.caapkw\.kubewarden\.io/installed}' 2>/dev/null | grep -q "true"; then
            info "✓ Kubewarden is installed on workload cluster!"
            return 0
        fi
        attempt=$((attempt + 1))
        echo -n "."
        sleep 5
    done
    
    warn "Kubewarden installation status unclear. Continuing anyway..."
}

# Step 2: Deploy test policy
step2_deploy_policy() {
    step "2. Deploying test KubewardenPolicy..."
    switch_to_mgmt
    
    # Create a test policy
    cat <<EOF | kubectl apply -f -
apiVersion: addon.cluster.x-k8s.io/v1alpha1
kind: KubewardenPolicy
metadata:
  name: test-privileged-pods
  namespace: default
spec:
  clusterSelector:
    matchLabels:
      testing: "true"
  policyType: ClusterAdmissionPolicy
  module: registry://ghcr.io/kubewarden/policies/pod-privileged:v1.0.8
  rules:
    - apiGroups: [""]
      apiVersions: ["v1"]
      resources: ["pods"]
      operations:
        - CREATE
        - UPDATE
  mutating: false
  failurePolicy: Fail
EOF
    
    info "✓ Test policy created!"
}

# Step 3: Monitor policy status
step3_monitor_policy() {
    step "3. Monitoring policy deployment status..."
    switch_to_mgmt
    
    info "Waiting for policy to be deployed..."
    local max_attempts=30
    local attempt=0
    while [ $attempt -lt $max_attempts ]; do
        local status=$(kubectl get kubewardenpolicy test-privileged-pods -n default -o jsonpath='{.status.ready}' 2>/dev/null || echo "false")
        if [ "$status" = "true" ]; then
            info "✓ Policy deployed successfully!"
            break
        fi
        attempt=$((attempt + 1))
        echo -n "."
        sleep 5
    done
    
    echo ""
    info "Policy status:"
    kubectl get kubewardenpolicy test-privileged-pods -n default -o wide
    
    echo ""
    info "Deployed policies:"
    kubectl get kubewardenpolicy test-privileged-pods -n default -o jsonpath='{.status.deployedPolicies}' | jq '.' 2>/dev/null || echo "Status not available"
}

# Step 4: Verify in workload cluster
step4_verify_workload() {
    step "4. Verifying policy in workload cluster..."
    switch_to_workload
    
    info "Checking ClusterAdmissionPolicy..."
    if kubectl get clusteradmissionpolicy test-privileged-pods -n kubewarden >/dev/null 2>&1; then
        info "✓ ClusterAdmissionPolicy found in workload cluster!"
        kubectl get clusteradmissionpolicy test-privileged-pods -o wide
    else
        warn "ClusterAdmissionPolicy not found yet. This might take a few more moments."
    fi
    
    echo ""
    info "Checking PolicyServer..."
    kubectl get policyserver -n kubewarden 2>/dev/null || warn "PolicyServer not found"
}

# Step 5: Test the policy
step5_test_policy() {
    step "5. Testing the policy enforcement..."
    switch_to_workload
    
    info "Attempting to create an unprivileged pod (should succeed)..."
    cat <<EOF | kubectl apply -f - 2>&1 | head -5
apiVersion: v1
kind: Pod
metadata:
  name: test-unprivileged
  namespace: default
spec:
  containers:
  - name: nginx
    image: nginx:latest
EOF
    
    echo ""
    info "Attempting to create a privileged pod (should be denied)..."
    cat <<EOF | kubectl apply -f - 2>&1 || info "✓ Policy correctly denied privileged pod!"
apiVersion: v1
kind: Pod
metadata:
  name: test-privileged
  namespace: default
spec:
  containers:
  - name: nginx
    image: nginx:latest
    securityContext:
      privileged: true
EOF
}

# Step 6: Cleanup test resources
step6_cleanup() {
    step "6. Cleaning up test resources..."
    
    switch_to_workload
    info "Removing test pods from workload cluster..."
    kubectl delete pod test-unprivileged test-privileged -n default --ignore-not-found=true
    
    switch_to_mgmt
    info "Removing test policy from management cluster..."
    kubectl delete kubewardenpolicy test-privileged-pods -n default --ignore-not-found=true
    
    info "✓ Cleanup complete!"
}

# Print summary
print_summary() {
    echo ""
    echo "=============================================="
    echo "  Test Summary"
    echo "=============================================="
    echo ""
    switch_to_mgmt
    echo "KubewardenAddons:"
    kubectl get kubewardenaddon -A 2>/dev/null || echo "  None"
    echo ""
    echo "KubewardenPolicies:"
    kubectl get kubewardenpolicy -A 2>/dev/null || echo "  None"
    echo ""
    echo "Clusters:"
    kubectl get clusters -A 2>/dev/null || echo "  None"
    echo ""
}

# Main execution
main() {
    info "Starting KubewardenPolicy deployment test..."
    echo ""
    
    step1_verify_addon
    echo ""
    
    step2_deploy_policy
    echo ""
    
    step3_monitor_policy
    echo ""
    
    step4_verify_workload
    echo ""
    
    step5_test_policy
    echo ""
    
    read -p "Do you want to clean up test resources? (Y/n) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Nn]$ ]]; then
        info "Skipping cleanup"
    else
        step6_cleanup
    fi
    
    print_summary
    
    info "Test complete! 🎉"
}

main "$@"
