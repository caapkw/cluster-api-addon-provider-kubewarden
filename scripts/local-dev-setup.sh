#!/usr/bin/env bash

# Local Development Setup Script for CAAPKW
# This script sets up a local Kind cluster with CAPI and CAAPKW for testing

set -o errexit
set -o nounset
set -o pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
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

# Configuration
MGMT_CLUSTER_NAME="${MGMT_CLUSTER_NAME:-caapkw-mgmt}"
WORKLOAD_CLUSTER_NAME="${WORKLOAD_CLUSTER_NAME:-caapkw-workload}"
CAPI_VERSION="${CAPI_VERSION:-v1.8.5}"

# Check prerequisites
check_prerequisites() {
    info "Checking prerequisites..."
    
    local missing=()
    
    command -v kind >/dev/null 2>&1 || missing+=("kind")
    command -v kubectl >/dev/null 2>&1 || missing+=("kubectl")
    command -v docker >/dev/null 2>&1 || missing+=("docker")
    command -v clusterctl >/dev/null 2>&1 || missing+=("clusterctl")
    
    if [ ${#missing[@]} -ne 0 ]; then
        error "Missing required tools: ${missing[*]}"
        echo ""
        echo "Install instructions:"
        echo "  kind:       https://kind.sigs.k8s.io/docs/user/quick-start/#installation"
        echo "  kubectl:    https://kubernetes.io/docs/tasks/tools/"
        echo "  clusterctl: https://cluster-api.sigs.k8s.io/user/quick-start.html#install-clusterctl"
        exit 1
    fi
    
    if ! docker info >/dev/null 2>&1; then
        error "Docker is not running. Please start Docker and try again."
        exit 1
    fi
    
    info "All prerequisites met!"
}

# Create management cluster
create_management_cluster() {
    info "Creating management cluster: ${MGMT_CLUSTER_NAME}"
    
    if kind get clusters | grep -q "^${MGMT_CLUSTER_NAME}$"; then
        warn "Management cluster '${MGMT_CLUSTER_NAME}' already exists. Skipping creation."
        return 0
    fi
    
    cat <<EOF | kind create cluster --name "${MGMT_CLUSTER_NAME}" --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraMounts:
  - hostPath: /var/run/docker.sock
    containerPath: /var/run/docker.sock
EOF
    
    info "Management cluster created successfully!"
}

# Initialize CAPI on management cluster
initialize_capi() {
    info "Initializing Cluster API on management cluster..."
    
    # Switch context to management cluster
    kubectl config use-context "kind-${MGMT_CLUSTER_NAME}"
    
    # Initialize CAPI with Docker provider (for local testing)
    export CLUSTER_TOPOLOGY=true
    clusterctl init --infrastructure docker
    
    # Wait for CAPI controllers to be ready
    info "Waiting for CAPI controllers to be ready..."
    kubectl wait --for=condition=Available --timeout=5m \
        -n capi-system deployment/capi-controller-manager
    kubectl wait --for=condition=Available --timeout=5m \
        -n capd-system deployment/capd-controller-manager
    
    # Wait for webhook endpoints to be ready
    info "Waiting for webhook services to be ready..."
    kubectl wait --for=condition=Ready --timeout=3m \
        -n capi-kubeadm-control-plane-system pod -l control-plane=controller-manager || true
    sleep 10  # Additional buffer for webhook registration
    
    info "Cluster API initialized successfully!"
}

# Install CRDs and controller
install_caapkw() {
    info "Installing CAAPKW CRDs and controller..."
    
    kubectl config use-context "kind-${MGMT_CLUSTER_NAME}"
    
    # Install CRDs
    info "Installing CRDs..."
    make -C "$(git rev-parse --show-toplevel)" install
    
    # Wait for CRDs to be established
    sleep 2
    
    info "CAAPKW CRDs installed successfully!"
    info "You can now run the controller locally with: make run"
}

# Build and load controller image
build_and_load_controller_image() {
    info "Building fresh Docker image for controller..."
    
    local repo_root=$(git rev-parse --show-toplevel)
    cd "${repo_root}"
    
    # Build the image (with automatic cache cleanup)
    make docker-build
    
    info "Loading controller image into Kind cluster: ${MGMT_CLUSTER_NAME}..."
    kind load docker-image ghcr.io/caapkw/cluster-api-addon-provider-kubewarden:v0.0.1 --name "${MGMT_CLUSTER_NAME}"
    
    info "Controller image built and loaded successfully!"
}

# Create a workload cluster using CAPI
create_workload_cluster() {
    info "Creating workload cluster: ${WORKLOAD_CLUSTER_NAME}"
    
    kubectl config use-context "kind-${MGMT_CLUSTER_NAME}"
    
    # Delete existing workload cluster if it exists (for fresh setup)
    if kubectl get cluster "${WORKLOAD_CLUSTER_NAME}" -n default >/dev/null 2>&1; then
        warn "Workload cluster '${WORKLOAD_CLUSTER_NAME}' already exists. Deleting it for fresh setup..."
        kubectl delete cluster "${WORKLOAD_CLUSTER_NAME}" -n default --ignore-not-found=true || true
        sleep 5  # Wait for deletion to complete
    fi
    
    # Create a simple workload cluster using CAPD (simplified config based on ClusterClass patterns)
    cat <<EOF | kubectl apply -f -
apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: ${WORKLOAD_CLUSTER_NAME}
  namespace: default
  labels:
    environment: development
    testing: "true"
spec:
  clusterNetwork:
    pods:
      cidrBlocks:
      - 192.168.0.0/16
    serviceDomain: cluster.local
    services:
      cidrBlocks:
      - 10.128.0.0/12
  controlPlaneRef:
    apiVersion: controlplane.cluster.x-k8s.io/v1beta1
    kind: KubeadmControlPlane
    name: ${WORKLOAD_CLUSTER_NAME}-control-plane
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
    kind: DockerCluster
    name: ${WORKLOAD_CLUSTER_NAME}
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: DockerCluster
metadata:
  name: ${WORKLOAD_CLUSTER_NAME}
  namespace: default
spec: {}
---
apiVersion: controlplane.cluster.x-k8s.io/v1beta1
kind: KubeadmControlPlane
metadata:
  name: ${WORKLOAD_CLUSTER_NAME}-control-plane
  namespace: default
spec:
  kubeadmConfigSpec:
    clusterConfiguration:
      apiServer:
        certSANs:
        - localhost
        - 127.0.0.1
        - 0.0.0.0
        - host.docker.internal
      controllerManager:
        extraArgs:
          enable-hostpath-provisioner: "true"
    initConfiguration:
      nodeRegistration: {}
    joinConfiguration:
      nodeRegistration: {}
  machineTemplate:
    infrastructureRef:
      apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
      kind: DockerMachineTemplate
      name: ${WORKLOAD_CLUSTER_NAME}-control-plane
  replicas: 1
  version: v1.30.0
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: DockerMachineTemplate
metadata:
  name: ${WORKLOAD_CLUSTER_NAME}-control-plane
  namespace: default
spec:
  template:
    spec:
      extraMounts:
      - containerPath: /var/run/docker.sock
        hostPath: /var/run/docker.sock
EOF
    
    info "Waiting for workload cluster to be ready (this may take a few minutes)..."
    kubectl wait --for=condition=Ready --timeout=10m \
        cluster/${WORKLOAD_CLUSTER_NAME} -n default || {
        warn "Cluster did not become ready within 10 minutes. Checking status..."
        kubectl get cluster,kubeadmcontrolplane,machine -n default
        kubectl describe machine -n default | tail -50
        return 1
    }
    
    info "Workload cluster created successfully!"
}

# Install CNI on workload cluster
install_cni() {
    info "Installing Calico CNI on workload cluster..."
    
    kubectl config use-context "kind-${MGMT_CLUSTER_NAME}"
    
    # Get the control plane container name (newest one, sorted by creation time)
    local cp_container=$(docker ps --filter "name=${WORKLOAD_CLUSTER_NAME}-control-plane" --format '{{.CreatedAt}}\t{{.Names}}' | sort -r | head -1 | awk '{print $NF}')
    
    if [ -z "$cp_container" ]; then
        error "Could not find control plane container for ${WORKLOAD_CLUSTER_NAME}"
        return 1
    fi
    
    info "Using control plane container: ${cp_container}"
    
    # Install Calico CNI directly via the control plane container
    info "Installing Calico CNI (this may take 30-60 seconds)..."
    # Download Calico manifest and apply with insecure flags
    docker exec -i "$cp_container" bash -c '
        curl -sfL https://raw.githubusercontent.com/projectcalico/calico/v3.26.1/manifests/calico.yaml | \
        kubectl --kubeconfig=/etc/kubernetes/admin.conf \
                --insecure-skip-tls-verify \
                apply --validate=false -f -
    ' >/dev/null 2>&1 || {
        # If that fails, try without TLS verification
        warn "First Calico installation attempt failed, retrying with different method..."
        docker exec "$cp_container" bash -c '
            kubectl --kubeconfig=/etc/kubernetes/admin.conf config set-cluster kubernetes --insecure-skip-tls-verify=true || true
            curl -sfL https://raw.githubusercontent.com/projectcalico/calico/v3.26.1/manifests/calico.yaml | \
            kubectl --kubeconfig=/etc/kubernetes/admin.conf apply -f - || true
        ' >/dev/null 2>&1
    }
    
    # Wait for nodes to be Ready
    info "Waiting for node to be Ready..."
    local max_attempts=30
    local attempt=0
    while [ $attempt -lt $max_attempts ]; do
        if docker exec "$cp_container" kubectl --kubeconfig=/etc/kubernetes/admin.conf get nodes | grep -q " Ready"; then
            info "Node is Ready!"
            break
        fi
        attempt=$((attempt + 1))
        sleep 2
    done
    
    if [ $attempt -eq $max_attempts ]; then
        warn "Node did not become Ready within expected time"
    fi
}

# Get kubeconfig for workload cluster
get_workload_kubeconfig() {
    info "Retrieving kubeconfig for workload cluster..."
    
    kubectl config use-context "kind-${MGMT_CLUSTER_NAME}"
    
    # Wait for kubeconfig secret to be available
    local max_attempts=30
    local attempt=0
    while [ $attempt -lt $max_attempts ]; do
        if kubectl get secret "${WORKLOAD_CLUSTER_NAME}-kubeconfig" -n default >/dev/null 2>&1; then
            break
        fi
        attempt=$((attempt + 1))
        sleep 10
    done
    
    # Get kubeconfig (make sure we're using the management cluster context)
    kubectl config use-context "kind-${MGMT_CLUSTER_NAME}"
    clusterctl get kubeconfig "${WORKLOAD_CLUSTER_NAME}" -n default > "${WORKLOAD_CLUSTER_NAME}.kubeconfig"
    
    # Fix kubeconfig to use localhost port instead of internal Docker IP
    # CAPD exposes API server on localhost with a random port
    local cp_container=$(docker ps --format '{{.Names}}' | grep "${WORKLOAD_CLUSTER_NAME}-control-plane" | head -1)
    if [ -n "$cp_container" ]; then
        local host_port=$(docker port "$cp_container" 6443 | cut -d: -f2)
        if [ -n "$host_port" ]; then
            # Replace internal Docker IP with localhost:port
            sed -i.bak "s|https://[0-9.]*:6443|https://127.0.0.1:${host_port}|g" "${WORKLOAD_CLUSTER_NAME}.kubeconfig"
            rm -f "${WORKLOAD_CLUSTER_NAME}.kubeconfig.bak"
            info "Kubeconfig updated to use localhost:${host_port}"
        fi
    fi
    
    info "Kubeconfig saved to: ${WORKLOAD_CLUSTER_NAME}.kubeconfig"
    info "You can access the workload cluster with: export KUBECONFIG=\$(pwd)/${WORKLOAD_CLUSTER_NAME}.kubeconfig"
}

# Print next steps
print_next_steps() {
    echo ""
    echo "=============================================="
    echo "  Local Development Setup Complete! 🎉"
    echo "=============================================="
    echo ""
    echo "Management cluster: ${MGMT_CLUSTER_NAME}"
    echo "Workload cluster:   ${WORKLOAD_CLUSTER_NAME}"
    echo ""
    echo "Next steps:"
    echo ""
    echo "1. Run the controller locally:"
    echo "   make run"
    echo ""
    echo "2. In another terminal, test KubewardenAddon:"
    echo "   kubectl apply -f config/samples/addon_v1alpha1_kubewardenaddon.yaml"
    echo ""
    echo "3. Watch the addon status:"
    echo "   kubectl get kubewardenaddon -w"
    echo ""
    echo "4. After Kubewarden is installed, test policies:"
    echo "   kubectl apply -f config/samples/addon_v1alpha1_kubewardenpolicy.yaml"
    echo ""
    echo "5. Watch the policy status:"
    echo "   kubectl get kubewardenpolicy -w"
    echo ""
    echo "6. Access workload cluster:"
    echo "   export KUBECONFIG=\$(pwd)/${WORKLOAD_CLUSTER_NAME}.kubeconfig"
    echo "   kubectl get pods -A"
    echo ""
    echo "Cleanup:"
    echo "   ./scripts/local-dev-cleanup.sh"
    echo ""
}

# Main execution
main() {
    info "Starting local development setup..."
    echo ""
    
    check_prerequisites
    create_management_cluster
    build_and_load_controller_image
    initialize_capi
    install_caapkw
    create_workload_cluster
    install_cni
    get_workload_kubeconfig
    
    print_next_steps
}

main "$@"
