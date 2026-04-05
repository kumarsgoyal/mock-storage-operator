# Mock Storage Operator - User Guide

## Table of Contents
1. [Overview](#overview)
2. [Prerequisites](#prerequisites)
3. [Installation Steps](#installation-steps)
4. [Creating VolumeGroupReplication Resources](#creating-volumegroupreplication-resources)
5. [Parameter Configuration](#parameter-configuration)
6. [Deployment Scenarios](#deployment-scenarios)
7. [Monitoring and Troubleshooting](#monitoring-and-troubleshooting)
8. [Common Issues](#common-issues)

---

## Overview

The Mock Storage Operator simulates a storage vendor's VolumeGroupReplication implementation for DR testing with Ramen. It uses VolSync internally for actual data replication while presenting a storage-vendor-like interface.

**Key Features:**
- Implements VolumeGroupReplication API (`replication.storage.openshift.io/v1alpha1`)
- Per-PVC configuration for scheduling, storage classes, and snapshot classes
- Submariner support for multi-cluster service discovery
- Multi-architecture support (AMD64/ARM64)

---

## Prerequisites

Before deploying the Mock Storage Operator, ensure the following components are installed on **both clusters** (primary and secondary):

### 1. VolumeGroupReplication CRDs

Install the CRDs from kubernetes-csi-addons:

```bash
# Install all CRDs (recommended)
kubectl apply -k "github.com/csi-addons/kubernetes-csi-addons/config/crd?ref=v0.14.0"

# OR install only VolumeGroupReplication CRDs
kubectl apply -f https://raw.githubusercontent.com/csi-addons/kubernetes-csi-addons/v0.14.0/config/crd/bases/replication.storage.openshift.io_volumegroupreplicationclasses.yaml
kubectl apply -f https://raw.githubusercontent.com/csi-addons/kubernetes-csi-addons/v0.14.0/config/crd/bases/replication.storage.openshift.io_volumegroupreplicationcontents.yaml
kubectl apply -f https://raw.githubusercontent.com/csi-addons/kubernetes-csi-addons/v0.14.0/config/crd/bases/replication.storage.openshift.io_volumegroupreplications.yaml
```

**Verify installation:**
```bash
kubectl get crd | grep replication.storage.openshift.io
```

Expected output:
```
volumegroupreplicationclasses.replication.storage.openshift.io
volumegroupreplicationcontents.replication.storage.openshift.io
volumegroupreplications.replication.storage.openshift.io
```

### 2. VolSync

Install VolSync using Helm:

```bash
# Add VolSync Helm repository
helm repo add backube https://backube.github.io/helm-charts/
helm repo update

# Install VolSync
helm install volsync backube/volsync \
  -n volsync-system \
  --create-namespace
```

**Verify installation:**
```bash
kubectl get pods -n volsync-system
```

Expected output:
```
NAME                       READY   STATUS    RESTARTS   AGE
volsync-7b8c9d5f4d-xxxxx   1/1     Running   0          1m
```

### 3. Submariner (Optional but Recommended)

For multi-cluster networking, install Submariner. Follow the [Submariner installation guide](https://submariner.io/getting-started/).

**Note:** If not using Submariner, you'll need to manually configure service addresses for cross-cluster communication.

### 4. Storage Classes

Ensure appropriate storage classes are available on both clusters:

```bash
# List available storage classes
kubectl get storageclass
```

You'll need:
- A storage class for PVC provisioning (e.g., `rook-cephfs`, `standard`)
- A volume snapshot class for snapshots (e.g., `csi-cephfsplugin-snapclass`)

---

## Installation Steps

### Step 1: Deploy Mock Storage Operator

Deploy the operator on **both clusters** (primary and secondary):

```bash
# Deploy using Kustomize from GitHub
kubectl apply -k https://github.com/BenamarMk/mock-storage-operator/config/default?ref=main
```

**What this does:**
- Creates `mock-storage-operator-system` namespace
- Deploys ServiceAccount, ClusterRole, and ClusterRoleBinding
- Deploys the operator pod using `quay.io/bmekhiss/mock-storage-operator:latest`

**Verify deployment:**
```bash
# Check operator pod is running
kubectl get pods -n mock-storage-operator-system

# Expected output:
# NAME                                    READY   STATUS    RESTARTS   AGE
# mock-storage-operator-xxxxxxxxxx-xxxxx  1/1     Running   0          30s

# Check operator logs
kubectl logs -n mock-storage-operator-system -l app=mock-storage-operator -f
```

### Step 2: Create Pre-Shared Key (PSK) Secrets

Create PSK secrets for rsync-tls authentication on **both clusters**:

```bash
# Generate a random PSK
PSK=$(openssl rand -base64 48)

# Create secret on primary cluster
kubectl create secret generic volsync-rsync-tls-secret \
  --from-literal=psk.txt="$PSK" \
  -n <your-namespace>

# Create the same secret on secondary cluster
kubectl create secret generic volsync-rsync-tls-secret \
  --from-literal=psk.txt="$PSK" \
  -n <your-namespace>
```

**Important:** The PSK must be identical on both clusters for the same VGR.

### Step 3: Create VolumeGroupReplicationClass

Create the VGRClass on **both clusters**. This defines how the operator should handle replication.

Save the following as `vgrclass.yaml`:

```yaml
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
    # Default capacity for ReplicationDestinations
    capacity: "10Gi"
    
    # Optional: Custom PSK secret name
    # pskSecretName: "volsync-rsync-tls-secret"
    
    # Per-PVC Configuration
    # Format: pvc=<pvc-name>/<namespace>: "schedulingInterval=<value>:storageClassName=<value>:volumeSnapshotClassName=<value>"
    
    # Example: Configure PVC named "mysql-data" in "myapp" namespace
    pvc=mysql-data/myapp: "schedulingInterval=5m:storageClassName=rook-cephfs:volumeSnapshotClassName=csi-cephfsplugin-snapclass"
    
    # Example: Configure PVC named "postgres-data" in "myapp" namespace
    pvc=postgres-data/myapp: "schedulingInterval=10m:storageClassName=standard:volumeSnapshotClassName=csi-snapclass"
```

Apply on both clusters:
```bash
kubectl apply -f vgrclass.yaml
```

**Verify:**
```bash
kubectl get volumegroupreplicationclass
```

---

## Creating VolumeGroupReplication Resources

### Understanding VGR States

The VolumeGroupReplication resource has three possible states:

| State | Description | Cluster Role |
|-------|-------------|--------------|
| `primary` | Creates ReplicationSources that push data | Source cluster |
| `secondary` | Creates ReplicationDestinations that receive data | Destination cluster |
| `resync` | Not implemented in mock operator | N/A |

### Step 4: Deploy Secondary VGR

Deploy the VGR on the **secondary cluster first**. This creates ReplicationDestinations and exposes services.

Save as `secondary-vgr.yaml`:

```yaml
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeGroupReplication
metadata:
  name: myapp-vgr
  namespace: myapp
spec:
  # Set to secondary to create ReplicationDestinations
  replicationState: secondary
  
  # Reference to the VolumeGroupReplicationClass
  volumeGroupReplicationClassName: mock-vgr-class
  
  # Selector to find PVCs to protect
  source:
    selector:
      matchLabels:
        app: myapp
  
  # Auto-resync when in secondary mode
  autoResync: true
```

Apply on secondary cluster:
```bash
kubectl apply -f secondary-vgr.yaml --context secondary
```

**Monitor deployment:**
```bash
# Watch VGR status
kubectl get vgr myapp-vgr -n myapp --context secondary -w

# Check ReplicationDestinations
kubectl get replicationdestinations -n myapp --context secondary

# Check operator logs for service addresses
kubectl logs -n mock-storage-operator-system -l app=mock-storage-operator --context secondary
```

**Expected log output:**
```
ReplicationDestination ready pvc=mysql-data address=mysql-data-rd.myapp.svc.clusterset.local keySecret=volsync-rsync-tls-secret
```

### Step 5: Deploy Primary VGR

Deploy the VGR on the **primary cluster**. This creates ReplicationSources that connect to the secondary.

Save as `primary-vgr.yaml`:

```yaml
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeGroupReplication
metadata:
  name: myapp-vgr
  namespace: myapp
spec:
  # Set to primary to create ReplicationSources
  replicationState: primary
  
  # Reference to the VolumeGroupReplicationClass
  volumeGroupReplicationClassName: mock-vgr-class
  
  # Selector to find PVCs to replicate
  source:
    selector:
      matchLabels:
        app: myapp
```

Apply on primary cluster:
```bash
kubectl apply -f primary-vgr.yaml --context primary
```

**Monitor replication:**
```bash
# Watch VGR status
kubectl get vgr myapp-vgr -n myapp --context primary -w

# Check ReplicationSources
kubectl get replicationsources -n myapp --context primary

# Check sync status
kubectl get vgr myapp-vgr -n myapp --context primary -o jsonpath='{.status.lastSyncTime}'
```

---

## Parameter Configuration

### VGRClass Parameters Explained

#### Global Parameters

| Parameter | Description | Example | Required |
|-----------|-------------|---------|----------|
| `capacity` | Default capacity for ReplicationDestinations | `"10Gi"` | Yes |
| `pskSecretName` | Custom PSK secret name | `"my-secret"` | No |

#### Per-PVC Parameters

Format: `pvc=<pvc-name>/<namespace>: "key1=value1:key2=value2:key3=value3"`

| Key | Description | Example | Required |
|-----|-------------|---------|----------|
| `schedulingInterval` | Sync frequency (cron or duration) | `"5m"` or `"*/5 * * * *"` | Yes |
| `storageClassName` | Storage class for destination PVC | `"rook-cephfs"` | Yes |
| `volumeSnapshotClassName` | Snapshot class for source snapshots | `"csi-cephfsplugin-snapclass"` | Yes |

### Configuration Examples

#### Example 1: Single PVC with 5-minute sync

```yaml
parameters:
  capacity: "10Gi"
  pvc=mysql-data/myapp: "schedulingInterval=5m:storageClassName=rook-cephfs:volumeSnapshotClassName=csi-snapclass"
```

#### Example 2: Multiple PVCs with different schedules

```yaml
parameters:
  capacity: "10Gi"
  # Database - sync every 5 minutes
  pvc=mysql-data/myapp: "schedulingInterval=5m:storageClassName=fast-ssd:volumeSnapshotClassName=csi-snapclass"
  # Application data - sync every 15 minutes
  pvc=app-data/myapp: "schedulingInterval=15m:storageClassName=standard:volumeSnapshotClassName=csi-snapclass"
  # Logs - sync every hour
  pvc=logs/myapp: "schedulingInterval=1h:storageClassName=slow-hdd:volumeSnapshotClassName=csi-snapclass"
```

#### Example 3: Using cron expressions

```yaml
parameters:
  capacity: "10Gi"
  # Sync every 5 minutes
  pvc=data1/myapp: "schedulingInterval=*/5 * * * *:storageClassName=standard:volumeSnapshotClassName=csi-snapclass"
  # Sync every hour at minute 0
  pvc=data2/myapp: "schedulingInterval=0 * * * *:storageClassName=standard:volumeSnapshotClassName=csi-snapclass"
  # Sync daily at 2 AM
  pvc=data3/myapp: "schedulingInterval=0 2 * * *:storageClassName=standard:volumeSnapshotClassName=csi-snapclass"
```

---

## Deployment Scenarios

### Scenario 1: Simple Two-Cluster Setup

**Topology:**
- Primary cluster: `cluster1`
- Secondary cluster: `cluster2`
- Application namespace: `myapp`
- Single PVC: `mysql-data`

**Steps:**

1. **Install prerequisites on both clusters**
   ```bash
   # On both clusters
   kubectl apply -k "github.com/csi-addons/kubernetes-csi-addons/config/crd?ref=v0.14.0"
   helm install volsync backube/volsync -n volsync-system --create-namespace
   ```

2. **Deploy operator on both clusters**
   ```bash
   # On both clusters
   kubectl apply -k https://github.com/BenamarMk/mock-storage-operator/config/default?ref=main
   ```

3. **Create namespace and PSK secret on both clusters**
   ```bash
   # On both clusters
   kubectl create namespace myapp
   PSK=$(openssl rand -base64 48)
   kubectl create secret generic volsync-rsync-tls-secret \
     --from-literal=psk.txt="$PSK" -n myapp
   ```

4. **Create VGRClass on both clusters**
   ```bash
   cat <<EOF | kubectl apply -f -
   apiVersion: replication.storage.openshift.io/v1alpha1
   kind: VolumeGroupReplicationClass
   metadata:
     name: mock-vgr-class
   spec:
     provisioner: mock.storage.io
     parameters:
       capacity: "10Gi"
       pvc=mysql-data/myapp: "schedulingInterval=5m:storageClassName=standard:volumeSnapshotClassName=csi-snapclass"
   EOF
   ```

5. **Create PVC on primary cluster**
   ```bash
   cat <<EOF | kubectl apply -f - --context cluster1
   apiVersion: v1
   kind: PersistentVolumeClaim
   metadata:
     name: mysql-data
     namespace: myapp
     labels:
       app: myapp
   spec:
     accessModes:
       - ReadWriteOnce
     resources:
       requests:
         storage: 10Gi
     storageClassName: standard
   EOF
   ```

6. **Deploy secondary VGR**
   ```bash
   cat <<EOF | kubectl apply -f - --context cluster2
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

7. **Wait for secondary to be ready**
   ```bash
   kubectl wait --for=condition=Ready vgr/myapp-vgr -n myapp --context cluster2 --timeout=5m
   ```

8. **Deploy primary VGR**
   ```bash
   cat <<EOF | kubectl apply -f - --context cluster1
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

9. **Verify replication**
   ```bash
   # Check primary status
   kubectl get vgr myapp-vgr -n myapp --context cluster1 -o yaml
   
   # Check secondary status
   kubectl get vgr myapp-vgr -n myapp --context cluster2 -o yaml
   ```

### Scenario 2: Multi-PVC Application

**Topology:**
- Application with 3 PVCs: `mysql-data`, `app-config`, `logs`
- Different sync schedules for each PVC

**VGRClass configuration:**

```yaml
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeGroupReplicationClass
metadata:
  name: mock-vgr-class
spec:
  provisioner: mock.storage.io
  parameters:
    capacity: "10Gi"
    # Database - critical, sync every 5 minutes
    pvc=mysql-data/myapp: "schedulingInterval=5m:storageClassName=fast-ssd:volumeSnapshotClassName=csi-snapclass"
    # Config - moderate, sync every 15 minutes
    pvc=app-config/myapp: "schedulingInterval=15m:storageClassName=standard:volumeSnapshotClassName=csi-snapclass"
    # Logs - low priority, sync hourly
    pvc=logs/myapp: "schedulingInterval=1h:storageClassName=slow-hdd:volumeSnapshotClassName=csi-snapclass"
```

Follow the same deployment steps as Scenario 1, but ensure all PVCs have the `app: myapp` label.

---

## Monitoring and Troubleshooting

### Checking VGR Status

```bash
# Get VGR status
kubectl get vgr <vgr-name> -n <namespace> -o yaml

# Check conditions
kubectl get vgr <vgr-name> -n <namespace> -o jsonpath='{.status.conditions[*]}'

# Check last sync time
kubectl get vgr <vgr-name> -n <namespace> -o jsonpath='{.status.lastSyncTime}'

# Check replicated PVCs
kubectl get vgr <vgr-name> -n <namespace> -o jsonpath='{.status.persistentVolumeClaimsRefList[*].name}'
```

### Checking VolSync Resources

```bash
# List ReplicationSources (primary)
kubectl get replicationsources -n <namespace>

# List ReplicationDestinations (secondary)
kubectl get replicationdestinations -n <namespace>

# Check ReplicationSource status
kubectl get replicationsource <name> -n <namespace> -o yaml

# Check ReplicationDestination status
kubectl get replicationdestination <name> -n <namespace> -o yaml
```

### Checking Operator Logs

```bash
# View operator logs
kubectl logs -n mock-storage-operator-system -l app=mock-storage-operator -f

# Search for specific PVC
kubectl logs -n mock-storage-operator-system -l app=mock-storage-operator | grep "mysql-data"

# Check for errors
kubectl logs -n mock-storage-operator-system -l app=mock-storage-operator | grep -i error
```

### Checking ServiceExport (Submariner)

```bash
# List ServiceExports
kubectl get serviceexports -n <namespace>

# Check ServiceExport details
kubectl get serviceexport <name> -n <namespace> -o yaml
```

---

## Common Issues

### Issue 1: VGR Not Becoming Ready

**Symptoms:**
- VGR status shows `Ready=False`
- Condition message: "VolumeGroupReplicationClass not found"

**Solution:**
```bash
# Verify VGRClass exists
kubectl get volumegroupreplicationclass

# Check VGRClass name matches VGR spec
kubectl get vgr <name> -n <namespace> -o jsonpath='{.spec.volumeGroupReplicationClassName}'
```

### Issue 2: No PVCs Found

**Symptoms:**
- VGR status shows empty `persistentVolumeClaimsRefList`
- Operator logs: "No PVCs found matching selector"

**Solution:**
```bash
# Check PVC labels
kubectl get pvc -n <namespace> --show-labels

# Verify selector matches PVC labels
kubectl get vgr <name> -n <namespace> -o jsonpath='{.spec.source.selector}'

# Add missing labels to PVCs
kubectl label pvc <pvc-name> -n <namespace> app=myapp
```

### Issue 3: ReplicationSource Not Created

**Symptoms:**
- No ReplicationSources on primary cluster
- Operator logs: "Failed to parse PVC parameters"

**Solution:**
```bash
# Check VGRClass parameters format
kubectl get volumegroupreplicationclass mock-vgr-class -o yaml

# Verify parameter format:
# pvc=<name>/<namespace>: "schedulingInterval=<value>:storageClassName=<value>:volumeSnapshotClassName=<value>"

# Fix parameter format if incorrect
kubectl edit volumegroupreplicationclass mock-vgr-class
```

### Issue 4: Replication Not Syncing

**Symptoms:**
- ReplicationSource shows `lastSyncTime` not updating
- Operator logs: "Failed to connect to remote service"

**Solution:**
```bash
# Check PSK secret exists on both clusters
kubectl get secret volsync-rsync-tls-secret -n <namespace>

# Verify PSK secret content matches
kubectl get secret volsync-rsync-tls-secret -n <namespace> -o jsonpath='{.data.psk\.txt}' | base64 -d

# Check Submariner connectivity (if using)
subctl show connections

# Check ReplicationDestination service
kubectl get svc -n <namespace> | grep rd
```

### Issue 5: Operator Pod CrashLooping

**Symptoms:**
- Operator pod status: `CrashLoopBackOff`
- Operator logs show errors

**Solution:**
```bash
# Check operator logs
kubectl logs -n mock-storage-operator-system -l app=mock-storage-operator --previous

# Verify CRDs are installed
kubectl get crd | grep replication.storage.openshift.io

# Reinstall CRDs if missing
kubectl apply -k "github.com/csi-addons/kubernetes-csi-addons/config/crd?ref=v0.14.0"

# Restart operator
kubectl rollout restart deployment -n mock-storage-operator-system
```

### Issue 6: Permission Denied Errors

**Symptoms:**
- Operator logs: "forbidden: User cannot create resource"

**Solution:**
```bash
# Check RBAC resources exist
kubectl get clusterrole mock-storage-operator-manager-role
kubectl get clusterrolebinding mock-storage-operator-manager-rolebinding

# Verify ServiceAccount
kubectl get sa -n mock-storage-operator-system

# Reapply RBAC if missing
kubectl apply -k https://github.com/BenamarMk/mock-storage-operator/config/rbac?ref=main
```

---

## Best Practices

1. **Always deploy secondary before primary** - This ensures ReplicationDestinations are ready before ReplicationSources try to connect.

2. **Use consistent PSK secrets** - The same PSK must exist on both clusters for each VGR.

3. **Label PVCs appropriately** - Ensure all PVCs you want to replicate have matching labels for the VGR selector.

4. **Monitor sync times** - Regularly check `lastSyncTime` in VGR status to ensure replication is working.

5. **Use appropriate scheduling intervals** - Balance RPO requirements with network bandwidth and storage performance.

6. **Test failover procedures** - Regularly test switching from primary to secondary to ensure DR readiness.

7. **Keep operator updated** - Pull the latest image from Quay.io for bug fixes and improvements.

8. **Use Submariner for production** - Manual service address configuration is error-prone and not recommended for production.

---

## Support and Resources

- **GitHub Repository**: https://github.com/BenamarMk/mock-storage-operator
- **Container Registry**: https://quay.io/repository/bmekhiss/mock-storage-operator
- **VolSync Documentation**: https://volsync.readthedocs.io/
- **Submariner Documentation**: https://submariner.io/
- **kubernetes-csi-addons**: https://github.com/csi-addons/kubernetes-csi-addons

---

## Appendix: Complete Example

Here's a complete working example for a simple application:

### 1. Prerequisites Installation

```bash
# On both clusters
kubectl apply -k "github.com/csi-addons/kubernetes-csi-addons/config/crd?ref=v0.14.0"
helm install volsync backube/volsync -n volsync-system --create-namespace
kubectl apply -k https://github.com/BenamarMk/mock-storage-operator/config/default?ref=main
```

### 2. Create Namespace and Secrets

```bash
# On both clusters
kubectl create namespace demo-app
PSK=$(openssl rand -base64 48)
kubectl create secret generic volsync-rsync-tls-secret \
  --from-literal=psk.txt="$PSK" -n demo-app
```

### 3. Create VGRClass

```bash
cat <<EOF | kubectl apply -f -
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeGroupReplicationClass
metadata:
  name: demo-vgr-class
spec:
  provisioner: mock.storage.io
  parameters:
    capacity: "5Gi"
    pvc=demo-data/demo-app: "schedulingInterval=3m:storageClassName=standard:volumeSnapshotClassName=csi-snapclass"
EOF
```

### 4. Create Application PVC (Primary)

```bash
cat <<EOF | kubectl apply -f - --context primary
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: demo-data
  namespace: demo-app
  labels:
    app: demo
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 5Gi
  storageClassName: standard
EOF
```

### 5. Deploy Secondary VGR

```bash
cat <<EOF | kubectl apply -f - --context secondary
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeGroupReplication
metadata:
  name: demo-vgr
  namespace: demo-app
spec:
  replicationState: secondary
  volumeGroupReplicationClassName: demo-vgr-class
  source:
    selector:
      matchLabels:
        app: demo
  autoResync: true
EOF
```

### 6. Deploy Primary VGR

```bash
cat <<EOF | kubectl apply -f - --context primary
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeGroupReplication
metadata:
  name: demo-vgr
  namespace: demo-app
spec:
  replicationState: primary
  volumeGroupReplicationClassName: demo-vgr-class
  source:
    selector:
      matchLabels:
        app: demo
EOF
```

### 7. Verify Replication

```bash
# Check primary
kubectl get vgr demo-vgr -n demo-app --context primary -o yaml

# Check secondary
kubectl get vgr demo-vgr -n demo-app --context secondary -o yaml

# Monitor sync
watch kubectl get vgr demo-vgr -n demo-app --context primary -o jsonpath='{.status.lastSyncTime}'
```

---

**Document Version:** 1.0  
**Last Updated:** 2026-04-05  
**Operator Version:** latest