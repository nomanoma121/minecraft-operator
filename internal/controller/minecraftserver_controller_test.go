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

	Context("Resource reconciliation", func() {
		It("creates StatefulSet/Service and sets expected spec fields", func() {
			namespace := "default"
			h := NewHarness(ctx, namespace, timeout, interval)
			networkName := fmt.Sprintf("test-network-%d", time.Now().UnixNano())
			serverName := fmt.Sprintf("test-server-%d", time.Now().UnixNano())
			h.CreateNetwork(networkName, CreateNetworkOpts{})

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

			sts := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      serverName,
					Namespace: namespace,
				}, sts)
			}, timeout, interval).Should(Succeed())

			Expect(*sts.Spec.Replicas).To(Equal(int32(1)))
			Expect(sts.Spec.Template.Spec.Containers).To(HaveLen(1))

			container := sts.Spec.Template.Spec.Containers[0]
			Expect(container.Image).To(Equal("itzg/minecraft-server:1.19.4"))
			Expect(container.Ports).To(ContainElement(corev1.ContainerPort{
				Name:          "minecraft",
				ContainerPort: 25565,
				Protocol:      corev1.ProtocolTCP,
			}))

			envMap := make(map[string]string)
			for _, e := range container.Env {
				envMap[e.Name] = e.Value
			}
			Expect(envMap["TYPE"]).To(Equal("Paper"))
			Expect(envMap["VERSION"]).To(Equal("1.19.4"))
			Expect(envMap["EULA"]).To(Equal("true"))
			Expect(envMap["DIFFICULTY"]).To(Equal("Normal"))
			Expect(envMap["LEVEL_TYPE"]).To(Equal("Normal"))

			Expect(sts.Spec.VolumeClaimTemplates).To(HaveLen(1))
			Expect(sts.Spec.VolumeClaimTemplates[0].Name).To(Equal("data"))

			svc := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      serverName,
					Namespace: namespace,
				}, svc)
			}, timeout, interval).Should(Succeed())

			Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(svc.Spec.Ports[0].Port).To(Equal(int32(25565)))

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

			for _, ref := range sts.OwnerReferences {
				if ref.Kind == "MinecraftServer" {
					Expect(ref.Name).To(Equal(serverName))
				}
			}

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

	Context("Network reference handling", func() {
		It("sets Ready=False with NetworkNotFound when referenced network does not exist", func() {
			namespace := "default"
			h := NewHarness(ctx, namespace, timeout, interval)
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

	Context("Spec updates", func() {
		It("updates StatefulSet image when spec.version changes", func() {
			namespace := "default"
			h := NewHarness(ctx, namespace, timeout, interval)
			networkName := fmt.Sprintf("test-network-%d", time.Now().UnixNano())
			h.CreateNetwork(networkName, CreateNetworkOpts{})
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

			sts := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name: name, Namespace: namespace,
				}, sts)
			}, timeout, interval).Should(Succeed())
			Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal("itzg/minecraft-server:1.19.4"))

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
