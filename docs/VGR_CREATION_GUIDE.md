# VolumeGroupReplication Creation Guide

This guide provides detailed instructions for creating VolumeGroupReplication (VGR) resources using the Mock Storage Operator with ConfigMap-based PVC configuration.

## Table of Contents
1. [Overview](#overview)
2. [Prerequisites](#prerequisites)
3. [Understanding the ConfigMap](#understanding-the-configmap)
4. [Creating the ConfigMap](#creating-the-configmap)
5. [Creating VGR Resources](#creating-vgr-resources)
6. [Verification](#verification)
7. [Common Scenarios](#common-scenarios)

---

## Overview

The Mock Storage Operator uses a ConfigMap-based approach for PVC configuration. This means:

1. **Primary Cluster**: Has the actual PVCs with application data
2. **Secondary Cluster**: Needs a ConfigMap that describes which PVCs to replicate and how
3. **ConfigMap**: Contains per-PVC configuration (scheduling, storage class, snapshot class)

### Workflow

```
Primary Cluster                    Secondary Cluster
┌─────────────────┐               ┌──────────────────┐
│ PVCs with data  │               │ ConfigMap        │
│ - mysql-data    │               │ - Describes PVCs │
│ - postgres-data │               │ - Configuration  │
└─────────────────┘               └──────────────────┘
        │                                  │
        │                                  │
        ▼                                  ▼
┌─────────────────┐               ┌──────────────────┐
│ VGR (primary)   │               │ VGR (secondary)  │
│ - Finds PVCs    │◄─────────────►│ - Reads ConfigMap│
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

- ✅ ConfigMap with PVC configuration is created
- ✅ Storage classes specified in ConfigMap exist
- ✅ Volume snapshot classes specified in ConfigMap exist

---

## Understanding the ConfigMap

### ConfigMap Structure

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: pvc-config
  namespace: myapp  # Must match VGR namespace
data:
  # Key format: "pvc=<pvc-name>/<namespace>"
  # Value format: "schedulingInterval=<value>:storageClassName=<value>:volumeSnapshotClassName=<value>"
  "pvc=mysql-data/myapp": "schedulingInterval=5m:storageClassName=standard:volumeSnapshotClassName=csi-snapclass"
```

### Key Format

**Pattern**: `pvc=<pvc-name>/<namespace>`

- **pvc=**: Required prefix
- **<pvc-name>**: Name of the PVC on the primary cluster
- **<namespace>**: Namespace where the PVC exists (must match VGR namespace)

**Examples:**
```yaml
"pvc=mysql-data/myapp"           # PVC named "mysql-data" in "myapp" namespace
"pvc=postgres-data/database"     # PVC named "postgres-data" in "database" namespace
"pvc=app-config/production"      # PVC named "app-config" in "production" namespace
```

### Value Format

**Pattern**: `schedulingInterval=<value>:storageClassName=<value>:volumeSnapshotClassName=<value>`

All three parameters are **required** and separated by colons (`:`).

#### schedulingInterval

How often to sync data from primary to secondary.

**Formats:**
- Duration: `3m`, `5m`, `1h`, `30s`
- Cron: `*/5 * * * *` (every 5 minutes), `0 * * * *` (hourly)

**Examples:**
```
schedulingInterval=3m          # Every 3 minutes
schedulingInterval=5m          # Every 5 minutes
schedulingInterval=1h          # Every hour
schedulingInterval=*/5 * * * * # Every 5 minutes (cron)
schedulingInterval=0 2 * * *   # Daily at 2 AM (cron)
```

#### storageClassName

Storage class to use for the ReplicationDestination PVC on the secondary cluster.

**Must be:**
- A valid storage class that exists on the secondary cluster
- Compatible with the access modes required
- Able to provision the required capacity

**Examples:**
```
storageClassName=standard
storageClassName=fast-ssd
storageClassName=rook-cephfs
storageClassName=rook-ceph-block
```

#### volumeSnapshotClassName

Volume snapshot class to use for creating snapshots on the primary cluster.

**Must be:**
- A valid volume snapshot class that exists on the primary cluster
- Compatible with the storage class of the source PVC
- Able to create snapshots of the PVC

**Examples:**
```
volumeSnapshotClassName=csi-snapclass
volumeSnapshotClassName=csi-cephfsplugin-snapclass
volumeSnapshotClassName=csi-rbdplugin-snapclass
```

### Complete Examples

#### Example 1: Single PVC

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: pvc-config
  namespace: myapp
data:
  "pvc=mysql-data/myapp": "schedulingInterval=5m:storageClassName=standard:volumeSnapshotClassName=csi-snapclass"
```

#### Example 2: Multiple PVCs with Different Settings

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: pvc-config
  namespace: myapp
data:
  # Database - critical, sync every 3 minutes
  "pvc=mysql-data/myapp": "schedulingInterval=3m:storageClassName=fast-ssd:volumeSnapshotClassName=csi-snapclass"
  
  # Application data - moderate, sync every 10 minutes
  "pvc=app-data/myapp": "schedulingInterval=10m:storageClassName=standard:volumeSnapshotClassName=csi-snapclass"
  
  # Logs - low priority, sync hourly
  "pvc=logs/myapp": "schedulingInterval=1h:storageClassName=slow-hdd:volumeSnapshotClassName=csi-snapclass"
```

#### Example 3: Using Cron Expressions

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: pvc-config
  namespace: production
data:
  # Sync every 5 minutes
  "pvc=data1/production": "schedulingInterval=*/5 * * * *:storageClassName=standard:volumeSnapshotClassName=csi-snapclass"
  
  # Sync every hour at minute 0
  "pvc=data2/production": "schedulingInterval=0 * * * *:storageClassName=standard:volumeSnapshotClassName=csi-snapclass"
  
  # Sync daily at 2 AM
  "pvc=data3/production": "schedulingInterval=0 2 * * *:storageClassName=standard:volumeSnapshotClassName=csi-snapclass"
```

---

## Creating the ConfigMap

### Step 1: List PVCs on Primary Cluster

First, identify which PVCs you want to replicate:

```bash
# List all PVCs in the namespace
kubectl get pvc -n myapp --context primary

# Get detailed information
kubectl get pvc -n myapp --context primary -o wide

# Check PVC labels
kubectl get pvc -n myapp --context primary --show-labels
```

**Example output:**
```
NAME            STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE   LABELS
mysql-data      Bound    pvc-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx   10Gi       RWO            standard       5d    app=myapp,tier=database
postgres-data   Bound    pvc-yyyyyyyy-yyyy-yyyy-yyyy-yyyyyyyyyyyy   5Gi        RWO            standard       5d    app=myapp,tier=database
app-config      Bound    pvc-zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz   1Gi        RWO            standard       5d    app=myapp,tier=config
```

### Step 2: Check Available Resources on Secondary

Verify that the required storage classes and snapshot classes exist:

```bash
# Check storage classes
kubectl get storageclass --context secondary

# Check volume snapshot classes
kubectl get volumesnapshotclass --context secondary
```

### Step 3: Create the ConfigMap

Create a ConfigMap file based on your PVCs:

```bash
cat <<EOF > pvc-configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: pvc-config
  namespace: myapp
data:
  # Add entries for each PVC you want to replicate
  "pvc=mysql-data/myapp": "schedulingInterval=5m:storageClassName=standard:volumeSnapshotClassName=csi-snapclass"
  "pvc=postgres-data/myapp": "schedulingInterval=5m:storageClassName=standard:volumeSnapshotClassName=csi-snapclass"
  "pvc=app-config/myapp": "schedulingInterval=15m:storageClassName=standard:volumeSnapshotClassName=csi-snapclass"
EOF
```

### Step 4: Apply the ConfigMap to Secondary Cluster

```bash
kubectl apply -f pvc-configmap.yaml --context secondary
```

### Step 5: Verify the ConfigMap

```bash
# Check ConfigMap exists
kubectl get configmap pvc-config -n myapp --context secondary

# View ConfigMap content
kubectl get configmap pvc-config -n myapp --context secondary -o yaml

# Verify the data section
kubectl get configmap pvc-config -n myapp --context secondary -o jsonpath='{.data}' | jq
```

---

## Creating VGR Resources

### Step 1: Create VolumeGroupReplicationClass

Create the VGRClass on **both clusters**:

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
  provisioner: mock.storage.io
  parameters:
    capacity: "10Gi"
    pskSecretName: "volsync-rsync-tls-secret"
    pvcConfigMap: "pvc-config"  # Reference to the ConfigMap
EOF

# Apply to secondary as well
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
  provisioner: mock.storage.io
  parameters:
    capacity: "10Gi"
    pskSecretName: "volsync-rsync-tls-secret"
    pvcConfigMap: "pvc-config"
EOF
```

**Important VGRClass Parameters:**

- `capacity`: Default capacity for ReplicationDestinations (can be overridden)
- `pskSecretName`: Name of the PSK secret for rsync-tls authentication
- `pvcConfigMap`: **Required** - Name of the ConfigMap containing PVC configurations

### Step 2: Create Secondary VGR

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
- `source.selector`: Label selector (not used on secondary, but required by API)
- `autoResync`: Enable automatic resync on secondary

### Step 3: Wait for Secondary to be Ready

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
    - name: app-config
  observedGeneration: 1
```

### Step 4: Create Primary VGR

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

### Step 5: Wait for Primary to be Ready

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
      message: "3 ReplicationSource(s) active"
  persistentVolumeClaimsRefList:
    - name: mysql-data
    - name: postgres-data
    - name: app-config
  lastSyncTime: "2026-04-05T10:30:00Z"
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
1. Create the new PVC with appropriate labels
2. Wait for PVC to be bound

**On Secondary:**
1. Update the ConfigMap to include the new PVC
2. The operator will automatically detect the change and create a new ReplicationDestination

```bash
# Update ConfigMap
kubectl edit configmap pvc-config -n myapp --context secondary

# Add new entry:
# "pvc=new-pvc/myapp": "schedulingInterval=5m:storageClassName=standard:volumeSnapshotClassName=csi-snapclass"

# Verify operator picks up the change
kubectl logs -n mock-storage-operator-system -l app=mock-storage-operator --context secondary --tail=20
```

### Scenario 2: Changing Sync Interval

Update the ConfigMap on the secondary cluster:

```bash
# Edit ConfigMap
kubectl edit configmap pvc-config -n myapp --context secondary

# Change schedulingInterval value:
# "pvc=mysql-data/myapp": "schedulingInterval=3m:storageClassName=standard:volumeSnapshotClassName=csi-snapclass"

# The operator will reconcile and update the ReplicationDestination
```

### Scenario 3: Removing a PVC from Replication

**On Secondary:**
1. Remove the PVC entry from the ConfigMap
2. The operator will clean up the ReplicationDestination

```bash
# Edit ConfigMap
kubectl edit configmap pvc-config -n myapp --context secondary

# Remove the line for the PVC you want to stop replicating

# Verify ReplicationDestination is removed
kubectl get replicationdestinations -n myapp --context secondary
```

### Scenario 4: Multi-Namespace Setup

If you have PVCs in different namespaces, create separate VGRs and ConfigMaps for each namespace:

```bash
# Namespace 1: myapp
kubectl create configmap pvc-config -n myapp --context secondary --from-literal='pvc=data1/myapp'='schedulingInterval=5m:storageClassName=standard:volumeSnapshotClassName=csi-snapclass'

# Namespace 2: database
kubectl create configmap pvc-config -n database --context secondary --from-literal='pvc=data2/database'='schedulingInterval=5m:storageClassName=standard:volumeSnapshotClassName=csi-snapclass'

# Create VGR for each namespace
kubectl apply -f myapp-vgr.yaml --context secondary
kubectl apply -f database-vgr.yaml --context secondary
```

---

## Troubleshooting

### ConfigMap Issues

**Problem**: ConfigMap not found

```bash
# Check if ConfigMap exists
kubectl get configmap pvc-config -n myapp --context secondary

# If missing, create it
kubectl apply -f pvc-configmap.yaml --context secondary
```

**Problem**: Invalid ConfigMap format

```bash
# Verify format
kubectl get configmap pvc-config -n myapp --context secondary -o yaml

# Check operator logs for parsing errors
kubectl logs -n mock-storage-operator-system -l app=mock-storage-operator --context secondary | grep "invalid key format"
```

### PVC Issues

**Problem**: PVC not found in ConfigMap

```bash
# Check operator logs
kubectl logs -n mock-storage-operator-system -l app=mock-storage-operator --context secondary | grep "No PVC configurations"

# Verify ConfigMap has entries
kubectl get configmap pvc-config -n myapp --context secondary -o jsonpath='{.data}'
```

**Problem**: Namespace mismatch

```bash
# Check VGR namespace
kubectl get vgr myapp-vgr -o jsonpath='{.metadata.namespace}' --context secondary

# Ensure ConfigMap PVC entries use the same namespace
# Key format: "pvc=<name>/<namespace>" where namespace matches VGR namespace
```

### Replication Issues

**Problem**: ReplicationDestination not created

```bash
# Check operator logs
kubectl logs -n mock-storage-operator-system -l app=mock-storage-operator --context secondary --tail=100

# Verify storage class exists
kubectl get storageclass <storage-class-name> --context secondary

# Verify snapshot class exists
kubectl get volumesnapshotclass <snapshot-class-name> --context secondary
```

---

## Best Practices

1. **Always create secondary VGR first** - This ensures ReplicationDestinations are ready before ReplicationSources try to connect

2. **Use consistent naming** - Keep PVC names, namespaces, and ConfigMap names consistent across clusters

3. **Document your ConfigMap** - Add comments in the ConfigMap to explain each PVC's purpose

4. **Test with one PVC first** - Before replicating multiple PVCs, test with a single PVC to ensure everything works

5. **Monitor sync times** - Regularly check `lastSyncTime` in VGR status to ensure replication is working

6. **Use appropriate intervals** - Balance RPO requirements with network bandwidth and storage performance

7. **Keep ConfigMap in version control** - Store your ConfigMap YAML in git for easy tracking and rollback

8. **Validate before applying** - Use `kubectl apply --dry-run=client` to validate YAML before applying

---

## Quick Reference

### ConfigMap Template

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: pvc-config
  namespace: <namespace>
data:
  "pvc=<pvc-name>/<namespace>": "schedulingInterval=<interval>:storageClassName=<class>:volumeSnapshotClassName=<snapclass>"
```

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

---

**Document Version:** 1.0  
**Last Updated:** 2026-04-05  
**Operator Version:** latest (ConfigMap-based configuration)