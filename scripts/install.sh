#!/bin/bash

# Copyright 2024.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

echo "Deploying CAAPKW to cluster"

: "${NAMESPACE:=caapkw-system}"
: "${IMG:=ghcr.io/caapkw/controller}"
: "${TAG:=v0.0.0-local}"

BASEDIR=$(dirname "$0")

kind create cluster --config "$BASEDIR/kind-cluster-with-extramounts.yaml"

helm repo add jetstack https://charts.jetstack.io
helm repo update

export EXP_CLUSTER_RESOURCE_SET=true
export CLUSTER_TOPOLOGY=true

echo "Installing cert-manager first"
helm install cert-manager jetstack/cert-manager \
    --namespace cert-manager \
    --create-namespace \
    --set crds.enabled=true

echo "Generating manifests"
make manifests generate fmt vet

echo "Creating namespace $NAMESPACE"
kubectl create ns $NAMESPACE

echo "Installing CRDs"
make install

echo "Intializing as CAPI Management cluster"
kubectl apply -f $BASEDIR/features.yaml
helm install capi-operator capi-operator/cluster-api-operator \
    --create-namespace -n capi-operator-system \
    --set infrastructure=docker \
    --set core=cluster-api \
    --set controlPlane=kubeadm \
    --set bootstrap=kubeadm \
    --set configSecret.name=capi-variables \
    --set configSecret.namespace=default \
    --set manager.featureGates.core.ClusterTopology=true \
    --set manager.featureGates.core.MachinePool=true \
    --set manager.featureGates.docker.ClusterTopology=true \
    --set manager.featureGates.docker.MachinePool=true \
    --timeout 90s --wait

echo "Building docker image $IMG:$TAG"
make docker-build CONTROLLER_IMG=$IMG TAG=$TAG

echo "Deploying CAAPKW controller"
kind load docker-image $IMG:$TAG --name caapkw-dev
CONTROLLER_IMG=$IMG make deploy
