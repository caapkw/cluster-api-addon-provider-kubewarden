package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KubewardenAddonSpec defines the desired state of KubewardenAddon.
type KubewardenAddonSpec struct {
	// Version specifies the version of Kubewarden to deploy.
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

	// Conditions represent the latest available observations of the addon state.
	Conditions []metav1.Condition `json:"conditions"`
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

func init() {
	SchemeBuilder.Register(&KubewardenAddon{}, &KubewardenAddonList{})
}
