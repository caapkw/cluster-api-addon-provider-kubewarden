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
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var kubewardenpolicylog = logf.Log.WithName("kubewardenpolicy-resource")

func (p *KubewardenPolicy) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(p).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-addon-cluster-x-k8s-io-v1alpha1-kubewardenpolicy,mutating=true,failurePolicy=fail,sideEffects=None,groups=addon.cluster.x-k8s.io,resources=kubewardenpolicies,verbs=create;update,versions=v1alpha1,name=mkubewardenpolicy.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &KubewardenPolicy{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (p *KubewardenPolicy) Default() {
	kubewardenpolicylog.Info("default", "name", p.GetName())

	// Set default PolicyType
	if p.Spec.PolicyType == "" {
		p.Spec.PolicyType = "ClusterAdmissionPolicy"
	}

	// Set default PolicyServer
	if p.Spec.PolicyServer == "" {
		p.Spec.PolicyServer = "default"
	}

	// Set default FailurePolicy
	if p.Spec.FailurePolicy == "" {
		p.Spec.FailurePolicy = "Fail"
	}

	// Set default PolicyName if not specified
	if p.Spec.PolicyName == "" {
		p.Spec.PolicyName = p.GetName()
	}

	// Set default TargetNamespace for AdmissionPolicy if not specified
	if p.Spec.PolicyType == "AdmissionPolicy" && p.Spec.TargetNamespace == "" {
		p.Spec.TargetNamespace = "default"
	}
}

// +kubebuilder:webhook:path=/validate-addon-cluster-x-k8s-io-v1alpha1-kubewardenpolicy,mutating=false,failurePolicy=fail,sideEffects=None,groups=addon.cluster.x-k8s.io,resources=kubewardenpolicies,verbs=create;update,versions=v1alpha1,name=vkubewardenpolicy.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &KubewardenPolicy{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (p *KubewardenPolicy) ValidateCreate() (admission.Warnings, error) {
	kubewardenpolicylog.Info("validate create", "name", p.GetName())

	return p.validateKubewardenPolicy()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (p *KubewardenPolicy) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	kubewardenpolicylog.Info("validate update", "name", p.Name)

	return p.validateKubewardenPolicy()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (p *KubewardenPolicy) ValidateDelete() (admission.Warnings, error) {
	kubewardenpolicylog.Info("validate delete", "name", p.Name)

	// No validation needed for delete
	return nil, nil
}

// validateKubewardenPolicy performs validation for KubewardenPolicy.
func (p *KubewardenPolicy) validateKubewardenPolicy() (admission.Warnings, error) {
	var warnings admission.Warnings

	// Validate module is not empty
	if p.Spec.Module == "" {
		return warnings, fmt.Errorf("module must be specified")
	}

	// Validate rules are not empty
	if len(p.Spec.Rules) == 0 {
		return warnings, fmt.Errorf("at least one rule must be specified")
	}

	// Validate each rule
	for i, rule := range p.Spec.Rules {
		if len(rule.APIVersions) == 0 {
			return warnings, fmt.Errorf("rule[%d]: apiVersions must be specified", i)
		}
		if len(rule.Resources) == 0 {
			return warnings, fmt.Errorf("rule[%d]: resources must be specified", i)
		}
		if len(rule.Operations) == 0 {
			return warnings, fmt.Errorf("rule[%d]: operations must be specified", i)
		}

		// Validate operations
		validOps := map[string]bool{"CREATE": true, "UPDATE": true, "DELETE": true, "CONNECT": true}
		for _, op := range rule.Operations {
			if !validOps[op] {
				return warnings, fmt.Errorf("rule[%d]: invalid operation '%s', must be one of: CREATE, UPDATE, DELETE, CONNECT", i, op)
			}
		}
	}

	// Validate PolicyType specific requirements
	if p.Spec.PolicyType == "AdmissionPolicy" {
		if p.Spec.TargetNamespace == "" {
			return warnings, fmt.Errorf("targetNamespace must be specified for AdmissionPolicy")
		}
	}

	// Add warning if PolicyType is AdmissionPolicy
	if p.Spec.PolicyType == "AdmissionPolicy" {
		warnings = append(warnings, "AdmissionPolicy requires Kubernetes 1.21.0 or greater in workload clusters")
	}

	return warnings, nil
}
