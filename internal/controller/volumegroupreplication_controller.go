package controller

import (
	"context"
	"fmt"
	"time"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	volrep "github.com/csi-addons/kubernetes-csi-addons/api/replication.storage/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/ramendr/mock-storage-operator/internal/volsync"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	requeueInterval     = 30 * time.Second
	vgrFinalizer        = "mock.storage.io/volumegroupreplication"
	mockProvisionerName = "mock.storage.io"
	remoteAddressKey    = "mock.storage.io/remote-address"
	remoteKeySecretKey  = "mock.storage.io/remote-key-secret"
)

// VolumeGroupReplicationReconciler reconciles VolumeGroupReplication objects
type VolumeGroupReplicationReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=replication.storage.openshift.io,resources=volumegroupreplications,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=replication.storage.openshift.io,resources=volumegroupreplications/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=replication.storage.openshift.io,resources=volumegroupreplications/finalizers,verbs=update
// +kubebuilder:rbac:groups=replication.storage.openshift.io,resources=volumegroupreplicationclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=volsync.backube,resources=replicationsources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=volsync.backube,resources=replicationdestinations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *VolumeGroupReplicationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	logger.V(1).Info("Reconciling VolumeGroupReplication", "volumeGroupReplication", req.NamespacedName) // controller/volumegroupreplication_controller.go"
	vgr := &volrep.VolumeGroupReplication{}
	if err := r.Get(ctx, req.NamespacedName, vgr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if this VGR is for our provisioner
	vgrClass := &volrep.VolumeGroupReplicationClass{}
	if err := r.Get(ctx, types.NamespacedName{Name: vgr.Spec.VolumeGroupReplicationClassName}, vgrClass); err != nil {
		logger.Error(err, "Failed to get VolumeGroupReplicationClass")
		return ctrl.Result{}, err
	}

	if vgrClass.GetLabels()["ramendr.openshift.io/global"] != "true" {
		logger.V(1).Info("VGR not for this provisioner, skipping", "provisioner", vgrClass.Spec.Provisioner)
		return ctrl.Result{}, nil
	}

	// Handle deletion
	if !vgr.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, logger, vgr)
	}

	// Ensure finalizer is present
	if !controllerutil.ContainsFinalizer(vgr, vgrFinalizer) {
		controllerutil.AddFinalizer(vgr, vgrFinalizer)
		if err := r.Update(ctx, vgr); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Populate the vgrClass.Spec.Parameters with PVC names. This is supposed to change later.
	// The key/values will be in the format of VGRClass parameters (pvc-<name>: "true")
	vgrClass.Spec.Parameters = map[string]string{
		"pvc-mock-pvc-test": "true",
		// "pvc-2": "true",
		"schedulingInterval":      "3m",
		"storageClassName":        "rook-cephfs-fs1",
		"pskSecretName":           "volsync-rsync-tls-vgr-1",
		"volumeSnapshotClassName": "csi-cephfsplugin-snapclass",
	}

	logger.V(1).Info("Reconciling", "as", vgr.Spec.ReplicationState)
	// Reconcile based on replication state
	switch vgr.Spec.ReplicationState {
	case volrep.Primary:
		return r.reconcilePrimary(ctx, logger, vgr, vgrClass)
	case volrep.Secondary:
		return r.reconcileSecondary(ctx, logger, vgr, vgrClass)
	case volrep.Resync:
		// For resync, we don't do anything special in this mock
		logger.Info("Resync requested but not implemented in mock")
		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	default:
		logger.Error(fmt.Errorf("unknown replication state %q", vgr.Spec.ReplicationState),
			"spec.replicationState must be primary, secondary, or resync")
		return ctrl.Result{}, nil
	}
}

// ── PRIMARY ──────────────────────────────────────────────────────────────────

func (r *VolumeGroupReplicationReconciler) reconcilePrimary(
	ctx context.Context,
	logger logr.Logger,
	vgr *volrep.VolumeGroupReplication,
	vgrClass *volrep.VolumeGroupReplicationClass,
) (ctrl.Result, error) {
	logger.Info("Reconciling VolumeGroupReplication as primary")

	// Get PVCs based on selector
	if vgr.Spec.Source.Selector == nil {
		logger.Info("No PVC selector specified")
		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	sel, err := metav1.LabelSelectorAsSelector(vgr.Spec.Source.Selector)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("invalid pvcSelector: %w", err)
	}

	pvcList := &corev1.PersistentVolumeClaimList{}
	if err := r.List(ctx, pvcList,
		client.InNamespace(vgr.Namespace),
		client.MatchingLabelsSelector{Selector: sel},
	); err != nil {
		return ctrl.Result{}, err
	}

	// Create VolSync handler
	schedulingInterval := vgrClass.Spec.Parameters["schedulingInterval"]
	if schedulingInterval == "" {
		schedulingInterval = vgrClass.Spec.Parameters["schedule"]
	}
	vsHandler := volsync.NewVSHandler(ctx, r.Client, logger, vgr, schedulingInterval)

	protectedPVCs := []corev1.LocalObjectReference{}
	var latestSync *metav1.Time

	logger.V(1).Info("Protecting PVCs", "pvcCount", len(pvcList.Items))
	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]

		// Skip PVCs owned by VolSync to avoid self-replication loops
		if isVolSyncOwned(pvc) {
			continue
		}

		// Get PSK secret name from parameters or use default
		pskSecretName := vgrClass.Spec.Parameters["pskSecretName"]
		if pskSecretName == "" {
			pskSecretName = "volsync-rsync-tls-" + vgr.Name
		}

		// Use Submariner service name for remote address
		// The remote service name follows the pattern: <service-name>.<namespace>.svc.clusterset.local
		remoteAddress := volsync.GetRemoteServiceNameForRDFromPVCName(pvc.Name, vgr.Namespace)

		// Get VolumeSnapshotClassName from parameters (optional)
		var volumeSnapshotClassName *string
		if vscName := vgrClass.Spec.Parameters["volumeSnapshotClassName"]; vscName != "" {
			volumeSnapshotClassName = &vscName
		}

		// Use VolSync handler to reconcile ReplicationSource (like Ramen's ReconcileRS)
		rs, err := vsHandler.ReconcileRS(
			pvc.Name,
			remoteAddress,
			pskSecretName,
			pvc.Spec.StorageClassName,
			pvc.Spec.AccessModes,
			volumeSnapshotClassName,
		)
		if err != nil {
			return ctrl.Result{}, err
		}

		protectedPVCs = append(protectedPVCs, corev1.LocalObjectReference{Name: pvc.Name})

		// Get last sync time from ReplicationSource status
		if rs != nil && rs.Status != nil {
			latestSync = rs.Status.LastSyncTime
		}
	}

	// Update status
	vgr.Status.State = volrep.PrimaryState
	vgr.Status.PersistentVolumeClaimsRefList = protectedPVCs
	vgr.Status.LastSyncTime = latestSync
	vgr.Status.ObservedGeneration = vgr.Generation
	setCondition(&vgr.Status.Conditions, "Ready", len(protectedPVCs) > 0,
		"ReplicationSourcesCreated",
		fmt.Sprintf("%d ReplicationSource(s) active", len(protectedPVCs)))

	if err := r.Status().Update(ctx, vgr); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Primary reconcile complete", "protectedPVCs", len(protectedPVCs))
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// ── SECONDARY ────────────────────────────────────────────────────────────────

func (r *VolumeGroupReplicationReconciler) reconcileSecondary(
	ctx context.Context,
	logger logr.Logger,
	vgr *volrep.VolumeGroupReplication,
	vgrClass *volrep.VolumeGroupReplicationClass,
) (ctrl.Result, error) {
	logger = logger.WithValues("vgr", vgr.Name, "vgrClass", vgrClass.Name)
	logger.V(1).Info("Reconciling as secondary")

	// Get PVCs based on selector (or use parameters to determine which PVCs to protect)
	pvcNames := []string{}

	// Extract PVC names from VGRClass parameters (pvc-<name>: "true")
	for key, value := range vgrClass.Spec.Parameters {
		if value == "true" && len(key) > 4 && key[:4] == "pvc-" {
			pvcName := key[4:] // Remove "pvc-" prefix
			pvcNames = append(pvcNames, pvcName)
		}
	}

	if len(pvcNames) == 0 {
		logger.Info("No PVCs configured in VGRClass parameters")
		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	// Create VolSync handler
	schedulingInterval := vgrClass.Spec.Parameters["schedulingInterval"]
	if schedulingInterval == "" {
		schedulingInterval = vgrClass.Spec.Parameters["schedule"]
	}
	vsHandler := volsync.NewVSHandler(ctx, r.Client, logger, vgr, schedulingInterval)

	// Get configuration from VGRClass
	capacity := vgrClass.Spec.Parameters["capacity"]
	if capacity == "" {
		capacity = "1Gi"
	}
	capacityQuantity, err := resource.ParseQuantity(capacity)
	if err != nil {
		logger.Error(err, "Failed to parse capacity", "capacity", capacity)
		return ctrl.Result{}, err
	}

	storageClassName := vgrClass.Spec.Parameters["storageClassName"]
	serviceType := volsync.DefaultRsyncServiceType

	protectedPVCs := []corev1.LocalObjectReference{}
	allReady := true

	for _, pvcName := range pvcNames {
		// Get PSK secret name from parameters or use default
		pskSecretName := vgrClass.Spec.Parameters["pskSecretName"]
		if pskSecretName == "" {
			pskSecretName = "volsync-rsync-tls-" + vgr.Name
		}

		// Use VolSync handler to reconcile ReplicationDestination (like Ramen's ReconcileRD)
		rd, err := vsHandler.ReconcileRD(
			pvcName,
			&capacityQuantity,
			&storageClassName,
			[]corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			pskSecretName,
			&serviceType,
		)
		if err != nil {
			return ctrl.Result{}, err
		}

		if rd == nil {
			// RD not ready yet
			allReady = false
			continue
		}

		protectedPVCs = append(protectedPVCs, corev1.LocalObjectReference{Name: pvcName})

		// Log the address and key secret for user to copy to primary
		if rd.Status != nil && rd.Status.RsyncTLS != nil {
			if rd.Status.RsyncTLS.Address != nil && rd.Status.RsyncTLS.KeySecret != nil {
				logger.Info("ReplicationDestination ready",
					"pvc", pvcName,
					"address", *rd.Status.RsyncTLS.Address,
					"keySecret", *rd.Status.RsyncTLS.KeySecret)
			}
		}
	}

	// Update status
	vgr.Status.State = volrep.SecondaryState
	vgr.Status.PersistentVolumeClaimsRefList = protectedPVCs
	vgr.Status.ObservedGeneration = vgr.Generation

	msg := fmt.Sprintf("%d destination(s) ready", len(protectedPVCs))
	if !allReady {
		msg = "waiting for service addresses to be assigned"
	}
	setCondition(&vgr.Status.Conditions, "Ready", allReady, "ReplicationDestinationsReady", msg)

	if err := r.Status().Update(ctx, vgr); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("Secondary reconcile complete", "destinations", len(protectedPVCs), "allReady", allReady)

	if !allReady {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}
	return ctrl.Result{RequeueAfter: requeueInterval}, nil
}

// ── DELETION ─────────────────────────────────────────────────────────────────

func (r *VolumeGroupReplicationReconciler) reconcileDelete(
	ctx context.Context,
	logger logr.Logger,
	vgr *volrep.VolumeGroupReplication,
) (ctrl.Result, error) {
	logger.Info("VolumeGroupReplication deleted — owned RS/RD objects garbage collected via ownerReference")

	controllerutil.RemoveFinalizer(vgr, vgrFinalizer)
	if err := r.Update(ctx, vgr); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// ── HELPERS ──────────────────────────────────────────────────────────────────

func isVolSyncOwned(pvc *corev1.PersistentVolumeClaim) bool {
	if _, ok := pvc.Labels["volsync.backube/owned-by"]; ok {
		return true
	}
	for _, ref := range pvc.OwnerReferences {
		if ref.APIVersion == "volsync.backube/v1alpha1" {
			return true
		}
	}
	return false
}

func setCondition(conditions *[]metav1.Condition, condType string, status bool, reason, message string) {
	s := metav1.ConditionFalse
	if status {
		s = metav1.ConditionTrue
	}
	now := metav1.Now()
	for i, c := range *conditions {
		if c.Type == condType {
			(*conditions)[i].Status = s
			(*conditions)[i].Reason = reason
			(*conditions)[i].Message = message
			(*conditions)[i].LastTransitionTime = now
			(*conditions)[i].ObservedGeneration = 0 // VGR doesn't track this in conditions
			return
		}
	}
	*conditions = append(*conditions, metav1.Condition{
		Type:               condType,
		Status:             s,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: now,
		ObservedGeneration: 0,
	})
}

func (r *VolumeGroupReplicationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&volrep.VolumeGroupReplication{}).
		Owns(&volsyncv1alpha1.ReplicationSource{}).
		Owns(&volsyncv1alpha1.ReplicationDestination{}).
		Complete(r)
}

// Made with Bob
