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

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// KubewardenAddonSpec defines the desired state of KubewardenAddon.
type KubewardenAddonSpec struct {
	// ClusterSelector selects Clusters in the same namespace with a label that matches the specified label selector. The Kubewarden
	// will be installed on all selected Clusters.
	ClusterSelector metav1.LabelSelector `json:"clusterSelector"`

	// Version specifies the version of Kubewarden to deploy. If it is not specified, kubewarden will use
	// and be kept up to date with the latest version.
	// +optional
	Version string `json:"version,omitempty"`

	// ImageRepository specifies the repository for pulling Kubewarden images.
	ImageRepository string `json:"imageRepository,omitempty"`

	// PolicyServerConfig holds configuration for the policy server.
	PolicyServerConfig PolicyServerConfig `json:"policyServerConfig"`
}

// PolicyServerConfig represents the configuration options for the policy server.
type PolicyServerConfig struct {
	// Resources defines the CPU and memory resources for the policy server.
	Resources ResourceRequirements `json:"resources,omitempty"`

	// Replicas specifies the number of replicas for high availability.
	Replicas int32 `json:"replicas,omitempty"`
}

// ResourceRequirements defines CPU and memory resource limits and requests.
type ResourceRequirements struct {
	// CPU request for the policy server.
	CPU string `json:"cpu,omitempty"`

	// Memory request for the policy server.
	Memory string `json:"memory,omitempty"`
}

// KubewardenAddonStatus defines the observed state of KubewardenAddon.
type KubewardenAddonStatus struct {
	// Ready indicates whether the addon is successfully deployed.
	Ready bool `json:"ready"`

	// Conditions defines current state of the KubewardenAddon.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`

	// MatchingClusters is the list of references to Clusters selected by the ClusterSelector.
	// +optional
	MatchingClusters []corev1.ObjectReference `json:"matchingClusters"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// KubewardenAddon is the Schema for the kubewardenaddons API.
type KubewardenAddon struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KubewardenAddonSpec   `json:"spec,omitempty"`
	Status KubewardenAddonStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KubewardenAddonList contains a list of KubewardenAddon.
type KubewardenAddonList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubewardenAddon `json:"items"`
}

// GetConditions returns the list of conditions for an KubewardenAddon API object.
func (c *KubewardenAddon) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

// SetConditions will set the given conditions on an KubewardenAddon object.
func (c *KubewardenAddon) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}

// SetMatchingClusters will set the given list of matching clusters on an KubewardenAddon object.
func (c *KubewardenAddon) SetMatchingClusters(clusterList []clusterv1.Cluster) {
	matchingClusters := make([]corev1.ObjectReference, 0, len(clusterList))
	for _, cluster := range clusterList {
		matchingClusters = append(matchingClusters, corev1.ObjectReference{
			Kind:       cluster.Kind,
			APIVersion: cluster.APIVersion,
			Name:       cluster.Name,
			Namespace:  cluster.Namespace,
		})
	}

	c.Status.MatchingClusters = matchingClusters
}

func init() {
	SchemeBuilder.Register(&KubewardenAddon{}, &KubewardenAddonList{})
}
