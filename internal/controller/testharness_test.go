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

type Harness struct {
	Ctx       context.Context
	Namespace string
	Timeout   time.Duration
	Interval  time.Duration
}

func NewHarness(ctx context.Context, namespace string, timeout time.Duration, interval time.Duration) *Harness {
	return &Harness{
		Ctx:       ctx,
		Namespace: namespace,
		Timeout:   timeout,
		Interval:  interval,
	}
}

type CreateNetworkOpts struct {
	DefaultServer *string
}

func (h *Harness) CreateNetwork(name string, opts CreateNetworkOpts) *minecraftv1alpha1.MinecraftNetwork {
	network := &minecraftv1alpha1.MinecraftNetwork{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: h.Namespace,
		},
	}
	if opts.DefaultServer != nil {
		network.Spec.DefaultServer = *opts.DefaultServer
	}
	Expect(k8sClient.Create(h.Ctx, network)).To(Succeed())
	DeferCleanup(func() {
		Expect(client.IgnoreNotFound(k8sClient.Delete(h.Ctx, network))).To(Succeed())
	})
	return network
}

type CreateServerOpts struct {
	Version    *string
	Type       *minecraftv1alpha1.MinecraftServerType
	EULA       *bool
	Difficulty *minecraftv1alpha1.MinecraftServerDifficulty
	WorldLevel *minecraftv1alpha1.MinecraftServerWorldLevel
	Whitelist  *[]string
}

func (h *Harness) CreateServer(
	networkName string,
	namePrefix string,
	opts CreateServerOpts,
) *minecraftv1alpha1.MinecraftServer {
	server := &minecraftv1alpha1.MinecraftServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%d", namePrefix, time.Now().UnixNano()),
			Namespace: h.Namespace,
		},
		Spec: applyServerSpecOpts(minecraftv1alpha1.MinecraftServerSpec{
			NetworkRef: networkName,
			Type:       minecraftv1alpha1.MinecraftServerTypePaper,
			Version:    "1.21.4",
			EULA:       true,
		}, &opts),
	}
	Expect(k8sClient.Create(h.Ctx, server)).To(Succeed())
	DeferCleanup(func() {
		Expect(client.IgnoreNotFound(k8sClient.Delete(h.Ctx, server))).To(Succeed())
	})

	return server
}

type CreateProxyOpts struct {
	Type     *minecraftv1alpha1.MinecraftProxyType
	Version  *string
	Replicas *int32
}

func (h *Harness) CreateProxy(networkName string, namePrefix string, opts CreateProxyOpts) *minecraftv1alpha1.MinecraftProxy {
	proxy := &minecraftv1alpha1.MinecraftProxy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%d", namePrefix, time.Now().UnixNano()),
			Namespace: h.Namespace,
		},
		Spec: applyProxySpecOpts(minecraftv1alpha1.MinecraftProxySpec{
			NetworkRef: networkName,
			Type:       minecraftv1alpha1.MinecraftProxyTypeVelocity,
			Version:    "latest",
			Replicas:   1,
		}, &opts),
	}
	Expect(k8sClient.Create(h.Ctx, proxy)).To(Succeed())
	DeferCleanup(func() {
		Expect(client.IgnoreNotFound(k8sClient.Delete(h.Ctx, proxy))).To(Succeed())
	})

	return proxy
}

func (h *Harness) SetServerReadyCondition(serverName string, ready bool) {
	Eventually(func() error {
		server := &minecraftv1alpha1.MinecraftServer{}
		if err := k8sClient.Get(h.Ctx, types.NamespacedName{Name: serverName, Namespace: h.Namespace}, server); err != nil {
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

		return k8sClient.Status().Update(h.Ctx, server)
	}, h.Timeout, h.Interval).Should(Succeed())
}

func (h *Harness) SetProxyReadyCondition(proxyName string, ready bool) {
	Eventually(func() error {
		proxy := &minecraftv1alpha1.MinecraftProxy{}
		if err := k8sClient.Get(h.Ctx, types.NamespacedName{Name: proxyName, Namespace: h.Namespace}, proxy); err != nil {
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

		return k8sClient.Status().Update(h.Ctx, proxy)
	}, h.Timeout, h.Interval).Should(Succeed())
}

func (h *Harness) ReconcileNetworkOnce(networkName string) {
	reconciler := &MinecraftNetworkReconciler{
		Client: k8sClient,
		Scheme: k8sClient.Scheme(),
	}

	_, err := reconciler.Reconcile(h.Ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: networkName, Namespace: h.Namespace},
	})
	Expect(err).NotTo(HaveOccurred())
}

func (h *Harness) ReconcileServerOnce(serverName string) {
	reconciler := &MinecraftServerReconciler{
		Client: k8sClient,
		Scheme: k8sClient.Scheme(),
	}

	_, err := reconciler.Reconcile(h.Ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: serverName, Namespace: h.Namespace},
	})
	Expect(err).NotTo(HaveOccurred())
}

func (h *Harness) ReconcileProxyOnce(proxyName string) {
	reconciler := &MinecraftProxyReconciler{
		Client: k8sClient,
		Scheme: k8sClient.Scheme(),
	}

	_, err := reconciler.Reconcile(h.Ctx, reconcile.Request{
		NamespacedName: types.NamespacedName{Name: proxyName, Namespace: h.Namespace},
	})
	Expect(err).NotTo(HaveOccurred())
}

func applyServerSpecOpts(defaultSpec minecraftv1alpha1.MinecraftServerSpec, opts *CreateServerOpts) minecraftv1alpha1.MinecraftServerSpec {
	if opts == nil {
		return defaultSpec
	}

	if opts.Difficulty != nil {
		defaultSpec.Difficulty = *opts.Difficulty
	}
	if opts.EULA != nil {
		defaultSpec.EULA = *opts.EULA
	}
	if opts.Type != nil {
		defaultSpec.Type = *opts.Type
	}
	if opts.Version != nil {
		defaultSpec.Version = *opts.Version
	}
	if opts.WorldLevel != nil {
		defaultSpec.WorldLevel = *opts.WorldLevel
	}
	if opts.Whitelist != nil {
		defaultSpec.WhiteList = *opts.Whitelist
	}
	return defaultSpec
}

func applyProxySpecOpts(defaultSpec minecraftv1alpha1.MinecraftProxySpec, opts *CreateProxyOpts) minecraftv1alpha1.MinecraftProxySpec {
	if opts == nil {
		return defaultSpec
	}

	if opts.Type != nil {
		defaultSpec.Type = *opts.Type
	}
	if opts.Version != nil {
		defaultSpec.Version = *opts.Version
	}
	if opts.Replicas != nil {
		defaultSpec.Replicas = *opts.Replicas
	}
	return defaultSpec
}