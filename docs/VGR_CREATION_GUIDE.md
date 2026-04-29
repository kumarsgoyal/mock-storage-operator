# VolumeGroupReplication Creation Guide

This guide provides detailed instructions for creating VolumeGroupReplication (VGR) resources using the Mock Storage Operator with label selector-based PVC discovery.

## Table of Contents
1. [Overview](#overview)
2. [Prerequisites](#prerequisites)
3. [Understanding Label Selectors](#understanding-label-selectors)
4. [Creating VGR Resources](#creating-vgr-resources)
5. [Verification](#verification)
6. [Common Scenarios](#common-scenarios)

---

## Overview

The Mock Storage Operator uses a label selector-based approach for PVC discovery. This means:

1. **Primary Cluster**: PVCs must have labels that match the VGR's selector
2. **Secondary Cluster**: The same PVCs (with matching labels) are discovered automatically
3. **Configuration**: Comes from VGRClass parameters and optional PVC annotations

### Workflow

```
Primary Cluster                    Secondary Cluster
┌─────────────────┐               ┌──────────────────┐
│ PVCs with       │               │ PVCs with        │
│ matching labels │               │ matching labels  │
│ - mysql-data    │               │ - mysql-data     │
│ - postgres-data │               │ - postgres-data  │
└─────────────────┘               └──────────────────┘
        │                                  │
        │                                  │
        ▼                                  ▼
┌─────────────────┐               ┌──────────────────┐
│ VGR (primary)   │               │ VGR (secondary)  │
│ - Finds PVCs    │◄─────────────►│ - Finds PVCs     │
│ - Creates RS    │   Replication │ - Creates RD     │
└─────────────────┘               └──────────────────┘
```

---

## Prerequisites

Before creating VGR resources, ensure:

### On Both Clusters

- ✅ Mock Storage Operator is deployed
- ✅ VolSync is installed
- ✅ VolumeGroupReplication CRDs are installed
- ✅ PSK secrets are created in the application namespace

### On Primary Cluster

- ✅ Application PVCs exist and are labeled correctly
- ✅ PVCs are bound and contain data

### On Secondary Cluster

- ✅ Storage classes exist for ReplicationDestinations
- ✅ Volume snapshot classes exist (if using snapshots)

---

## Understanding Label Selectors

### How It Works

The VGR uses a Kubernetes label selector to find PVCs to replicate. This is the same mechanism used by Deployments, Services, and other Kubernetes resources.

### Label Selector Format

```yaml
spec:
  source:
    selector:
      matchLabels:
        app: myapp
        tier: database
```

This selector will match any PVC with **both** labels:
- `app=myapp`
- `tier=database`

### PVC Labeling

PVCs must have labels that match the VGR selector:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: mysql-data
  namespace: myapp
  labels:
    app: myapp        # Matches selector
    tier: database    # Matches selector
spec:
  # ... PVC spec
```

### Best Practices

1. **Use consistent labels** - Apply the same labels across all PVCs you want to replicate
2. **Be specific** - Use multiple labels to avoid accidentally selecting unwanted PVCs
3. **Document your labels** - Keep a record of which labels are used for replication
4. **Avoid conflicts** - Don't use labels that might match VolSync-owned PVCs

### Common Label Patterns

#### Pattern 1: Application-based
```yaml
labels:
  app: myapp
```
Matches all PVCs for a specific application.

#### Pattern 2: Tier-based
```yaml
labels:
  app: myapp
  tier: database
```
Matches only database PVCs for an application.

#### Pattern 3: Environment-based
```yaml
labels:
  app: myapp
  environment: production
```
Matches PVCs for a specific environment.

---

## Creating VGR Resources

### Step 1: Label Your PVCs

First, ensure your PVCs on the **primary cluster** have appropriate labels:

```bash
# Check existing PVC labels
kubectl get pvc -n myapp --show-labels --context primary

# Add labels to existing PVCs if needed
kubectl label pvc mysql-data -n myapp app=myapp --context primary
kubectl label pvc postgres-data -n myapp app=myapp --context primary
```

Or create new PVCs with labels:

```bash
cat <<EOF | kubectl apply -f - --context primary
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: mysql-data
  namespace: myapp
  labels:
    app: myapp
    tier: database
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
  storageClassName: standard
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: postgres-data
  namespace: myapp
  labels:
    app: myapp
    tier: database
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 5Gi
  storageClassName: standard
EOF
```

### Step 2: Create VolumeGroupReplicationClass

Create VGRClass on **both clusters**:

```bash
cat <<EOF | kubectl apply -f - --context primary
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeGroupReplicationClass
metadata:
  name: mock-vgr-class
  annotations:
    replication.storage.openshift.io/is-default-class: "true"
  labels:
    ramendr.openshift.io/groupreplicationid: mock-storage-group-id
    ramendr.openshift.io/storageid: mock-storage-id
    ramendr.openshift.io/global: "true"
spec:
  provisioner: kubernetes.io/no-provisioner
  parameters:
    # Default scheduling interval (can be overridden per-PVC via annotations)
    schedulingInterval: "5m"
    
    # Default capacity for ReplicationDestinations
    capacity: "10Gi"
    
    # Default storage class
    storageClassName: "standard"
    
    # PSK secret name
    pskSecretName: "volsync-rsync-tls-secret"
    
    # Volume snapshot class (optional)
    volumeSnapshotClassName: "csi-snapclass"
EOF

# Apply the same VGRClass on secondary
kubectl apply -f - --context secondary <<EOF
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeGroupReplicationClass
metadata:
  name: mock-vgr-class
  annotations:
    replication.storage.openshift.io/is-default-class: "true"
  labels:
    ramendr.openshift.io/groupreplicationid: mock-storage-group-id
    ramendr.openshift.io/storageid: mock-storage-id
    ramendr.openshift.io/global: "true"
spec:
  provisioner: kubernetes.io/no-provisioner
  parameters:
    schedulingInterval: "5m"
    capacity: "10Gi"
    storageClassName: "standard"
    pskSecretName: "volsync-rsync-tls-secret"
    volumeSnapshotClassName: "csi-snapclass"
EOF
```

**Important VGRClass Parameters:**

- `schedulingInterval`: How often to sync (e.g., `5m`, `1h`, or cron format `*/5 * * * *`)
- `capacity`: Default capacity for ReplicationDestinations
- `storageClassName`: Default storage class for ReplicationDestinations
- `pskSecretName`: Name of the PSK secret for rsync-tls authentication
- `volumeSnapshotClassName`: Volume snapshot class (optional)

### Step 3: Create Secondary VGR

Create the VGR on the **secondary cluster first**:

```bash
cat <<EOF | kubectl apply -f - --context secondary
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeGroupReplication
metadata:
  name: myapp-vgr
  namespace: myapp
spec:
  replicationState: secondary
  volumeGroupReplicationClassName: mock-vgr-class
  source:
    selector:
      matchLabels:
        app: myapp
  autoResync: true
EOF
```

**VGR Spec Fields:**

- `replicationState`: Set to `secondary` for the destination cluster
- `volumeGroupReplicationClassName`: Reference to the VGRClass
- `source.selector`: Label selector to find PVCs to replicate
- `autoResync`: Enable automatic resync on secondary

### Step 4: Wait for Secondary to be Ready

```bash
# Watch VGR status
kubectl get vgr myapp-vgr -n myapp --context secondary -w

# Wait for Ready condition
kubectl wait --for=condition=Ready vgr/myapp-vgr -n myapp --context secondary --timeout=5m

# Check status
kubectl get vgr myapp-vgr -n myapp --context secondary -o yaml
```

**Expected status:**
```yaml
status:
  state: Secondary
  conditions:
    - type: Ready
      status: "True"
      reason: ReconcileComplete
      message: "2 ReplicationDestination(s) active"
  persistentVolumeClaimsRefList:
    - name: mysql-data
    - name: postgres-data
  observedGeneration: 1
```

### Step 5: Create Primary VGR

Create the VGR on the **primary cluster**:

```bash
cat <<EOF | kubectl apply -f - --context primary
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeGroupReplication
metadata:
  name: myapp-vgr
  namespace: myapp
spec:
  replicationState: primary
  volumeGroupReplicationClassName: mock-vgr-class
  source:
    selector:
      matchLabels:
        app: myapp
EOF
```

**VGR Spec Fields:**

- `replicationState`: Set to `primary` for the source cluster
- `volumeGroupReplicationClassName`: Reference to the VGRClass
- `source.selector`: Label selector to find PVCs to replicate

### Step 6: Wait for Primary to be Ready

```bash
# Watch VGR status
kubectl get vgr myapp-vgr -n myapp --context primary -w

# Wait for Ready condition
kubectl wait --for=condition=Ready vgr/myapp-vgr -n myapp --context primary --timeout=5m

# Check status
kubectl get vgr myapp-vgr -n myapp --context primary -o yaml
```

**Expected status:**
```yaml
status:
  state: Primary
  conditions:
    - type: Ready
      status: "True"
      reason: ReplicationSourcesCreated
      message: "2 ReplicationSource(s) active"
  persistentVolumeClaimsRefList:
    - name: mysql-data
    - name: postgres-data
  lastSyncTime: "2026-04-22T10:30:00Z"
  observedGeneration: 1
```

---

## Verification

### Check VGR Status

```bash
# Primary cluster
kubectl get vgr myapp-vgr -n myapp --context primary

# Secondary cluster
kubectl get vgr myapp-vgr -n myapp --context secondary
```

### Check VolSync Resources

```bash
# Primary: ReplicationSources
kubectl get replicationsources -n myapp --context primary

# Secondary: ReplicationDestinations
kubectl get replicationdestinations -n myapp --context secondary
```

### Check Operator Logs

```bash
# Primary logs
kubectl logs -n mock-storage-operator-system -l app=mock-storage-operator --context primary --tail=50

# Secondary logs
kubectl logs -n mock-storage-operator-system -l app=mock-storage-operator --context secondary --tail=50
```

### Monitor Sync Progress

```bash
# Check last sync time
kubectl get vgr myapp-vgr -n myapp --context primary -o jsonpath='{.status.lastSyncTime}'

# Watch for updates
watch kubectl get vgr myapp-vgr -n myapp --context primary -o jsonpath='{.status.lastSyncTime}'
```

---

## Common Scenarios

### Scenario 1: Adding a New PVC to Replication

**On Primary:**
1. Create the new PVC with matching labels
2. The operator will automatically detect it and create a ReplicationSource

```bash
# Create new PVC with matching labels
cat <<EOF | kubectl apply -f - --context primary
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: redis-data
  namespace: myapp
  labels:
    app: myapp  # Matches VGR selector
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 2Gi
  storageClassName: standard
EOF

# Verify operator picks up the new PVC
kubectl logs -n mock-storage-operator-system -l app=mock-storage-operator --context primary --tail=20
```

**On Secondary:**
The operator will automatically create a ReplicationDestination for the new PVC.

### Scenario 2: Changing Sync Interval for Specific PVC

You can override the default scheduling interval using PVC annotations:

```bash
# Add annotation to PVC on primary
kubectl annotate pvc mysql-data -n myapp \
  replication.storage.openshift.io/scheduling-interval=3m \
  --context primary

# The operator will reconcile and update the ReplicationSource
```

### Scenario 3: Removing a PVC from Replication

**On Primary:**
1. Remove the matching label from the PVC
2. The operator will clean up the ReplicationSource

```bash
# Remove the label
kubectl label pvc redis-data -n myapp app- --context primary

# Verify ReplicationSource is removed
kubectl get replicationsources -n myapp --context primary
```

### Scenario 4: Multi-Namespace Setup

If you have PVCs in different namespaces, create separate VGRs for each namespace:

```bash
# Namespace 1: myapp
cat <<EOF | kubectl apply -f - --context primary
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeGroupReplication
metadata:
  name: myapp-vgr
  namespace: myapp
spec:
  replicationState: primary
  volumeGroupReplicationClassName: mock-vgr-class
  source:
    selector:
      matchLabels:
        app: myapp
EOF

# Namespace 2: database
cat <<EOF | kubectl apply -f - --context primary
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeGroupReplication
metadata:
  name: database-vgr
  namespace: database
spec:
  replicationState: primary
  volumeGroupReplicationClassName: mock-vgr-class
  source:
    selector:
      matchLabels:
        app: database
EOF
```

### Scenario 5: Using Multiple Label Selectors

For more precise PVC selection, use multiple labels:

```bash
cat <<EOF | kubectl apply -f - --context primary
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeGroupReplication
metadata:
  name: myapp-vgr
  namespace: myapp
spec:
  replicationState: primary
  volumeGroupReplicationClassName: mock-vgr-class
  source:
    selector:
      matchLabels:
        app: myapp
        tier: database
        environment: production
EOF
```

This will only replicate PVCs that have **all three** labels.

---

## Troubleshooting

### No PVCs Found

**Problem**: VGR shows no PVCs in status

```bash
# Check VGR selector
kubectl get vgr myapp-vgr -n myapp -o jsonpath='{.spec.source.selector}' --context primary

# Check PVC labels
kubectl get pvc -n myapp --show-labels --context primary

# Verify labels match
kubectl get pvc -n myapp -l app=myapp --context primary
```

**Solution**: Ensure PVC labels match the VGR selector exactly.

### PVC Not Being Replicated

**Problem**: Specific PVC not showing up in VGR status

```bash
# Check if PVC has matching labels
kubectl get pvc <pvc-name> -n myapp --show-labels --context primary

# Check if PVC is owned by VolSync (should not be)
kubectl get pvc <pvc-name> -n myapp -o jsonpath='{.metadata.ownerReferences}' --context primary

# Check operator logs
kubectl logs -n mock-storage-operator-system -l app=mock-storage-operator --context primary | grep <pvc-name>
```

**Solution**: 
- Add missing labels to the PVC
- Ensure PVC is not owned by VolSync

### Replication Not Syncing

**Problem**: lastSyncTime not updating

```bash
# Check ReplicationSource status
kubectl get replicationsource -n myapp --context primary -o yaml

# Check ReplicationDestination status
kubectl get replicationdestination -n myapp --context secondary -o yaml

# Verify PSK secrets match
kubectl get secret volsync-rsync-tls-secret -n myapp --context primary -o jsonpath='{.data.psk\.txt}' | base64 -d
kubectl get secret volsync-rsync-tls-secret -n myapp --context secondary -o jsonpath='{.data.psk\.txt}' | base64 -d

# Check network connectivity (if using Submariner)
subctl show connections
```

---

## Best Practices

1. **Always create secondary VGR first** - This ensures ReplicationDestinations are ready before ReplicationSources try to connect

2. **Use consistent labeling** - Apply the same labels across all PVCs you want to replicate

3. **Be specific with selectors** - Use multiple labels to avoid accidentally selecting unwanted PVCs

4. **Document your labels** - Keep a record of which labels are used for replication

5. **Test with one PVC first** - Before replicating multiple PVCs, test with a single PVC to ensure everything works

6. **Monitor sync times** - Regularly check `lastSyncTime` in VGR status to ensure replication is working

7. **Use appropriate intervals** - Balance RPO requirements with network bandwidth and storage performance

8. **Keep VGRClass consistent** - Use the same VGRClass on both clusters for consistency

---

## Quick Reference

### VGR Template (Secondary)

```yaml
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeGroupReplication
metadata:
  name: <vgr-name>
  namespace: <namespace>
spec:
  replicationState: secondary
  volumeGroupReplicationClassName: mock-vgr-class
  source:
    selector:
      matchLabels:
        app: <app-label>
  autoResync: true
```

### VGR Template (Primary)

```yaml
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeGroupReplication
metadata:
  name: <vgr-name>
  namespace: <namespace>
spec:
  replicationState: primary
  volumeGroupReplicationClassName: mock-vgr-class
  source:
    selector:
      matchLabels:
        app: <app-label>
```

### PVC Template with Labels

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: <pvc-name>
  namespace: <namespace>
  labels:
    app: <app-label>
    tier: <tier-label>
  annotations:
    # Optional: Override default scheduling interval
    replication.storage.openshift.io/scheduling-interval: "3m"
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: <size>
  storageClassName: <storage-class>
```

---

**Document Version:** 2.0  
**Last Updated:** 2026-04-22  
**Operator Version:** latest (Label selector-based configuration)