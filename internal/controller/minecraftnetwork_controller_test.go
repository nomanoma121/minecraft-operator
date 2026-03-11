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
	"k8s.io/apimachinery/pkg/types"

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
			h := NewHarness(ctx, "default", timeout, interval)
			networkName := fmt.Sprintf("network-%d", time.Now().UnixNano())

			h.CreateNetwork(networkName, CreateNetworkOpts{})
			server1 := h.CreateServer(networkName, "server-ready", CreateServerOpts{})
			server2 := h.CreateServer(networkName, "server-not-ready", CreateServerOpts{})

			h.SetServerReadyCondition(server1.Name, true)
			h.SetServerReadyCondition(server2.Name, false)
			h.ReconcileNetworkOnce(networkName)

			Eventually(func() [2]int32 {
				n := &minecraftv1alpha1.MinecraftNetwork{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: networkName, Namespace: h.Namespace}, n); err != nil {
					return [2]int32{-1, -1}
				}
				return [2]int32{n.Status.TotalServers, n.Status.ReadyServers}
			}, timeout, interval).Should(Equal([2]int32{2, 1}))
		})
		It("sets ProxyReady true when at least one related proxy is ready", func() {
			ctx := context.Background()
			h := NewHarness(ctx, "default", timeout, interval)
			networkName := fmt.Sprintf("network-%d", time.Now().UnixNano())

			h.CreateNetwork(networkName, CreateNetworkOpts{})
			proxy1 := h.CreateProxy(networkName, "proxy-ready", CreateProxyOpts{})
			proxy2 := h.CreateProxy(networkName, "proxy-not-ready", CreateProxyOpts{})

			h.SetProxyReadyCondition(proxy1.Name, true)
			h.SetProxyReadyCondition(proxy2.Name, false)
			h.ReconcileNetworkOnce(networkName)

			Eventually(func() bool {
				n := &minecraftv1alpha1.MinecraftNetwork{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: networkName, Namespace: h.Namespace}, n); err != nil {
					return false
				}
				return n.Status.ProxyReady
			}, timeout, interval).Should(BeTrue())
		})
		It("sets Ready condition true when ProxyReady=true and ReadyServers>0", func() {
			ctx := context.Background()
			h := NewHarness(ctx, "default", timeout, interval)
			networkName := fmt.Sprintf("network-%d", time.Now().UnixNano())

			h.CreateNetwork(networkName, CreateNetworkOpts{})
			proxy := h.CreateProxy(networkName, "proxy-ready", CreateProxyOpts{})
			server := h.CreateServer(networkName, "server-ready", CreateServerOpts{})

			h.SetProxyReadyCondition(proxy.Name, true)
			h.SetServerReadyCondition(server.Name, true)
			h.ReconcileNetworkOnce(networkName)

			Eventually(func() bool {
				n := &minecraftv1alpha1.MinecraftNetwork{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: networkName, Namespace: h.Namespace}, n); err != nil {
					return false
				}
				return meta.IsStatusConditionTrue(n.Status.Conditions, "Ready")
			}, timeout, interval).Should(BeTrue())
		})
		It("sets Ready condition false when either proxy/server readiness is insufficient", func() {
			ctx := context.Background()
			h := NewHarness(ctx, "default", timeout, interval)
			networkName := fmt.Sprintf("network-%d", time.Now().UnixNano())

			h.CreateNetwork(networkName, CreateNetworkOpts{})
			proxy := h.CreateProxy(networkName, "proxy-not-ready", CreateProxyOpts{})
			server := h.CreateServer(networkName, "server-not-ready", CreateServerOpts{})

			h.SetProxyReadyCondition(proxy.Name, false)
			h.SetServerReadyCondition(server.Name, false)
			h.ReconcileNetworkOnce(networkName)

			Eventually(func() bool {
				n := &minecraftv1alpha1.MinecraftNetwork{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: networkName, Namespace: h.Namespace}, n); err != nil {
					return false
				}
				return meta.IsStatusConditionFalse(n.Status.Conditions, "Ready")
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("velocity.toml generation", func() {
		It("writes [servers] entries with service FQDN and port 25565", func() {
			ctx := context.Background()
			h := NewHarness(ctx, "default", timeout, interval)
			networkName := fmt.Sprintf("network-%d", time.Now().UnixNano())

			h.CreateNetwork(networkName, CreateNetworkOpts{})
			server := h.CreateServer(networkName, "server-ready", CreateServerOpts{})
			h.SetServerReadyCondition(server.Name, true)
			h.ReconcileNetworkOnce(networkName)

			Eventually(func() string {
				p := &minecraftv1alpha1.MinecraftProxy{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: server.Name, Namespace: h.Namespace}, p); err != nil {
					return ""
				}
				return p.Status.Address
			}, timeout, interval).Should(ContainSubstring(fmt.Sprintf(`address = "%s.%s.svc.cluster.local:25565"`, server.Name, h.Namespace)))
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
