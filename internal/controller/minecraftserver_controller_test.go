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

var _ = Describe("MinecraftServer Controller", func() {
	const (
		timeout  = 30 * time.Second
		interval = 250 * time.Millisecond
	)

	var (
		namespace   string
		networkName string
		serverName  string
		h           *Harness
	)

	BeforeEach(func() {
		namespace = "default"
		networkName = fmt.Sprintf("test-network-%d", time.Now().UnixNano())
		serverName = fmt.Sprintf("test-server-%d", time.Now().UnixNano())
		h = NewHarness(ctx, namespace, timeout, interval)

		h.CreateNetwork(networkName, CreateNetworkOpts{})
	})

	Context("When creating a MinecraftServer", func() {
		It("Should create a StatefulSet and Service", func() {
			server := &minecraftv1alpha1.MinecraftServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      serverName,
					Namespace: namespace,
				},
				Spec: minecraftv1alpha1.MinecraftServerSpec{
					NetworkRef: networkName,
					Type:       minecraftv1alpha1.MinecraftServerTypePaper,
					Version:    "1.19.4",
					EULA:       true,
					Difficulty: minecraftv1alpha1.MinecraftServerDifficultyNormal,
					WorldLevel: minecraftv1alpha1.MinecraftServerWorldLevelNormal,
				},
			}
			Expect(k8sClient.Create(ctx, server)).To(Succeed())
			h.ReconcileServerOnce(serverName)

			// Verify StatefulSet is created
			sts := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      serverName,
					Namespace: namespace,
				}, sts)
			}, timeout, interval).Should(Succeed())

			By("Checking StatefulSet spec")
			Expect(*sts.Spec.Replicas).To(Equal(int32(1)))
			Expect(sts.Spec.Template.Spec.Containers).To(HaveLen(1))

			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.Image).To(Equal("itzg/minecraft-server:1.19.4"))
			Expect(container.Ports).To(ContainElement(corev1.ContainerPort{
				Name:          "minecraft",
				ContainerPort: 25565,
				Protocol:      corev1.ProtocolTCP,
			}))

			By("Checking environment variables")
			envMap := make(map[string]string)
			for _, e := range container.Env {
				envMap[e.Name] = e.Value
			}
			Expect(envMap["TYPE"]).To(Equal("Paper"))
			Expect(envMap["VERSION"]).To(Equal("1.19.4"))
			Expect(envMap["EULA"]).To(Equal("true"))
			Expect(envMap["DIFFICULTY"]).To(Equal("Normal"))
			Expect(envMap["LEVEL_TYPE"]).To(Equal("Normal"))

			By("Checking VolumeClaimTemplates")
			Expect(sts.Spec.VolumeClaimTemplates).To(HaveLen(1))
			Expect(sts.Spec.VolumeClaimTemplates[0].Name).To(Equal("data"))

			// Verify Service is created
			svc := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      serverName,
					Namespace: namespace,
				}, svc)
			}, timeout, interval).Should(Succeed())

			By("Checking Service spec")
			Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(25565)))

			// Verify owner reference: Network → Server
			By("Checking owner reference on Server (Network owns Server)")
			updatedServer := &minecraftv1alpha1.MinecraftServer{}
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serverName,
					Namespace: namespace,
				}, updatedServer); err != nil {
					return false
				}
				for _, ref := range updatedServer.OwnerReferences {
					if ref.Kind == "MinecraftNetwork" && ref.Name == networkName {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())

			// Verify owner reference: Server → StatefulSet
			By("Checking owner reference on StatefulSet (Server owns StatefulSet)")
			for _, ref := range sts.OwnerReferences {
				if ref.Kind == "MinecraftServer" {
					Expect(ref.Name).To(Equal(serverName))
				}
			}

			// Verify status address
			By("Checking status address")
			Eventually(func() string {
				s := &minecraftv1alpha1.MinecraftServer{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      serverName,
					Namespace: namespace,
				}, s); err != nil {
					return ""
				}
				return s.Status.Address
			}, timeout, interval).Should(Equal(
				fmt.Sprintf("%s.%s.svc.cluster.local:25565", serverName, namespace),
			))
		})
	})

	Context("When networkRef references a non-existent Network", func() {
		It("Should set Ready condition to False with NetworkNotFound reason", func() {
			orphanName := fmt.Sprintf("orphan-server-%d", time.Now().UnixNano())
			server := &minecraftv1alpha1.MinecraftServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      orphanName,
					Namespace: namespace,
				},
				Spec: minecraftv1alpha1.MinecraftServerSpec{
					NetworkRef: "non-existent-network",
					Type:       minecraftv1alpha1.MinecraftServerTypePaper,
					Version:    "1.19.4",
					EULA:       true,
				},
			}
			Expect(k8sClient.Create(ctx, server)).To(Succeed())
			h.ReconcileServerOnce(orphanName)

			Eventually(func() bool {
				s := &minecraftv1alpha1.MinecraftServer{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      orphanName,
					Namespace: namespace,
				}, s); err != nil {
					return false
				}
				for _, c := range s.Status.Conditions {
					if c.Type == "Ready" && c.Status == metav1.ConditionFalse && c.Reason == "NetworkNotFound" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When updating spec.version", func() {
		It("Should update the StatefulSet image", func() {
			name := fmt.Sprintf("update-server-%d", time.Now().UnixNano())
			server := &minecraftv1alpha1.MinecraftServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: minecraftv1alpha1.MinecraftServerSpec{
					NetworkRef: networkName,
					Type:       minecraftv1alpha1.MinecraftServerTypePaper,
					Version:    "1.19.4",
					EULA:       true,
				},
			}
			Expect(k8sClient.Create(ctx, server)).To(Succeed())
			h.ReconcileServerOnce(name)

			// Wait for StatefulSet to be created
			sts := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name: name, Namespace: namespace,
				}, sts)
			}, timeout, interval).Should(Succeed())
			Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal("itzg/minecraft-server:1.19.4"))

			// Update version
			Eventually(func() error {
				s := &minecraftv1alpha1.MinecraftServer{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: name, Namespace: namespace,
				}, s); err != nil {
					return err
				}
				s.Spec.Version = "1.21.4"
				return k8sClient.Update(ctx, s)
			}, timeout, interval).Should(Succeed())
			h.ReconcileServerOnce(name)

			// Verify image is updated
			Eventually(func() string {
				s := &appsv1.StatefulSet{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: name, Namespace: namespace,
				}, s); err != nil {
					return ""
				}
				return s.Spec.Template.Spec.Containers[0].Image
			}, timeout, interval).Should(Equal("itzg/minecraft-server:1.21.4"))
		})
	})
})
