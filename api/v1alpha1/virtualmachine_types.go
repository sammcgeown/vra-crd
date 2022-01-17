/*
Copyright 2022.

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

	"github.com/vmware/vra-sdk-go/pkg/models"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// StatusPhase is a string representation of the status phase
type StatusPhase string

// PowerState is a string representation of the power state
type PowerState string

// StatusPhase constants
const (
	RunningStatusPhase    StatusPhase = "RUNNING"
	CreatingStatusPhase   StatusPhase = "CREATING"
	PendingStatusPhase    StatusPhase = "PENDING"
	ErrorStatusPhase      StatusPhase = "ERROR"
	InProgressStatusPhase StatusPhase = "INPROGRESS"

	OnPowerState       PowerState = "ON"
	OffPowerState      PowerState = "OFF"
	GuestOffPowerState PowerState = "GUEST_OFF"
	UnknownPowerState  PowerState = "UNKNOWN"
	SuspendPowerState  PowerState = "SUSPEND"
)

// VirtualMachineSpec defines the desired state of VirtualMachine
type VirtualMachineSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Required: true
	// Links map[string]struct {

	// href
	Href string `json:"href,omitempty"`

	// 	// hrefs
	// 	Hrefs []string `json:"hrefs"`
	// } `json:"_links"`

	// Primary address allocated or in use by this machine. The actual type of the address depends on the adapter type. Typically it is either the public or the external IP address.
	// Example: 34.242.21.5
	// +optional
	Address string `json:"address,omitempty"`

	// The cloud config data in json-escaped yaml syntax
	// +optional
	BootConfig *models.MachineBootConfig `json:"bootConfig,omitempty"`

	// Set of ids of the cloud accounts this resource belongs to.
	// Example: [9e49]
	// Unique: true
	// +optional
	CloudAccountIds []string `json:"cloudAccountIds"`

	// Date when the entity was created. The date is in ISO 8601 and UTC.
	// Example: 2012-09-27
	// +optional
	CreatedAt string `json:"createdAt,omitempty"`

	// Additional properties that may be used to extend the base resource.
	// Example: { \"property\" : \"value\" }
	// +optional
	CustomProperties map[string]string `json:"customProperties,omitempty"`

	// Deployment id that is associated with this resource.
	// Example: 123e4567-e89b-12d3-a456-426655440000
	// +optional
	DeploymentID string `json:"deploymentId,omitempty"`

	// A human-friendly description.
	// Example: my-description
	// +optional
	Description string `json:"description,omitempty"`

	// External entity Id on the provider side.
	// Example: i-cfe4-e241-e53b-756a9a2e25d2
	// +optional
	ExternalID string `json:"externalId,omitempty"`

	// The external regionId of the resource.
	// Example: us-east-1
	// Required: false
	// +optional
	ExternalRegionID *string `json:"externalRegionId"`

	// The external zoneId of the resource.
	// Example: us-east-1a
	// Required: false
	// +optional
	ExternalZoneID *string `json:"externalZoneId"`

	// Hostname associated with this machine instance.
	Hostname string `json:"hostname,omitempty"`

	// The id of this resource instance
	// Example: 9e49
	// Required: false
	// +optional
	ID *string `json:"id"`

	// The id of the organization this entity belongs to.
	// Example: 9e49
	OrgID string `json:"orgId,omitempty"`

	// This field is deprecated. Use orgId instead. The id of the organization this entity belongs to.
	// Example: deprecated
	OrganizationID string `json:"organizationId,omitempty"`

	// Email of the user that owns the entity.
	// Example: csp@vmware.com
	Owner string `json:"owner,omitempty"`

	// Power state of machine.
	// Example: ON, OFF
	// Required: false
	// Enum: [ON OFF GUEST_OFF UNKNOWN SUSPEND]
	// +optional
	PowerState *string `json:"powerState"`

	// The id of the project this resource belongs to.
	// Example: 9e49
	// Required: true
	ProjectID string `json:"projectId,omitempty"`

	// // Settings to apply salt configuration on the provisioned machine.
	// SaltConfiguration *models.SaltConfiguration `json:"saltConfiguration,omitempty"`

	// Date when the entity was last updated. The date is ISO 8601 and UTC.
	// Example: 2012-09-27
	UpdatedAt string `json:"updatedAt,omitempty"`

	// Flavor
	// Required: true
	Flavor string `json:"flavor,omitempty"`

	// Image
	// Required: true
	Image string `json:"image,omitempty"`

	// Constraint tags
	// Required: false
	Constraints []Constraint `json:"constraints,omitempty"`

	// Label tags
	// +optional
	Tags []Tag `json:"tags"`
}

// Constraint are the constraint tags for a virtual machine
type Constraint struct {
	Mandatory  bool   `json:"mandatory"`
	Expression string `json:"expression"`
}

// Tag are the label tags for a virtual machine
type Tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// VirtualMachineStatus defines the observed state of VirtualMachine
type VirtualMachineStatus struct {
	Phase             StatusPhase `json:"phase"`
	LastMessage       string      `json:"lastMessage"`
	ExternalRequestID string      `json:"externalRequestID"`
	ExternalID        string      `json:"externalID"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="External_ID",type=string,JSONPath=`.status.externalID`
// +kubebuilder:printcolumn:name="External_Request_ID",type=string,JSONPath=`.status.externalRequestID`
// +kubebuilder:printcolumn:name="Last_Message",type=string,JSONPath=`.status.lastMessage`

// VirtualMachine is the Schema for the virtualmachines API
type VirtualMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualMachineSpec   `json:"spec,omitempty"`
	Status VirtualMachineStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VirtualMachineList contains a list of VirtualMachine
type VirtualMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VirtualMachine{}, &VirtualMachineList{})
}
