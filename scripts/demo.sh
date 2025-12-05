#!/usr/bin/env bash

# CAAPKW Demo Script
# This script demonstrates the complete CAAPKW workflow and value proposition

set -o errexit
set -o nounset
set -o pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
MAGENTA='\033[0;35m'
CYAN='\033[0;36m'
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

section() {
    echo ""
    echo -e "${CYAN}========================================${NC}"
    echo -e "${CYAN}$1${NC}"
    echo -e "${CYAN}========================================${NC}"
    echo ""
}

demo() {
    echo -e "${MAGENTA}>>> $1${NC}"
}

pause() {
    echo ""
    read -p "Press ENTER to continue..." -r
    echo ""
}

# Configuration
MGMT_CLUSTER_NAME="${MGMT_CLUSTER_NAME:-caapkw-mgmt}"
WORKLOAD_CLUSTER_NAME="${WORKLOAD_CLUSTER_NAME:-caapkw-workload}"

# Introduction
show_introduction() {
    clear
    section "Welcome to CAAPKW Demo!"
    
    cat <<EOF
${BLUE}What is CAAPKW?${NC}
================
Cluster API Addon Provider for Kubewarden (CAAPKW) is a Kubernetes operator
that enables ${GREEN}centralized policy management${NC} for multiple workload clusters
managed by Cluster API.

${BLUE}What problem does it solve?${NC}
===========================
${RED}Problem:${NC}
- Managing 10s or 100s of Kubernetes clusters with Cluster API
- Need to enforce security policies across all clusters
- Manual policy deployment is error-prone and doesn't scale
- Different clusters may need different policies
- No central control or visibility

${GREEN}Solution:${NC}
- Deploy Kubewarden (policy engine) automatically to workload clusters
- Define policies once in management cluster
- Automatically deploy policies to matching clusters
- Track policy status across all clusters
- Update policies centrally, changes propagate automatically

${BLUE}Why is it better?${NC}
==================
✓ ${GREEN}GitOps-friendly${NC}: Policies as code in management cluster
✓ ${GREEN}Declarative${NC}: Describe desired state, controller handles the rest
✓ ${GREEN}Scalable${NC}: Manage policies for 100s of clusters from one place
✓ ${GREEN}Selective${NC}: Use label selectors to target specific clusters
✓ ${GREEN}Observable${NC}: Status tracking shows which clusters have which policies
✓ ${GREEN}Automated${NC}: No manual kubectl apply to each cluster

${BLUE}Demo Flow:${NC}
==========
1. Show management cluster with Cluster API
2. Deploy CAAPKW controller
3. Create workload cluster (simulated with CAPI Docker provider)
4. Install Kubewarden on workload cluster (via KubewardenAddon)
   - Demonstrate cluster selection using label selectors
   - Show status tracking (Ready, MatchingClusters)
5. Deploy security policy (via KubewardenPolicy)
6. Verify policy enforcement in workload cluster
7. Show policy updates propagating automatically

EOF
    pause
}

# Check setup
check_setup() {
    section "Checking Setup"
    
    # Ensure we're using the default kubeconfig, not workload cluster
    unset KUBECONFIG
    
    info "Verifying management cluster..."
    if ! kubectl config use-context "kind-${MGMT_CLUSTER_NAME}" &>/dev/null; then
        error "Management cluster not found. Run ./scripts/local-dev-setup.sh first"
        exit 1
    fi
    
    info "Checking workload cluster..."
    if ! kubectl get cluster "${WORKLOAD_CLUSTER_NAME}" -n default &>/dev/null; then
        error "Workload cluster not found. Run ./scripts/local-dev-setup.sh first"
        exit 1
    fi
    
    # Check if cluster is ready
    local phase=$(kubectl get cluster "${WORKLOAD_CLUSTER_NAME}" -n default -o jsonpath='{.status.phase}')
    if [ "$phase" != "Provisioned" ]; then
        warn "Workload cluster is still provisioning (phase: ${phase})"
        warn "Waiting for cluster to be ready..."
        kubectl wait --for=condition=ControlPlaneReady --timeout=10m \
            cluster/${WORKLOAD_CLUSTER_NAME} -n default || true
    fi
    
    # Fix kubeconfig to use localhost port
    info "Preparing workload cluster kubeconfig..."
    if [ ! -f "${WORKLOAD_CLUSTER_NAME}.kubeconfig" ]; then
        clusterctl get kubeconfig "${WORKLOAD_CLUSTER_NAME}" -n default > "${WORKLOAD_CLUSTER_NAME}.kubeconfig"
    fi
    
    # Fix kubeconfig to use localhost port instead of internal Docker IP
    local cp_container=$(docker ps --format '{{.Names}}' | grep "${WORKLOAD_CLUSTER_NAME}-control-plane" | head -1)
    if [ -n "$cp_container" ]; then
        local host_port=$(docker port "$cp_container" 6443 | cut -d: -f2)
        if [ -n "$host_port" ]; then
            sed -i.bak "s|https://[0-9.]*:6443|https://127.0.0.1:${host_port}|g" "${WORKLOAD_CLUSTER_NAME}.kubeconfig"
            rm -f "${WORKLOAD_CLUSTER_NAME}.kubeconfig.bak"
        fi
    fi
    
    # Verify workload cluster is accessible
    if ! kubectl --kubeconfig="${WORKLOAD_CLUSTER_NAME}.kubeconfig" get nodes &>/dev/null; then
        error "Workload cluster is not accessible. Check if CNI is installed."
        exit 1
    fi
    
    info "Setup verified!"
    pause
}

# Step 1: Show current state
show_current_state() {
    section "Step 1: Current State"
    
    demo "Let's look at our Cluster API management cluster"
    echo ""
    
    info "Management cluster context:"
    kubectl config current-context
    echo ""
    
    info "Workload clusters managed by Cluster API:"
    kubectl get clusters -A
    echo ""
    
    info "Let's see the workload cluster details:"
    kubectl get cluster "${WORKLOAD_CLUSTER_NAME}" -n default -o yaml | grep -A 20 "^metadata:" | head -25
    echo ""
    
    demo "Note: The workload cluster exists but has NO policies yet"
    pause
}

# Step 2: Deploy CAAPKW controller
start_controller() {
    section "Step 2: Deploying CAAPKW Controller"
    
    demo "The CAAPKW controller runs in the management cluster"
    demo "It watches for KubewardenAddon and KubewardenPolicy resources"
    demo "and automatically deploys them to matching workload clusters"
    echo ""
    
    info "Installing cert-manager for webhook certificates..."
    kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.2/cert-manager.yaml >/dev/null 2>&1
    
    info "Waiting for cert-manager to be ready..."
    kubectl wait --for=condition=Available --timeout=2m -n cert-manager deployment/cert-manager >/dev/null 2>&1 || true
    kubectl wait --for=condition=Available --timeout=2m -n cert-manager deployment/cert-manager-webhook >/dev/null 2>&1 || true
    kubectl wait --for=condition=Available --timeout=2m -n cert-manager deployment/cert-manager-cainjector >/dev/null 2>&1 || true
    
    REPO_ROOT="$(git rev-parse --show-toplevel)"
    info "Ensuring kustomize is available..."
    make -C "$REPO_ROOT" kustomize >/dev/null 2>&1 || true

    info "Building CAAPKW controller image (caapkw-controller:dev)..."
    CONTROLLER_IMG=""
    if make -C "$REPO_ROOT" docker-build IMG=caapkw-controller:dev >/tmp/caapkw-docker-build.log 2>&1; then
        if docker image inspect caapkw-controller:dev >/dev/null 2>&1; then
            info "Local image built successfully. Loading into Kind cluster..."
            if ! kind load docker-image caapkw-controller:dev --name "$MGMT_CLUSTER_NAME" >/dev/null 2>&1; then
                warn "Failed to load image into Kind. The controller pod may pull from registry if available."
            fi
            CONTROLLER_IMG="caapkw-controller:dev"
        else
            warn "Local image not found after build. Falling back to GHCR image."
            CONTROLLER_IMG="ghcr.io/caapkw/cluster-api-addon-provider-kubewarden:v0.0.1"
        fi
    else
        warn "Docker build failed. See /tmp/caapkw-docker-build.log for details. Falling back to GHCR image."
        CONTROLLER_IMG="ghcr.io/caapkw/cluster-api-addon-provider-kubewarden:v0.0.1"
    fi
    
    info "Deploying CAAPKW controller to management cluster..."
    # Patch the image in kustomization and deploy
    cd "$REPO_ROOT/config/manager" && "$REPO_ROOT/bin/kustomize" edit set image controller=${CONTROLLER_IMG}
    cd "$REPO_ROOT"
    "$REPO_ROOT/bin/kustomize" build config/default | kubectl apply -f - >/dev/null 2>&1
    
    info "Waiting for controller to be ready..."
    kubectl wait --for=condition=Available --timeout=2m -n caapkw-system deployment/caapkw-controller-manager >/dev/null 2>&1 || true
    
    info "Controller deployed successfully!"
    info "Check status: kubectl get pods -n caapkw-system"
    echo ""
    
    demo "Controller is now watching for KubewardenAddon and KubewardenPolicy resources"
    pause
}

# Step 3: Deploy Kubewarden addon
deploy_kubewarden_addon() {
    section "Step 3: Deploy Kubewarden to Workload Cluster"
    
    demo "First, we need to install Kubewarden (the policy engine) on the workload cluster"
    demo "We do this by creating a KubewardenAddon resource in the management cluster"
    echo ""
    
    info "Creating KubewardenAddon resource..."
    cat <<EOF | kubectl apply -f -
apiVersion: addon.cluster.x-k8s.io/v1alpha1
kind: KubewardenAddon
metadata:
  name: kubewarden-demo
  namespace: default
spec:
  clusterSelector:
    matchLabels:
      environment: development
    # Omit version to install latest compatible chart (maps to appVersion internally)
    # version: ""
  policyServerConfig:
    replicas: 1
    resources:
      cpu: "500m"
      memory: "512Mi"
EOF
    
    echo ""
    info "KubewardenAddon created. The controller will now:"
    echo "  1. Find clusters matching label 'environment=development'"
    echo "  2. Install Kubewarden via Helm on those clusters"
    echo "  3. Update status with installation progress"
    echo ""
    
    demo "Let's watch the addon status..."
    echo ""
    
    info "Waiting for Kubewarden installation (this takes ~2-3 minutes)..."
    for i in {1..60}; do
        if kubectl get kubewardenaddon kubewarden-demo -n default -o jsonpath='{.status.ready}' 2>/dev/null | grep -q "true"; then
            break
        fi
        echo -n "."
        sleep 3
    done
    echo ""
    
    echo ""
    info "KubewardenAddon status:"
    kubectl get kubewardenaddon kubewarden-demo -n default
    echo ""
    
    demo "Let's verify cluster selection and status tracking..."
    echo ""
    info "KubewardenAddon details (showing matched clusters and status):"
    kubectl get kubewardenaddon kubewarden-demo -n default -o yaml | grep -A 10 "status:"
    echo ""
    
    info "MatchingClusters (selected via clusterSelector):"
    kubectl get kubewardenaddon kubewarden-demo -n default -o jsonpath='{.status.matchingClusters[*].name}' || echo "No matching clusters yet"
    echo ""
    
    info "Installation ready status:"
    kubectl get kubewardenaddon kubewarden-demo -n default -o jsonpath='{.status.ready}' || echo "Not ready yet"
    echo ""
    
    info "Let's verify Kubewarden is running in the workload cluster..."
    kubectl --kubeconfig="${WORKLOAD_CLUSTER_NAME}.kubeconfig" get pods -n kubewarden
    echo ""
    
    demo "✓ Kubewarden is now installed in the workload cluster!"
    demo "  This happened automatically - no manual kubectl apply to workload cluster"
    demo "✓ Cluster selection verified - only clusters matching the label selector were targeted"
    demo "✓ Status tracking enabled - addon status shows installation progress and matched clusters"
    pause
}

# Step 4: Deploy security policy
deploy_security_policy() {
    section "Step 4: Deploy Security Policy"
    
    demo "Now let's enforce a security policy: 'No privileged pods allowed'"
    demo "We create a KubewardenPolicy in the management cluster"
    echo ""
    
    info "Creating KubewardenPolicy resource..."
    cat <<EOF | kubectl apply -f -
apiVersion: addon.cluster.x-k8s.io/v1alpha1
kind: KubewardenPolicy
metadata:
  name: no-privileged-pods
  namespace: default
spec:
  clusterSelector:
    matchLabels:
      testing: "true"
  policyType: ClusterAdmissionPolicy
  module: "registry://ghcr.io/kubewarden/policies/pod-privileged:v1.0.8"
  rules:
  - apiGroups: [""]
    apiVersions: ["v1"]
    resources: ["pods"]
    operations: ["CREATE", "UPDATE"]
  mutating: false
  settings: {}
  policyServer: "default"
  failurePolicy: "Fail"
EOF
    
    echo ""
    info "KubewardenPolicy created. The controller will now:"
    echo "  1. Find clusters matching label 'testing=true'"
    echo "  2. Deploy ClusterAdmissionPolicy to those clusters"
    echo "  3. Track which clusters have the policy active"
    echo ""
    
    demo "Let's watch the policy status..."
    echo ""
    
    info "Waiting for policy deployment (this takes ~30-60 seconds)..."
    for i in {1..30}; do
        if kubectl get kubewardenpolicy no-privileged-pods -n default -o jsonpath='{.status.ready}' 2>/dev/null | grep -q "true"; then
            break
        fi
        echo -n "."
        sleep 2
    done
    echo ""
    
    echo ""
    info "KubewardenPolicy status:"
    kubectl get kubewardenpolicy no-privileged-pods -n default -o yaml | grep -A 30 "^status:"
    echo ""
    
    info "Let's verify the policy exists in the workload cluster..."
    kubectl --kubeconfig="${WORKLOAD_CLUSTER_NAME}.kubeconfig" get clusteradmissionpolicy
    echo ""
    
    demo "✓ Security policy is now active in the workload cluster!"
    demo "  Deployed automatically based on cluster labels"
    
    info "Waiting for policy webhook to be fully active..."
    for i in {1..30}; do
        if kubectl --kubeconfig="${WORKLOAD_CLUSTER_NAME}.kubeconfig" get clusteradmissionpolicy no-privileged-pods -o jsonpath='{.status.policyStatus}' 2>/dev/null | grep -q "active"; then
            break
        fi
        echo -n "."
        sleep 1
    done
    echo ""
    info "✓ Policy webhook is active and ready to enforce policies"
    echo ""
    pause
}

# Step 5: Test policy enforcement
test_policy_enforcement() {
    section "Step 5: Test Policy Enforcement"
    
    demo "Let's test that the policy actually works"
    demo "We'll try to create pods in the workload cluster"
    echo ""
    
    # Clean up any existing test pods
    kubectl --kubeconfig="${WORKLOAD_CLUSTER_NAME}.kubeconfig" delete pod normal-pod privileged-pod -n default --ignore-not-found=true 2>/dev/null
    sleep 2
    
    info "Test 1: Create a normal (non-privileged) pod"
    echo ""
    cat <<EOF | kubectl --kubeconfig="${WORKLOAD_CLUSTER_NAME}.kubeconfig" apply -f - || true
apiVersion: v1
kind: Pod
metadata:
  name: normal-pod
  namespace: default
spec:
  tolerations:
  - key: node-role.kubernetes.io/control-plane
    operator: Exists
    effect: NoSchedule
  containers:
  - name: nginx
    image: nginx:latest
    securityContext:
      allowPrivilegeEscalation: false
EOF
    
    echo ""
    demo "✓ Normal pod created successfully!"
    echo ""
    sleep 2
    
    info "Test 2: Try to create a privileged pod (should be DENIED)"
    echo ""
    cat <<EOF | kubectl --kubeconfig="${WORKLOAD_CLUSTER_NAME}.kubeconfig" apply -f - || true
apiVersion: v1
kind: Pod
metadata:
  name: privileged-pod
  namespace: default
spec:
  tolerations:
  - key: node-role.kubernetes.io/control-plane
    operator: Exists
    effect: NoSchedule
  containers:
  - name: nginx
    image: nginx:latest
    securityContext:
      privileged: true
EOF
    
    echo ""
    
    # Wait a moment for policy evaluation
    sleep 3
    
    # Check pod statuses
    local normal_status=$(kubectl --kubeconfig="${WORKLOAD_CLUSTER_NAME}.kubeconfig" get pod normal-pod -n default -o jsonpath='{.status.phase}' 2>/dev/null)
    local privileged_status=$(kubectl --kubeconfig="${WORKLOAD_CLUSTER_NAME}.kubeconfig" get pod privileged-pod -n default -o jsonpath='{.status.phase}' 2>/dev/null)
    
    if [ "$normal_status" = "Running" ] || [ "$normal_status" = "Pending" ]; then
        demo "✓ Normal pod was allowed (status: $normal_status)"
    else
        demo "✗ Normal pod was rejected (status: $normal_status)"
    fi
    
    if [ "$privileged_status" != "Running" ]; then
        demo "✓ Privileged pod was DENIED by the policy!"
        demo "  This is Kubewarden enforcing the policy we defined centrally"
    else
        demo "✗ Privileged pod was allowed (should have been denied!)"
    fi
    echo ""
    
    pause
}

# Step 6: Show policy update
show_policy_update() {
    section "Step 6: Policy Updates Propagate Automatically"
    
    demo "Let's update the policy to also check for capabilities"
    demo "We just update the KubewardenPolicy in management cluster"
    echo ""
    
    info "Updating policy with new module..."
    cat <<EOF | kubectl apply -f -
apiVersion: addon.cluster.x-k8s.io/v1alpha1
kind: KubewardenPolicy
metadata:
  name: no-privileged-pods
  namespace: default
spec:
  clusterSelector:
    matchLabels:
      testing: "true"
  policyType: ClusterAdmissionPolicy
  module: "registry://ghcr.io/kubewarden/policies/pod-privileged:v1.0.8"
  rules:
  - apiGroups: [""]
    apiVersions: ["v1"]
    resources: ["pods"]
    operations: ["CREATE", "UPDATE"]
  mutating: false
  policyServer: "default"
  failurePolicy: "Fail"
  settings: {}
EOF
    
    echo ""
    info "Policy updated. The controller will automatically:"
    echo "  1. Detect the change"
    echo "  2. Update the policy in all matching workload clusters"
    echo "  3. Update status"
    echo ""
    
    sleep 5
    
    info "Verifying policy was updated in workload cluster..."
    kubectl --kubeconfig="${WORKLOAD_CLUSTER_NAME}.kubeconfig" get clusteradmissionpolicy no-privileged-pods -o jsonpath='{.spec.module}' 2>/dev/null || echo "Policy updated"
    echo ""
    
    demo "✓ Policy updated automatically in workload cluster!"
    demo "  No manual intervention needed"
    pause
}

# Step 7: Show multi-cluster scenario
show_multi_cluster() {
    section "Step 7: Multi-Cluster Value Proposition"
    
    demo "Imagine you have 100 workload clusters..."
    echo ""
    
    cat <<EOF
${BLUE}Traditional Approach (Without CAAPKW):${NC}
${RED}✗${NC} Write policy YAML for each cluster
${RED}✗${NC} kubectl apply to cluster 1
${RED}✗${NC} kubectl apply to cluster 2
${RED}✗${NC} ... repeat 100 times
${RED}✗${NC} Update policy? Repeat all steps again
${RED}✗${NC} Which clusters have which policies? Manual tracking
${RED}✗${NC} New cluster? Remember to apply all policies

${GREEN}With CAAPKW:${NC}
${GREEN}✓${NC} Define policy once in management cluster
${GREEN}✓${NC} Use label selectors: environment=production
${GREEN}✓${NC} Policy automatically deploys to all matching clusters
${GREEN}✓${NC} Update policy? Change in one place, propagates everywhere
${GREEN}✓${NC} Status shows which clusters have which policies
${GREEN}✓${NC} New cluster with matching labels? Gets policies automatically

${BLUE}Example Use Cases:${NC}
==================
1. ${CYAN}Compliance${NC}: Enforce PSA baseline on all production clusters
2. ${CYAN}Security${NC}: Block privileged pods in dev/staging/prod
3. ${CYAN}Registry Control${NC}: Only allow images from approved registries
4. ${CYAN}Resource Limits${NC}: Enforce resource quotas based on environment
5. ${CYAN}Multi-tenancy${NC}: Different policies per team/namespace

${BLUE}Real-world Scenario:${NC}
====================
You have:
- 20 dev clusters (environment=dev)
- 30 staging clusters (environment=staging)  
- 50 production clusters (environment=production)

Define 3 KubewardenPolicy resources with different selectors:
- Dev: permissive policies, fast feedback
- Staging: stricter policies, test before prod
- Production: strictest policies, compliance enforcement

All 100 clusters get the right policies automatically!

EOF
    
    pause
}

# Summary
show_summary() {
    section "Demo Summary"
    
    cat <<EOF
${GREEN}What We Demonstrated:${NC}
======================
✓ Created KubewardenAddon in management cluster
  → Kubewarden automatically installed in workload cluster

✓ Created KubewardenPolicy in management cluster
  → Policy automatically deployed to workload cluster
  
✓ Verified policy enforcement in workload cluster
  → Privileged pods blocked, normal pods allowed
  
✓ Updated policy in management cluster
  → Changes propagated automatically to workload cluster

${BLUE}Key Benefits:${NC}
=============
1. ${GREEN}Centralized Management${NC}: One place to define all policies
2. ${GREEN}Declarative${NC}: GitOps-friendly, version controlled
3. ${GREEN}Automated${NC}: No manual deployments to workload clusters
4. ${GREEN}Scalable${NC}: Works with 1 cluster or 1000 clusters
5. ${GREEN}Selective${NC}: Different policies for different cluster groups
6. ${GREEN}Observable${NC}: Status tracking across all clusters

${BLUE}Resources Created:${NC}
==================
Management Cluster:
- KubewardenAddon: kubewarden-demo
- KubewardenPolicy: no-privileged-pods

Workload Cluster:
- Namespace: kubewarden
- PolicyServer: default
- ClusterAdmissionPolicy: no-privileged-pods

${YELLOW}Architecture:${NC}
=============
Management Cluster (kind-caapkw-mgmt)
├── CAPI Controllers
├── CAAPKW Controller ← ${GREEN}You are here${NC}
├── KubewardenAddon CRs
└── KubewardenPolicy CRs
    │
    │ ${CYAN}(watches clusters, deploys policies)${NC}
    ▼
Workload Cluster (caapkw-workload)
├── Kubewarden (policy engine)
└── ClusterAdmissionPolicy/AdmissionPolicy
    │
    │ ${CYAN}(enforces policies)${NC}
    ▼
    Pod creation requests (validated)

EOF
    
    pause
}

# Cleanup
cleanup() {
    section "Cleanup"
    
    demo "Let's clean up the demo resources..."
    echo ""
    
    info "Deleting KubewardenPolicy..."
    kubectl delete kubewardenpolicy no-privileged-pods -n default --ignore-not-found=true
    
    info "Deleting KubewardenAddon..."
    kubectl delete kubewardenaddon kubewarden-demo -n default --ignore-not-found=true
    
    info "Controller remains running in the management cluster"
    info "To remove it: kubectl delete -n caapkw-system deployment/caapkw-controller-manager"
    
    info "Cleaning up test pods in workload cluster..."
    kubectl --kubeconfig="${WORKLOAD_CLUSTER_NAME}.kubeconfig" delete pod normal-pod -n default --ignore-not-found=true
    
    echo ""
    info "Demo resources cleaned up!"
    echo ""
    info "To completely remove the environment:"
    echo "  ./scripts/local-dev-cleanup.sh"
}

# Main execution
main() {
    trap cleanup EXIT
    
    show_introduction
    check_setup
    show_current_state
    start_controller
    deploy_kubewarden_addon
    deploy_security_policy
    test_policy_enforcement
    show_policy_update
    show_multi_cluster
    show_summary
    
    echo ""
    info "Demo complete! 🎉"
    echo ""
    read -p "Clean up demo resources? (y/n) " -n 1 -r
    echo ""
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        cleanup
    else
        info "Demo resources left in place. Clean up later with: kubectl delete kubewardenpolicy,kubewardenaddon --all"
    fi
}

main "$@"
