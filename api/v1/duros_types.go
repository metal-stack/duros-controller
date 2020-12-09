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

// Duros is the Schema for the Duros API
// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="ProjectID",type=string,JSONPath=`.spec.metalProjectID`
// +kubebuilder:printcolumn:name="Replicas",type=string,JSONPath=`.spec.replicas`
type Duros struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DurosSpec   `json:"spec,omitempty"`
	Status DurosStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DurosList contains a list of Duros
type DurosList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Duros `json:"items"`
}

// DurosSpec defines the desired state of Duros
type DurosSpec struct {
	// MetalProjectID is the projectID of this deployment
	MetalProjectID string `json:"metalProjectID,omitempty"`
	// Replicas defines for which replicas a storageclass should be deployed
	Replicas []Replica `json:"replicas,omitempty"`
}

// DurosStatus defines the observed state of Duros
type DurosStatus struct {
	// SecretRef to the create JWT Token
	// TODO, this can be used to detect required key rotation
	SecretRef string `json:"secret,omitempty" description:"Reference to JWT Token generated on the duros storage side for this project"`
}

// Replica defines the storageClass parameters
type Replica struct {
	Name        string `json:"name"`
	Count       int8   `json:"count"`
	Compression bool   `json:"compression"`
}

func init() {
	SchemeBuilder.Register(&Duros{}, &DurosList{})
}
