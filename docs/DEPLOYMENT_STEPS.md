# Mock Storage Operator - Complete Deployment Steps

This document provides step-by-step instructions for deploying the Mock Storage Operator and setting up VolumeGroupReplication for disaster recovery testing.

## Table of Contents
1. [Environment Setup](#environment-setup)
2. [Prerequisites Installation](#prerequisites-installation)
3. [Operator Deployment](#operator-deployment)
4. [Configuration](#configuration)
5. [Testing Replication](#testing-replication)
6. [Verification](#verification)

---

## Environment Setup

### Required Infrastructure

You need two Kubernetes clusters:
- **Primary Cluster**: Source cluster where applications run
- **Secondary Cluster**: Destination cluster for DR

### Cluster Requirements

- Kubernetes 1.25+
- kubectl configured with contexts for both clusters
- Network connectivity between clusters (or Submariner for multi-cluster networking)
- Storage provisioner with snapshot support

### Verify Cluster Access

```bash
# List available contexts
kubectl config get-contexts

# Test primary cluster access
kubectl get nodes --context primary

# Test secondary cluster access
kubectl get nodes --context secondary
```

---

## Prerequisites Installation

### Step 1: Install VolumeGroupReplication CRDs

Install on **both clusters**:

```bash
# Method 1: Install all kubernetes-csi-addons CRDs (recommended)
kubectl apply -k "github.com/csi-addons/kubernetes-csi-addons/config/crd?ref=v0.14.0" --context primary
kubectl apply -k "github.com/csi-addons/kubernetes-csi-addons/config/crd?ref=v0.14.0" --context secondary

# Method 2: Install only VolumeGroupReplication CRDs
kubectl apply -f https://raw.githubusercontent.com/csi-addons/kubernetes-csi-addons/v0.14.0/config/crd/bases/replication.storage.openshift.io_volumegroupreplicationclasses.yaml --context primary
kubectl apply -f https://raw.githubusercontent.com/csi-addons/kubernetes-csi-addons/v0.14.0/config/crd/bases/replication.storage.openshift.io_volumegroupreplicationcontents.yaml --context primary
kubectl apply -f https://raw.githubusercontent.com/csi-addons/kubernetes-csi-addons/v0.14.0/config/crd/bases/replication.storage.openshift.io_volumegroupreplications.yaml --context primary

kubectl apply -f https://raw.githubusercontent.com/csi-addons/kubernetes-csi-addons/v0.14.0/config/crd/bases/replication.storage.openshift.io_volumegroupreplicationclasses.yaml --context secondary
kubectl apply -f https://raw.githubusercontent.com/csi-addons/kubernetes-csi-addons/v0.14.0/config/crd/bases/replication.storage.openshift.io_volumegroupreplicationcontents.yaml --context secondary
kubectl apply -f https://raw.githubusercontent.com/csi-addons/kubernetes-csi-addons/v0.14.0/config/crd/bases/replication.storage.openshift.io_volumegroupreplications.yaml --context secondary
```

**Verify installation:**

```bash
# Check CRDs on primary
kubectl get crd | grep replication.storage.openshift.io --context primary

# Check CRDs on secondary
kubectl get crd | grep replication.storage.openshift.io --context secondary
```

**Expected output:**
```
volumegroupreplicationclasses.replication.storage.openshift.io    2026-04-05T10:00:00Z
volumegroupreplicationcontents.replication.storage.openshift.io   2026-04-05T10:00:00Z
volumegroupreplications.replication.storage.openshift.io          2026-04-05T10:00:00Z
```

### Step 2: Install VolSync

Install on **both clusters** using Helm:

```bash
# Add VolSync Helm repository
helm repo add backube https://backube.github.io/helm-charts/
helm repo update

# Install VolSync on primary cluster
helm install volsync backube/volsync \
  -n volsync-system \
  --create-namespace \
  --kube-context primary

# Install VolSync on secondary cluster
helm install volsync backube/volsync \
  -n volsync-system \
  --create-namespace \
  --kube-context secondary
```

**Verify installation:**

```bash
# Check VolSync on primary
kubectl get pods -n volsync-system --context primary

# Check VolSync on secondary
kubectl get pods -n volsync-system --context secondary
```

**Expected output:**
```
NAME                       READY   STATUS    RESTARTS   AGE
volsync-7b8c9d5f4d-xxxxx   1/1     Running   0          1m
```

### Step 3: Install Submariner (Optional)

If using Submariner for multi-cluster networking, follow the [Submariner installation guide](https://submariner.io/getting-started/).

**Quick Submariner setup:**

```bash
# Install subctl CLI
curl -Ls https://get.submariner.io | bash

# Join clusters to broker
subctl join broker-info.subm --kubeconfig primary-kubeconfig
subctl join broker-info.subm --kubeconfig secondary-kubeconfig

# Verify connectivity
subctl show connections
```

---

## Operator Deployment

### Step 4: Deploy Mock Storage Operator

Deploy on **both clusters**:

```bash
# Deploy on primary cluster
kubectl apply -k https://github.com/BenamarMk/mock-storage-operator/config/default?ref=agnostic-storage --context primary

# Deploy on secondary cluster
kubectl apply -k https://github.com/BenamarMk/mock-storage-operator/config/default?ref=agnostic-storage --context secondary
```

**What this deploys:**
- Namespace: `mock-storage-operator-system`
- ServiceAccount: `mock-storage-operator-controller-manager`
- ClusterRole: `mock-storage-operator-manager-role`
- ClusterRoleBinding: `mock-storage-operator-manager-rolebinding`
- Deployment: `mock-storage-operator-controller-manager`

**Verify deployment:**

```bash
# Check operator on primary
kubectl get pods -n mock-storage-operator-system --context primary

# Check operator on secondary
kubectl get pods -n mock-storage-operator-system --context secondary
```

**Expected output:**
```
NAME                                                    READY   STATUS    RESTARTS   AGE
mock-storage-operator-controller-manager-xxxxxxxxxx-xxxxx   1/1     Running   0          30s
```

**Check operator logs:**

```bash
# Primary cluster logs
kubectl logs -n mock-storage-operator-system -l app=mock-storage-operator --context primary -f

# Secondary cluster logs
kubectl logs -n mock-storage-operator-system -l app=mock-storage-operator --context secondary -f
```

---

## Configuration

### Step 5: Create Application Namespace

Create namespace on **both clusters**:

```bash
# Create namespace on primary
kubectl create namespace myapp --context primary

# Create namespace on secondary
kubectl create namespace myapp --context secondary
```

### Step 6: Create PSK Secrets

Create Pre-Shared Key secrets for rsync-tls authentication on **both clusters**:

```bash
# Generate a random PSK (run once)
PSK=$(openssl rand -base64 48)

# Create secret on primary cluster
kubectl create secret generic volsync-rsync-tls-secret \
  --from-literal=psk.txt="$PSK" \
  -n myapp \
  --context primary

# Create the SAME secret on secondary cluster
kubectl create secret generic volsync-rsync-tls-secret \
  --from-literal=psk.txt="$PSK" \
  -n myapp \
  --context secondary
```

**Verify secrets:**

```bash
# Check secret on primary
kubectl get secret volsync-rsync-tls-secret -n myapp --context primary

# Check secret on secondary
kubectl get secret volsync-rsync-tls-secret -n myapp --context secondary

# Verify PSK content matches (optional)
kubectl get secret volsync-rsync-tls-secret -n myapp --context primary -o jsonpath='{.data.psk\.txt}' | base64 -d
kubectl get secret volsync-rsync-tls-secret -n myapp --context secondary -o jsonpath='{.data.psk\.txt}' | base64 -d
```

### Step 7: Create Application PVCs on Primary

Create PVCs on **primary cluster** with appropriate labels:

```bash
cat <<EOF | kubectl apply -f - --context primary
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: mysql-data
  namespace: myapp
  labels:
    app: myapp  # This label is used by VGR selector
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
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 5Gi
  storageClassName: standard
EOF
```

**Verify PVCs:**

```bash
# Check PVC status
kubectl get pvc -n myapp --context primary

# Verify labels
kubectl get pvc -n myapp --context primary --show-labels
```

**Expected output:**
```
NAME            STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE   LABELS
mysql-data      Bound    pvc-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx   10Gi       RWO            standard       1m    app=myapp
postgres-data   Bound    pvc-yyyyyyyy-yyyy-yyyy-yyyy-yyyyyyyyyyyy   5Gi        RWO            standard       1m    app=myapp
```

### Step 8: Create VolumeGroupReplicationClass

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
  provisioner: k8s.io/minikube-hostpath
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
cat <<EOF | kubectl apply -f - --context secondary
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
  provisioner: k8s.io/minikube-hostpath
  parameters:
    schedulingInterval: "5m"
    capacity: "10Gi"
    storageClassName: "standard"
    pskSecretName: "volsync-rsync-tls-secret"
    volumeSnapshotClassName: "csi-snapclass"
EOF
```

**Verify VGRClass:**

```bash
# Check on primary
kubectl get volumegroupreplicationclass mock-vgr-class --context primary

# Check on secondary
kubectl get volumegroupreplicationclass mock-vgr-class --context secondary
```

---

## Testing Replication

### Step 9: Deploy Secondary VGR

Deploy VGR on **secondary cluster first**:

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

**Monitor secondary VGR:**

```bash
# Watch VGR status
kubectl get vgr myapp-vgr -n myapp --context secondary -w

# Check detailed status
kubectl describe vgr myapp-vgr -n myapp --context secondary

# Check ReplicationDestinations
kubectl get replicationdestinations -n myapp --context secondary

# Check operator logs
kubectl logs -n mock-storage-operator-system -l app=mock-storage-operator --context secondary --tail=50
```

**Wait for Ready condition:**

```bash
kubectl wait --for=condition=Ready vgr/myapp-vgr -n myapp --context secondary --timeout=5m
```

**Expected log output:**
```
Found PVCs matching selector count=2
ReplicationDestination created for PVC mysql-data
ReplicationDestination created for PVC postgres-data
ReplicationDestination ready pvc=mysql-data address=mysql-data.myapp.svc.clusterset.local
ReplicationDestination ready pvc=postgres-data address=postgres-data.myapp.svc.clusterset.local
ServiceExport created for ReplicationDestination mysql-data
ServiceExport created for ReplicationDestination postgres-data
```

### Step 10: Deploy Primary VGR

Deploy VGR on **primary cluster**:

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

**Monitor primary VGR:**

```bash
# Watch VGR status
kubectl get vgr myapp-vgr -n myapp --context primary -w

# Check detailed status
kubectl describe vgr myapp-vgr -n myapp --context primary

# Check ReplicationSources
kubectl get replicationsources -n myapp --context primary

# Check operator logs
kubectl logs -n mock-storage-operator-system -l app=mock-storage-operator --context primary --tail=50
```

**Wait for Ready condition:**

```bash
kubectl wait --for=condition=Ready vgr/myapp-vgr -n myapp --context primary --timeout=5m
```

**Expected log output:**
```
ReplicationSource created for PVC mysql-data
ReplicationSource created for PVC postgres-data
ReplicationSource ready pvc=mysql-data
ReplicationSource ready pvc=postgres-data
Sync completed successfully for mysql-data
Sync completed successfully for postgres-data
```

---

## Verification

### Step 11: Verify Replication Status

**Check VGR status on both clusters:**

```bash
# Primary cluster
kubectl get vgr myapp-vgr -n myapp --context primary -o yaml

# Secondary cluster
kubectl get vgr myapp-vgr -n myapp --context secondary -o yaml
```

**Key status fields to check:**

```yaml
status:
  state: Primary  # or Secondary
  conditions:
    - type: Ready
      status: "True"
      reason: ReconcileComplete
  lastSyncTime: "2026-04-05T10:30:00Z"
  persistentVolumeClaimsRefList:
    - name: mysql-data
    - name: postgres-data
```

### Step 12: Verify VolSync Resources

**On primary cluster:**

```bash
# List ReplicationSources
kubectl get replicationsources -n myapp --context primary

# Check ReplicationSource details
kubectl get replicationsource <name> -n myapp --context primary -o yaml
```

**On secondary cluster:**

```bash
# List ReplicationDestinations
kubectl get replicationdestinations -n myapp --context secondary

# Check ReplicationDestination details
kubectl get replicationdestination <name> -n myapp --context secondary -o yaml
```

### Step 13: Monitor Sync Progress

**Check last sync time:**

```bash
# On primary
kubectl get vgr myapp-vgr -n myapp --context primary -o jsonpath='{.status.lastSyncTime}'

# Watch for updates
watch kubectl get vgr myapp-vgr -n myapp --context primary -o jsonpath='{.status.lastSyncTime}'
```

**Check ReplicationSource sync status:**

```bash
kubectl get replicationsource <name> -n myapp --context primary -o jsonpath='{.status.lastSyncTime}'
```

### Step 14: Test Data Replication

**Write test data on primary:**

```bash
# Create a test pod that writes to the PVC
cat <<EOF | kubectl apply -f - --context primary
apiVersion: v1
kind: Pod
metadata:
  name: test-writer
  namespace: myapp
spec:
  containers:
  - name: writer
    image: busybox
    command: ["/bin/sh", "-c"]
    args:
      - |
        echo "Test data at \$(date)" > /data/test.txt
        cat /data/test.txt
        sleep 3600
    volumeMounts:
    - name: data
      mountPath: /data
  volumes:
  - name: data
    persistentVolumeClaim:
      claimName: mysql-data
EOF
```

**Wait for sync to complete:**

```bash
# Wait for next sync cycle (based on schedulingInterval)
sleep 300  # Wait 5 minutes if schedulingInterval is 5m

# Check sync time updated
kubectl get vgr myapp-vgr -n myapp --context primary -o jsonpath='{.status.lastSyncTime}'
```

**Verify data on secondary:**

```bash
# Create a test pod on secondary to read the data
cat <<EOF | kubectl apply -f - --context secondary
apiVersion: v1
kind: Pod
metadata:
  name: test-reader
  namespace: myapp
spec:
  containers:
  - name: reader
    image: busybox
    command: ["/bin/sh", "-c"]
    args:
      - |
        if [ -f /data/test.txt ]; then
          echo "Data found:"
          cat /data/test.txt
        else
          echo "Data not found"
        fi
        sleep 3600
    volumeMounts:
    - name: data
      mountPath: /data
  volumes:
  - name: data
    persistentVolumeClaim:
      claimName: mysql-data
EOF

# Check the output
kubectl logs test-reader -n myapp --context secondary
```

**Expected output:**
```
Data found:
Test data at Fri Apr  5 10:30:00 UTC 2026
```

---

## Troubleshooting

### Common Issues and Solutions

#### Issue: VGR Not Becoming Ready

```bash
# Check VGR conditions
kubectl get vgr myapp-vgr -n myapp -o jsonpath='{.status.conditions[*]}' --context secondary

# Check operator logs
kubectl logs -n mock-storage-operator-system -l app=mock-storage-operator --context secondary --tail=100
```

#### Issue: No PVCs Found

```bash
# Error: No PVCs found matching selector
# Solution: Verify PVC labels match VGR selector

# Check VGR selector
kubectl get vgr myapp-vgr -n myapp -o jsonpath='{.spec.source.selector}' --context primary

# Check PVC labels
kubectl get pvc -n myapp --show-labels --context primary
```

#### Issue: Replication Not Syncing

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

## Cleanup

To remove the deployment:

```bash
# Delete VGRs
kubectl delete vgr myapp-vgr -n myapp --context primary
kubectl delete vgr myapp-vgr -n myapp --context secondary

# Delete VGRClass
kubectl delete volumegroupreplicationclass mock-vgr-class --context primary
kubectl delete volumegroupreplicationclass mock-vgr-class --context secondary

# Delete operator
kubectl delete -k https://github.com/BenamarMk/mock-storage-operator/config/default?ref=agnostic-storage --context primary
kubectl delete -k https://github.com/BenamarMk/mock-storage-operator/config/default?ref=agnostic-storage --context secondary

# Delete VolSync
helm uninstall volsync -n volsync-system --kube-context primary
helm uninstall volsync -n volsync-system --kube-context secondary

# Delete CRDs (optional)
kubectl delete -k "github.com/csi-addons/kubernetes-csi-addons/config/crd?ref=v0.14.0" --context primary
kubectl delete -k "github.com/csi-addons/kubernetes-csi-addons/config/crd?ref=v0.14.0" --context secondary
```

---

## Next Steps

- Review the [User Guide](USER_GUIDE.md) for detailed configuration options
- Check the [VGR Quick Reference](VGR_QUICK_REFERENCE.md) for VGR resource examples
- Explore [examples/](../examples/) for more YAML configurations
- Test failover scenarios by switching replication states

---

**Document Version:** 3.0  
**Last Updated:** 2026-04-22  
**Operator Version:** latest (Label selector-based configuration)