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
	"io"
	"os"
	"path/filepath"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/yaml"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	addonv1alpha1 "github.com/caapkw/cluster-api-provider-addon-kubewarden/api/v1alpha1"
	"github.com/pkg/errors"
)

// KubewardenAddonReconciler reconciles a KubewardenAddon object
type KubewardenAddonReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// RemoteClientGetter is used for accessing workload clusters
	RemoteClientGetter remote.ClusterClientGetter

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

// SetupWithManager sets up the controller with the Manager.
func (r *KubewardenAddonReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	log := ctrl.LoggerFrom(ctx)

	if r.RemoteClientGetter == nil {
		r.RemoteClientGetter = remote.NewClusterClient
	}

	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&addonv1alpha1.KubewardenAddon{}).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(log, r.WatchFilterValue)).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(r.ClusterToKubewardenAddonMapper),
		).
		Complete(r)
}

// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=addon.cluster.x-k8s.io,resources=kubewardenaddons,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=addon.cluster.x-k8s.io,resources=kubewardenaddons/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=addon.cluster.x-k8s.io,resources=kubewardenaddons/finalizers,verbs=update

// Reconcile reconciles a KubewardenAddon object, ensuring the addon is deployed to the workload cluster
func (r *KubewardenAddonReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(2).Info("Beginning reconciliation for KubewardenAddon", "requestNamespace", req.Namespace, "requestName", req.Name)

	// fetch the kubewarden addon
	addon := &addonv1alpha1.KubewardenAddon{}
	if err := r.Client.Get(ctx, req.NamespacedName, addon); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(2).Info("KubewardenAddon resource not found, skipping reconciliation", "kubewardenAddon", req.NamespacedName)
			return ctrl.Result{Requeue: true}, nil
		}

		return ctrl.Result{Requeue: true}, err
	}

	patchHelper, err := patch.NewHelper(addon, r.Client)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to init patch helper")
	}

	defer func() {
		log.V(2).Info("Preparing to patch KubewardenAddon", "kubewardenAddon", addon.Name)
		if err := patchKubewardenAddon(ctx, patchHelper, addon); err != nil && reterr == nil {
			reterr = err
			log.Error(err, "failed to patch KubewardenAddon", "kubewardenAddon", addon.Name)

			return
		}
		log.V(2).Info("Successfully patched KubewardenAddon", "kubewardenAddon", addon.Name)
	}()

	// Determine the target clusters based on ClusterNamespace, ClusterName, and ClusterSelector
	var clusters []clusterv1.Cluster
	// Select clusters based on ClusterSelector
	selector := addon.Spec.ClusterSelector
	log.V(2).Info("Finding matching clusters for KubewardenAddon with selector selector", "kubewardenAddon", addon.Name, "selector", selector)
	clusterList, err := r.listClustersWithLabels(ctx, addon.Namespace, selector)
	if err != nil {
		conditions.MarkFalse(addon, addonv1alpha1.KubewardenAddonSpecsUpToDateCondition, addonv1alpha1.ClusterSelectionFailedReason, clusterv1.ConditionSeverityError, "%s", err.Error())
		return ctrl.Result{}, err
	}
	clusters = clusterList.Items

	addon.SetMatchingClusters(clusters)

	if !addon.DeletionTimestamp.IsZero() {
		// this won't be the case as there is no finalizer yet
		return ctrl.Result{}, nil
	}

	return r.reconcileNormal(ctx, addon, clusters)
}

func (r *KubewardenAddonReconciler) reconcileNormal(ctx context.Context, addon *addonv1alpha1.KubewardenAddon, clusters []clusterv1.Cluster) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	for _, cluster := range clusters {
		log = log.WithValues("cluster", cluster.Name)

		// cluster must be ready before we can deploy kubewarden
		if !cluster.Status.ControlPlaneReady && !conditions.IsTrue(&cluster, clusterv1.ControlPlaneReadyCondition) {
			return ctrl.Result{RequeueAfter: defaultRequeueDuration}, nil
		}

		if HasAnnotation(&cluster, KubewardenInstalledAnnotation) {
			log.Info("Kubewarden already installed on cluster, skipping", "cluster", cluster.Name)
			return ctrl.Result{}, nil
		}

		// create a remote client to connect to the workload cluster
		remoteClient, err := r.RemoteClientGetter(ctx, cluster.Name, r.Client, client.ObjectKeyFromObject(&cluster))
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("getting remote cluster client: %w", err)
		}

		// create kubewarden namespace
		log.Info("Creating namespace for Kubewarden", "cluster", cluster.Name)
		if err := createKubewardenNamespace(ctx, remoteClient); err != nil {
			return ctrl.Result{}, fmt.Errorf("creating kubewarden namespace: %w", err)
		}

		// create kubewarden crds
		log.Info("Applying Kubewarden CRDs", "cluster", cluster.Name)
		err = r.installKubewardenCRDs(ctx, kubewardenVersion, remoteClient) // kubewardenVersion is a placeholder until we can use the value from KubewardenAddon.Spec.Version
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("creating kubewarden CRDs: %w", err)
		}

		// install kubewarden-controller
		log.Info("Installing Kubewarden controller", "cluster", cluster.Name)
		if err := r.installKubewardenController(ctx, remoteClient, addon); err != nil {
			return ctrl.Result{}, fmt.Errorf("installing kubewarden controller: %w", err)
		}

		// install kubewarden-defaults
		log.Info("Installing default 'PolicyServer'", "cluster", cluster.Name)
		if err := r.installKubewardenDefaults(ctx, remoteClient, addon); err != nil {
			return ctrl.Result{}, fmt.Errorf("installing kubewarden defaults: %w", err)
		}

		// annotate cluster so we don't try to deploy kubewarden again
		log.Info(fmt.Sprintf("Successfully deployed Kubewarden to cluster %s: annotating with %s",
			cluster.Name,
			KubewardenInstalledAnnotation))

		annotations := cluster.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}

		clusterCopy := cluster.DeepCopy()
		annotations[KubewardenInstalledAnnotation] = "true"
		cluster.SetAnnotations(annotations)

		patch := client.MergeFrom(clusterCopy)
		if err := r.Client.Patch(ctx, &cluster, patch); err != nil {
			return ctrl.Result{}, fmt.Errorf("update cluster annotations: %w", err)
		}
	}

	return ctrl.Result{}, nil
}

// ClusterToKubewardenAddonMapper is a mapper function that maps a Cluster to the KubewardenAddons that would select the Cluster.
func (r *KubewardenAddonReconciler) ClusterToKubewardenAddonMapper(ctx context.Context, o client.Object) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)

	cluster, ok := o.(*clusterv1.Cluster)
	if !ok {
		// Suppress the error for now
		log.Error(errors.Errorf("expected a Cluster but got %T", o), "failed to map object to KubewardenAddon")
		return nil
	}

	addons := &addonv1alpha1.KubewardenAddonList{}

	// List all KubewardenAddon resources in the same namespace as the cluster
	if err := r.Client.List(ctx, addons, client.InNamespace(cluster.Namespace)); err != nil {
		log.Error(err, "failed to list KubewardenAddon resources")
		return nil
	}

	results := []ctrl.Request{}
	for _, addon := range addons.Items {
		selector, err := metav1.LabelSelectorAsSelector(&addon.Spec.ClusterSelector)
		if err != nil {
			// Suppress the error for now
			log.Error(err, "failed to parse ClusterSelector for HelmChartProxy", "helmChartProxy", addon.Name)
			return nil
		}

		if selector.Matches(labels.Set(cluster.Labels)) {
			results = append(results, ctrl.Request{
				NamespacedName: client.ObjectKey{Namespace: addon.Namespace, Name: addon.Name},
			})
		}
	}

	return results

}

func (r *KubewardenAddonReconciler) installKubewardenCRDs(ctx context.Context, version string, remoteClient client.Client) error {
	// kubewarden crds are published as a tarball on github releases
	crdsURL := fmt.Sprintf("%s/%s/%s/CRDS.tar.gz", kubewardenControllerRepository, githubReleasesPath, version)
	crdsPath, err := downloadFile(crdsURL)
	if err != nil {
		return fmt.Errorf("download CRDs tarball: %w", err)
	}
	defer func() {
		if err := os.Remove(crdsPath); err != nil {
			fmt.Printf("Error removing CRDs tarball: %v\n", err)
		}
	}()

	extractDir, err := extractTarGz(crdsPath)
	if err != nil {
		return fmt.Errorf("extract CRDs: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(extractDir); err != nil {
			fmt.Printf("Error removing extracted files: %v\n", err)
		}
	}()

	files, err := filepath.Glob(filepath.Join(extractDir, "*.yaml"))
	if err != nil {
		return fmt.Errorf("list extracted files: %w", err)
	}
	for _, file := range files {
		if err := r.applyManifest(ctx, remoteClient, file); err != nil {
			return fmt.Errorf("apply CRD from file %s: %w", file, err)
		}
	}

	return nil
}

func (r *KubewardenAddonReconciler) installKubewardenController(ctx context.Context, remoteClient client.Client, addon *addonv1alpha1.KubewardenAddon) error {
	// render kubewarden-controller helm chart and apply it to the cluster
	renderedPath, err := renderHelmChart(ctx, "kubewarden-controller", addon.Spec.Version, nil)
	if err != nil {
		return fmt.Errorf("render kubewarden-controller helm chart: %w", err)
	}
	if err := r.applyManifest(ctx, remoteClient, renderedPath); err != nil {
		return fmt.Errorf("apply kubewarden-controller manifest: %w", err)
	}

	return nil
}

func (r *KubewardenAddonReconciler) installKubewardenDefaults(ctx context.Context, remoteClient client.Client, addon *addonv1alpha1.KubewardenAddon) error {
	// render kubewarden-defaults helm chart and apply it to the cluster
	renderedPath, err := renderHelmChart(ctx, "kubewarden-defaults", addon.Spec.Version, nil)
	if err != nil {
		return fmt.Errorf("render kubewarden-defaults helm chart: %w", err)
	}
	if err := r.applyManifest(ctx, remoteClient, renderedPath); err != nil {
		return fmt.Errorf("apply kubewarden-defaults manifest: %w", err)
	}

	return nil
}

// applyManifest applies a single YAML manifest to the cluster
func (r *KubewardenAddonReconciler) applyManifest(ctx context.Context, k8sClient client.Client, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			fmt.Printf("Error closing file: %v\n", err)
		}
	}()

	decoder := yaml.NewYAMLOrJSONDecoder(file, 1024)
	for {
		// use unknown to be able to decode any k8s object
		unk := &runtime.Unknown{}
		err := decoder.Decode(unk)
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("failed to decode manifest: %w", err)
		}

		// decode into a runtime.Object using the controller's scheme
		codecFactory := serializer.NewCodecFactory(r.Scheme)
		runtimeObject, kind, err := codecFactory.UniversalDeserializer().Decode(unk.Raw, nil, nil)
		if kind == nil {
			// this is an invalid object, skip it
			break
		}
		if err != nil {
			return fmt.Errorf("failed to decode runtime object: %w", err)
		}
		obj, ok := runtimeObject.(client.Object)
		if !ok {
			return fmt.Errorf("failed to cast runtime object to client.Object")
		}
		// only create objects if they don't exist in the cluster already
		if err := k8sClient.Create(ctx, obj); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to apply resource: %w", err)
			}
		}
	}

	return nil
}

// listClustersWithLabels returns a list of Clusters that match the given label selector.
func (r *KubewardenAddonReconciler) listClustersWithLabels(ctx context.Context, namespace string, selector metav1.LabelSelector) (*clusterv1.ClusterList, error) {
	clusterList := &clusterv1.ClusterList{}
	// To support for the matchExpressions field, convert LabelSelector to labels.Selector to specify labels.Selector for ListOption. (Issue #15)
	labelselector, err := metav1.LabelSelectorAsSelector(&selector)
	if err != nil {
		return nil, err
	}

	if err := r.Client.List(ctx, clusterList, client.InNamespace(namespace), client.MatchingLabelsSelector{Selector: labelselector}); err != nil {
		return nil, err
	}

	return clusterList, nil
}

// patchKubewardenAddon patches the hKubewardenAddon object and sets the ReadyCondition as an aggregate of the other condition set.
func patchKubewardenAddon(ctx context.Context, patchHelper *patch.Helper, kubewardenAddon *addonv1alpha1.KubewardenAddon) error {
	conditions.SetSummary(kubewardenAddon,
		conditions.WithConditions(
			addonv1alpha1.KubewardenAddonSpecsUpToDateCondition,
			addonv1alpha1.KubewardenAddonsReadyCondition,
		),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	return patchHelper.Patch(
		ctx,
		kubewardenAddon,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			addonv1alpha1.KubewardenAddonSpecsUpToDateCondition,
			addonv1alpha1.KubewardenAddonsReadyCondition,
		}},
		patch.WithStatusObservedGeneration{},
	)
}
