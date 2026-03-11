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
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	minecraftv1alpha1 "github.com/nomanoma121/minecraft-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// MinecraftNetworkReconciler reconciles a MinecraftNetwork object
type MinecraftNetworkReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=minecraft.nomanoma-dev.com,resources=minecraftnetworks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=minecraft.nomanoma-dev.com,resources=minecraftnetworks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=minecraft.nomanoma-dev.com,resources=minecraftnetworks/finalizers,verbs=update
// +kubebuilder:rbac:groups=minecraft.nomanoma-dev.com,resources=minecraftservers,verbs=get;list;watch
// +kubebuilder:rbac:groups=minecraft.nomanoma-dev.com,resources=minecraftproxies,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MinecraftNetwork object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.1/pkg/reconcile
func (r *MinecraftNetworkReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var network minecraftv1alpha1.MinecraftNetwork
	if err := r.Get(ctx, req.NamespacedName, &network); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	servers, err := r.listRelatedServers(ctx, &network)
	if err != nil {
		return ctrl.Result{}, err
	}

	proxies, err := r.listRelatedProxies(ctx, &network)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileVelocityConfigMaps(ctx, &network, servers, proxies); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileStatus(ctx, &network, servers, proxies); err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Reconciled MinecraftNetwork", "name", network.Name, "servers", len(servers), "proxies", len(proxies))
	return ctrl.Result{}, nil
}

func (r *MinecraftNetworkReconciler) listRelatedServers(
	ctx context.Context,
	network *minecraftv1alpha1.MinecraftNetwork,
) ([]minecraftv1alpha1.MinecraftServer, error) {
	var serverList minecraftv1alpha1.MinecraftServerList
	if err := r.List(ctx, &serverList, client.InNamespace(network.Namespace)); err != nil {
		return nil, err
	}

	servers := make([]minecraftv1alpha1.MinecraftServer, 0, len(serverList.Items))
	for i := range serverList.Items {
		server := serverList.Items[i]
		if server.Spec.NetworkRef != network.Name {
			continue
		}
		servers = append(servers, server)
	}

	return servers, nil
}

func (r *MinecraftNetworkReconciler) listRelatedProxies(
	ctx context.Context,
	network *minecraftv1alpha1.MinecraftNetwork,
) ([]minecraftv1alpha1.MinecraftProxy, error) {
	var proxyList minecraftv1alpha1.MinecraftProxyList
	if err := r.List(ctx, &proxyList, client.InNamespace(network.Namespace)); err != nil {
		return nil, err
	}

	proxies := make([]minecraftv1alpha1.MinecraftProxy, 0, len(proxyList.Items))
	for i := range proxyList.Items {
		proxy := proxyList.Items[i]
		if proxy.Spec.NetworkRef != network.Name {
			continue
		}
		proxies = append(proxies, proxy)
	}

	return proxies, nil
}

func (r *MinecraftNetworkReconciler) reconcileVelocityConfigMaps(
	ctx context.Context,
	network *minecraftv1alpha1.MinecraftNetwork,
	servers []minecraftv1alpha1.MinecraftServer,
	proxies []minecraftv1alpha1.MinecraftProxy,
) error {
	toml := renderVelocityToml(network, servers)

	for i := range proxies {
		proxy := &proxies[i]
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      proxy.Name + "-velocity-config",
				Namespace: proxy.Namespace,
			},
		}

		if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
			if cm.Data == nil {
				cm.Data = map[string]string{}
			}
			cm.Data["velocity.toml"] = toml
			return controllerutil.SetControllerReference(proxy, cm, r.Scheme)
		}); err != nil {
			return err
		}
	}

	return nil
}

func (r *MinecraftNetworkReconciler) reconcileStatus(
	ctx context.Context,
	network *minecraftv1alpha1.MinecraftNetwork,
	servers []minecraftv1alpha1.MinecraftServer,
	proxies []minecraftv1alpha1.MinecraftProxy,
) error {
	readyServers := int32(0)
	for i := range servers {
		if meta.IsStatusConditionTrue(servers[i].Status.Conditions, "Ready") {
			readyServers++
		}
	}

	proxyReady := false
	for i := range proxies {
		if meta.IsStatusConditionTrue(proxies[i].Status.Conditions, "Ready") {
			proxyReady = true
			break
		}
	}

	network.Status.TotalServers = int32(len(servers))
	network.Status.ReadyServers = readyServers
	network.Status.ProxyReady = proxyReady

	if proxyReady && readyServers > 0 {
		meta.SetStatusCondition(&network.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "NetworkReady",
			Message:            "At least one proxy and one server are ready",
			ObservedGeneration: network.Generation,
		})
	} else {
		meta.SetStatusCondition(&network.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "NetworkNotReady",
			Message:            "Waiting for at least one ready proxy and one ready server",
			ObservedGeneration: network.Generation,
		})
	}

	return r.Status().Update(ctx, network)
}

func renderVelocityToml(network *minecraftv1alpha1.MinecraftNetwork, servers []minecraftv1alpha1.MinecraftServer) string {
	sortedServers := make([]minecraftv1alpha1.MinecraftServer, len(servers))
	copy(sortedServers, servers)
	sort.Slice(sortedServers, func(i, j int) bool {
		return sortedServers[i].Name < sortedServers[j].Name
	})

	lines := []string{"[servers]"}
	for i := range sortedServers {
		serverName := sortedServers[i].Name
		address := fmt.Sprintf("%s.%s.svc.cluster.local:25565", serverName, network.Namespace)
		lines = append(lines, fmt.Sprintf(`%s = "%s"`, serverName, address))
	}

	tryServers := buildTryServers(network.Spec.DefaultServer, sortedServers)
	quotedTry := make([]string, 0, len(tryServers))
	for _, serverName := range tryServers {
		quotedTry = append(quotedTry, fmt.Sprintf("%q", serverName))
	}

	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("try = [%s]", strings.Join(quotedTry, ", ")))

	return strings.Join(lines, "\n") + "\n"
}

func buildTryServers(defaultServer string, sortedServers []minecraftv1alpha1.MinecraftServer) []string {
	if len(sortedServers) == 0 {
		return []string{}
	}

	names := make([]string, 0, len(sortedServers))
	exists := make(map[string]struct{}, len(sortedServers))
	for i := range sortedServers {
		name := sortedServers[i].Name
		names = append(names, name)
		exists[name] = struct{}{}
	}

	priority := ""
	if defaultServer != "" {
		if _, ok := exists[defaultServer]; ok {
			priority = defaultServer
		}
	}
	if priority == "" {
		if _, ok := exists["lobby"]; ok {
			priority = "lobby"
		}
	}
	if priority == "" {
		return names
	}

	tryServers := []string{priority}
	for _, name := range names {
		if name == priority {
			continue
		}
		tryServers = append(tryServers, name)
	}
	return tryServers
}

func (r *MinecraftNetworkReconciler) mapServerToNetwork(_ context.Context, obj client.Object) []reconcile.Request {
	server, ok := obj.(*minecraftv1alpha1.MinecraftServer)
	if !ok || server.Spec.NetworkRef == "" {
		return nil
	}
	return []reconcile.Request{{
		NamespacedName: client.ObjectKey{
			Name:      server.Spec.NetworkRef,
			Namespace: server.Namespace,
		},
	}}
}

func (r *MinecraftNetworkReconciler) mapProxyToNetwork(_ context.Context, obj client.Object) []reconcile.Request {
	proxy, ok := obj.(*minecraftv1alpha1.MinecraftProxy)
	if !ok || proxy.Spec.NetworkRef == "" {
		return nil
	}
	return []reconcile.Request{{
		NamespacedName: client.ObjectKey{
			Name:      proxy.Spec.NetworkRef,
			Namespace: proxy.Namespace,
		},
	}}
}

// SetupWithManager sets up the controller with the Manager.
func (r *MinecraftNetworkReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&minecraftv1alpha1.MinecraftNetwork{}).
		Watches(&minecraftv1alpha1.MinecraftServer{}, handler.EnqueueRequestsFromMapFunc(r.mapServerToNetwork)).
		Watches(&minecraftv1alpha1.MinecraftProxy{}, handler.EnqueueRequestsFromMapFunc(r.mapProxyToNetwork)).
		Named("minecraftnetwork").
		Complete(r)
}
