package volsync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// VRGOwnerLabel is used to label VolSync resources with their owner
	VRGOwnerLabel = "volumegroupreplication-owner"

	// SchedulingIntervalMinLength is the minimum length for scheduling interval
	SchedulingIntervalMinLength = 2

	// CronSpecMaxDayOfMonth is the maximum day of month for cron spec
	CronSpecMaxDayOfMonth = 28

	// tlsPSKDataSize is the size of the TLS pre-shared key data
	tlsPSKDataSize = 64

	// ServiceExport constants for Submariner
	ServiceExportKind    = "ServiceExport"
	ServiceExportGroup   = "multicluster.x-k8s.io"
	ServiceExportVersion = "v1alpha1"
)

var (
	// DefaultScheduleCronSpec is the default schedule for replication
	DefaultScheduleCronSpec = "*/5 * * * *" // Every 5 mins

	// DefaultRsyncServiceType is ClusterIP for use with Submariner
	DefaultRsyncServiceType corev1.ServiceType = corev1.ServiceTypeClusterIP
)

// VSHandler handles VolSync ReplicationSource and ReplicationDestination resources
type VSHandler struct {
	ctx                context.Context
	client             client.Client
	log                logr.Logger
	owner              metav1.Object
	schedulingInterval string
}

// NewVSHandler creates a new VolSync handler
func NewVSHandler(
	ctx context.Context,
	client client.Client,
	log logr.Logger,
	owner metav1.Object,
	schedulingInterval string,
) *VSHandler {
	return &VSHandler{
		ctx:                ctx,
		client:             client,
		log:                log,
		owner:              owner,
		schedulingInterval: schedulingInterval,
	}
}

// ReconcileRD reconciles a ReplicationDestination for secondary cluster
func (v *VSHandler) ReconcileRD(
	pvcName string,
	capacity *resource.Quantity,
	storageClassName *string,
	accessModes []corev1.PersistentVolumeAccessMode,
	pskSecretName string,
	serviceType *corev1.ServiceType,
) (*volsyncv1alpha1.ReplicationDestination, error) {
	l := v.log.WithValues("pvcName", pvcName)

	// Check if a ReplicationSource is still here (Can happen if transitioning from primary to secondary)
	// Before creating a new RD for this PVC, make sure any ReplicationSource for this PVC is cleaned up first
	err := v.DeleteRS(pvcName)
	if err != nil {
		return nil, err
	}

	// Validate that the PSK secret exists
	secretExists, err := v.validateSecretAndAddOwnerRef(pskSecretName)
	if err != nil || !secretExists {
		return nil, err
	}

	// Create RD first (without PVC initially)
	rd, err := v.createOrUpdateRD(pvcName, capacity, storageClassName, accessModes, pskSecretName, serviceType)
	if err != nil {
		return nil, err
	}

	// Now create destination PVC with RD as owner (like Ramen's EnsurePVCforDirectCopy)
	err = v.ensureDestinationPVCWithOwner(rd, pvcName, capacity, storageClassName, accessModes)
	if err != nil {
		return nil, err
	}

	// Create ServiceExport for Submariner
	err = v.reconcileServiceExportForRD(rd)
	if err != nil {
		return nil, err
	}

	if !rdStatusReady(rd, l) {
		return nil, nil
	}

	l.V(1).Info("ReplicationDestination Reconcile Complete")

	return rd, nil
}

// rdStatusReady checks if ReplicationDestination is ready
func rdStatusReady(rd *volsyncv1alpha1.ReplicationDestination, log logr.Logger) bool {
	if rd.Status == nil {
		return false
	}

	if rd.Status.RsyncTLS == nil || rd.Status.RsyncTLS.Address == nil {
		log.V(1).Info("ReplicationDestination waiting for Address ...")
		return false
	}

	return true
}

// ensureDestinationPVCWithOwner creates the destination PVC if it doesn't exist
// The PVC is owned by the ReplicationDestination (not VGR) for proper lifecycle management
// This is similar to Ramen's EnsurePVCforDirectCopy function
func (v *VSHandler) ensureDestinationPVCWithOwner(
	rd *volsyncv1alpha1.ReplicationDestination,
	pvcName string,
	capacity *resource.Quantity,
	storageClassName *string,
	accessModes []corev1.PersistentVolumeAccessMode,
) error {
	l := v.log.WithValues("pvcName", pvcName, "rdName", rd.Name)

	if len(accessModes) == 0 {
		return fmt.Errorf("accessModes must be provided for PVC %s", pvcName)
	}

	if capacity == nil {
		return fmt.Errorf("capacity must be provided for PVC %s", pvcName)
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: v.owner.GetNamespace(),
		},
	}

	op, err := ctrlutil.CreateOrUpdate(v.ctx, v.client, pvc, func() error {
		// Set ReplicationDestination as owner (not VGR)
		if err := ctrl.SetControllerReference(rd, pvc, v.client.Scheme()); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}

		// Only set spec fields if PVC is being created (not already exists)
		if pvc.CreationTimestamp.IsZero() {
			pvc.Spec.AccessModes = accessModes
			pvc.Spec.StorageClassName = storageClassName
			volumeMode := corev1.PersistentVolumeFilesystem
			pvc.Spec.VolumeMode = &volumeMode
		}

		// Always update capacity (can be expanded)
		pvc.Spec.Resources.Requests = corev1.ResourceList{
			corev1.ResourceStorage: *capacity,
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to create/update destination PVC: %w", err)
	}

	l.V(1).Info("Destination PVC ensured with RD as owner", "operation", op, "pvc", pvcName, "owner", rd.Name)

	return nil
}

// createOrUpdateRD creates or updates a ReplicationDestination
func (v *VSHandler) createOrUpdateRD(
	pvcName string,
	capacity *resource.Quantity,
	storageClassName *string,
	accessModes []corev1.PersistentVolumeAccessMode,
	pskSecretName string,
	serviceType *corev1.ServiceType,
) (*volsyncv1alpha1.ReplicationDestination, error) {
	l := v.log.WithValues("pvcName", pvcName)

	if len(accessModes) == 0 {
		accessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}

	rd := &volsyncv1alpha1.ReplicationDestination{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getReplicationDestinationName(pvcName),
			Namespace: v.owner.GetNamespace(),
		},
	}

	op, err := ctrlutil.CreateOrUpdate(v.ctx, v.client, rd, func() error {
		if err := ctrl.SetControllerReference(v.owner, rd, v.client.Scheme()); err != nil {
			l.Error(err, "unable to set controller reference")
			return fmt.Errorf("%w", err)
		}

		addVRGOwnerLabel(v.owner, rd)

		rd.Spec.RsyncTLS = &volsyncv1alpha1.ReplicationDestinationRsyncTLSSpec{
			ServiceType: serviceType,
			KeySecret:   &pskSecretName,
			ReplicationDestinationVolumeOptions: volsyncv1alpha1.ReplicationDestinationVolumeOptions{
				CopyMethod:       volsyncv1alpha1.CopyMethodSnapshot,
				Capacity:         capacity,
				StorageClassName: storageClassName,
				AccessModes:      accessModes,
				DestinationPVC:   &pvcName, // Use the pre-created PVC as destination
			},
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	l.V(1).Info("ReplicationDestination createOrUpdate Complete", "op", op)

	return rd, nil
}

// ReconcileRS reconciles a ReplicationSource for primary cluster
func (v *VSHandler) ReconcileRS(
	pvcName string,
	remoteAddress string,
	pskSecretName string,
	storageClassName *string,
	accessModes []corev1.PersistentVolumeAccessMode,
	volumeSnapshotClassName *string,
) (*volsyncv1alpha1.ReplicationSource, error) {
	l := v.log.WithValues("pvcName", pvcName)

	// Validate that the PSK secret exists
	secretExists, err := v.validateSecretAndAddOwnerRef(pskSecretName)
	if err != nil || !secretExists {
		return nil, err
	}

	// Check if a ReplicationDestination is still here (Can happen if transitioning from secondary to primary)
	// Before creating a new RS for this PVC, make sure any ReplicationDestination for this PVC is cleaned up first
	err = v.DeleteRD(pvcName)
	if err != nil {
		return nil, err
	}

	replicationSource, err := v.createOrUpdateRS(pvcName, remoteAddress, pskSecretName, storageClassName, accessModes, volumeSnapshotClassName)
	if err != nil {
		return nil, err
	}

	l.V(1).Info("ReplicationSource Reconcile Complete")

	return replicationSource, nil
}

// createOrUpdateRS creates or updates a ReplicationSource
func (v *VSHandler) createOrUpdateRS(
	pvcName string,
	remoteAddress string,
	pskSecretName string,
	storageClassName *string,
	accessModes []corev1.PersistentVolumeAccessMode,
	volumeSnapshotClassName *string,
) (*volsyncv1alpha1.ReplicationSource, error) {
	l := v.log.WithValues("pvcName", pvcName)

	rs := &volsyncv1alpha1.ReplicationSource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      getReplicationSourceName(pvcName),
			Namespace: v.owner.GetNamespace(),
		},
	}

	op, err := ctrlutil.CreateOrUpdate(v.ctx, v.client, rs, func() error {
		if err := ctrl.SetControllerReference(v.owner, rs, v.client.Scheme()); err != nil {
			l.Error(err, "unable to set controller reference")
			return fmt.Errorf("%w", err)
		}

		addVRGOwnerLabel(v.owner, rs)

		rs.Spec.SourcePVC = pvcName

		// Set schedule
		scheduleCronSpec, err := v.getScheduleCronSpec()
		if err != nil {
			l.Error(err, "unable to parse schedulingInterval")
			return err
		}
		rs.Spec.Trigger = &volsyncv1alpha1.ReplicationSourceTriggerSpec{
			Schedule: scheduleCronSpec,
		}

		rs.Spec.RsyncTLS = &volsyncv1alpha1.ReplicationSourceRsyncTLSSpec{
			KeySecret: &pskSecretName,
			Address:   &remoteAddress,
			ReplicationSourceVolumeOptions: volsyncv1alpha1.ReplicationSourceVolumeOptions{
				CopyMethod:              volsyncv1alpha1.CopyMethodSnapshot,
				StorageClassName:        storageClassName,
				AccessModes:             accessModes,
				VolumeSnapshotClassName: volumeSnapshotClassName,
			},
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	l.V(1).Info("ReplicationSource createOrUpdate Complete", "op", op)

	return rs, nil
}

// validateSecretAndAddOwnerRef validates that a secret exists and adds owner reference
// The secret must be pre-created by the user
func (v *VSHandler) validateSecretAndAddOwnerRef(secretName string) (bool, error) {
	secret := &corev1.Secret{}

	err := v.client.Get(v.ctx,
		types.NamespacedName{
			Name:      secretName,
			Namespace: v.owner.GetNamespace(),
		}, secret)
	if err != nil {
		if kerrors.IsNotFound(err) {
			v.log.Error(err, "Secret not found - must be pre-created", "secretName", secretName)
			return false, fmt.Errorf("secret %s not found in namespace %s - must be created before replication",
				secretName, v.owner.GetNamespace())
		}

		v.log.Error(err, "Failed to get secret", "secretName", secretName)
		return false, fmt.Errorf("error getting secret (%w)", err)
	}

	v.log.Info("Secret exists", "secretName", secretName)

	// Add owner reference
	if err := v.addOwnerReferenceAndUpdate(secret, v.owner); err != nil {
		v.log.Error(err, "Unable to update secret", "secretName", secretName)
		return true, err
	}

	v.log.V(1).Info("VolSync secret validated", "secret name", secretName)

	return true, nil
}

// DeleteRS deletes a ReplicationSource by PVC name
func (v *VSHandler) DeleteRS(pvcName string) error {
	currentRSListByOwner, err := v.listRSByOwner()
	if err != nil {
		return err
	}

	for i := range currentRSListByOwner.Items {
		rs := currentRSListByOwner.Items[i]

		if rs.GetName() == getReplicationSourceName(pvcName) {
			if err := v.client.Delete(v.ctx, &rs); err != nil {
				v.log.Error(err, "Error cleaning up ReplicationSource", "name", rs.GetName())
			} else {
				v.log.Info("Deleted ReplicationSource", "name", rs.GetName())
			}
		}
	}

	return nil
}

// DeleteRD deletes a ReplicationDestination by PVC name
func (v *VSHandler) DeleteRD(pvcName string) error {
	currentRDListByOwner, err := v.listRDByOwner()
	if err != nil {
		return err
	}

	for i := range currentRDListByOwner.Items {
		rd := currentRDListByOwner.Items[i]

		if rd.GetName() == getReplicationDestinationName(pvcName) {
			if err := v.client.Delete(v.ctx, &rd); err != nil {
				v.log.Error(err, "Error cleaning up ReplicationDestination", "name", rd.GetName())
			} else {
				v.log.Info("Deleted ReplicationDestination", "name", rd.GetName())
			}
		}
	}

	return nil
}

// listRSByOwner lists ReplicationSources owned by this VGR
func (v *VSHandler) listRSByOwner() (volsyncv1alpha1.ReplicationSourceList, error) {
	rsList := volsyncv1alpha1.ReplicationSourceList{}
	if err := v.listByOwner(&rsList); err != nil {
		v.log.Error(err, "Failed to list ReplicationSources", "owner", v.owner.GetName())
		return rsList, err
	}

	return rsList, nil
}

// reconcileServiceExportForRD creates a ServiceExport for the ReplicationDestination service
// This allows Submariner to export the service for cross-cluster access
func (v *VSHandler) reconcileServiceExportForRD(rd *volsyncv1alpha1.ReplicationDestination) error {
	// Using unstructured to avoid needing to require serviceexport in client scheme
	svcExport := &unstructured.Unstructured{}
	svcExport.Object = map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      getLocalServiceNameForRD(rd.GetName()), // Get name of the local service (this needs to be exported)
			"namespace": rd.GetNamespace(),
		},
	}
	svcExport.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   ServiceExportGroup,
		Kind:    ServiceExportKind,
		Version: ServiceExportVersion,
	})

	op, err := ctrlutil.CreateOrUpdate(v.ctx, v.client, svcExport, func() error {
		// Make this ServiceExport owned by the replication destination itself rather than the VRG
		// This way on relocate scenarios or failover/failback, when the RD is cleaned up the associated
		// ServiceExport will get cleaned up with it.
		if err := ctrlutil.SetOwnerReference(rd, svcExport, v.client.Scheme()); err != nil {
			v.log.Error(err, "unable to set controller reference", "resource", svcExport)
			return fmt.Errorf("%w", err)
		}

		return nil
	})

	v.log.V(1).Info("ServiceExport createOrUpdate Complete", "op", op)

	if err != nil {
		v.log.Error(err, "error creating or updating ServiceExport", "replication destination name", rd.GetName(),
			"namespace", rd.GetNamespace())
		return fmt.Errorf("error creating or updating ServiceExport (%w)", err)
	}

	v.log.V(1).Info("ServiceExport Reconcile Complete")

	return nil
}

// listRDByOwner lists ReplicationDestinations owned by this VGR
func (v *VSHandler) listRDByOwner() (volsyncv1alpha1.ReplicationDestinationList, error) {
	rdList := volsyncv1alpha1.ReplicationDestinationList{}
	if err := v.listByOwner(&rdList); err != nil {
		v.log.Error(err, "Failed to list ReplicationDestinations", "owner", v.owner.GetName())
		return rdList, err
	}

	return rdList, nil
}

// listByOwner lists resources by owner label
func (v *VSHandler) listByOwner(list client.ObjectList) error {
	matchLabels := map[string]string{
		VRGOwnerLabel: v.owner.GetName(),
	}
	listOptions := []client.ListOption{
		client.InNamespace(v.owner.GetNamespace()),
		client.MatchingLabels(matchLabels),
	}

	if err := v.client.List(v.ctx, list, listOptions...); err != nil {
		v.log.Error(err, "Failed to list by label", "matchLabels", matchLabels)
		return fmt.Errorf("error listing by label (%w)", err)
	}

	return nil
}

// addOwnerReferenceAndUpdate adds owner reference and updates the object
func (v *VSHandler) addOwnerReferenceAndUpdate(obj client.Object, owner metav1.Object) error {
	needsUpdate, err := v.addOwnerReference(obj, owner)
	if err != nil {
		return err
	}

	if needsUpdate {
		if err := v.client.Update(v.ctx, obj); err != nil {
			v.log.Error(err, "Failed to add owner reference to obj", "obj", obj.GetName())
			return fmt.Errorf("failed to add owner reference to %s (%w)", obj.GetName(), err)
		}

		v.log.Info("ownerRef added to object", "obj", obj.GetName())
	}

	return nil
}

// addOwnerReference adds an owner reference to an object
func (v *VSHandler) addOwnerReference(obj, owner metav1.Object) (bool, error) {
	currentOwnerRefs := obj.GetOwnerReferences()

	err := ctrlutil.SetOwnerReference(owner, obj, v.client.Scheme())
	if err != nil {
		return false, fmt.Errorf("%w", err)
	}

	// Check if owner references changed
	needsUpdate := len(obj.GetOwnerReferences()) != len(currentOwnerRefs)
	if !needsUpdate {
		for i := range obj.GetOwnerReferences() {
			if i >= len(currentOwnerRefs) || obj.GetOwnerReferences()[i] != currentOwnerRefs[i] {
				needsUpdate = true
				break
			}
		}
	}

	return needsUpdate, nil
}

// getScheduleCronSpec returns the schedule in cron format
func (v *VSHandler) getScheduleCronSpec() (*string, error) {
	if v.schedulingInterval != "" {
		return ConvertSchedulingIntervalToCronSpec(v.schedulingInterval)
	}

	// Use default value if not specified
	v.log.Info("Warning - scheduling interval is empty, using default Schedule for volsync",
		"DefaultScheduleCronSpec", DefaultScheduleCronSpec)

	return &DefaultScheduleCronSpec, nil
}

// Helper functions

// addVRGOwnerLabel adds owner label to an object
func addVRGOwnerLabel(owner, obj metav1.Object) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	labels[VRGOwnerLabel] = owner.GetName()
	obj.SetLabels(labels)
}

// getReplicationDestinationName returns the name for a ReplicationDestination
func getReplicationDestinationName(pvcName string) string {
	return pvcName // Use PVC name as name of ReplicationDestination
}

// getReplicationSourceName returns the name for a ReplicationSource
func getReplicationSourceName(pvcName string) string {
	return pvcName // Use PVC name as name of ReplicationSource
}

// getLocalServiceNameForRD returns the local service name for a ReplicationDestination
// This is the name VolSync will use for the service
func getLocalServiceNameForRD(rdName string) string {
	return fmt.Sprintf("volsync-rsync-tls-dst-%s", rdName)
}

// GetRemoteServiceNameForRDFromPVCName returns the remote service name for cross-cluster access
// This assumes Submariner and that a ServiceExport is created for the service
func GetRemoteServiceNameForRDFromPVCName(pvcName, rdNamespace string) string {
	rdName := getReplicationDestinationName(pvcName)
	return fmt.Sprintf("%s.%s.svc.clusterset.local", getLocalServiceNameForRD(rdName), rdNamespace)
}

// Made with Bob

// ConvertSchedulingIntervalToCronSpec converts scheduling interval to cron spec
// Format: <num><m,h,d> where m=minutes, h=hours, d=days
// Example: "5m" -> "*/5 * * * *", "2h" -> "0 */2 * * *", "1d" -> "0 0 */1 * *"
func ConvertSchedulingIntervalToCronSpec(schedulingInterval string) (*string, error) {
	// format needs to have at least 1 number and end with m or h or d
	if len(schedulingInterval) < SchedulingIntervalMinLength {
		return nil, fmt.Errorf("scheduling interval %s is invalid", schedulingInterval)
	}

	mhd := schedulingInterval[len(schedulingInterval)-1:]
	mhd = string([]rune(mhd)[0]) // Get first character

	// Convert to lowercase
	if mhd == "M" {
		mhd = "m"
	} else if mhd == "H" {
		mhd = "h"
	} else if mhd == "D" {
		mhd = "d"
	}

	num := schedulingInterval[:len(schedulingInterval)-1]

	numInt := 0
	_, err := fmt.Sscanf(num, "%d", &numInt)
	if err != nil {
		return nil, fmt.Errorf("scheduling interval prefix %s cannot be converted to an int value", num)
	}

	var cronSpec string

	switch mhd {
	case "m":
		cronSpec = fmt.Sprintf("*/%s * * * *", num)
	case "h":
		// TODO: cronspec has a max here of 23 hours - do we try to convert into days?
		cronSpec = fmt.Sprintf("0 */%s * * *", num)
	case "d":
		if numInt > CronSpecMaxDayOfMonth {
			// Max # of days in interval we'll allow is 28 - otherwise there are issues converting to a cronspec
			// which is expected to be a day of the month (1-31).  I.e. if we tried to set to */31 we'd get
			// every 31st day of the month
			num = "28"
		}

		cronSpec = fmt.Sprintf("0 0 */%s * *", num)
	}

	if cronSpec == "" {
		return nil, fmt.Errorf("scheduling interval %s is invalid. Unable to parse m/h/d", schedulingInterval)
	}

	return &cronSpec, nil
}

// generateVolSyncReplicationSecret generates a new VolSync replication secret with PSK
func (v *VSHandler) generateVolSyncReplicationSecret(secretName string) (*corev1.Secret, error) {
	tlsKey, err := genTLSPreSharedKey(v.log)
	if err != nil {
		v.log.Error(err, "Unable to generate new tls secret for VolSync replication")
		return nil, err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: v.owner.GetNamespace(),
		},
		StringData: map[string]string{
			"psk.txt": "volsyncmock:" + tlsKey,
		},
	}

	return secret, nil
}

// genTLSPreSharedKey generates a TLS pre-shared key
func genTLSPreSharedKey(log logr.Logger) (string, error) {
	pskData := make([]byte, tlsPSKDataSize)
	if _, err := rand.Read(pskData); err != nil {
		log.Error(err, "error generating tls key")
		return "", err
	}

	return hex.EncodeToString(pskData), nil
}
