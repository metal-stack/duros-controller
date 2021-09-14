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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/metal-stack/duros-go"
	durosv2 "github.com/metal-stack/duros-go/api/duros/v2"

	storagev1 "github.com/metal-stack/duros-controller/api/v1"
	v1 "github.com/metal-stack/duros-controller/api/v1"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DurosReconciler reconciles a Duros object
type DurosReconciler struct {
	client.Client
	Shoot       client.Client
	Log         logr.Logger
	Scheme      *runtime.Scheme
	Namespace   string
	DurosClient durosv2.DurosAPIClient
	Endpoints   duros.EPs
	AdminKey    []byte
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
	var duros storagev1.Duros
	if err := r.Get(ctx, req.NamespacedName, &duros); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("no duros storage defined")
			return requeue, err
		}
	}
	err := validateDuros(duros)
	if err != nil {
		return requeue, err
	}
	projectID := duros.Spec.MetalProjectID
	storageClasses := duros.Spec.StorageClasses

	p, err := r.createProjectIfNotExist(ctx, projectID)
	if err != nil {
		return requeue, err
	}
	log.Info("created project", "name", p.Name)

	cred, err := r.createProjectCredentialsIfNotExist(ctx, projectID, r.AdminKey)
	if err != nil {
		return requeue, err
	}
	log.Info("created credential", "id", cred.ID, "project", cred.ProjectName)

	// Deploy StorageClass Secret
	err = r.deployStorageClassSecret(ctx, cred, r.AdminKey)
	if err != nil {
		return requeue, err
	}
	// Deploy StorageClass
	err = r.deployStorageClass(ctx, projectID, storageClasses)
	if err != nil {
		return requeue, err
	}

	err = r.reconcileStatus(ctx, duros)
	if err != nil {
		return requeue, err
	}

	return ctrl.Result{}, nil
}

func (r *DurosReconciler) reconcileStatus(ctx context.Context, duros storagev1.Duros) error {
	var (
		updateTime = metav1.NewTime(time.Now())
		ds         = &appsv1.DaemonSet{}
		sts        = &appsv1.StatefulSet{}
	)

	duros.Status.SecretRef = "" // TODO?

	err := r.Shoot.Get(ctx, types.NamespacedName{Name: "lb-csi-node", Namespace: namespace}, ds)
	if err != nil {
		return fmt.Errorf("error getting daemon set: %w", err)
	}

	dsStatus := v1.ManagedResourceStatus{
		Name:           ds.Name,
		Group:          ds.GetObjectKind().GroupVersionKind().String(),
		State:          v1.HealthStateRunning,
		Description:    "All replicas are ready",
		LastUpdateTime: updateTime,
	}

	if ds.Status.DesiredNumberScheduled != ds.Status.NumberReady {
		dsStatus.State = v1.HealthStateNotRunning
		dsStatus.Description = fmt.Sprintf("%d/%d replicas are ready", ds.Status.NumberReady, ds.Status.DesiredNumberScheduled)
	}

	err = r.Shoot.Get(ctx, types.NamespacedName{Name: "lb-csi-controller", Namespace: namespace}, sts)
	if err != nil {
		return fmt.Errorf("error getting statefulset: %w", err)
	}

	stsStatus := v1.ManagedResourceStatus{
		Name:           sts.Name,
		Group:          sts.GetObjectKind().GroupVersionKind().String(),
		State:          v1.HealthStateRunning,
		Description:    "All replicas are ready",
		LastUpdateTime: updateTime,
	}

	replicas := int32(1)
	if sts.Spec.Replicas != nil {
		replicas = *sts.Spec.Replicas
	}

	if replicas != sts.Status.ReadyReplicas {
		stsStatus.State = v1.HealthStateNotRunning
		stsStatus.Description = fmt.Sprintf("%d/%d replicas are ready", sts.Status.ReadyReplicas, replicas)
	}

	duros.Status.ManagedResourceStatuses = []v1.ManagedResourceStatus{dsStatus, stsStatus}
	err = r.Status().Update(ctx, &duros)
	if err != nil {
		return fmt.Errorf("error updating status: %w", err)
	}

	return nil
}

// SetupWithManager boilerplate to setup the Reconciler
func (r *DurosReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&storagev1.Duros{}).
		Complete(r)
}

func validateDuros(duros v1.Duros) error {
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
