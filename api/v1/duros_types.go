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

// DurosSpec defines the desired state of Duros
type DurosSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	MetalProjectID string `json:"metal_project_id,omitempty"`
	// Replicas defines for which replicas a storageclass should be deployed
	Replicas []string `json:"replicas,omitempty"`
	// AdminKeySecretRef points to the secret where the duros admin key is stored
	AdminKeySecretRef string `json:"adminKeySecretRef,omitempty"`
}

// DurosStatus defines the observed state of Duros
type DurosStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	SecretRef string `json:"secret,omitempty" description:"Reference to JWT Token generated on the duros storage side for this project"`
}

// +kubebuilder:object:root=true

// Duros is the Schema for the Duros API
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

func init() {
	SchemeBuilder.Register(&Duros{}, &DurosList{})
}
