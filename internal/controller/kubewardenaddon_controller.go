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
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// KubewardenAddonReconciler reconciles a KubewardenAddon object
type KubewardenAddonReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// RemoteClientGetter is used for accessing workload clusters
	RemoteClientGetter remote.ClusterClientGetter
}

// TODO: deploy kubewarden to any CAPI clusters
// TODO: deploy kubewarden to CAPI clusters defined in KubewardenAddon.Spec
// TODO: deploy specific policies to CAPI clusters

// SetupWithManager sets up the controller with the Manager.
func (r *KubewardenAddonReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	if r.RemoteClientGetter == nil {
		r.RemoteClientGetter = remote.NewClusterClient
	}
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&addonv1alpha1.KubewardenAddon{}).
		Build(r)
	if err != nil {
		return fmt.Errorf("creating new controller: %w", err)
	}

	// NOTE: watch CAPI clusters
	err = c.Watch(
		source.Kind[client.Object](mgr.GetCache(), &clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(r.clusterToKubewardenAddon(ctx))),
	)
	if err != nil {
		return fmt.Errorf("adding watch for cluster upgrade group: %w", err)
	}

	return nil
}

// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=addon.cluster.x-k8s.io,resources=kubewardenaddons,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=addon.cluster.x-k8s.io,resources=kubewardenaddons/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=addon.cluster.x-k8s.io,resources=kubewardenaddons/finalizers,verbs=update

// Reconcile reconciles a KubewardenAddon object, ensuring the addon is deployed to the workload cluster
func (r *KubewardenAddonReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Reconciling Kubewarden addon")

	// fetch the kubewarden addon
	addon := &addonv1alpha1.KubewardenAddon{}
	if err := r.Client.Get(ctx, req.NamespacedName, addon); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{Requeue: true}, nil
		}

		return ctrl.Result{Requeue: true}, err
	}

	// for now we will deploy kubewarden to all clusters

	/*
		// fetch capi clusters
		cluster := &clusterv1.Cluster{}
		if err := r.Client.Get(ctx, req.NamespacedName, cluster); err != nil {
			if apierrors.IsNotFound(err) {
				log.Info("Cluster not found")
				return ctrl.Result{Requeue: true}, nil
			}

			return ctrl.Result{Requeue: true}, err
		}
	*/

	if !addon.DeletionTimestamp.IsZero() {
		// this won't be the case as there is no finalizer yet
		return ctrl.Result{}, nil
	}

	return r.reconcileNormal(ctx, addon)
}

func (r *KubewardenAddonReconciler) reconcileNormal(ctx context.Context, addon *addonv1alpha1.KubewardenAddon) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	log.Info("Listing clusters in addon namespace to deploy Kubewarden to")

	clusters := []clusterv1.Cluster{}
	var err error
	if deployToAll {
		clusters, err = r.getAllCapiClusters(ctx, addon.Namespace)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("getting capi clusters: %w", err)
		}
	} else {
		// This is just a placeholder for now, should be the only way to deploy kubewarden in subsequent versions
		// `deployToAll` is just a temporary configuration to have it running for now
		// we can set a specific value of KubewardenAddon.Sepc.TargetCluster that means all clusters
		// but it should be the only source of truth
		// clusters = addon.Spec.TargetClusters
		log.Info("Deploying to specific clusters: not supported yet -> won't deploy to any clusters")
	}

	for _, cluster := range clusters {
		log = log.WithValues("cluster", cluster.Name)

		// cluster must be ready before we can deploy kubewarden
		if !cluster.Status.ControlPlaneReady && !conditions.IsTrue(&cluster, clusterv1.ControlPlaneReadyCondition) {
			log.Info("clusters control plane is not ready, requeue")
			return ctrl.Result{RequeueAfter: defaultRequeueDuration}, nil
		}

		// create a remote client to connect to the workload cluster
		remoteClient, err := r.RemoteClientGetter(ctx, cluster.Name, r.Client, client.ObjectKeyFromObject(&cluster))
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("getting remote cluster client: %w", err)
		}

		// create kubewarden namespace
		log.Info(fmt.Sprintf("Creating namespace '%s'", kubewardenNamespace))
		if err := createKubewardenNamespace(ctx, remoteClient); err != nil {
			return ctrl.Result{}, fmt.Errorf("creating kubewarden namespace: %w", err)
		}

		// create kubewarden crds
		log.Info(fmt.Sprintf("Applying Kubewarden CRDs to cluster %s", cluster.Name))
		err = r.installKubewardenCRDs(ctx, kubewardenVersion, remoteClient) // kubewardenVersion is a placeholder until we can use the value from KubewardenAddon.Spec.Version
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("creating kubewarden CRDs: %w", err)
		}

		// - [ ] install kubewarden-controller
		log.Info(fmt.Sprintf("Installing Kubewarden controller to cluster %s", cluster.Name))

		// - [ ] install kubewarden-defaults
		log.Info(fmt.Sprintf("Installing Kubewarden defaults controller to cluster %s", cluster.Name))

		// - [ ] deploy kubewaarden policy server
		log.Info(fmt.Sprintf("Deploying Kubewarden policy server controller to cluster %s", cluster.Name))
	}

	return ctrl.Result{}, nil
}

func (r *KubewardenAddonReconciler) clusterToKubewardenAddon(ctx context.Context) handler.MapFunc {
	log := log.FromContext(ctx)

	return func(_ context.Context, o client.Object) []ctrl.Request {
		// this cluster object will be used when we want to select which clusters
		// to deploy kubewarden to
		cluster, ok := o.(*clusterv1.Cluster)
		if !ok {
			log.Error(nil, fmt.Sprintf("Expected a Cluster but got a %T", o))

			return nil
		}
		fmt.Println("cluster: ", cluster.Name)

		addons := addonv1alpha1.KubewardenAddonList{}
		if err := r.Client.List(ctx, &addons); err != nil {
			return nil
		}

		requests := []ctrl.Request{}
		for _, addon := range addons.Items {
			// this applies to the all cluster functionality
			// next is to apply changes to clusters specified in the addon spec
			// would required a condition here
			requests = append(requests, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      addon.Name,
					Namespace: addon.Namespace,
				},
			})
		}

		return requests
	}
}

func (r *KubewardenAddonReconciler) getAllCapiClusters(ctx context.Context, ns string) ([]clusterv1.Cluster, error) {
	clusters := &clusterv1.ClusterList{}
	opts := []client.ListOption{
		client.InNamespace(ns),
	}
	if err := r.Client.List(ctx, clusters, opts...); err != nil {
		return nil, fmt.Errorf("listing clusters: %w", err)
	}

	return clusters.Items, nil
}

func createKubewardenNamespace(ctx context.Context, remoteClient client.Client) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: kubewardenNamespace,
		},
	}

	if err := remoteClient.Create(ctx, ns); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
	}

	return nil
}

func (r *KubewardenAddonReconciler) applyKubewardenCRDs() ([]*apiextensionsv1.CustomResourceDefinition, error) {
	return nil, nil
}

func (r *KubewardenAddonReconciler) installKubewardenCRDs(ctx context.Context, version string, remoteClient client.Client) error {
	// Download the CRDs tarball
	crdsURL := fmt.Sprintf("%s/%s/%s/CRDS.tar.gz", kubewardenControllerRepository, githubReleasesPath, version)
	crdsPath, err := downloadFile(crdsURL)
	if err != nil {
		return fmt.Errorf("download CRDs tarball: %w", err)
	}
	defer os.Remove(crdsPath)

	// Extract the tarball
	extractDir, err := extractTarGz(crdsPath)
	if err != nil {
		return fmt.Errorf("extract CRDs: %w", err)
	}
	defer os.RemoveAll(extractDir)

	// Apply each YAML file
	files, err := filepath.Glob(filepath.Join(extractDir, "*.yaml"))
	if err != nil {
		return fmt.Errorf("list extracted files: %w", err)
	}
	for _, file := range files {
		if err := applyManifest(ctx, remoteClient, file); err != nil {
			return fmt.Errorf("apply CRD from file %s: %w", file, err)
		}
	}

	return nil
}
