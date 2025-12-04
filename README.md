# CAPI Addon Provider Kubewarden - CAAPKW

<a href="https://cluster-api.sigs.k8s.io"><img alt="capi" src="./logos/kubernetes-cluster-logos_final-02.svg" width="160x" /></a>
<a href="https://docs.kubewarden.io/"><img alt="kubewarden" src="./logos/kubewarden.png" width="160x" /></a>
<p>
<a href="https://github.com/caapkw/cluster-api-addon-provider-kubewarden"><img src="https://godoc.org/sigs.k8s.io/cluster-api?status.svg"></a>
</p>

# Cluster API Add-on Provider for Kubewarden

### 👋 Welcome to CAAPKW! Here are some links to help you get started:

- [Quick start guide](./docs/quick-start.md)
- [Local development guide](./docs/local-development.md) - **Start here for testing!**
- [KubewardenPolicy user guide](./docs/kubewardenpolicy-guide.md)
- [Development guide](./docs/development.md)

## ✨ What is Cluster API Add-on Provider for Kubewarden?

Cluster API Add-on Provider for Kubewarden extends Cluster API by managing the installation and configuration of [Kubewarden](https://docs.kubewarden.io/) in CAPI clusters. Kubewarden (a CNCF Sandbox project) is a Kubernetes Policy Engine that aims to be the universal Policy Engine for Kubernetes.

CAAPKW provides two Custom Resources:

1. **`KubewardenAddon`** - Manages the installation of the Kubewarden policy engine to CAPI workload clusters
2. **`KubewardenPolicy`** - Manages the deployment and enforcement of Kubewarden policies (ClusterAdmissionPolicy or AdmissionPolicy) to selected workload clusters

This simplifies security compliance for your CAPI-provisioned clusters by automating both the infrastructure (policy engine) and the policies themselves.

This project is a concrete implementation of a `ClusterAddonProvider`, a pluggable component to be deployed on the Management Cluster. You can read the proposal document for `ClusterAddonProvider` [here](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20220712-cluster-api-addon-orchestration.md).

## 🚀 Features

- **Automated Kubewarden Installation**: Deploy Kubewarden policy engine to workload clusters with a single CRD
- **Policy Management**: Define and deploy policies centrally from the management cluster
- **Cluster Selection**: Use label selectors to target specific workload clusters
- **Support for Both Policy Types**: 
  - ClusterAdmissionPolicy (cluster-wide policies)
  - AdmissionPolicy (namespace-scoped policies)
- **Policy Configuration**: Full support for policy settings, rules, failure policies, and match conditions
- **Status Tracking**: Monitor policy deployment status across all workload clusters
