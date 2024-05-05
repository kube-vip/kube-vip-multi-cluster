/*
Copyright 2024.

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

// MultiClusterSpec defines the desired state of MultiCluster
type MultiClusterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	Address      string   `json:"address,omitempty"` // if blank will default to 0.0.0.0
	LoadBalancer string   `json:"serviceAddress,omitempty"`
	Port         int      `json:"port"`
	Fleet        []string `json:"fleet,omitempty"`
	Callsign     string   `json:"callsign"`
	Ready        bool     `json:"ready"`
	Rank         int      `json:"rank"`
	Payload      string   `json:"payload"`
}

// MultiClusterStatus defines the observed state of MultiCluster
type MultiClusterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	LeaderAddress string `json:"leaderAddress"`
	Ready         bool   `json:"ready"`
	Leading       bool   `json:"leading"`
	Port          int    `json:"port"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// MultiCluster is the Schema for the multiclusters API
type MultiCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MultiClusterSpec   `json:"spec,omitempty"`
	Status MultiClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MultiClusterList contains a list of MultiCluster
type MultiClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MultiCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MultiCluster{}, &MultiClusterList{})
}
