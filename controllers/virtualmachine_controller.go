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

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	machinev1alpha1 "github.com/sammcgeown/vra/api/v1alpha1"
	vraclient "github.com/vmware/vra-sdk-go/pkg/client"
	"github.com/vmware/vra-sdk-go/pkg/client/compute"
	"github.com/vmware/vra-sdk-go/pkg/client/request"
	"github.com/vmware/vra-sdk-go/pkg/models"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	virtualMachineFinalizer = "virtualmachine.machine.cmbu.local/finalizer"
	defaultRequeue          = 20 * time.Second
)

// VirtualMachineReconciler reconciles a VirtualMachine object
type VirtualMachineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	VRA    *vraclient.MulticloudIaaS
	Log    logr.Logger
}

//+kubebuilder:rbac:groups=machine.cmbu.local,resources=virtualmachines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=machine.cmbu.local,resources=virtualmachines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=machine.cmbu.local,resources=virtualmachines/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the VirtualMachine object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *VirtualMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	//_ = log.FromContext(ctx)
	log := r.Log.WithValues("virtualmachine", req.NamespacedName)

	var virtualMachine machinev1alpha1.VirtualMachine
	if err := r.Get(ctx, req.NamespacedName, &virtualMachine); err != nil {
		log.Error(err, "unable to fetch VirtualMachine")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	msg := fmt.Sprintf("received reconcile request for %q (namespace: %q)", virtualMachine.GetName(), virtualMachine.GetNamespace())
	log.Info(msg)

	// Check if there is a RequestID for the VirtualMachine
	if virtualMachine.Status.ExternalRequestID != "" {
		// There is a request ID, check the status of the request
		requestTracker, err := r.VRA.Request.GetRequestTracker(request.NewGetRequestTrackerParams().WithID(virtualMachine.Status.ExternalRequestID))
		if err != nil {
			//return "", models.RequestTrackerStatusFAILED, err
			virtualMachine.Status = createStatus(
				machinev1alpha1.ErrorStatusPhase,
				"request tracker failed",
				err,
				virtualMachine.Status.ExternalRequestID,
				"",
			)
			return ctrl.Result{}, errors.Wrap(r.Client.Status().Update(ctx, &virtualMachine), "could not update status")
		}
		status := requestTracker.Payload.Status
		log.Info("virtual machine request status: " + *status)

		switch *status {
		case models.RequestTrackerStatusFAILED:
			virtualMachine.Status = createStatus(
				machinev1alpha1.ErrorStatusPhase,
				"request failed",
				fmt.Errorf(requestTracker.Payload.Message),
				virtualMachine.Status.ExternalRequestID,
				"",
			)
		case models.RequestTrackerStatusINPROGRESS:
			virtualMachine.Status = createStatus(
				machinev1alpha1.InProgressStatusPhase,
				"request in progress",
				nil,
				virtualMachine.Status.ExternalRequestID,
				"",
			)
		case models.RequestTrackerStatusFINISHED:
			// Remove the ExternalRequestID from the VirtualMachineStatus
			virtualMachine.Status = createStatus(
				machinev1alpha1.RunningStatusPhase,
				"request completed",
				nil,
				"",
				"",
			)
		default:
			virtualMachine.Status = createStatus(
				machinev1alpha1.ErrorStatusPhase,
				requestTracker.Payload.Message,
				fmt.Errorf("machineStateRefreshFunc: unknown status %v", *status),
				virtualMachine.Status.ExternalRequestID,
				"",
			)
		}
		return ctrl.Result{RequeueAfter: defaultRequeue}, errors.Wrap(r.Client.Status().Update(ctx, &virtualMachine), "could not update status")
	}

	// Delete if it's marked for deletion
	if !virtualMachine.ObjectMeta.DeletionTimestamp.IsZero() {
		log.Info("Virtual Machine marked for deletion")
		// The object is being deleted
		if containsString(virtualMachine.ObjectMeta.Finalizers, virtualMachineFinalizer) {
			// // our finalizer is present, so lets handle any external dependency
			if err := r.deleteExternalResources(ctx, &virtualMachine); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried
				return ctrl.Result{}, err
			}
			// If the resource does not have a running request...
			if virtualMachine.Status.ExternalRequestID == "" {
				// remove our finalizer from the list and update it.
				virtualMachine.ObjectMeta.Finalizers = removeString(virtualMachine.ObjectMeta.Finalizers, virtualMachineFinalizer)
				if err := r.Update(ctx, &virtualMachine); err != nil {
					return ctrl.Result{}, errors.Wrap(err, "could not remove finalizer")
				}
			}
		}
		// finalizer already removed, nothing to do
		return ctrl.Result{}, nil
	}

	// register our finalizer if it does not exist
	if !containsString(virtualMachine.ObjectMeta.Finalizers, virtualMachineFinalizer) {
		virtualMachine.ObjectMeta.Finalizers = append(virtualMachine.ObjectMeta.Finalizers, virtualMachineFinalizer)
		if err := r.Update(ctx, &virtualMachine); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "could not add finalizer")
		}
	}

	// Check if the VirtualMachine exists
	exists := true
	var filter = "tags.item.key eq 'k8s_name' and tags.item.value eq '" + virtualMachine.GetName() + "'"
	log.Info("filter: " + filter)
	var machine *models.Machine
	machines, err := r.VRA.Compute.GetMachines(compute.NewGetMachinesParams().WithDollarFilter(&filter))
	if err != nil {
		virtualMachine.Status = createStatus(machinev1alpha1.ErrorStatusPhase, "unable to get VirtualMachine from vRealize Automation", err, "", "")
		return ctrl.Result{}, errors.Wrap(r.Client.Status().Update(ctx, &virtualMachine), "could not update status")
	}
	if machines.Payload.TotalElements == 0 {
		log.Info("VirtualMachine does not exist in vRealize Automation")
		exists = false
	} else if machines.Payload.TotalElements > 1 {
		return ctrl.Result{}, fmt.Errorf("found more than one VirtualMachine with tag k8s_name:%q", virtualMachine.GetName())
	} else {
		// There should be 1 and only 1 VirtualMachine with the tag k8s_name:<name>
		log.Info("found VirtualMachine with ID: " + *machines.Payload.Content[0].ID)
		machine = machines.Payload.Content[0]
	}

	// Create the VirtualMachine, if it doesn't exist
	if !exists {
		if virtualMachine.Status.ExternalRequestID == "" {
			log.Info("creating virtual machine request")
			requestID, err := r.createMachine(virtualMachine)
			//log.Info(*requestID)
			if err != nil {
				virtualMachine.Status = createStatus(machinev1alpha1.ErrorStatusPhase, "unable to create VirtualMachine in vRealize Automation", err, "", "")
			} else {
				// Update VirtualMachineStatus with the request ID
				virtualMachine.Status = createStatus(machinev1alpha1.CreatingStatusPhase, "created VirtualMachine in vRealize Automation", nil, *requestID, "")
			}
			return ctrl.Result{}, errors.Wrap(r.Client.Status().Update(ctx, &virtualMachine), "could not update status")
		}
	}

	// Check the state matches the desired state

	// Update the VirtualMachine
	virtualMachine.Spec.Address = machine.Address
	virtualMachine.Spec.BootConfig = machine.BootConfig
	virtualMachine.Spec.CloudAccountIds = machine.CloudAccountIds
	virtualMachine.Spec.CreatedAt = machine.CreatedAt
	virtualMachine.Spec.CustomProperties = machine.CustomProperties
	virtualMachine.Spec.DeploymentID = machine.DeploymentID
	virtualMachine.Spec.Description = machine.Description
	virtualMachine.Spec.ExternalID = machine.ExternalID
	virtualMachine.Spec.ExternalRegionID = machine.ExternalRegionID
	virtualMachine.Spec.ExternalZoneID = machine.ExternalZoneID
	virtualMachine.Spec.Hostname = machine.Hostname
	virtualMachine.Spec.ID = machine.ID
	virtualMachine.Spec.OrgID = machine.OrgID
	virtualMachine.Spec.Owner = machine.Owner
	virtualMachine.Spec.PowerState = machine.PowerState
	virtualMachine.Spec.ProjectID = machine.ProjectID
	virtualMachine.Spec.UpdatedAt = machine.UpdatedAt

	updateVMError := r.Client.Update(ctx, &virtualMachine)
	if updateVMError != nil {
		log.Error(updateVMError, "unable to update VirtualMachine state")
	}

	// Create the Status
	virtualMachine.Status = createStatus(machinev1alpha1.RunningStatusPhase, "ready", nil, "", *machine.ID)

	return ctrl.Result{}, errors.Wrap(r.Client.Status().Update(ctx, &virtualMachine), "could not update status")

}

// SetupWithManager sets up the controller with the Manager.
func (r *VirtualMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&machinev1alpha1.VirtualMachine{}).
		Complete(r)
}

func (r *VirtualMachineReconciler) deleteExternalResources(ctx context.Context, virtualMachine *machinev1alpha1.VirtualMachine) error {
	// Ensure that delete implementation is idempotent and safe to invoke
	// multiple times for same object.
	log := r.Log.WithValues("virtualmachine", virtualMachine.Namespace)

	// If there is an ExternalRequestID we neeed to wait for the request to complete
	if virtualMachine.Status.ExternalRequestID != "" {
		log.Info("waiting for request to complete")
		return nil
	}

	if virtualMachine.Status.ExternalID != "" {
		deleteRequest, deleteError := r.VRA.Compute.DeleteMachine(compute.NewDeleteMachineParams().WithID(*virtualMachine.Spec.ID))
		if deleteError != nil {
			return deleteError
		}
		// Add the external request ID to the VirtualMachineStatus
		virtualMachine.Status = createStatus(machinev1alpha1.PendingStatusPhase, "deleting Virtual Machine", nil, *deleteRequest.Payload.ID, virtualMachine.Status.ExternalID)

	}
	// Update the VirtualMachineStatus and return nil, or error if update fails
	return errors.Wrap(r.Client.Status().Update(ctx, virtualMachine), "could not update status")
}

func createStatus(phase machinev1alpha1.StatusPhase, msg string, err error, requestID string, machineID string) machinev1alpha1.VirtualMachineStatus {
	if err != nil {
		msg = msg + ": " + err.Error()
	}

	status := machinev1alpha1.VirtualMachineStatus{
		Phase:             phase,
		LastMessage:       msg,
		ExternalRequestID: requestID,
		ExternalID:        machineID,
	}
	return status
}

func (r *VirtualMachineReconciler) createMachine(virtualMachine machinev1alpha1.VirtualMachine) (*string, error) {
	name := virtualMachine.GetName()
	namespace := virtualMachine.GetNamespace()
	constraints := expandConstraints(virtualMachine.Spec.Constraints)
	k8sName := "k8s_name"
	k8sNamespace := "k8s_namespace"
	tags := expandTags(virtualMachine.Spec.Tags)
	tags = append(tags, &models.Tag{
		Key:   &k8sName,
		Value: &name,
	})
	tags = append(tags, &models.Tag{
		Key:   &k8sNamespace,
		Value: &namespace,
	})

	machineSpecification := models.MachineSpecification{
		Name:        &name,
		Flavor:      &virtualMachine.Spec.Flavor,
		ProjectID:   &virtualMachine.Spec.ProjectID,
		Constraints: constraints,
		Tags:        tags,
		Image:       &virtualMachine.Spec.Image,
	}
	createMachineCreated, err := r.VRA.Compute.CreateMachine(compute.NewCreateMachineParams().WithBody(&machineSpecification))
	if err != nil {
		return nil, err
	}
	return createMachineCreated.Payload.ID, nil
}

func expandConstraints(configConstraints []machinev1alpha1.Constraint) []*models.Constraint {
	constraints := make([]*models.Constraint, 0, len(configConstraints))
	for _, configConstraint := range configConstraints {
		constraint := models.Constraint{
			Mandatory:  &configConstraint.Mandatory,
			Expression: &configConstraint.Expression,
		}

		constraints = append(constraints, &constraint)
	}
	return constraints
}

func expandTags(configTags []machinev1alpha1.Tag) []*models.Tag {
	//tags := make([]*models.Tag, 0, len(configTags))

	var tags []*models.Tag

	for _, configTag := range configTags {
		tag := models.Tag{
			Key:   &configTag.Key,
			Value: &configTag.Value,
		}

		tags = append(tags, &tag)
	}

	return tags
}

// Helper functions to check and remove string from a slice of strings.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}
