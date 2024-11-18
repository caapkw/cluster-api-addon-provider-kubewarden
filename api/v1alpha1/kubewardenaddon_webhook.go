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
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var kubewardenaddonlog = logf.Log.WithName("kubewardenaddon-resource")

func (r *KubewardenAddon) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-addons-cluster-x-k8s-io-v1alpha1-kubewardenaddon,mutating=true,failurePolicy=fail,sideEffects=None,groups=addons.cluster.x-k8s.io,resources=kubewardenaddons,verbs=create;update,versions=v1alpha1,name=kubewardenaddon.kb.io,admissionReviewVersions=v1
var _ webhook.Defaulter = &KubewardenAddon{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (p *KubewardenAddon) Default() {
	kubewardenaddonlog.Info("default", "name", p.Name)

	if p.Spec.ImageRepository == "" {
		p.Spec.ImageRepository = "ghcr.io/kubewarden"
	}

	if p.Spec.Version == "" {
		p.Spec.Version = "latest"
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-addons-cluster-x-k8s-io-v1alpha1-kubewardenaddon,mutating=false,failurePolicy=fail,sideEffects=None,groups=addons.cluster.x-k8s.io,resources=kubewardenaddons,verbs=create;update,versions=v1alpha1,name=vkubewardenaddon.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &KubewardenAddon{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (r *KubewardenAddon) ValidateCreate() (admission.Warnings, error) {
	kubewardenaddonlog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (r *KubewardenAddon) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	kubewardenaddonlog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (r *KubewardenAddon) ValidateDelete() (admission.Warnings, error) {
	kubewardenaddonlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}
