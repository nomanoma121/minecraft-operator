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
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	minecraftv1alpha1 "github.com/nomanoma121/minecraft-operator/api/v1alpha1"
)

var _ = Describe("MinecraftNetwork Controller", func() {
	const (
		timeout  = 30 * time.Second
		interval = 250 * time.Millisecond
	)

	Context("Status aggregation", func() {
		It("sets TotalServers and ReadyServers from related servers", func() {
			ctx := context.Background()
			namespace := "default"
			networkName := fmt.Sprintf("network-%d", time.Now().UnixNano())

			createNetwork(ctx, namespace, networkName)
			server1 := createServer(ctx, namespace, networkName, "server-ready")
			server2 := createServer(ctx, namespace, networkName, "server-not-ready")

			setServerReadyCondition(ctx, namespace, server1.Name, true, timeout, interval)
			setServerReadyCondition(ctx, namespace, server2.Name, false, timeout, interval)
			reconcileNetworkOnce(ctx, namespace, networkName)

			Eventually(func() [2]int32 {
				n := &minecraftv1alpha1.MinecraftNetwork{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: networkName, Namespace: namespace}, n); err != nil {
					return [2]int32{-1, -1}
				}
				return [2]int32{n.Status.TotalServers, n.Status.ReadyServers}
			}, timeout, interval).Should(Equal([2]int32{2, 1}))
		})
		It("sets ProxyReady true when at least one related proxy is ready", func() {
			ctx := context.Background()
			namespace := "default"
			networkName := fmt.Sprintf("network-%d", time.Now().UnixNano())

			createNetwork(ctx, namespace, networkName)
			proxy1 := createProxy(ctx, namespace, networkName, "proxy-ready")
			proxy2 := createProxy(ctx, namespace, networkName, "proxy-not-ready")

			setProxyReadyCondition(ctx, namespace, proxy1.Name, true, timeout, interval)
			setProxyReadyCondition(ctx, namespace, proxy2.Name, false, timeout, interval)
			reconcileNetworkOnce(ctx, namespace, networkName)

			Eventually(func() bool {
				n := &minecraftv1alpha1.MinecraftNetwork{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: networkName, Namespace: namespace}, n); err != nil {
					return false
				}
				return n.Status.ProxyReady
			}, timeout, interval).Should(BeTrue())
		})
		It("sets Ready condition true when ProxyReady=true and ReadyServers>0", func() {
			ctx := context.Background()
			namespace := "default"
			networkName := fmt.Sprintf("network-%d", time.Now().UnixNano())

			createNetwork(ctx, namespace, networkName)
			proxy := createProxy(ctx, namespace, networkName, "proxy-ready")
			server := createServer(ctx, namespace, networkName, "server-ready")

			setProxyReadyCondition(ctx, namespace, proxy.Name, true, timeout, interval)
			setServerReadyCondition(ctx, namespace, server.Name, true, timeout, interval)
			reconcileNetworkOnce(ctx, namespace, networkName)

			Eventually(func() bool {
				n := &minecraftv1alpha1.MinecraftNetwork{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: networkName, Namespace: namespace}, n); err != nil {
					return false
				}
				return meta.IsStatusConditionTrue(n.Status.Conditions, "Ready")
			}, timeout, interval).Should(BeTrue())
		})
		It("sets Ready condition false when either proxy/server readiness is insufficient", func() {
			ctx := context.Background()
			namespace := "default"
			networkName := fmt.Sprintf("network-%d", time.Now().UnixNano())

			createNetwork(ctx, namespace, networkName)
			proxy := createProxy(ctx, namespace, networkName, "proxy-not-ready")
			server := createServer(ctx, namespace, networkName, "server-not-ready")

			setProxyReadyCondition(ctx, namespace, proxy.Name, false, timeout, interval)
			setServerReadyCondition(ctx, namespace, server.Name, false, timeout, interval)
			reconcileNetworkOnce(ctx, namespace, networkName)

			Eventually(func() bool {
				n := &minecraftv1alpha1.MinecraftNetwork{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: networkName, Namespace: namespace}, n); err != nil {
					return false
				}
				return meta.IsStatusConditionFalse(n.Status.Conditions, "Ready")
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("velocity.toml generation", func() {
		It("writes [servers] entries with service FQDN and port 25565", func() {

		})
		It("puts defaultServer first when it exists", func() {
			// Create a MinecraftNetwork and related MinecraftServers with a default server
			// Verify that the default server is listed first in the velocity.toml
		})
		It("falls back to lobby first when defaultServer is empty or invalid", func() {
			// Create a MinecraftNetwork and related MinecraftServers without a valid default server
			// Verify that the lobby server is listed first in the velocity.toml
		})
		It("sorts remaining try entries by server name", func() {
			// Create a MinecraftNetwork and related MinecraftServers with multiple servers
			// Verify that the remaining servers are sorted alphabetically in the velocity.toml
		})
		It("writes try=[] when no related servers exist", func() {
			// Create a MinecraftNetwork without any related MinecraftServers
			// Verify that the try array is empty in the velocity.toml
		})
	})

	Context("Resource selection scope", func() {
		It("selects only resources in the same namespace", func() {
			// Create a MinecraftNetwork and related resources in different namespaces
			// Verify that only resources in the same namespace are selected
		})
		It("selects only resources whose networkRef matches network name", func() {
			// Create a MinecraftNetwork and related resources with different networkRef values
			// Verify that only resources with matching networkRef are selected
		})
	})

	Context("Reconcile triggers from related resources", func() {
		It("reconciles network when a server newly references it", func() {
			// Create a MinecraftNetwork and related MinecraftServers
			// Verify that the network is reconciled when a server references it
		})
		It("reconciles network when a proxy newly references it", func() {
			// Create a MinecraftNetwork and related MinecraftProxies
			// Verify that the network is reconciled when a proxy references it
		})
		It("updates network status when related server/proxy readiness changes", func() {
			// Create a MinecraftNetwork and related MinecraftServers/Proxies
			// Verify that the network status is updated when related resource readiness changes
		})
	})

	Context("Edge cases", func() {
		It("does not fail hard when defaultServer is not found", func() {
			// Create a MinecraftNetwork and related MinecraftServers without a valid default server
			// Verify that the network does not fail and handles the missing default server gracefully
		})
		It("keeps reconciliation idempotent across repeated runs", func() {
			// Create a MinecraftNetwork and related resources
			// Verify that repeated reconciliations do not cause unintended side effects
		})
	})

})

func createNetwork(ctx context.Context, namespace string, networkName string) {
	network := &minecraftv1alpha1.MinecraftNetwork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      networkName,
			Namespace: namespace,
		},
	}
	Expect(k8sClient.Create(ctx, network)).To(Succeed())
	DeferCleanup(func() {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, network))).To(Succeed())
	})
}

func createServer(
	ctx context.Context,
	namespace string,
	networkName string,
	namePrefix string,
) *minecraftv1alpha1.MinecraftServer {
	server := &minecraftv1alpha1.MinecraftServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%d", namePrefix, time.Now().UnixNano()),
			Namespace: namespace,
		},
		Spec: minecraftv1alpha1.MinecraftServerSpec{
			NetworkRef: networkName,
			Type:       minecraftv1alpha1.MinecraftServerTypePaper,
			Version:    "1.21.4",
			EULA:       true,
		},
	}
	Expect(k8sClient.Create(ctx, server)).To(Succeed())
	DeferCleanup(func() {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, server))).To(Succeed())
	})

	return server
}

func createProxy(
	ctx context.Context,
	namespace string,
	networkName string,
	namePrefix string,
) *minecraftv1alpha1.MinecraftProxy {
	proxy := &minecraftv1alpha1.MinecraftProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%d", namePrefix, time.Now().UnixNano()),
			Namespace: namespace,
		},
		Spec: minecraftv1alpha1.MinecraftProxySpec{
			NetworkRef: networkName,
			Type:       minecraftv1alpha1.MinecraftProxyTypeVelocity,
			Version:    "latest",
			Replicas:   1,
		},
	}
	Expect(k8sClient.Create(ctx, proxy)).To(Succeed())
	DeferCleanup(func() {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, proxy))).To(Succeed())
	})

	return proxy
}

func setServerReadyCondition(
	ctx context.Context,
	namespace string,
	serverName string,
	ready bool,
	timeout time.Duration,
	interval time.Duration,
) {
	Eventually(func() error {
		server := &minecraftv1alpha1.MinecraftServer{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: serverName, Namespace: namespace}, server); err != nil {
			return err
		}

		status := metav1.ConditionFalse
		reason := "TestNotReady"
		message := "server is not ready for aggregation test"
		if ready {
			status = metav1.ConditionTrue
			reason = "TestReady"
			message = "server is ready for aggregation test"
		}

		meta.SetStatusCondition(&server.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             status,
			Reason:             reason,
			Message:            message,
			ObservedGeneration: server.Generation,
		})

		return k8sClient.Status().Update(ctx, server)
	}, timeout, interval).Should(Succeed())
}

func setProxyReadyCondition(
	ctx context.Context,
	namespace string,
	proxyName string,
	ready bool,
	timeout time.Duration,
	interval time.Duration,
) {
	Eventually(func() error {
		proxy := &minecraftv1alpha1.MinecraftProxy{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: proxyName, Namespace: namespace}, proxy); err != nil {
			return err
		}

		status := metav1.ConditionFalse
		reason := "TestNotReady"
		message := "proxy is not ready for aggregation test"
		if ready {
			status = metav1.ConditionTrue
			reason = "TestReady"
			message = "proxy is ready for aggregation test"
		}

		meta.SetStatusCondition(&proxy.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             status,
			Reason:             reason,
			Message:            message,
			ObservedGeneration: proxy.Generation,
		})

		return k8sClient.Status().Update(ctx, proxy)
	}, timeout, interval).Should(Succeed())
}

func reconcileNetworkOnce(ctx context.Context, namespace string, networkName string) {
	reconciler := &MinecraftNetworkReconciler{
		Client: k8sClient,
		Scheme: k8sClient.Scheme(),
	}

	_, err := reconciler.Reconcile(ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: networkName, Namespace: namespace},
	})
	Expect(err).NotTo(HaveOccurred())
}
