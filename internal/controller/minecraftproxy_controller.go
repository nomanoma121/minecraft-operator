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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	minecraftv1alpha1 "github.com/nomanoma121/minecraft-operator/api/v1alpha1"
)

// MinecraftProxyReconciler reconciles a MinecraftProxy object
type MinecraftProxyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=minecraft.nomanoma-dev.com,resources=minecraftproxies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=minecraft.nomanoma-dev.com,resources=minecraftproxies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=minecraft.nomanoma-dev.com,resources=minecraftproxies/finalizers,verbs=update
// +kubebuilder:rbac:groups=minecraft.nomanoma-dev.com,resources=minecraftnetworks,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

func (r *MinecraftProxyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var proxy minecraftv1alpha1.MinecraftProxy
	if err := r.Get(ctx, req.NamespacedName, &proxy); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	shouldContinue, err := r.reconcileNetworkOwnership(ctx, &proxy)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !shouldContinue {
		return ctrl.Result{}, nil
	}

	if err := r.Get(ctx, req.NamespacedName, &proxy); err != nil {
		return ctrl.Result{}, err
	}

	cm, err := r.reconcileConfigMap(ctx, &proxy)
	if err != nil {
		return ctrl.Result{}, err
	}

	deploy, err := r.reconcileDeployment(ctx, &proxy, cm)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileService(ctx, &proxy); err != nil {
		return ctrl.Result{}, err
	}

	ready, err := r.reconcileStatus(ctx, &proxy, deploy)
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Reconciled MinecraftProxy", "name", proxy.Name, "ready", ready)
	return ctrl.Result{}, nil
}

func (r *MinecraftProxyReconciler) reconcileNetworkOwnership(ctx context.Context, proxy *minecraftv1alpha1.MinecraftProxy) (bool, error) {
	log := logf.FromContext(ctx)

	var network minecraftv1alpha1.MinecraftNetwork
	if err := r.Get(ctx, types.NamespacedName{
		Name:      proxy.Spec.NetworkRef,
		Namespace: proxy.Namespace,
	}, &network); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Referenced MinecraftNetwork not found", "networkRef", proxy.Spec.NetworkRef)
			meta.SetStatusCondition(&proxy.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				Reason:             "NetworkNotFound",
				Message:            fmt.Sprintf("MinecraftNetwork %q not found", proxy.Spec.NetworkRef),
				ObservedGeneration: proxy.Generation,
			})
			if err := r.Status().Update(ctx, proxy); err != nil {
				return false, err
			}
			return false, nil
		}
		return false, err
	}

	if err := controllerutil.SetOwnerReference(&network, proxy, r.Scheme); err != nil {
		return false, err
	}
	if err := r.Update(ctx, proxy); err != nil {
		return false, err
	}

	return true, nil
}

func (r *MinecraftProxyReconciler) reconcileConfigMap(ctx context.Context, proxy *minecraftv1alpha1.MinecraftProxy) (*corev1.ConfigMap, error) {
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
		// Initialize with empty velocity.toml if not set (Network controller will update it)
		if _, ok := cm.Data["velocity.toml"]; !ok {
			cm.Data["velocity.toml"] = "[servers]\n\ntry = []\n"
		}
		return controllerutil.SetControllerReference(proxy, cm, r.Scheme)
	}); err != nil {
		return nil, err
	}

	return cm, nil
}

func (r *MinecraftProxyReconciler) reconcileDeployment(
	ctx context.Context,
	proxy *minecraftv1alpha1.MinecraftProxy,
	cm *corev1.ConfigMap,
) (*appsv1.Deployment, error) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      proxy.Name,
			Namespace: proxy.Namespace,
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, deploy, func() error {
		r.buildDeployment(proxy, deploy, cm)
		return controllerutil.SetControllerReference(proxy, deploy, r.Scheme)
	}); err != nil {
		return nil, err
	}

	return deploy, nil
}

func (r *MinecraftProxyReconciler) reconcileService(ctx context.Context, proxy *minecraftv1alpha1.MinecraftProxy) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      proxy.Name,
			Namespace: proxy.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		r.buildService(proxy, svc)
		return controllerutil.SetControllerReference(proxy, svc, r.Scheme)
	})
	return err
}

func (r *MinecraftProxyReconciler) reconcileStatus(
	ctx context.Context,
	proxy *minecraftv1alpha1.MinecraftProxy,
	deploy *appsv1.Deployment,
) (bool, error) {
	ready := deploy.Status.AvailableReplicas >= 1
	if ready {
		meta.SetStatusCondition(&proxy.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "DeploymentReady",
			Message:            "Deployment has available replicas",
			ObservedGeneration: proxy.Generation,
		})
	} else {
		meta.SetStatusCondition(&proxy.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "DeploymentNotReady",
			Message:            "Waiting for Deployment to become ready",
			ObservedGeneration: proxy.Generation,
		})
	}
	proxy.Status.ReadyReplicas = deploy.Status.ReadyReplicas
	proxy.Status.Address = fmt.Sprintf("%s.%s.svc.cluster.local:25577", proxy.Name, proxy.Namespace)

	if err := r.Status().Update(ctx, proxy); err != nil {
		return false, err
	}

	return ready, nil
}

func (r *MinecraftProxyReconciler) buildDeployment(proxy *minecraftv1alpha1.MinecraftProxy, deploy *appsv1.Deployment, cm *corev1.ConfigMap) {
	labels := map[string]string{
		"app.kubernetes.io/name":       "minecraft-proxy",
		"app.kubernetes.io/instance":   proxy.Name,
		"app.kubernetes.io/managed-by": "minecraft-operator",
	}

	replicas := proxy.Spec.Replicas
	if replicas == 0 {
		replicas = 1
	}

	env := []corev1.EnvVar{
		{Name: "TYPE", Value: strings.ToUpper(string(proxy.Spec.Type))},
	}

	deploy.Labels = labels
	deploy.Spec = appsv1.DeploymentSpec{
		Replicas: &replicas,
		Selector: &metav1.LabelSelector{
			MatchLabels: labels,
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: labels,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "minecraft-proxy",
						Image: fmt.Sprintf("itzg/mc-proxy:%s", proxy.Spec.Version),
						Ports: []corev1.ContainerPort{
							{
								Name:          "proxy",
								ContainerPort: 25577,
								Protocol:      corev1.ProtocolTCP,
							},
						},
						Env: env,
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "velocity-config",
								MountPath: "/server/velocity.toml",
								SubPath:   "velocity.toml",
							},
						},
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: "velocity-config",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: cm.Name,
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *MinecraftProxyReconciler) buildService(proxy *minecraftv1alpha1.MinecraftProxy, svc *corev1.Service) {
	labels := map[string]string{
		"app.kubernetes.io/name":       "minecraft-proxy",
		"app.kubernetes.io/instance":   proxy.Name,
		"app.kubernetes.io/managed-by": "minecraft-operator",
	}

	svc.Labels = labels
	svc.Spec = corev1.ServiceSpec{
		Type:     corev1.ServiceTypeClusterIP,
		Selector: labels,
		Ports: []corev1.ServicePort{
			{
				Name:     "proxy",
				Port:     25577,
				Protocol: corev1.ProtocolTCP,
			},
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *MinecraftProxyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&minecraftv1alpha1.MinecraftProxy{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Named("minecraftproxy").
		Complete(r)
}
