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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
			Expect(err).NotTo(HaveOccurred())

			resourcesToCleanup := []client.Object{
				resource,
				capiCluster,
			}

			for _, resource := range resourcesToCleanup {
				By(fmt.Sprintf("Cleanup the object %s", resource.GetName()))
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
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
				//By("Cluster should have installed annotation")
				//g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(cluster), cluster)).To(Succeed())
				//annotations := cluster.GetAnnotations()
				//_, ok := annotations[KubewardenInstalledAnnotation]
				//g.Expect(ok).To(BeTrue())
			}).Should(Succeed())
		})
	})
})
