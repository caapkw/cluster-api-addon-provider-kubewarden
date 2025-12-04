#!/usr/bin/env bash

# Cleanup script for local development environment

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

# Configuration
MGMT_CLUSTER_NAME="${MGMT_CLUSTER_NAME:-caapkw-mgmt}"
WORKLOAD_CLUSTER_NAME="${WORKLOAD_CLUSTER_NAME:-caapkw-workload}"

cleanup() {
    info "Cleaning up local development environment..."
    
    # Delete management cluster (this will also delete the workload cluster)
    if kind get clusters | grep -q "^${MGMT_CLUSTER_NAME}$"; then
        info "Deleting management cluster: ${MGMT_CLUSTER_NAME}"
        kind delete cluster --name "${MGMT_CLUSTER_NAME}"
    else
        warn "Management cluster '${MGMT_CLUSTER_NAME}' not found"
    fi
    
    # Clean up CAPD workload cluster Docker containers
    info "Cleaning up CAPD workload cluster containers..."
    local containers=$(docker ps -aq --filter "name=${WORKLOAD_CLUSTER_NAME}")
    if [ -n "$containers" ]; then
        info "Removing ${WORKLOAD_CLUSTER_NAME} containers..."
        echo "$containers" | xargs docker rm -f >/dev/null 2>&1 || true
    fi
    
    # Clean up kubeconfig files
    if [ -f "${WORKLOAD_CLUSTER_NAME}.kubeconfig" ]; then
        info "Removing kubeconfig file: ${WORKLOAD_CLUSTER_NAME}.kubeconfig"
        rm -f "${WORKLOAD_CLUSTER_NAME}.kubeconfig"
    fi
    
    info "Cleanup complete!"
}

# Confirm before cleanup
read -p "This will delete all local development clusters. Continue? (y/N) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    cleanup
else
    info "Cleanup cancelled"
fi
