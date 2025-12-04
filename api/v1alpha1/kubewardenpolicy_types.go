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
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// KubewardenPolicySpec defines the desired state of KubewardenPolicy.
type KubewardenPolicySpec struct {
	// ClusterSelector selects Clusters in the same namespace with a label that matches the specified label selector.
	// The policy will be deployed to all selected Clusters.
	ClusterSelector metav1.LabelSelector `json:"clusterSelector"`

	// PolicyType specifies the type of policy to create.
	// Valid values are "ClusterAdmissionPolicy" (cluster-wide) and "AdmissionPolicy" (namespace-scoped).
	// +kubebuilder:validation:Enum=ClusterAdmissionPolicy;AdmissionPolicy
	// +kubebuilder:default=ClusterAdmissionPolicy
	PolicyType string `json:"policyType,omitempty"`

	// PolicyName is the name of the policy to create in the workload cluster.
	// If not specified, the name of the KubewardenPolicy resource will be used.
	// +optional
	PolicyName string `json:"policyName,omitempty"`

	// TargetNamespace is the namespace where the policy will be created in the workload cluster.
	// Only applicable when PolicyType is "AdmissionPolicy". For ClusterAdmissionPolicy, this field is ignored.
	// +optional
	TargetNamespace string `json:"targetNamespace,omitempty"`

	// PolicyServer identifies the PolicyServer that will serve this policy.
	// If not specified, the "default" PolicyServer will be used.
	// +optional
	// +kubebuilder:default=default
	PolicyServer string `json:"policyServer,omitempty"`

	// Module is the location of the Kubewarden policy.
	// Examples:
	//   - registry://ghcr.io/kubewarden/policies/pod-privileged:v1.0.8
	//   - https://github.com/kubewarden/pod-privileged-policy/releases/download/v0.2.2/policy.wasm
	Module string `json:"module"`

	// Rules define which Kubernetes resources and operations this policy applies to.
	Rules []PolicyRule `json:"rules"`

	// Mutating indicates whether this policy can mutate incoming requests.
	// +optional
	// +kubebuilder:default=false
	Mutating bool `json:"mutating,omitempty"`

	// Settings is a free-form object that contains the policy configuration values.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Type=object
	Settings runtime.RawExtension `json:"settings,omitempty"`

	// FailurePolicy defines how to handle failures from the policy.
	// Valid values are "Ignore" and "Fail".
	// +kubebuilder:validation:Enum=Ignore;Fail
	// +kubebuilder:default=Fail
	// +optional
	FailurePolicy string `json:"failurePolicy,omitempty"`

	// MatchConditions is a list of conditions that must be met for the policy to be evaluated.
	// This is an optional advanced feature.
	// +optional
	MatchConditions []MatchCondition `json:"matchConditions,omitempty"`
}

// PolicyRule defines the scope of a policy.
type PolicyRule struct {
	// APIGroups is a list of API groups this policy applies to.
	// Example: ["", "apps"]
	// +optional
	APIGroups []string `json:"apiGroups,omitempty"`

	// APIVersions is a list of API versions this policy applies to.
	// Example: ["v1"]
	APIVersions []string `json:"apiVersions"`

	// Resources is a list of resource types this policy applies to.
	// Example: ["pods", "deployments"]
	Resources []string `json:"resources"`

	// Operations is a list of operations this policy applies to.
	// Valid values are CREATE, UPDATE, DELETE, CONNECT.
	// +kubebuilder:validation:MinItems=1
	Operations []string `json:"operations"`

	// Scope specifies the scope of the rule.
	// Valid values are "*", "Cluster", "Namespaced".
	// +kubebuilder:validation:Enum=*;Cluster;Namespaced
	// +optional
	Scope string `json:"scope,omitempty"`
}

// MatchCondition represents a condition that must be met for the policy to be evaluated.
type MatchCondition struct {
	// Name is the identifier for this match condition.
	Name string `json:"name"`

	// Expression is a CEL expression that must evaluate to true for the policy to be evaluated.
	Expression string `json:"expression"`
}

// KubewardenPolicyStatus defines the observed state of KubewardenPolicy.
type KubewardenPolicyStatus struct {
	// Ready indicates whether the policy is successfully deployed.
	Ready bool `json:"ready"`

	// Conditions defines current state of the KubewardenPolicy.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`

	// MatchingClusters is the list of references to Clusters selected by the ClusterSelector.
	// +optional
	MatchingClusters []corev1.ObjectReference `json:"matchingClusters"`

	// DeployedPolicies tracks the policies deployed to each cluster.
	// +optional
	DeployedPolicies []DeployedPolicyStatus `json:"deployedPolicies,omitempty"`
}

// DeployedPolicyStatus represents the status of a policy deployed to a specific cluster.
type DeployedPolicyStatus struct {
	// ClusterName is the name of the cluster where the policy is deployed.
	ClusterName string `json:"clusterName"`

	// ClusterNamespace is the namespace of the cluster resource.
	ClusterNamespace string `json:"clusterNamespace"`

	// PolicyName is the name of the policy in the workload cluster.
	PolicyName string `json:"policyName"`

	// PolicyType is the type of policy (ClusterAdmissionPolicy or AdmissionPolicy).
	PolicyType string `json:"policyType"`

	// Active indicates whether the policy is active in the workload cluster.
	Active bool `json:"active"`

	// LastTransitionTime is the last time the status transitioned.
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`

	// Message provides additional information about the policy status.
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Policy Type",type=string,JSONPath=`.spec.policyType`
// +kubebuilder:printcolumn:name="Module",type=string,JSONPath=`.spec.module`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// KubewardenPolicy is the Schema for the kubewardenpolicies API.
type KubewardenPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KubewardenPolicySpec   `json:"spec,omitempty"`
	Status KubewardenPolicyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// KubewardenPolicyList contains a list of KubewardenPolicy.
type KubewardenPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KubewardenPolicy `json:"items"`
}

// GetConditions returns the list of conditions for a KubewardenPolicy API object.
func (p *KubewardenPolicy) GetConditions() clusterv1.Conditions {
	return p.Status.Conditions
}

// SetConditions will set the given conditions on a KubewardenPolicy object.
func (p *KubewardenPolicy) SetConditions(conditions clusterv1.Conditions) {
	p.Status.Conditions = conditions
}

// SetMatchingClusters will set the given list of matching clusters on a KubewardenPolicy object.
func (p *KubewardenPolicy) SetMatchingClusters(clusterList []clusterv1.Cluster) {
	matchingClusters := make([]corev1.ObjectReference, 0, len(clusterList))
	for _, cluster := range clusterList {
		matchingClusters = append(matchingClusters, corev1.ObjectReference{
			Kind:       cluster.Kind,
			APIVersion: cluster.APIVersion,
			Name:       cluster.Name,
			Namespace:  cluster.Namespace,
		})
	}

	p.Status.MatchingClusters = matchingClusters
}

func init() {
	SchemeBuilder.Register(&KubewardenPolicy{}, &KubewardenPolicyList{})
}
