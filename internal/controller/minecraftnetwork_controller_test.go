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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
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
			proxy := h.CreateProxy(networkName, "proxy", CreateProxyOpts{})
			server := h.CreateServer(networkName, "server-ready", CreateServerOpts{})

			// Proxy側が初期ConfigMapを作る前提
			h.ReconcileProxyOnce(proxy.Name)
			h.SetServerReadyCondition(server.Name, true)
			h.ReconcileNetworkOnce(networkName)

			Eventually(func() string {
				cm := &corev1.ConfigMap{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      proxy.Name + "-velocity-config",
					Namespace: h.Namespace,
				}, cm); err != nil {
					return ""
				}
				return cm.Data["velocity.toml"]
			}, timeout, interval).Should(And(
				ContainSubstring("[servers]"),
				ContainSubstring(fmt.Sprintf(
					`%s = "%s.%s.svc.cluster.local:25565"`,
					server.Name,
					server.Name,
					h.Namespace,
				)),
			))
		})

		It("puts defaultServer first when it exists", func() {
			ctx := context.Background()
			h := NewHarness(ctx, "default", timeout, interval)
			networkName := fmt.Sprintf("network-%d", time.Now().UnixNano())
			h.CreateNetwork(networkName, CreateNetworkOpts{})
			proxy := h.CreateProxy(networkName, "proxy", CreateProxyOpts{})
			defaultServer := h.CreateServer(networkName, "server-default", CreateServerOpts{})
			otherServer := h.CreateServer(networkName, "server-other", CreateServerOpts{})

			// 作成後に実名を defaultServer へ設定
			Eventually(func() error {
				n := &minecraftv1alpha1.MinecraftNetwork{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: networkName, Namespace: h.Namespace}, n); err != nil {
					return err
				}
				n.Spec.DefaultServer = defaultServer.Name
				return k8sClient.Update(ctx, n)
			}, timeout, interval).Should(Succeed())

			h.ReconcileProxyOnce(proxy.Name)
			h.SetServerReadyCondition(defaultServer.Name, true)
			h.SetServerReadyCondition(otherServer.Name, true)
			h.ReconcileNetworkOnce(networkName)

			Eventually(func() []string {
				return extractTryServers(getVelocityConfigToml(ctx, h, proxy.Name))
			}, timeout, interval).Should(SatisfyAll(
				Not(BeEmpty()),
				WithTransform(func(servers []string) string { return servers[0] }, Equal(defaultServer.Name)),
			))
		})

		It("falls back to lobby first when defaultServer is empty or invalid", func() {
			ctx := context.Background()
			h := NewHarness(ctx, "default", timeout, interval)
			networkName := fmt.Sprintf("network-%d", time.Now().UnixNano())
			invalidDefault := "not-exist-server"

			h.CreateNetwork(networkName, CreateNetworkOpts{DefaultServer: &invalidDefault})
			proxy := h.CreateProxy(networkName, "proxy", CreateProxyOpts{})
			lobby := h.CreateServer(networkName, "lobby", CreateServerOpts{})
			other := h.CreateServer(networkName, "server-other", CreateServerOpts{})

			h.ReconcileProxyOnce(proxy.Name)
			h.SetServerReadyCondition(lobby.Name, true)
			h.SetServerReadyCondition(other.Name, true)
			h.ReconcileNetworkOnce(networkName)

			Eventually(func() []string {
				return extractTryServers(getVelocityConfigToml(ctx, h, proxy.Name))
			}, timeout, interval).Should(SatisfyAll(
				Not(BeEmpty()),
				WithTransform(func(servers []string) string { return servers[0] }, Equal(lobby.Name)),
			))
		})

		It("sorts remaining try entries by server name", func() {
			ctx := context.Background()
			h := NewHarness(ctx, "default", timeout, interval)
			networkName := fmt.Sprintf("network-%d", time.Now().UnixNano())

			h.CreateNetwork(networkName, CreateNetworkOpts{})
			proxy := h.CreateProxy(networkName, "proxy", CreateProxyOpts{})
			serverB := h.CreateServer(networkName, "b-server", CreateServerOpts{})
			serverA := h.CreateServer(networkName, "a-server", CreateServerOpts{})
			serverC := h.CreateServer(networkName, "c-server", CreateServerOpts{})

			h.ReconcileProxyOnce(proxy.Name)
			h.SetServerReadyCondition(serverA.Name, true)
			h.SetServerReadyCondition(serverB.Name, true)
			h.SetServerReadyCondition(serverC.Name, true)
			h.ReconcileNetworkOnce(networkName)

			Eventually(func() []string {
				return extractTryServers(getVelocityConfigToml(ctx, h, proxy.Name))
			}, timeout, interval).Should(SatisfyAll(
				WithTransform(func(servers []string) int { return len(servers) }, BeNumerically(">=", 3)),
				WithTransform(func(servers []string) []string {
					if len(servers) < 3 {
						return nil
					}
					return servers[:3]
				}, Equal([]string{serverA.Name, serverB.Name, serverC.Name})),
			))
		})

		It("writes try=[] when no related servers exist", func() {
			ctx := context.Background()
			h := NewHarness(ctx, "default", timeout, interval)
			networkName := fmt.Sprintf("network-%d", time.Now().UnixNano())

			h.CreateNetwork(networkName, CreateNetworkOpts{})
			proxy := h.CreateProxy(networkName, "proxy", CreateProxyOpts{})

			h.ReconcileProxyOnce(proxy.Name)
			h.ReconcileNetworkOnce(networkName)

			Eventually(func() string {
				return getVelocityConfigToml(ctx, h, proxy.Name)
			}, timeout, interval).Should(ContainSubstring("try = []"))
		})
	})

	Context("Resource selection scope", func() {
		It("selects only resources in the same namespace", func() {
			ctx := context.Background()
			h := NewHarness(ctx, "default", timeout, interval)
			networkName := fmt.Sprintf("network-%d", time.Now().UnixNano())
			h.CreateNetwork(networkName, CreateNetworkOpts{})
			proxy := h.CreateProxy(networkName, "proxy", CreateProxyOpts{})
			server := h.CreateServer(networkName, "server-ready", CreateServerOpts{})

			// 同名相当のリソースを別namespaceに作成
			otherNamespace := h.CreateNamespace("other-ns")
			otherH := NewHarness(ctx, otherNamespace, timeout, interval)
			otherH.CreateServer(networkName, server.Name, CreateServerOpts{})

			h.SetProxyReadyCondition(proxy.Name, true)
			h.SetServerReadyCondition(server.Name, true)
			h.ReconcileNetworkOnce(networkName)

			Eventually(func() [2]int32 {
				n := &minecraftv1alpha1.MinecraftNetwork{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: networkName, Namespace: h.Namespace}, n); err != nil {
					return [2]int32{-1, -1}
				}
				return [2]int32{n.Status.TotalServers, n.Status.ReadyServers}
			}, timeout, interval).Should(Equal([2]int32{1, 1}))
		})
		It("selects only resources whose networkRef matches network name", func() {
			ctx := context.Background()
			h := NewHarness(ctx, "default", timeout, interval)
			network1 := fmt.Sprintf("network1-%d", time.Now().UnixNano())
			network2 := fmt.Sprintf("network2-%d", time.Now().UnixNano())

			h.CreateNetwork(network1, CreateNetworkOpts{})
			h.CreateNetwork(network2, CreateNetworkOpts{})

			proxy1 := h.CreateProxy(network1, "proxy1", CreateProxyOpts{})
			proxy2 := h.CreateProxy(network2, "proxy2", CreateProxyOpts{})
			server1 := h.CreateServer(network1, "server1", CreateServerOpts{})
			server2 := h.CreateServer(network2, "server2", CreateServerOpts{})

			h.SetProxyReadyCondition(proxy1.Name, true)
			h.SetProxyReadyCondition(proxy2.Name, true)
			h.SetServerReadyCondition(server1.Name, true)
			h.SetServerReadyCondition(server2.Name, true)
			h.ReconcileNetworkOnce(network1)
			h.ReconcileNetworkOnce(network2)

			Eventually(func() [2]int32 {
				n1 := &minecraftv1alpha1.MinecraftNetwork{}
				n2 := &minecraftv1alpha1.MinecraftNetwork{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: network1, Namespace: h.Namespace}, n1); err != nil {
					return [2]int32{-1, -1}
				}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: network2, Namespace: h.Namespace}, n2); err != nil {
					return [2]int32{-1, -1}
				}
				return [2]int32{n1.Status.ReadyServers, n2.Status.ReadyServers}
			}, timeout, interval).Should(Equal([2]int32{1, 1}))
		})
	})

	Context("Explicit reconcile behavior", func() {
		It("updates network status when reconciled after a server starts referencing it", func() {
			ctx := context.Background()
			h := NewHarness(ctx, "default", timeout, interval)
			networkName := fmt.Sprintf("network-%d", time.Now().UnixNano())

			h.CreateNetwork(networkName, CreateNetworkOpts{})
			proxy := h.CreateProxy(networkName, "proxy", CreateProxyOpts{})

			// 最初はserverがいない状態
			h.ReconcileProxyOnce(proxy.Name)
			h.ReconcileNetworkOnce(networkName)

			Eventually(func() int32 {
				n := &minecraftv1alpha1.MinecraftNetwork{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: networkName, Namespace: h.Namespace}, n); err != nil {
					return -1
				}
				return n.Status.TotalServers
			}, timeout, interval).Should(Equal(int32(0)))

			// serverを作成してnetworkを参照させる
			server := h.CreateServer(networkName, "server-ready", CreateServerOpts{})
			h.SetServerReadyCondition(server.Name, true)
			h.ReconcileNetworkOnce(networkName)

			Eventually(func() int32 {
				n := &minecraftv1alpha1.MinecraftNetwork{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: networkName, Namespace: h.Namespace}, n); err != nil {
					return -1
				}
				return n.Status.TotalServers
			}, timeout, interval).Should(Equal(int32(1)))
			Eventually(func() bool {
				n := &minecraftv1alpha1.MinecraftNetwork{}
				if err := k8sClient.Get(ctx, types.NamespacedName{Name: networkName, Namespace: h.Namespace}, n); err != nil {
					return false
				}
				return meta.IsStatusConditionTrue(n.Status.Conditions, "Ready")
			}, timeout, interval).Should(BeTrue())
		})
	})
})

func getVelocityConfigToml(ctx context.Context, h *Harness, proxyName string) string {
	cm := &corev1.ConfigMap{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      proxyName + "-velocity-config",
		Namespace: h.Namespace,
	}, cm); err != nil {
		return ""
	}
	return cm.Data["velocity.toml"]
}

func extractTryServers(toml string) []string {
	for _, line := range strings.Split(toml, "\n") {
		trimmedLine := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmedLine, "try =") {
			continue
		}

		start := strings.Index(trimmedLine, "[")
		end := strings.LastIndex(trimmedLine, "]")
		if start == -1 || end == -1 || end < start {
			return nil
		}

		body := strings.TrimSpace(trimmedLine[start+1 : end])
		if body == "" {
			return []string{}
		}

		parts := strings.Split(body, ",")
		servers := make([]string, 0, len(parts))
		for _, part := range parts {
			server := strings.TrimSpace(part)
			server = strings.Trim(server, `"'`)
			if server != "" {
				servers = append(servers, server)
			}
		}
		return servers
	}

	return nil
}
