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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type MinecraftProxyType string

const (
	MinecraftProxyTypeVelocity MinecraftProxyType = "Velocity"
)

// MinecraftProxySpec defines the desired state of MinecraftProxy
type MinecraftProxySpec struct {
	// +kubebuilder:validation:MinLength=1
	// +required
	Version  string             `json:"version"`
	// +kubebuilder:validation:Enum=Velocity
	// +required
	Type     MinecraftProxyType `json:"type"`
	// +kubebuilder:default=1
	// +optional
	Replicas int32              `json:"replicas,omitempty"`
}

// MinecraftProxyStatus defines the observed state of MinecraftProxy.
type MinecraftProxyStatus struct {
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions    []metav1.Condition `json:"conditions,omitempty"`
	ReadyReplicas int32              `json:"readyReplicas,omitempty"`
	Address       string             `json:"address,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// MinecraftProxy is the Schema for the minecraftproxies API
type MinecraftProxy struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of MinecraftProxy
	// +required
	Spec MinecraftProxySpec `json:"spec"`

	// status defines the observed state of MinecraftProxy
	// +optional
	Status MinecraftProxyStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// MinecraftProxyList contains a list of MinecraftProxy
type MinecraftProxyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []MinecraftProxy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MinecraftProxy{}, &MinecraftProxyList{})
}
