/*
Copyright 2024 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

// KubewardenAddon Conditions and Reasons.
const (
	// KubewardenAddonSpecsUpToDateCondition indicates that the KubewardenAddon specs are up to date,
	// meaning that the KubewardenAddons are created/updated/deleted.
	KubewardenAddonSpecsUpToDateCondition clusterv1.ConditionType = "KubewardenAddonSpecsUpToDate"

	// KubewardenAddonSpecsUpdatingReason indicates that the KubewardenAddon entity is not yet updated by the corresponding controller.
	KubewardenAddonSpecsUpdatingReason = "KubewardenAddonSpecsUpdating"

	// KubewardenAddonCreationFailedReason indicates that the KubewardenAddon controller failed to create a KubewardenAddon.
	KubewardenAddonCreationFailedReason = "KubewardenAddonCreationFailed"

	// KubewardenAddonDeletionFailedReason indicates that the KubewardenAddon controller failed to delete a KubewardenAddon.
	KubewardenAddonDeletionFailedReason = "KubewardenAddonDeletionFailed"

	// KubewardenAddonReinstallingReason indicates that the KubewardenAddon controller is reinstalling a KubewardenAddon.
	KubewardenAddonReinstallingReason = "KubewardenAddonReinstalling"

	// ClusterSelectionFailedReason indicates that the KubewardenAddon controller failed to select the workload Clusters.
	ClusterSelectionFailedReason = "ClusterSelectionFailed"

	// KubewardenAddonsReadyCondition indicates that the KubewardenAddons are ready, meaning that the KubewardenAddon installation, upgrade
	// or deletion is complete.
	KubewardenAddonsReadyCondition clusterv1.ConditionType = "KubewardenAddonReady"
)
