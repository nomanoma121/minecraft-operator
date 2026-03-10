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

type MinecraftServerType string

const (
	MinecraftServerTypePaper   MinecraftServerType = "Paper"
	MinecraftServerTypeVanilla MinecraftServerType = "Vanilla"
)

type MinecraftServerWorldLevel string

const (
	MinecraftServerWorldLevelNormal MinecraftServerWorldLevel = "Normal"
	MinecraftServerWorldLevelFlat   MinecraftServerWorldLevel = "Flat"
	MinecraftServerWorldLevelAmplified   MinecraftServerWorldLevel = "Amplified"
)

type MinecraftServerDifficulty string

const (
	MinecraftServerDifficultyPeaceful MinecraftServerDifficulty = "Peaceful"
	MinecraftServerDifficultyEasy     MinecraftServerDifficulty = "Easy"
	MinecraftServerDifficultyNormal   MinecraftServerDifficulty = "Normal"
	MinecraftServerDifficultyHard     MinecraftServerDifficulty = "Hard"
)

// MinecraftServerSpec defines the desired state of MinecraftServer
type MinecraftServerSpec struct {
	// +kubebuilder:validation:Enum=Paper;Vanilla
	// +required
	Type       MinecraftServerType       `json:"type"`
	// +kubebuilder:validation:MinLength=1
	// +required
	Version    string                    `json:"version"`
	// +required
	EULA       bool                      `json:"eula"`
	// +optional
	// +kubebuilder:validation:MinItems=1
	WhiteList  []string                  `json:"whiteList,omitempty"`
	// +kubebuilder:validation:Enum=Normal;Flat;Amplified
	// +optional
	WorldLevel MinecraftServerWorldLevel `json:"worldLevel,omitempty"`
	// +kubebuilder:validation:Enum=Peaceful;Easy;Normal;Hard
	// +optional
	Difficulty MinecraftServerDifficulty `json:"difficulty,omitempty"`
}

// MinecraftServerStatus defines the observed state of MinecraftServer.
type MinecraftServerStatus struct {
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	Address    string             `json:"address,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// MinecraftServer is the Schema for the minecraftservers API
type MinecraftServer struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of MinecraftServer
	// +required
	Spec MinecraftServerSpec `json:"spec"`

	// status defines the observed state of MinecraftServer
	// +optional
	Status MinecraftServerStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// MinecraftServerList contains a list of MinecraftServer
type MinecraftServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []MinecraftServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MinecraftServer{}, &MinecraftServerList{})
}
