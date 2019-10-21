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

// ServiceExpositionSpec defines the desired state of ServiceExposition
type ServiceExpositionSpec struct {
	// REQUIRED: The name of the service to be exposed.
	Name string `json:"name,omitempty"`
	// REQUIRED: The group in which the service being exposed. Can be more than one group (?)
	MeshFedConfigSelector map[string]string `json:"mesh_fed_config_selector,omitempty"`
	// OPTIONAL: This is an optional field. If not specified, the service name will be
	// used as the exposed service name.
	Alias string `json:"alias,omitempty"`
	// OPTIONAL: `subset` allows the operator to choose a specific subset (service
	// version) in cases when there are multiple subsets available for the
	// exposed service. Applicable only to services within the mesh. The subset
	//  must be defined in a corresponding DestinationRule.
	// For binding services, it represents the service as a subset if specified.
	Subset string `json:"subset,omitempty"`
	// REQUIRED: The port of the exposed service.
	// TODO: consider adding support for multiple ports, their types and names.
	Port uint32 `json:"port,omitempty"`
	// To be filled in by mesh controller
	Endpoints            []string `json:"endpoints,omitempty"`
	Clusters             []string `json:"clusters,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

// ServiceExpositionStatus defines the observed state of ServiceExposition
type ServiceExpositionStatus struct {
	Ready                bool     `json:"ready,omitempty"`
	XXX_NoUnkeyedLiteral struct{} `json:"-"`
	XXX_unrecognized     []byte   `json:"-"`
	XXX_sizecache        int32    `json:"-"`
}

// +kubebuilder:object:root=true

// ServiceExposition is the Schema for the serviceexpositions API
type ServiceExposition struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ServiceExpositionSpec   `json:"spec,omitempty"`
	Status ServiceExpositionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ServiceExpositionList contains a list of ServiceExposition
type ServiceExpositionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ServiceExposition `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ServiceExposition{}, &ServiceExpositionList{})
}
