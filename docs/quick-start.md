## Getting started

This section will guide you on how to install the provider and get started with an example.

You can follow along with this guide by creating your local Kubernetes cluster using a tool like [Kind](https://kind.sigs.k8s.io/).

### Installing CAAPKW

#### Pre-requisites

CAAPKW extends the Cluster API functionality so your cluster must be initialized as a CAPI Management Cluster. To do this, you can use [clusterctl](https://cluster-api.sigs.k8s.io/clusterctl/overview), a CLI tool that interacts with CAPI and handles the lifecycle of a CAPI Management Cluster, including installing and configuring providers and using existing provider sample templates.

We recommend you follow the [official guide](https://cluster-api.sigs.k8s.io/user/quick-start#install-clusterctl) on how to install `clusterctl`.

Any provider that implements the Provider Contract can be deployed via `clusterctl`. By default, `clusterctl` ships with providers sponsored by SIG Cluster Lifecycle. You can use `clusterctl config repositories` to get a list of supported providers and their repository configuration.

In the case of CAAPKW, we need to customize the list of available providers using the `clusterctl` configuration file (you can find your own in `$HOME/.cluster-api/clusterctl.yaml`), as shown in the following example:

```
providers:
  - name: "caapkw"
    url: "https://github.com/caapkw/cluster-api-addon-provider-kubewarden/releases/latest/addon-components.yaml"
    type: "AddonProvider"
```

This configuration will tell `clusterctl` to look for `caapkw` add-on provider in the specified URL. You can validate that the changes have been applied by running `clusterctl config repositories` again and checking that `caapkw` has been added to the list.

#### Initializing the Management Cluster

We will be provisioning an Azure AKS cluster for this example, but you are free to select your preferred infrastructure provider and most instructions will apply. Let's initialize the CAPI Management Cluster with `capz` and `caapkw`: this will install core CAPI and both providers.

The Cluster API Provider for Azure requires you to authenticate against Azure which you do by passing the following environment variables. Remember to replace placeholders with your own values:
```
export AZURE_SUBSCRIPTION_ID="<SubscriptionId>"

# Create an Azure Service Principal and paste the output here
export AZURE_TENANT_ID="<Tenant>"
export AZURE_CLIENT_ID="<AppId>"
export AZURE_CLIENT_ID_USER_ASSIGNED_IDENTITY=$AZURE_CLIENT_ID # for compatibility with CAPZ v1.16 templates
export AZURE_CLIENT_SECRET="<Password>"

# Settings needed for AzureClusterIdentity used by the AzureCluster
export AZURE_CLUSTER_IDENTITY_SECRET_NAME="cluster-identity-secret"
export CLUSTER_IDENTITY_NAME="cluster-identity"
export AZURE_CLUSTER_IDENTITY_SECRET_NAMESPACE="default"

# Additionally, the Machine Pool feature must be enabled
export EXP_MACHINE_POOL=true
```

Now you can create the secret that contains this data:
```
kubectl create secret generic "${AZURE_CLUSTER_IDENTITY_SECRET_NAME}" --from-literal=clientSecret="${AZURE_CLIENT_SECRET}" --namespace "${AZURE_CLUSTER_IDENTITY_SECRET_NAMESPACE}"
```

Let's initialize the CAPI Management Cluster:
```
clusterctl init --infrastructure azure --addon caapkw
```

Take some time to inspect the new controllers that have been created in the cluster.

### Provisioning a cluster

In this example we will create an Azure AKS cluster with a simplified configuration.

```
apiVersion: cluster.x-k8s.io/v1beta1
kind: Cluster
metadata:
  name: ${CLUSTER_NAME}
  namespace: default
spec:
  clusterNetwork:
    services:
      cidrBlocks:
      - 192.168.0.0/16
  controlPlaneRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
    kind: AzureManagedControlPlane
    name: ${CLUSTER_NAME}
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
    kind: AzureManagedCluster
    name: ${CLUSTER_NAME}
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: AzureManagedControlPlane
metadata:
  name: ${CLUSTER_NAME}
  namespace: default
spec:
  identityRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
    kind: AzureClusterIdentity
    name: ${CLUSTER_IDENTITY_NAME}
  location: ${AZURE_LOCATION}
  oidcIssuerProfile:
    enabled: true
  resourceGroupName: ${AZURE_RESOURCE_GROUP}
  sshPublicKey: ${AZURE_SSH_PUBLIC_KEY_B64}
  subscriptionID: ${AZURE_SUBSCRIPTION_ID}
  version: ${KUBERNETES_VERSION}
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: AzureManagedCluster
metadata:
  name: ${CLUSTER_NAME}
  namespace: default
---
apiVersion: cluster.x-k8s.io/v1beta1
kind: MachinePool
metadata:
  name: ${CLUSTER_NAME}-pool0
  namespace: default
spec:
  clusterName: ${CLUSTER_NAME}
  replicas: ${WORKER_MACHINE_COUNT}
  template:
    metadata: {}
    spec:
      bootstrap:
        dataSecretName: ""
      clusterName: ${CLUSTER_NAME}
      infrastructureRef:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
        kind: AzureManagedMachinePool
        name: ${CLUSTER_NAME}-pool0
      version: ${KUBERNETES_VERSION}
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: AzureManagedMachinePool
metadata:
  name: ${CLUSTER_NAME}-pool0
  namespace: default
spec:
  mode: System
  name: pool0
  sku: ${AZURE_NODE_MACHINE_TYPE}
---
apiVersion: cluster.x-k8s.io/v1beta1
kind: MachinePool
metadata:
  name: ${CLUSTER_NAME}-pool1
  namespace: default
spec:
  clusterName: ${CLUSTER_NAME}
  replicas: ${WORKER_MACHINE_COUNT}
  template:
    metadata: {}
    spec:
      bootstrap:
        dataSecretName: ""
      clusterName: ${CLUSTER_NAME}
      infrastructureRef:
        apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
        kind: AzureManagedMachinePool
        name: ${CLUSTER_NAME}-pool1
      version: ${KUBERNETES_VERSION}
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: AzureManagedMachinePool
metadata:
  name: ${CLUSTER_NAME}-pool1
  namespace: default
spec:
  mode: User
  name: pool1
  sku: ${AZURE_NODE_MACHINE_TYPE}
---
apiVersion: infrastructure.cluster.x-k8s.io/v1beta1
kind: AzureClusterIdentity
metadata:
  labels:
    clusterctl.cluster.x-k8s.io/move-hierarchy: "true"
  name: ${CLUSTER_IDENTITY_NAME}
  namespace: default
spec:
  allowedNamespaces: {}
  clientID: ${AZURE_CLIENT_ID}
  clientSecret:
    name: ${AZURE_CLUSTER_IDENTITY_SECRET_NAME}
    namespace: ${AZURE_CLUSTER_IDENTITY_SECRET_NAMESPACE}
  tenantID: ${AZURE_TENANT_ID}
  type: ServicePrincipal
```

As you can tell, there are multiple parameterized variables. Most of these variables you have already exported before initializing the Azure provider. You will need to set other cluster configuration values, like cluster name, machine counts, machine type, etc.

With all the required variables exported, you can use a tool like `envsubst` to automatically apply substitution on the template:
```
envsubst < cluster-template.yaml >> output-cluster-template.yaml
```

Applying the resulting cluster template will start the provisioning of an AKS cluster.

```
kubectl apply -f output-cluster-template.yaml
```

### Creating a KubewardenAddon

While we now have the CAPI cluster, the CAAPKW controller reconciles the custom resource definition `KubewardenAddon`. At this stage of the project, fields in this resource are only placeholders but you need to create an object to trigger reconciliation.

```
apiVersion: addon.cluster.x-k8s.io/v1alpha1
kind: KubewardenAddon
metadata:
  labels:
    app.kubernetes.io/name: cluster-api-addon-provider-kubewarden
    app.kubernetes.io/managed-by: kustomize
  name: kubewardenaddon-sample
spec:
  version: ""
  imageRepository: ghcr.io/kubewarden/kubewarden-controller
  clusterSelector:
    matchLabels:
      environment: production
    matchExpressions:
      - { key: tier, operator: In, values: [frontend, backend] }
  policyServerConfig: 
    replicas: 1
    resources:
      cpu: 100m
      memory: 128Mi
```

### Verifying workload cluster configuration

The CAAPKW controller will start printing out logs and you can follow along with the add-on provider logic.

* Remember that the cluster's control plane must be ready before CAAPKW installs Kubewarden.

Finally, you can inspect your CAPI cluster and verify that Kubewarden is installed and `kubewarden-controller` is running. Now it's time to start enforcing policies!
