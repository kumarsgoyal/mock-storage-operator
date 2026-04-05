# VolumeGroupReplication Quick Reference Guide

## Quick Start Checklist

- [ ] Prerequisites installed (CRDs, VolSync, Operator)
- [ ] PSK secrets created on both clusters
- [ ] VolumeGroupReplicationClass created
- [ ] PVCs labeled correctly
- [ ] Secondary VGR deployed and Ready
- [ ] Primary VGR deployed

---

## VolumeGroupReplication Resource Structure

### Basic Template

```yaml
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeGroupReplication
metadata:
  name: <vgr-name>
  namespace: <namespace>
spec:
  replicationState: <primary|secondary|resync>
  volumeGroupReplicationClassName: <vgrclass-name>
  source:
    selector:
      matchLabels:
        <label-key>: <label-value>
  autoResync: <true|false>  # Optional, for secondary only
```

---

## Field Descriptions

### Required Fields

| Field | Description | Values | Example |
|-------|-------------|--------|---------|
| `metadata.name` | Name of the VGR resource | Any valid K8s name | `myapp-vgr` |
| `metadata.namespace` | Namespace where VGR lives | Existing namespace | `myapp` |
| `spec.replicationState` | Role of this cluster | `primary`, `secondary`, `resync` | `primary` |
| `spec.volumeGroupReplicationClassName` | Reference to VGRClass | Existing VGRClass name | `mock-vgr-class` |
| `spec.source.selector` | Label selector for PVCs | K8s label selector | See below |

### Optional Fields

| Field | Description | Default | When to Use |
|-------|-------------|---------|-------------|
| `spec.autoResync` | Auto-resync on secondary | `false` | Set to `true` on secondary for automatic recovery |

---

## Label Selectors

### Simple Label Match

Match PVCs with a single label:

```yaml
source:
  selector:
    matchLabels:
      app: myapp
```

Matches PVCs with label: `app=myapp`

### Multiple Labels (AND logic)

Match PVCs with multiple labels (all must match):

```yaml
source:
  selector:
    matchLabels:
      app: myapp
      tier: database
      environment: production
```

Matches PVCs with all three labels.

### Expression-Based Matching

More complex matching using expressions:

```yaml
source:
  selector:
    matchExpressions:
      - key: app
        operator: In
        values:
          - myapp
          - yourapp
      - key: tier
        operator: NotIn
        values:
          - cache
```

**Operators:**
- `In`: Label value must be in the list
- `NotIn`: Label value must not be in the list
- `Exists`: Label key must exist (any value)
- `DoesNotExist`: Label key must not exist

---

## Complete Examples

### Example 1: Simple Single-App Replication

**Scenario:** Replicate all PVCs for "myapp" application

**Primary VGR:**
```yaml
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
```

**Secondary VGR:**
```yaml
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
```

**PVC Labels:**
```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: mysql-data
  namespace: myapp
  labels:
    app: myapp  # Must match selector
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 10Gi
```

### Example 2: Multi-Tier Application

**Scenario:** Replicate only database tier PVCs

**VGR:**
```yaml
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeGroupReplication
metadata:
  name: database-vgr
  namespace: myapp
spec:
  replicationState: primary
  volumeGroupReplicationClassName: mock-vgr-class
  source:
    selector:
      matchLabels:
        app: myapp
        tier: database
```

**PVC Labels:**
```yaml
# This PVC will be replicated
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: mysql-data
  namespace: myapp
  labels:
    app: myapp
    tier: database  # Matches selector
spec:
  # ... spec details

---
# This PVC will NOT be replicated
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: cache-data
  namespace: myapp
  labels:
    app: myapp
    tier: cache  # Does not match selector
spec:
  # ... spec details
```

### Example 3: Environment-Specific Replication

**Scenario:** Replicate only production PVCs

**VGR:**
```yaml
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeGroupReplication
metadata:
  name: prod-vgr
  namespace: myapp
spec:
  replicationState: primary
  volumeGroupReplicationClassName: mock-vgr-class
  source:
    selector:
      matchLabels:
        app: myapp
        environment: production
```

### Example 4: Multiple Applications

**Scenario:** Replicate PVCs from multiple related applications

**VGR:**
```yaml
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeGroupReplication
metadata:
  name: suite-vgr
  namespace: myapp
spec:
  replicationState: primary
  volumeGroupReplicationClassName: mock-vgr-class
  source:
    selector:
      matchExpressions:
        - key: app
          operator: In
          values:
            - frontend
            - backend
            - database
```

### Example 5: Exclude Specific PVCs

**Scenario:** Replicate all PVCs except cache

**VGR:**
```yaml
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
      matchExpressions:
        - key: app
          operator: In
          values:
            - myapp
        - key: tier
          operator: NotIn
          values:
            - cache
            - temp
```

---

## Status Fields Reference

After creating a VGR, check its status:

```bash
kubectl get vgr <name> -n <namespace> -o yaml
```

### Important Status Fields

| Field | Description | Example Value |
|-------|-------------|---------------|
| `status.state` | Current replication state | `Primary`, `Secondary`, `Unknown` |
| `status.conditions` | Array of condition objects | See below |
| `status.lastSyncTime` | Last successful sync timestamp | `2026-04-05T10:30:00Z` |
| `status.observedGeneration` | Generation of spec that produced this status | `1` |
| `status.persistentVolumeClaimsRefList` | List of PVCs being replicated | Array of PVC references |

### Condition Types

```yaml
status:
  conditions:
    - type: Ready
      status: "True"  # or "False"
      reason: "ReconcileComplete"
      message: "VolumeGroupReplication is ready"
      lastTransitionTime: "2026-04-05T10:30:00Z"
```

**Condition Status Values:**
- `True`: VGR is ready and working
- `False`: VGR has issues (check reason and message)
- `Unknown`: Status cannot be determined

---

## Common Patterns

### Pattern 1: Label All PVCs for an Application

```bash
# Label existing PVCs
kubectl label pvc <pvc-name> -n <namespace> app=myapp

# Label multiple PVCs at once
kubectl label pvc -n <namespace> -l app=myapp tier=database

# Verify labels
kubectl get pvc -n <namespace> --show-labels
```

### Pattern 2: Test Selector Before Creating VGR

```bash
# List PVCs that match your selector
kubectl get pvc -n <namespace> -l app=myapp

# Count matching PVCs
kubectl get pvc -n <namespace> -l app=myapp --no-headers | wc -l
```

### Pattern 3: Update VGR Selector

```bash
# Edit VGR to change selector
kubectl edit vgr <name> -n <namespace>

# Or patch it
kubectl patch vgr <name> -n <namespace> --type=merge -p '
spec:
  source:
    selector:
      matchLabels:
        app: myapp
        tier: database
'
```

### Pattern 4: Switch Replication State

```bash
# Promote secondary to primary
kubectl patch vgr <name> -n <namespace> --type=merge -p '
spec:
  replicationState: primary
'

# Demote primary to secondary
kubectl patch vgr <name> -n <namespace> --type=merge -p '
spec:
  replicationState: secondary
  autoResync: true
'
```

---

## Validation Checklist

Before creating a VGR, verify:

### Prerequisites
- [ ] VolumeGroupReplicationClass exists
  ```bash
  kubectl get volumegroupreplicationclass <class-name>
  ```

- [ ] PSK secret exists in the namespace
  ```bash
  kubectl get secret volsync-rsync-tls-secret -n <namespace>
  ```

- [ ] PVCs exist and are labeled correctly
  ```bash
  kubectl get pvc -n <namespace> --show-labels
  ```

### VGR Configuration
- [ ] `replicationState` is set correctly (`primary` or `secondary`)
- [ ] `volumeGroupReplicationClassName` matches existing VGRClass
- [ ] `source.selector` matches at least one PVC
- [ ] Namespace matches where PVCs exist

### Post-Creation
- [ ] VGR status shows `Ready=True`
  ```bash
  kubectl get vgr <name> -n <namespace> -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}'
  ```

- [ ] PVCs are listed in status
  ```bash
  kubectl get vgr <name> -n <namespace> -o jsonpath='{.status.persistentVolumeClaimsRefList[*].name}'
  ```

- [ ] VolSync resources created
  ```bash
  # On primary
  kubectl get replicationsources -n <namespace>
  
  # On secondary
  kubectl get replicationdestinations -n <namespace>
  ```

---

## Troubleshooting Quick Commands

```bash
# Check VGR status
kubectl get vgr -n <namespace>

# Get detailed VGR info
kubectl describe vgr <name> -n <namespace>

# Check VGR conditions
kubectl get vgr <name> -n <namespace> -o jsonpath='{.status.conditions[*]}'

# List replicated PVCs
kubectl get vgr <name> -n <namespace> -o jsonpath='{.status.persistentVolumeClaimsRefList[*].name}'

# Check last sync time
kubectl get vgr <name> -n <namespace> -o jsonpath='{.status.lastSyncTime}'

# View operator logs
kubectl logs -n mock-storage-operator-system -l app=mock-storage-operator -f

# Check VolSync resources
kubectl get replicationsources,replicationdestinations -n <namespace>

# Verify PVC labels match selector
kubectl get pvc -n <namespace> -l <your-selector> --show-labels
```

---

## Quick Deployment Script

Save as `deploy-vgr.sh`:

```bash
#!/bin/bash

# Configuration
NAMESPACE="myapp"
VGR_NAME="myapp-vgr"
VGRCLASS_NAME="mock-vgr-class"
APP_LABEL="myapp"
REPLICATION_STATE="${1:-secondary}"  # primary or secondary

# Validate input
if [[ "$REPLICATION_STATE" != "primary" && "$REPLICATION_STATE" != "secondary" ]]; then
  echo "Usage: $0 [primary|secondary]"
  exit 1
fi

# Create VGR
cat <<EOF | kubectl apply -f -
apiVersion: replication.storage.openshift.io/v1alpha1
kind: VolumeGroupReplication
metadata:
  name: ${VGR_NAME}
  namespace: ${NAMESPACE}
spec:
  replicationState: ${REPLICATION_STATE}
  volumeGroupReplicationClassName: ${VGRCLASS_NAME}
  source:
    selector:
      matchLabels:
        app: ${APP_LABEL}
  $([ "$REPLICATION_STATE" = "secondary" ] && echo "autoResync: true")
EOF

# Wait for VGR to be ready
echo "Waiting for VGR to be ready..."
kubectl wait --for=condition=Ready vgr/${VGR_NAME} -n ${NAMESPACE} --timeout=5m

# Show status
echo "VGR Status:"
kubectl get vgr ${VGR_NAME} -n ${NAMESPACE}

echo "Replicated PVCs:"
kubectl get vgr ${VGR_NAME} -n ${NAMESPACE} -o jsonpath='{.status.persistentVolumeClaimsRefList[*].name}'
echo ""
```

**Usage:**
```bash
# Deploy secondary
./deploy-vgr.sh secondary

# Deploy primary
./deploy-vgr.sh primary
```

---

## Reference Links

- **Full User Guide**: [USER_GUIDE.md](USER_GUIDE.md)
- **GitHub Repository**: https://github.com/BenamarMk/mock-storage-operator
- **Example YAMLs**: [../examples/](../examples/)

---

**Document Version:** 1.0  
**Last Updated:** 2026-04-05