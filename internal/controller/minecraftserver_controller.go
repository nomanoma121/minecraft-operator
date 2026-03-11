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
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	minecraftv1alpha1 "github.com/nomanoma121/minecraft-operator/api/v1alpha1"
)

// MinecraftServerReconciler reconciles a MinecraftServer object
type MinecraftServerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=minecraft.nomanoma-dev.com,resources=minecraftservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=minecraft.nomanoma-dev.com,resources=minecraftservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=minecraft.nomanoma-dev.com,resources=minecraftservers/finalizers,verbs=update
// +kubebuilder:rbac:groups=minecraft.nomanoma-dev.com,resources=minecraftnetworks,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

func (r *MinecraftServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var server minecraftv1alpha1.MinecraftServer
	if err := r.Get(ctx, req.NamespacedName, &server); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	shouldContinue, err := r.reconcileNetworkOwnership(ctx, &server)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !shouldContinue {
		return ctrl.Result{}, nil
	}

	if err := r.Get(ctx, req.NamespacedName, &server); err != nil {
		return ctrl.Result{}, err
	}

	sts, err := r.reconcileStatefulSet(ctx, &server)
	if err != nil {
		return ctrl.Result{}, err
	}

	if err := r.reconcileService(ctx, &server); err != nil {
		return ctrl.Result{}, err
	}

	ready, err := r.reconcileStatus(ctx, &server, sts)
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Reconciled MinecraftServer", "name", server.Name, "ready", ready)
	return ctrl.Result{}, nil
}

func (r *MinecraftServerReconciler) reconcileNetworkOwnership(ctx context.Context, server *minecraftv1alpha1.MinecraftServer) (bool, error) {
	log := logf.FromContext(ctx)

	var network minecraftv1alpha1.MinecraftNetwork
	if err := r.Get(ctx, types.NamespacedName{
		Name:      server.Spec.NetworkRef,
		Namespace: server.Namespace,
	}, &network); err != nil {
		if errors.IsNotFound(err) {
			log.Info("Referenced MinecraftNetwork not found", "networkRef", server.Spec.NetworkRef)
			meta.SetStatusCondition(&server.Status.Conditions, metav1.Condition{
				Type:               "Ready",
				Status:             metav1.ConditionFalse,
				Reason:             "NetworkNotFound",
				Message:            fmt.Sprintf("MinecraftNetwork %q not found", server.Spec.NetworkRef),
				ObservedGeneration: server.Generation,
			})
			if err := r.Status().Update(ctx, server); err != nil {
				return false, err
			}
			return false, nil
		}
		return false, err
	}

	if err := controllerutil.SetOwnerReference(&network, server, r.Scheme); err != nil {
		return false, err
	}
	if err := r.Update(ctx, server); err != nil {
		return false, err
	}

	return true, nil
}

func (r *MinecraftServerReconciler) reconcileStatefulSet(
	ctx context.Context,
	server *minecraftv1alpha1.MinecraftServer,
) (*appsv1.StatefulSet, error) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      server.Name,
			Namespace: server.Namespace,
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, sts, func() error {
		r.buildStatefulSet(server, sts)
		return controllerutil.SetControllerReference(server, sts, r.Scheme)
	}); err != nil {
		return nil, err
	}

	return sts, nil
}

func (r *MinecraftServerReconciler) reconcileService(ctx context.Context, server *minecraftv1alpha1.MinecraftServer) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      server.Name,
			Namespace: server.Namespace,
		},
	}

	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		r.buildService(server, svc)
		return controllerutil.SetControllerReference(server, svc, r.Scheme)
	})
	return err
}

func (r *MinecraftServerReconciler) reconcileStatus(
	ctx context.Context,
	server *minecraftv1alpha1.MinecraftServer,
	sts *appsv1.StatefulSet,
) (bool, error) {
	ready := sts.Status.ReadyReplicas >= 1
	if ready {
		meta.SetStatusCondition(&server.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionTrue,
			Reason:             "StatefulSetReady",
			Message:            "StatefulSet has ready replicas",
			ObservedGeneration: server.Generation,
		})
	} else {
		meta.SetStatusCondition(&server.Status.Conditions, metav1.Condition{
			Type:               "Ready",
			Status:             metav1.ConditionFalse,
			Reason:             "StatefulSetNotReady",
			Message:            "Waiting for StatefulSet to become ready",
			ObservedGeneration: server.Generation,
		})
	}
	server.Status.Address = fmt.Sprintf("%s.%s.svc.cluster.local:25565", server.Name, server.Namespace)

	if err := r.Status().Update(ctx, server); err != nil {
		return false, err
	}

	return ready, nil
}

func (r *MinecraftServerReconciler) buildStatefulSet(server *minecraftv1alpha1.MinecraftServer, sts *appsv1.StatefulSet) {
	labels := map[string]string{
		"app.kubernetes.io/name":       "minecraft-server",
		"app.kubernetes.io/instance":   server.Name,
		"app.kubernetes.io/managed-by": "minecraft-operator",
	}

	replicas := int32(1)

	env := []corev1.EnvVar{
		{Name: "TYPE", Value: string(server.Spec.Type)},
		{Name: "VERSION", Value: server.Spec.Version},
		{Name: "EULA", Value: strconv.FormatBool(server.Spec.EULA)},
	}

	if server.Spec.Difficulty != "" {
		env = append(env, corev1.EnvVar{Name: "DIFFICULTY", Value: string(server.Spec.Difficulty)})
	}
	if server.Spec.WorldLevel != "" {
		env = append(env, corev1.EnvVar{Name: "LEVEL_TYPE", Value: string(server.Spec.WorldLevel)})
	}
	if len(server.Spec.WhiteList) > 0 {
		env = append(env, corev1.EnvVar{Name: "WHITELIST", Value: strings.Join(server.Spec.WhiteList, ",")})
		env = append(env, corev1.EnvVar{Name: "ENFORCE_WHITELIST", Value: "true"})
	}

	sts.Labels = labels
	sts.Spec = appsv1.StatefulSetSpec{
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
						Name:  "minecraft-server",
						Image: fmt.Sprintf("itzg/minecraft-server:%s", server.Spec.Version),
						Ports: []corev1.ContainerPort{
							{
								Name:          "minecraft",
								ContainerPort: 25565,
								Protocol:      corev1.ProtocolTCP,
							},
						},
						Env: env,
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "data",
								MountPath: "/data",
							},
						},
					},
				},
			},
		},
		VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "data",
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("10Gi"),
						},
					},
				},
			},
		},
	}
}

func (r *MinecraftServerReconciler) buildService(server *minecraftv1alpha1.MinecraftServer, svc *corev1.Service) {
	labels := map[string]string{
		"app.kubernetes.io/name":       "minecraft-server",
		"app.kubernetes.io/instance":   server.Name,
		"app.kubernetes.io/managed-by": "minecraft-operator",
	}

	svc.Labels = labels
	svc.Spec = corev1.ServiceSpec{
		Type:     corev1.ServiceTypeClusterIP,
		Selector: labels,
		Ports: []corev1.ServicePort{
			{
				Name:     "minecraft",
				Port:     25565,
				Protocol: corev1.ProtocolTCP,
			},
		},
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *MinecraftServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&minecraftv1alpha1.MinecraftServer{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.Service{}).
		Named("minecraftserver").
		Complete(r)
}
