/*

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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MeshFedConfigSpec defines the desired state of MeshFedConfig
type MeshFedConfigSpec struct {
	// If specified, selects the group (secret) to apply this configuration to
	Mode                   string            `json:"mode,omitempty"`
	TlsContextSelector     map[string]string `json:"tls_context_selector,omitempty"`
	UseEgressGateway       bool              `json:"use_egress_gateway,omitempty"`
	EgressGatewaySelector  map[string]string `json:"egress_gateway_selector,omitempty"`
	EgressGatewayPort      uint32            `json:"egress_gateway_port,omitempty"`
	UseIngressGateway      bool              `json:"use_ingress_gateway,omitempty"`
	IngressGatewaySelector map[string]string `json:"ingress_gateway_selector,omitempty"`
	IngressGatewayPort     uint32            `json:"ingress_gateway_port,omitempty"`
}

// MeshFedConfigStatus defines the observed state of MeshFedConfig
type MeshFedConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true

// MeshFedConfig is the Schema for the MeshFedConfigs API
type MeshFedConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MeshFedConfigSpec   `json:"spec,omitempty"`
	Status MeshFedConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MeshFedConfigList contains a list of MeshFedConfig
type MeshFedConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MeshFedConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MeshFedConfig{}, &MeshFedConfigList{})
}
