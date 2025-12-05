/*
Copyright 2024.

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

package controller

import (
	"context"
	"fmt"
	"time"

	policiesv1 "github.com/kubewarden/kubewarden-controller/api/policies/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/source"

	addonv1alpha1 "github.com/caapkw/cluster-api-provider-addon-kubewarden/api/v1alpha1"
)

// KubewardenPolicyReconciler reconciles a KubewardenPolicy object
type KubewardenPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// RemoteClientGetter is used for accessing workload clusters
	RemoteClientGetter remote.ClusterClientGetter
}

// SetupWithManager sets up the controller with the Manager.
func (r *KubewardenPolicyReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	if r.RemoteClientGetter == nil {
		r.RemoteClientGetter = remote.NewClusterClient
	}
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&addonv1alpha1.KubewardenPolicy{}).
		Build(r)
	if err != nil {
		return fmt.Errorf("creating new controller: %w", err)
	}

	// Watch CAPI clusters to trigger policy reconciliation
	err = c.Watch(
		source.Kind[client.Object](mgr.GetCache(), &clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(r.clusterToKubewardenPolicy(ctx))),
	)
	if err != nil {
		return fmt.Errorf("adding watch for clusters: %w", err)
	}

	return nil
}

// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=addon.cluster.x-k8s.io,resources=kubewardenpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=addon.cluster.x-k8s.io,resources=kubewardenpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=addon.cluster.x-k8s.io,resources=kubewardenpolicies/finalizers,verbs=update

// Reconcile reconciles a KubewardenPolicy object, ensuring policies are deployed to workload clusters
func (r *KubewardenPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Reconciling KubewardenPolicy")

	// Fetch the KubewardenPolicy
	policy := &addonv1alpha1.KubewardenPolicy{}
	if err := r.Client.Get(ctx, req.NamespacedName, policy); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !policy.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, policy)
	}

	return r.reconcileNormal(ctx, policy)
}

func (r *KubewardenPolicyReconciler) reconcileNormal(ctx context.Context, policy *addonv1alpha1.KubewardenPolicy) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Get matching clusters based on ClusterSelector
	clusters, err := r.getMatchingClusters(ctx, policy)
	if err != nil {
		log.Error(err, "Failed to get matching clusters")
		return ctrl.Result{}, err
	}

	if len(clusters) == 0 {
		log.Info("No matching clusters found for policy", "policy", policy.Name)
		policy.Status.Ready = false
		policy.Status.DeployedPolicies = []addonv1alpha1.DeployedPolicyStatus{}
		if err := r.Client.Status().Update(ctx, policy); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: defaultRequeueDuration}, nil
	}

	// Update matching clusters in status
	policy.SetMatchingClusters(clusters)

	deployedPolicies := []addonv1alpha1.DeployedPolicyStatus{}
	allReady := true

	for _, cluster := range clusters {
		log := log.WithValues("cluster", cluster.Name)

		// Check if cluster is ready
		if !cluster.Status.ControlPlaneReady || !conditions.IsTrue(&cluster, clusterv1.ControlPlaneReadyCondition) {
			log.Info("Cluster control plane not ready, skipping")
			allReady = false
			continue
		}

		// Check if Kubewarden is installed on the cluster
		if !HasAnnotation(&cluster, KubewardenInstalledAnnotation) {
			log.Info("Kubewarden not installed on cluster, skipping policy deployment")
			allReady = false
			continue
		}

		// Create a remote client to connect to the workload cluster
		remoteClient, err := r.RemoteClientGetter(ctx, cluster.Name, r.Client, client.ObjectKeyFromObject(&cluster))
		if err != nil {
			log.Error(err, "Failed to get remote cluster client")
			allReady = false
			continue
		}

		// Deploy or update the policy
		policyStatus, err := r.deployPolicy(ctx, remoteClient, policy, cluster)
		if err != nil {
			log.Error(err, "Failed to deploy policy")
			allReady = false
			policyStatus.Active = false
			policyStatus.Message = err.Error()
		}

		deployedPolicies = append(deployedPolicies, policyStatus)
	}

	// Update status
	policy.Status.Ready = allReady
	policy.Status.DeployedPolicies = deployedPolicies

	if err := r.Client.Status().Update(ctx, policy); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *KubewardenPolicyReconciler) reconcileDelete(ctx context.Context, policy *addonv1alpha1.KubewardenPolicy) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Deleting KubewardenPolicy")

	// Get matching clusters to clean up policies
	clusters, err := r.getMatchingClusters(ctx, policy)
	if err != nil {
		log.Error(err, "Failed to get matching clusters during deletion")
		return ctrl.Result{}, err
	}

	for _, cluster := range clusters {
		log := log.WithValues("cluster", cluster.Name)

		remoteClient, err := r.RemoteClientGetter(ctx, cluster.Name, r.Client, client.ObjectKeyFromObject(&cluster))
		if err != nil {
			log.Error(err, "Failed to get remote cluster client during deletion")
			continue
		}

		// Delete the policy from the workload cluster
		if err := r.deletePolicy(ctx, remoteClient, policy); err != nil {
			log.Error(err, "Failed to delete policy from workload cluster")
		}
	}

	return ctrl.Result{}, nil
}

func (r *KubewardenPolicyReconciler) deployPolicy(
	ctx context.Context,
	remoteClient client.Client,
	policy *addonv1alpha1.KubewardenPolicy,
	cluster clusterv1.Cluster,
) (addonv1alpha1.DeployedPolicyStatus, error) {
	log := log.FromContext(ctx)

	now := metav1.Now()
	status := addonv1alpha1.DeployedPolicyStatus{
		ClusterName:        cluster.Name,
		ClusterNamespace:   cluster.Namespace,
		PolicyName:         policy.Spec.PolicyName,
		PolicyType:         policy.Spec.PolicyType,
		Active:             false,
		LastTransitionTime: &now,
	}

	var err error
	if policy.Spec.PolicyType == "ClusterAdmissionPolicy" {
		err = r.deployClusterAdmissionPolicy(ctx, remoteClient, policy)
	} else {
		err = r.deployAdmissionPolicy(ctx, remoteClient, policy)
	}

	if err != nil {
		status.Message = fmt.Sprintf("Failed to deploy: %v", err)
		return status, err
	}

	// Verify the policy is active
	active, err := r.isPolicyActive(ctx, remoteClient, policy)
	if err != nil {
		log.Error(err, "Failed to check policy status")
		status.Message = fmt.Sprintf("Failed to verify status: %v", err)
		return status, err
	}

	status.Active = active
	if active {
		status.Message = "Policy successfully deployed and active"
	} else {
		status.Message = "Policy deployed but not yet active"
	}

	return status, nil
}

func (r *KubewardenPolicyReconciler) deployClusterAdmissionPolicy(
	ctx context.Context,
	remoteClient client.Client,
	policy *addonv1alpha1.KubewardenPolicy,
) error {
	log := log.FromContext(ctx)

	cap := &policiesv1.ClusterAdmissionPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: policy.Spec.PolicyName,
		},
		Spec: policiesv1.ClusterAdmissionPolicySpec{
			PolicySpec: r.buildPolicySpec(policy),
		},
	}

	// Try to get existing policy
	existing := &policiesv1.ClusterAdmissionPolicy{}
	err := remoteClient.Get(ctx, client.ObjectKeyFromObject(cap), existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Create new policy
			log.Info("Creating ClusterAdmissionPolicy", "name", cap.Name)
			return remoteClient.Create(ctx, cap)
		}
		return err
	}

	// Update existing policy
	log.Info("Updating ClusterAdmissionPolicy", "name", cap.Name)
	existing.Spec = cap.Spec
	return remoteClient.Update(ctx, existing)
}

func (r *KubewardenPolicyReconciler) deployAdmissionPolicy(
	ctx context.Context,
	remoteClient client.Client,
	policy *addonv1alpha1.KubewardenPolicy,
) error {
	log := log.FromContext(ctx)

	ap := &policiesv1.AdmissionPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policy.Spec.PolicyName,
			Namespace: policy.Spec.TargetNamespace,
		},
		Spec: policiesv1.AdmissionPolicySpec{
			PolicySpec: r.buildPolicySpec(policy),
		},
	}

	// Try to get existing policy
	existing := &policiesv1.AdmissionPolicy{}
	err := remoteClient.Get(ctx, client.ObjectKeyFromObject(ap), existing)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Create new policy
			log.Info("Creating AdmissionPolicy", "name", ap.Name, "namespace", ap.Namespace)
			return remoteClient.Create(ctx, ap)
		}
		return err
	}

	// Update existing policy
	log.Info("Updating AdmissionPolicy", "name", ap.Name, "namespace", ap.Namespace)
	existing.Spec = ap.Spec
	return remoteClient.Update(ctx, existing)
}

func (r *KubewardenPolicyReconciler) buildPolicySpec(policy *addonv1alpha1.KubewardenPolicy) policiesv1.PolicySpec {
	spec := policiesv1.PolicySpec{
		PolicyServer: policy.Spec.PolicyServer,
		Module:       policy.Spec.Module,
		Mutating:     policy.Spec.Mutating,
	}

	// Convert rules
	for _, rule := range policy.Spec.Rules {
		spec.Rules = append(spec.Rules, admissionregistrationv1.RuleWithOperations{
			Operations: convertOperations(rule.Operations),
			Rule: admissionregistrationv1.Rule{
				APIGroups:   rule.APIGroups,
				APIVersions: rule.APIVersions,
				Resources:   rule.Resources,
				Scope:       convertScope(rule.Scope),
			},
		})
	}

	// Set failure policy
	if policy.Spec.FailurePolicy != "" {
		fp := admissionregistrationv1.FailurePolicyType(policy.Spec.FailurePolicy)
		spec.FailurePolicy = &fp
	}

	// Convert settings
	if len(policy.Spec.Settings.Raw) > 0 {
		spec.Settings = policy.Spec.Settings
	}

	// Convert match conditions
	for _, mc := range policy.Spec.MatchConditions {
		spec.MatchConditions = append(spec.MatchConditions, admissionregistrationv1.MatchCondition{
			Name:       mc.Name,
			Expression: mc.Expression,
		})
	}

	return spec
}

func convertOperations(ops []string) []admissionregistrationv1.OperationType {
	result := make([]admissionregistrationv1.OperationType, len(ops))
	for i, op := range ops {
		result[i] = admissionregistrationv1.OperationType(op)
	}
	return result
}

func convertScope(scope string) *admissionregistrationv1.ScopeType {
	if scope == "" {
		return nil
	}
	s := admissionregistrationv1.ScopeType(scope)
	return &s
}

func (r *KubewardenPolicyReconciler) isPolicyActive(
	ctx context.Context,
	remoteClient client.Client,
	policy *addonv1alpha1.KubewardenPolicy,
) (bool, error) {
	if policy.Spec.PolicyType == "ClusterAdmissionPolicy" {
		cap := &policiesv1.ClusterAdmissionPolicy{}
		err := remoteClient.Get(ctx, types.NamespacedName{Name: policy.Spec.PolicyName}, cap)
		if err != nil {
			return false, err
		}
		return cap.Status.PolicyStatus == policiesv1.PolicyStatusActive, nil
	}

	ap := &policiesv1.AdmissionPolicy{}
	err := remoteClient.Get(ctx, types.NamespacedName{
		Name:      policy.Spec.PolicyName,
		Namespace: policy.Spec.TargetNamespace,
	}, ap)
	if err != nil {
		return false, err
	}
	return ap.Status.PolicyStatus == policiesv1.PolicyStatusActive, nil
}

func (r *KubewardenPolicyReconciler) deletePolicy(
	ctx context.Context,
	remoteClient client.Client,
	policy *addonv1alpha1.KubewardenPolicy,
) error {
	log := log.FromContext(ctx)

	if policy.Spec.PolicyType == "ClusterAdmissionPolicy" {
		cap := &policiesv1.ClusterAdmissionPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name: policy.Spec.PolicyName,
			},
		}
		log.Info("Deleting ClusterAdmissionPolicy", "name", cap.Name)
		err := remoteClient.Delete(ctx, cap)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	} else {
		ap := &policiesv1.AdmissionPolicy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      policy.Spec.PolicyName,
				Namespace: policy.Spec.TargetNamespace,
			},
		}
		log.Info("Deleting AdmissionPolicy", "name", ap.Name, "namespace", ap.Namespace)
		err := remoteClient.Delete(ctx, ap)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

func (r *KubewardenPolicyReconciler) getMatchingClusters(
	ctx context.Context,
	policy *addonv1alpha1.KubewardenPolicy,
) ([]clusterv1.Cluster, error) {
	clusterList := &clusterv1.ClusterList{}

	// Use label selector to filter clusters
	selector, err := metav1.LabelSelectorAsSelector(&policy.Spec.ClusterSelector)
	if err != nil {
		return nil, fmt.Errorf("invalid cluster selector: %w", err)
	}

	opts := []client.ListOption{
		client.InNamespace(policy.Namespace),
		client.MatchingLabelsSelector{Selector: selector},
	}

	if err := r.Client.List(ctx, clusterList, opts...); err != nil {
		return nil, fmt.Errorf("listing clusters: %w", err)
	}

	return clusterList.Items, nil
}

func (r *KubewardenPolicyReconciler) clusterToKubewardenPolicy(ctx context.Context) handler.MapFunc {
	log := log.FromContext(ctx)

	return func(_ context.Context, o client.Object) []ctrl.Request {
		cluster, ok := o.(*clusterv1.Cluster)
		if !ok {
			log.Error(nil, fmt.Sprintf("Expected a Cluster but got a %T", o))
			return nil
		}

		policies := addonv1alpha1.KubewardenPolicyList{}
		if err := r.Client.List(ctx, &policies, client.InNamespace(cluster.Namespace)); err != nil {
			return nil
		}

		requests := []ctrl.Request{}
		for _, policy := range policies.Items {
			selector, err := metav1.LabelSelectorAsSelector(&policy.Spec.ClusterSelector)
			if err != nil {
				continue
			}

			if selector.Matches(labels.Set(cluster.Labels)) {
				requests = append(requests, ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      policy.Name,
						Namespace: policy.Namespace,
					},
				})
			}
		}

		return requests
	}
}
