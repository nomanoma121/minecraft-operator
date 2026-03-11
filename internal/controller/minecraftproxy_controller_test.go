/*
Copyright 2026.

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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	minecraftv1alpha1 "github.com/nomanoma121/minecraft-operator/api/v1alpha1"
)

var _ = Describe("MinecraftProxy Controller", func() {
	const (
		timeout  = 30 * time.Second
		interval = 250 * time.Millisecond
	)

	var (
		namespace   string
		networkName string
		h           *Harness
	)

	BeforeEach(func() {
		namespace = "default"
		networkName = fmt.Sprintf("test-network-%d", time.Now().UnixNano())
		h = NewHarness(ctx, namespace, timeout, interval)

		h.CreateNetwork(networkName, CreateNetworkOpts{})
	})

	Context("When creating a MinecraftProxy", func() {
		It("Should create Deployment, Service and ConfigMap for Velocity", func() {
			proxyName := fmt.Sprintf("test-proxy-%d", time.Now().UnixNano())

			proxy := &minecraftv1alpha1.MinecraftProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      proxyName,
					Namespace: namespace,
				},
				Spec: minecraftv1alpha1.MinecraftProxySpec{
					NetworkRef: networkName,
					Version:    "latest",
					Type:       minecraftv1alpha1.MinecraftProxyTypeVelocity,
					Replicas:   1,
				},
			}
			Expect(k8sClient.Create(ctx, proxy)).To(Succeed())
			h.ReconcileProxyOnce(proxyName)

			By("Checking Deployment spec")
			deploy := &appsv1.Deployment{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: proxyName, Namespace: namespace}, deploy)
			}, timeout, interval).Should(Succeed())
			Expect(deploy.Spec.Template.Spec.Containers).To(HaveLen(1))
			container := deploy.Spec.Template.Spec.Containers[0]
			Expect(container.Image).To(Equal("itzg/mc-proxy:latest"))
			Expect(container.Ports).To(ContainElement(corev1.ContainerPort{
				Name:          "proxy",
				ContainerPort: 25577,
				Protocol:      corev1.ProtocolTCP,
			}))
			Expect(container.Env).To(ContainElement(corev1.EnvVar{
				Name:  "TYPE",
				Value: "VELOCITY",
			}))

			By("Checking ConfigMap for velocity.toml")
			cm := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      proxyName + "-velocity-config",
					Namespace: namespace,
				}, cm)
			}, timeout, interval).Should(Succeed())
			Expect(cm.Data).To(HaveKey("velocity.toml"))

			By("Checking Service spec")
			svc := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{Name: proxyName, Namespace: namespace}, svc)
			}, timeout, interval).Should(Succeed())
			Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(svc.Spec.Ports).To(HaveLen(1))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(25577)))

			By("Checking status address")
			Eventually(func() string {
				p := &minecraftv1alpha1.MinecraftProxy{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: proxyName, Namespace: namespace}, p); err != nil {
					return ""
				}
				return p.Status.Address
			}, timeout, interval).Should(Equal(fmt.Sprintf("%s.%s.svc.cluster.local:25577", proxyName, namespace)))

			By("Checking owner reference on Proxy (Network owns Proxy)")
			Eventually(func() bool {
				p := &minecraftv1alpha1.MinecraftProxy{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: proxyName, Namespace: namespace}, p); err != nil {
					return false
				}
				for _, ref := range p.OwnerReferences {
					if ref.Kind == "MinecraftNetwork" && ref.Name == networkName {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When networkRef references a non-existent Network", func() {
		It("Should set Ready condition to False with NetworkNotFound reason", func() {
			proxyName := fmt.Sprintf("orphan-proxy-%d", time.Now().UnixNano())
			proxy := &minecraftv1alpha1.MinecraftProxy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      proxyName,
					Namespace: namespace,
				},
				Spec: minecraftv1alpha1.MinecraftProxySpec{
					NetworkRef: "non-existent-network",
					Version:    "latest",
					Type:       minecraftv1alpha1.MinecraftProxyTypeVelocity,
				},
			}
			Expect(k8sClient.Create(ctx, proxy)).To(Succeed())
			h.ReconcileProxyOnce(proxyName)

			Eventually(func() bool {
				p := &minecraftv1alpha1.MinecraftProxy{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: proxyName, Namespace: namespace}, p); err != nil {
					return false
				}
				for _, c := range p.Status.Conditions {
					if c.Type == "Ready" && c.Status == metav1.ConditionFalse && c.Reason == "NetworkNotFound" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})
	})
})
