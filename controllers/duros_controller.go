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
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/metal-stack/duros-go"
	durosv2 "github.com/metal-stack/duros-go/api/duros/v2"

	storagev1 "github.com/metal-stack/duros-controller/api/v1"
)

const (
	requeueInterval = time.Second * 10
)

// DurosReconciler reconciles a Duros object
type DurosReconciler struct {
	client.Client
	Log         logr.Logger
	Scheme      *runtime.Scheme
	Namespace   string
	DurosClient durosv2.DurosAPIClient
	Endpoints   duros.EPs
}

// +kubebuilder:rbac:groups=storage.metal-stack.io,resources=duros,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=storage.metal-stack.io,resources=duros/status,verbs=get;update;patch

// Reconcile the Duros CRD
func (r *DurosReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	log := r.Log.WithValues("duros", req.NamespacedName)
	requeue := ctrl.Result{
		RequeueAfter: requeueInterval,
	}

	log.Info("running in", "namespace", req.Namespace, "configured for", r.Namespace)
	if req.Namespace != r.Namespace {
		return ctrl.Result{}, nil
	}
	// first get the metal-api projectID
	var lbs storagev1.Duros
	if err := r.Get(ctx, req.NamespacedName, &lbs); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("no durosstorage defined")
			return requeue, err
		}
	}

	var adminKeySecret v1.Secret
	if err := r.Get(ctx, types.NamespacedName{Name: lbs.Spec.AdminKeySecretRef, Namespace: r.Namespace}, &adminKeySecret); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("no adminkey secret found")
			return requeue, err
		}
	}
	adminKey, ok := adminKeySecret.Data["adminKey"]
	if !ok {
		return requeue, fmt.Errorf("no adminKey found in adminKeySecret")
	}

	projectID := lbs.Spec.MetalProjectID
	replicas := lbs.Spec.Replicas

	err := r.createProjectIfNotExist(ctx, projectID)
	if err != nil {
		return requeue, err
	}

	cred, err := r.createProjectCredentialsIfNotExist(ctx, projectID, adminKey)
	if err != nil {
		return requeue, err
	}
	// Deploy StorageClass Secret
	err = r.deployStorageClassSecret(ctx, cred, adminKey)
	if err != nil {
		return requeue, err
	}
	// Deploy StorageClass
	err = r.deployStorageClass(ctx, projectID, replicas)
	if err != nil {
		return requeue, err
	}
	// Deploy required ClusterwideNetworkPolicy
	err = r.deployClusterWideNetworkPolicy(ctx, r.Endpoints)
	if err != nil {
		return requeue, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager boilerplate to setup the Reconciler
func (r *DurosReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&storagev1.Duros{}).
		Complete(r)
}
