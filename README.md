# CAPI Addon Provider Kubewarden - CAAPKW

<a href="https://cluster-api.sigs.k8s.io"><img alt="capi" src="./logos/kubernetes-cluster-logos_final-02.svg" width="160x" /></a>
<a href="https://docs.kubewarden.io/"><img alt="kubewarden" src="./logos/kubewarden.png" width="160x" /></a>
<p>
<a href="https://github.com/caapkw/cluster-api-addon-provider-kubewarden"><img src="https://godoc.org/sigs.k8s.io/cluster-api?status.svg"></a>
</p>

# Cluster API Add-on Provider for Kubewarden

### 👋 Welcome to CAAPKW! Here are some links to help you get started:

- [Quick start guide](./docs/quick-start.md)
- [Development guide](./docs/development.md)

## ✨ What is Cluster API Add-on Provider for Kubewarden?

Cluster API Add-on Provider for Kubewarden extends Cluster API by managing the installation and configuration of [Kubewarden](https://docs.kubewarden.io/) in CAPI clusters. Kubewarden (a CNCF Sandbox project) is a Kubernetes Policy Engine that aims to be the universal Policy Engine for Kubernetes.

Given a `KubewardenAddon` specification, CAAPKW manages the installation of the policy engine to CAPI workload clusters simplifying security compliance for your CAPI-provisioned clusters.

This project is a concrete implementation of a `ClusterAddonProvider`, a pluggable component to be deployed on the Management Cluster. You can read the proposal document for `ClusterAddonProvider` [here](https://github.com/kubernetes-sigs/cluster-api/blob/main/docs/proposals/20220712-cluster-api-addon-orchestration.md).
