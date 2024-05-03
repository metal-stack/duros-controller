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

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	durosv2 "github.com/metal-stack/duros-go/api/duros/v2"

	duroscontrollerv1 "github.com/metal-stack/duros-controller/api/v1"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	DurosFinalizerName = "storage.metal-stack.io/finalizer"
)

// DurosReconciler reconciles a Duros object
type DurosReconciler struct {
	client.Client
	Shoot           client.Client
	DiscoveryClient *discovery.DiscoveryClient
	Log             logr.Logger
	Namespace       string
	DurosClient     durosv2.DurosAPIClient
	Endpoints       string
	AdminKey        []byte
	PSPDisabled     bool
}

// Reconcile the Duros CRD
// +kubebuilder:rbac:groups=storage.metal-stack.io,resources=duros,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=storage.metal-stack.io,resources=duros/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=storage.k8s.io,resources=csidrivers;csinodes;volumeattachments;storageclasses,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=podsecuritypolicies,verbs=get;list;watch;create;update;patch;delete;use
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:apps:groups=policy,resources=statefulsets;daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:apps:groups="",resources=configmaps;events;secrets;serviceaccounts;nodes;persistentvolumes;persistentvolumeclaims;persistentvolumeclaims/status;pods,verbs=get;list;watch;create;update;patch;delete
func (r *DurosReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("duros", req.NamespacedName)
	requeue := ctrl.Result{
		RequeueAfter: time.Second * 10,
	}

	log.Info("running in", "namespace", req.Namespace, "configured for", r.Namespace)
	if req.Namespace != r.Namespace {
		return ctrl.Result{}, nil
	}

	// first get the metal-api projectID
	duros := &duroscontrollerv1.Duros{}
	if err := r.Get(ctx, req.NamespacedName, duros); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("no duros storage defined")
			return ctrl.Result{}, nil
		}
		return requeue, err
	}

	if duros.GetDeletionTimestamp() != nil && !duros.GetDeletionTimestamp().IsZero() {
		log.Info("deletion timestamp is set, not doing anything, Gardener will do the cleanup")
		return ctrl.Result{}, nil
	}

	var err error

	defer func() {
		now := metav1.NewTime(time.Now())

		duros.Status.ReconcileStatus = duroscontrollerv1.ReconcileStatus{
			LastReconcile: &now,
			Error:         nil,
		}

		if err != nil {
			msg := err.Error()
			duros.Status.ReconcileStatus.Error = &msg
		}

		r.setManagedResourceStatus(ctx, duros)

		if err := r.Status().Update(ctx, duros); err != nil {
			log.Error(err, "error updating status of duros resource", "name", duros.Name)
			return
		}

		r.Log.Info("status updated", "name", duros.Name)
	}()

	err = validateDuros(duros)
	if err != nil {
		return requeue, err
	}

	projectID := duros.Spec.MetalProjectID
	storageClasses := duros.Spec.StorageClasses

	p, err := r.createProjectIfNotExist(ctx, projectID)
	if err != nil {
		return requeue, err
	}
	log.Info("created project", "name", p.GetName())

	cred, err := r.createProjectCredentialsIfNotExist(ctx, projectID, r.AdminKey)
	if err != nil {
		return requeue, err
	}
	log.Info("created credential", "id", cred.GetID(), "project", cred.GetProjectName())

	err = r.reconcileStorageClassSecret(ctx, cred, r.AdminKey)
	if err != nil {
		return requeue, err
	}

	err = r.deployCSI(ctx, projectID, storageClasses)
	if err != nil {
		return requeue, err
	}

	return ctrl.Result{
		// we requeue in a small interval to ensure resources are recreated quickly
		// and status is updated regularly
		RequeueAfter: 30 * time.Second,
	}, nil
}

func (r *DurosReconciler) setManagedResourceStatus(ctx context.Context, duros *duroscontrollerv1.Duros) {
	var (
		updateTime = metav1.NewTime(time.Now())
		ds         = &appsv1.DaemonSet{}
		sts        = &appsv1.StatefulSet{}

		dsState = duroscontrollerv1.HealthStateRunning
		dsMsg   = "All replicas are ready"

		stsState = duroscontrollerv1.HealthStateRunning
		stsMsg   = "All replicas are ready"
	)

	err := r.Shoot.Get(ctx, types.NamespacedName{Name: lbCSINodeName, Namespace: namespace}, ds)
	if err != nil {
		r.Log.Error(err, "error getting daemon set")
		dsState = duroscontrollerv1.HealthStateNotRunning
		dsMsg = err.Error()
	}

	dsStatus := duroscontrollerv1.ManagedResourceStatus{
		Name:           ds.Name,
		Group:          "DaemonSet", // ds.GetObjectKind().GroupVersionKind().String() --> this does not work :(
		State:          dsState,
		Description:    dsMsg,
		LastUpdateTime: updateTime,
	}

	if ds.Status.DesiredNumberScheduled != ds.Status.NumberReady {
		dsStatus.State = duroscontrollerv1.HealthStateNotRunning
		dsStatus.Description = fmt.Sprintf("%d/%d replicas are ready", ds.Status.NumberReady, ds.Status.DesiredNumberScheduled)
	}

	err = r.Shoot.Get(ctx, types.NamespacedName{Name: lbCSIControllerName, Namespace: namespace}, sts)
	if err != nil {
		r.Log.Error(err, "error getting stateful set")
		stsState = duroscontrollerv1.HealthStateNotRunning
		stsMsg = err.Error()
	}

	stsStatus := duroscontrollerv1.ManagedResourceStatus{
		Name:           sts.Name,
		Group:          "StatefulSet", // sts.GetObjectKind().GroupVersionKind().String() --> this does not work :(
		State:          stsState,
		Description:    stsMsg,
		LastUpdateTime: updateTime,
	}

	replicas := int32(1)
	if sts.Spec.Replicas != nil {
		replicas = *sts.Spec.Replicas
	}

	if replicas != sts.Status.ReadyReplicas {
		stsStatus.State = duroscontrollerv1.HealthStateNotRunning
		stsStatus.Description = fmt.Sprintf("%d/%d replicas are ready", sts.Status.ReadyReplicas, replicas)
	}

	duros.Status.ManagedResourceStatuses = []duroscontrollerv1.ManagedResourceStatus{dsStatus, stsStatus}
}

// SetupWithManager boilerplate to setup the Reconciler
func (r *DurosReconciler) SetupWithManager(mgr ctrl.Manager) error {
	pred := predicate.GenerationChangedPredicate{} // prevents reconcile on status sub resource update
	return ctrl.NewControllerManagedBy(mgr).
		For(&duroscontrollerv1.Duros{}).
		WithEventFilter(pred).
		Complete(r)
}

func validateDuros(duros *duroscontrollerv1.Duros) error {
	if len(duros.Spec.MetalProjectID) == 0 {
		return fmt.Errorf("metalProjectID is empty")
	}
	if len(duros.Spec.StorageClasses) == 0 {
		return fmt.Errorf("at least one storageclass must be defined")
	}
	for _, sc := range duros.Spec.StorageClasses {
		if len(sc.Name) == 0 {
			return fmt.Errorf("storageclass.name is empty")
		}
		if sc.ReplicaCount < 1 {
			return fmt.Errorf("storageclass.replicacount must be greater than 0")
		}
	}
	return nil
}
