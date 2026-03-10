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

type ServerRef struct {
	Name string `json:"name,omitempty"`
}

// MinecraftNetworkSpec defines the desired state of MinecraftNetwork
type MinecraftNetworkSpec struct {
	// +required
	ProxyRef      string      `json:"proxyRef"`
	// +kubebuilder:validation:MinItems=1
	// +required
	Servers       []ServerRef `json:"servers"`
	// +required
	DefaultServer string      `json:"defaultServer"`
}

// MinecraftNetworkStatus defines the observed state of MinecraftNetwork.
type MinecraftNetworkStatus struct {
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions   []metav1.Condition `json:"conditions,omitempty"`
	ReadyServers int32              `json:"readyServers,omitempty"`
	ProxyReady   bool               `json:"proxyReady,omitempty"`
	TotalServers int32              `json:"totalServers,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// MinecraftNetwork is the Schema for the minecraftnetworks API
type MinecraftNetwork struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of MinecraftNetwork
	// +required
	Spec MinecraftNetworkSpec `json:"spec"`

	// status defines the observed state of MinecraftNetwork
	// +optional
	Status MinecraftNetworkStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// MinecraftNetworkList contains a list of MinecraftNetwork
type MinecraftNetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []MinecraftNetwork `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MinecraftNetwork{}, &MinecraftNetworkList{})
}
