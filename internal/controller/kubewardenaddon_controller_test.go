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
	"strings"

	policiesv1 "github.com/kubewarden/kubewarden-controller/api/policies/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/remote"
	"sigs.k8s.io/cluster-api/util/secret"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	addonv1alpha1 "github.com/caapkw/cluster-api-provider-addon-kubewarden/api/v1alpha1"
)

var _ = Describe("KubewardenAddon Controller", func() {
	var (
		capiCluster          *clusterv1.Cluster
		capiKubeconfigSecret *corev1.Secret
	)
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		kubewardenaddon := &addonv1alpha1.KubewardenAddon{}

		BeforeEach(func() {
			capiCluster = &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "default",
				},
			}

			capiKubeconfigSecret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-kubeconfig", capiCluster.Name),
					Namespace: "default",
				},
				Data: map[string][]byte{
					secret.KubeconfigDataName: kubeConfigBytes,
				},
			}

			err := k8sClient.Get(ctx, typeNamespacedName, kubewardenaddon)
			if err != nil && errors.IsNotFound(err) {
				resource := &addonv1alpha1.KubewardenAddon{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &addonv1alpha1.KubewardenAddon{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				By(fmt.Sprintf("Cleanup the object %s", resource.GetName()))
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			// Clean up all clusters in default namespace
			clusterList := &clusterv1.ClusterList{}
			err = k8sClient.List(ctx, clusterList, client.InNamespace("default"))
			if err == nil {
				for _, cluster := range clusterList.Items {
					By(fmt.Sprintf("Cleanup cluster %s", cluster.GetName()))
					Expect(k8sClient.Delete(ctx, &cluster)).To(Succeed())
				}
			}

			// Clean up kubeconfig secrets
			secretList := &corev1.SecretList{}
			err = k8sClient.List(ctx, secretList, client.InNamespace("default"))
			if err == nil {
				for _, secret := range secretList.Items {
					if strings.Contains(secret.GetName(), "kubeconfig") {
						By(fmt.Sprintf("Cleanup secret %s", secret.GetName()))
						Expect(k8sClient.Delete(ctx, &secret)).To(Succeed())
					}
				}
			}
		})

		It("should successfully reconcile the resource", func() {
			By("Create CAPI Cluster & get remote client")
			cluster := capiCluster.DeepCopy()
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())
			cluster.Status.ControlPlaneReady = true
			Expect(k8sClient.Status().Update(ctx, cluster)).To(Succeed())

			Expect(k8sClient.Create(ctx, capiKubeconfigSecret)).To(Succeed())

			workloadClient, err := remote.NewClusterClient(ctx, cluster.Name, k8sClient, client.ObjectKeyFromObject(cluster))
			Expect(err).NotTo(HaveOccurred())

			controllerReconciler := &KubewardenAddonReconciler{
				Client:             k8sClient,
				Scheme:             k8sClient.Scheme(),
				RemoteClientGetter: remote.NewClusterClient,
			}

			By("Reconciling the created resource")
			Eventually(func(g Gomega) {
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespacedName,
				})
				g.Expect(err).NotTo(HaveOccurred())

				By("Kubewarden namespace should exist in workload cluster")
				kubewardenNs := &corev1.Namespace{}
				g.Expect(workloadClient.Get(ctx, client.ObjectKey{Name: kubewardenNamespace}, kubewardenNs)).To(Succeed())

				By("Kubewarden CRDs should exist in workload cluster")
				kubewardenCRDs := []string{
					"admissionpolicies.policies.kubewarden.io",
					"clusteradmissionpolicies.policies.kubewarden.io",
					"policyservers.policies.kubewarden.io",
				}
				for _, crd := range kubewardenCRDs {
					By(fmt.Sprintf("Checking CRD %s", crd))
					policyCRD := &apiextensionsv1.CustomResourceDefinition{}
					g.Expect(workloadClient.Get(ctx, client.ObjectKey{Name: crd}, policyCRD)).To(Succeed())
				}

				By("Kubewarden controller should be installed in workload cluster")
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster)).To(Succeed())
				deployment := &appsv1.Deployment{}
				err := workloadClient.Get(ctx, client.ObjectKey{Name: fmt.Sprintf("%s-kubewarden-controller", kubewardenHelmReleaseName), Namespace: kubewardenNamespace}, deployment)
				g.Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("%s-kubewarden-controller deployment should exist", kubewardenHelmReleaseName))
				g.Expect(deployment.Name).To(Equal(fmt.Sprintf("%s-kubewarden-controller", kubewardenHelmReleaseName)))

				By("Kubewarden defaults should be installed in workload cluster")
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster)).To(Succeed())
				policyServer := &policiesv1.PolicyServer{}
				err = workloadClient.Get(ctx, client.ObjectKey{Name: kubewardenHelmDefaultPolicyServerName, Namespace: kubewardenNamespace}, policyServer)
				g.Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("%s policy server should exist", kubewardenHelmDefaultPolicyServerName))
				g.Expect(policyServer.GetName()).To(Equal(kubewardenHelmDefaultPolicyServerName))

				By("Cluster should have installed annotation")
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster)).To(Succeed())
				annotations := cluster.GetAnnotations()
				_, ok := annotations[KubewardenInstalledAnnotation]
				g.Expect(ok).To(BeTrue())
			}).Should(Succeed())
		})

		It("should select clusters based on label selector", func() {
			By("Creating multiple clusters with different labels")
			cluster1 := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster-prod",
					Namespace: "default",
					Labels: map[string]string{
						"environment": "production",
						"tier":        "backend",
					},
				},
			}
			cluster2 := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster-dev",
					Namespace: "default",
					Labels: map[string]string{
						"environment": "development",
						"tier":        "frontend",
					},
				},
			}
			cluster3 := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster-staging",
					Namespace: "default",
					Labels: map[string]string{
						"environment": "staging",
					},
				},
			}

			Expect(k8sClient.Create(ctx, cluster1)).To(Succeed())
			Expect(k8sClient.Create(ctx, cluster2)).To(Succeed())
			Expect(k8sClient.Create(ctx, cluster3)).To(Succeed())

			controllerReconciler := &KubewardenAddonReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			By("Testing matchLabels selector")
			selector := metav1.LabelSelector{
				MatchLabels: map[string]string{
					"environment": "production",
				},
			}
			selected, err := controllerReconciler.selectClusters([]clusterv1.Cluster{*cluster1, *cluster2, *cluster3}, selector)
			Expect(err).NotTo(HaveOccurred())
			Expect(selected).To(HaveLen(1))
			Expect(selected[0].Name).To(Equal("cluster-prod"))

			By("Testing matchExpressions selector with In operator")
			selector = metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "environment",
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{"production", "staging"},
					},
				},
			}
			selected, err = controllerReconciler.selectClusters([]clusterv1.Cluster{*cluster1, *cluster2, *cluster3}, selector)
			Expect(err).NotTo(HaveOccurred())
			Expect(selected).To(HaveLen(2))
			Expect(selected[0].Name).To(Equal("cluster-prod"))
			Expect(selected[1].Name).To(Equal("cluster-staging"))

			By("Testing matchExpressions selector with NotIn operator")
			selector = metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "tier",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"frontend"},
					},
				},
			}
			selected, err = controllerReconciler.selectClusters([]clusterv1.Cluster{*cluster1, *cluster2, *cluster3}, selector)
			Expect(err).NotTo(HaveOccurred())
			Expect(selected).To(HaveLen(2))

			By("Testing selector that matches no clusters")
			selector = metav1.LabelSelector{
				MatchLabels: map[string]string{
					"environment": "nonexistent",
				},
			}
			selected, err = controllerReconciler.selectClusters([]clusterv1.Cluster{*cluster1, *cluster2, *cluster3}, selector)
			Expect(err).NotTo(HaveOccurred())
			Expect(selected).To(HaveLen(0))
		})

		It("should update Status.MatchingClusters with selected clusters", func() {
			By("Creating test clusters with matching labels")
			cluster1 := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster-1",
					Namespace: "default",
					Labels: map[string]string{
						"environment": "test",
					},
				},
			}
			cluster2 := &clusterv1.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster-2",
					Namespace: "default",
					Labels: map[string]string{
						"environment": "test",
					},
				},
			}

			Expect(k8sClient.Create(ctx, cluster1)).To(Succeed())
			Expect(k8sClient.Create(ctx, cluster2)).To(Succeed())

			By("Creating KubewardenAddon with matching label selector")
			addon := &addonv1alpha1.KubewardenAddon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "status-test",
					Namespace: "default",
				},
				Spec: addonv1alpha1.KubewardenAddonSpec{
					ClusterSelector: metav1.LabelSelector{
						MatchLabels: map[string]string{
							"environment": "test",
						},
					},
					PolicyServerConfig: addonv1alpha1.PolicyServerConfig{
						Replicas: 1,
					},
				},
			}
			Expect(k8sClient.Create(ctx, addon)).To(Succeed())

			controllerReconciler := &KubewardenAddonReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			By("Selecting clusters and updating status")
			allClusters, err := controllerReconciler.getAllCapiClusters(ctx, "default")
			Expect(err).NotTo(HaveOccurred())

			selected, err := controllerReconciler.selectClusters(allClusters, addon.Spec.ClusterSelector)
			Expect(err).NotTo(HaveOccurred())
			Expect(selected).To(HaveLen(2))

			addon.SetMatchingClusters(selected)

			By("Verifying Status.MatchingClusters is populated")
			Expect(addon.Status.MatchingClusters).To(HaveLen(2))
			clusterNames := []string{addon.Status.MatchingClusters[0].Name, addon.Status.MatchingClusters[1].Name}
			Expect(clusterNames).To(ContainElement("test-cluster-1"))
			Expect(clusterNames).To(ContainElement("test-cluster-2"))
		})
	})
})
