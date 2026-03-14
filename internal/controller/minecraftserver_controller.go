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
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

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

	configMapName, err := r.reconcileConfigMap(ctx, &server)
	if err != nil {
		return ctrl.Result{}, err
	}

	sts, err := r.reconcileStatefulSet(ctx, &server, configMapName)
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
	configMapName string,
) (*appsv1.StatefulSet, error) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      server.Name,
			Namespace: server.Namespace,
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, sts, func() error {
		r.buildStatefulSet(server, sts, configMapName)
		return controllerutil.SetControllerReference(server, sts, r.Scheme)
	}); err != nil {
		return nil, err
	}

	return sts, nil
}

func (r *MinecraftServerReconciler) reconcileConfigMap(ctx context.Context, server *minecraftv1alpha1.MinecraftServer) (string, error) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      server.Name + "-config",
			Namespace: server.Namespace,
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, cm, func() error {
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}

		cm.Data["paper-global.yml"] = strings.Join([]string{
			"proxies:",
			"  velocity:",
			"    enabled: true",
			"    online-mode: true",
			`    secret: "${CFG_VELOCITY_SECRET}"`,
			"",
		}, "\n")

		return controllerutil.SetControllerReference(server, cm, r.Scheme)
	}); err != nil {
		return "", err
	}

	return cm.Name, nil
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

func (r *MinecraftServerReconciler) buildStatefulSet(
	server *minecraftv1alpha1.MinecraftServer,
	sts *appsv1.StatefulSet,
	configMapName string,
) {
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
		{Name: "ONLINE_MODE", Value: "false"},
		{
			Name: "CFG_VELOCITY_SECRET",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: server.Spec.NetworkRef + "-forwarding-secret",
					},
					Key: "forwarding.secret",
				},
			},
		},
	}

	if server.Spec.Difficulty != "" {
		env = append(env, corev1.EnvVar{Name: "DIFFICULTY", Value: string(server.Spec.Difficulty)})
	}
	if server.Spec.WorldLevel != "" {
		env = append(env, corev1.EnvVar{Name: "LEVEL_TYPE", Value: string(server.Spec.WorldLevel)})
	}
	if server.Spec.Gamemode != "" {
		env = append(env, corev1.EnvVar{Name: "MODE", Value: strings.ToLower(string(server.Spec.Gamemode))})
	}
	if server.Spec.PVP {
		env = append(env, corev1.EnvVar{Name: "PVP", Value: "true"})
	}
	if len(server.Spec.Ops) > 0 {
		env = append(env, corev1.EnvVar{Name: "OPS", Value: strings.Join(server.Spec.Ops, ",")})
	}
	if server.Spec.Seed != 0 {
		env = append(env, corev1.EnvVar{Name: "SEED", Value: strconv.FormatInt(server.Spec.Seed, 10)})
	}
	if server.Spec.MOTD != "" {
		env = append(env, corev1.EnvVar{Name: "MOTD", Value: server.Spec.MOTD})
	}
	if server.Spec.MaxPlayers > 0 {
		env = append(env, corev1.EnvVar{Name: "MAX_PLAYERS", Value: strconv.Itoa(int(server.Spec.MaxPlayers))})
	}
	enableWhitelist := server.Spec.WhiteListEnabled || len(server.Spec.WhiteList) > 0
	if enableWhitelist {
		env = append(env, corev1.EnvVar{Name: "ENABLE_WHITELIST", Value: "true"})
	}
	if len(server.Spec.WhiteList) > 0 {
		env = append(env, corev1.EnvVar{Name: "WHITELIST", Value: strings.Join(server.Spec.WhiteList, ",")})
	}
	enforceWhitelist := server.Spec.EnforceWhitelist || len(server.Spec.WhiteList) > 0
	if enforceWhitelist {
		env = append(env, corev1.EnvVar{Name: "ENFORCE_WHITELIST", Value: "true"})
	}
	if server.Spec.Memory != "" {
		env = append(env, corev1.EnvVar{Name: "MEMORY", Value: server.Spec.Memory})
	}
	if server.Spec.SimulationDistance > 0 {
		env = append(env, corev1.EnvVar{Name: "SIMULATION_DISTANCE", Value: strconv.Itoa(int(server.Spec.SimulationDistance))})
	}
	if server.Spec.ViewDistance > 0 {
		env = append(env, corev1.EnvVar{Name: "VIEW_DISTANCE", Value: strconv.Itoa(int(server.Spec.ViewDistance))})
	}
	if len(server.Spec.Plugins) > 0 {
		env = append(env, corev1.EnvVar{Name: "PLUGINS", Value: strings.Join(server.Spec.Plugins, ",")})
	}
	if len(server.Spec.Mods) > 0 {
		env = append(env, corev1.EnvVar{Name: "MODS", Value: strings.Join(server.Spec.Mods, ",")})
	}

	storageRequest := resource.MustParse("10Gi")
	if server.Spec.StorageSize != "" {
		if q, err := resource.ParseQuantity(server.Spec.StorageSize); err == nil {
			storageRequest = q
		}
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
						Image: "itzg/minecraft-server:latest",
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
							{
								Name:      "server-config",
								MountPath: "/config/paper-global.yml",
								SubPath:   "paper-global.yml",
							},
						},
					},
				},
				Volumes: []corev1.Volume{
					{
						Name: "server-config",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: configMapName,
								},
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
							corev1.ResourceStorage: storageRequest,
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
