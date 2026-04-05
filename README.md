# mock-storage-operator

A Kubernetes operator that acts as a **mock storage vendor** implementing the VolumeGroupReplication API for DR testing with Ramen. It uses VolSync internally for actual data replication while presenting a storage-vendor-like interface to Ramen.

## Purpose

This operator allows Ramen to test its agnostic DR solution without requiring actual storage vendor hardware. It reconciles `VolumeGroupReplication` CRs (from replication.storage.io API) and uses VolSync ReplicationSource/ReplicationDestination resources internally to perform the actual data replication.

## How it works

```
RAMEN (DR Orchestrator)
  |
  | Creates VolumeGroupReplication CR
  |
  v
MOCK STORAGE OPERATOR (provisioner: mock.storage.io)
  |
  | Reconciles VGR based on replicationState
  |
  +-- PRIMARY (replicationState: primary)
  |     |
  |     +--> Creates VolSync ReplicationSource per PVC
  |     +--> Pushes data to secondary via rsync-tls
  |
  +-- SECONDARY (replicationState: secondary)
        |
        +--> Creates VolSync ReplicationDestination per PVC
        +--> Exposes service addresses for primary to connect
```

## Prerequisites

1. **VolSync** must be installed on both clusters:
```bash
helm repo add backube https://backube.github.io/helm-charts/
helm install volsync backube/volsync -n volsync-system --create-namespace
```

2. **VolumeGroupReplication CRDs** from kubernetes-csi-addons must be installed:
```bash
# Install all CRDs from kubernetes-csi-addons v0.14.0
kubectl apply -k "github.com/csi-addons/kubernetes-csi-addons/config/crd?ref=v0.14.0"

# Or install only the VolumeGroupReplication CRDs:
kubectl apply -f https://raw.githubusercontent.com/csi-addons/kubernetes-csi-addons/v0.14.0/config/crd/bases/replication.storage.openshift.io_volumegroupreplicationclasses.yaml
kubectl apply -f https://raw.githubusercontent.com/csi-addons/kubernetes-csi-addons/v0.14.0/config/crd/bases/replication.storage.openshift.io_volumegroupreplicationcontents.yaml
kubectl apply -f https://raw.githubusercontent.com/csi-addons/kubernetes-csi-addons/v0.14.0/config/crd/bases/replication.storage.openshift.io_volumegroupreplications.yaml
```

**Note**: This operator uses the VolumeGroupReplication API (`replication.storage.openshift.io/v1alpha1`) from the [kubernetes-csi-addons](https://github.com/csi-addons/kubernetes-csi-addons) project. It does not define its own CRDs.

## Installation

### Option 1: Deploy from Quay.io using Kustomize (Recommended for OpenShift)

The operator is available as a container image on Quay.io and can be deployed directly from GitHub using Kustomize:

```bash
# Deploy everything with one command
kubectl apply -k https://github.com/BenamarMk/mock-storage-operator/config/default?ref=main
```

This will:
- Create the `mock-storage-operator-system` namespace
- Deploy all RBAC resources (ServiceAccount, ClusterRole, ClusterRoleBinding)
- Deploy the operator using `quay.io/bmekhiss/mock-storage-operator:latest`

**Or deploy components separately:**
```bash
# Deploy only RBAC
kubectl apply -k https://github.com/BenamarMk/mock-storage-operator/config/rbac?ref=main

# Deploy only manager
kubectl apply -k https://github.com/BenamarMk/mock-storage-operator/config/manager?ref=main
```

### Option 2: Build and Push to Quay.io

If you want to build and push your own version:

```bash
# Login to Quay.io
podman login quay.io

# Build and push (will tag both VERSION and latest)
make quay-push VERSION=v0.1.0

# Or just build without pushing
make quay-build VERSION=v0.1.0
```

### Option 3: Local Development

```bash
# Build locally
make build

# Run locally (requires kubeconfig)
make run

# Or build container image for local testing
make docker-build IMG=localhost/mock-storage-operator:dev
```

### Option 4: Deploy to Minikube

```bash
# Build and load into Minikube
make docker-build IMG=mock-storage-operator:latest
make minikube-load MINIKUBE_PROFILE=dr1

# Deploy using Kustomize
kubectl apply -k config/default
```

## Setup Order

### 1. Create VolumeGroupReplicationClass

First, create the VolumeGroupReplicationClass that defines the mock provisioner:

```bash
kubectl apply -f examples/volumegroupreplicationclass.yaml
```

This class specifies:
- `provisioner: mock.storage.io` - tells the operator to handle VGRs using this class
- Storage parameters (capacity, storageClassName, schedule, etc.)
- Remote destination addresses (filled in after secondary setup)

### 2. Deploy on Secondary Cluster

```bash
kubectl apply -f examples/secondary-vgr.yaml --context secondary
```

Wait for the VGR to become Ready:
```bash
kubectl get vgr myapp-vgr -n myapp --context secondary -w
```

Check the operator logs for ReplicationDestination addresses:
```bash
kubectl logs -n mock-storage-operator-system -l control-plane=controller-manager --context secondary
```

You'll see log messages like:
```
ReplicationDestination ready pvc=mysql-data address=192.168.1.100 keySecret=volsync-rsync-tls-dst-mockdr-mysql-data
```

### 3. Copy Secrets and Update VGRClass

Copy the rsync-tls key secrets from secondary to primary:
```bash
kubectl get secret volsync-rsync-tls-dst-mockdr-mysql-data -n myapp --context secondary -o yaml \
  | kubectl apply --context primary -f -
```

Update the VolumeGroupReplicationClass with the remote addresses:
```yaml
apiVersion: replication.storage.io/v1alpha1
kind: VolumeGroupReplicationClass
metadata:
  name: mock-vgr-class
spec:
  provisioner: mock.storage.io
  parameters:
    schedule: "*/5 * * * *"
    capacity: "10Gi"
    storageClassName: "standard"
    serviceType: "LoadBalancer"
    pvc-mysql-data: "true"
    # Add these after secondary is ready:
    mock.storage.io/remote-address-mysql-data: "192.168.1.100"
    mock.storage.io/remote-key-secret-mysql-data: "volsync-rsync-tls-dst-mockdr-mysql-data"
```

### 4. Deploy on Primary Cluster

```bash
kubectl apply -f examples/primary-vgr.yaml --context primary
```

Verify replication is working:
```bash
# Check VGR status
kubectl get vgr myapp-vgr -n myapp --context primary -o yaml

# Check ReplicationSources
kubectl get replicationsources -n myapp --context primary

# Check sync status
kubectl get vgr myapp-vgr -n myapp --context primary -o jsonpath='{.status.lastSyncTime}'
```

## VolumeGroupReplication States

The operator handles three replication states:

| State | Behavior |
|-------|----------|
| `primary` | Creates VolSync ReplicationSources, pushes data to secondary |
| `secondary` | Creates VolSync ReplicationDestinations, receives data from primary |
| `resync` | Not implemented in this mock (no-op) |

## Status Fields

The operator updates the VGR status with:

- `state`: Current replication state (Primary/Secondary/Unknown)
- `persistentVolumeClaimsRefList`: List of PVCs being replicated
- `lastSyncTime`: Time of last successful sync
- `observedGeneration`: Generation of spec that produced this status
- `conditions`: Ready condition indicating if setup is complete

## Integration with Ramen

Ramen will:
1. Create VolumeGroupReplication CRs with the appropriate `replicationState`
2. Monitor VGR status to determine replication health
3. Use VGR to orchestrate failover/failback operations

The mock operator simulates a storage vendor's behavior, allowing Ramen to test its DR workflows without actual storage hardware.

## Differences from Real Storage Vendors

- Uses VolSync for data movement instead of storage array replication
- Requires manual setup of remote addresses (real vendors handle this automatically)
- No support for `resync` operation
- Simpler status reporting

## Development

```bash
# Run tests
go test ./...

# Build
go build ./...

# Run locally (requires kubeconfig)
go run ./cmd/main.go
```

## Architecture

```
VolumeGroupReplication CR
  ├── Spec
  │   ├── replicationState: primary|secondary|resync
  │   ├── volumeGroupReplicationClassName: mock-vgr-class
  │   └── source.selector: matchLabels
  │
  └── Status
      ├── state: Primary|Secondary|Unknown
      ├── persistentVolumeClaimsRefList: [...]
      ├── lastSyncTime: <timestamp>
      └── conditions: [Ready]

VolumeGroupReplicationClass
  ├── Spec
  │   ├── provisioner: mock.storage.io
  │   └── parameters:
  │       ├── schedule: "*/5 * * * *"
  │       ├── capacity: "10Gi"
  │       ├── storageClassName: "standard"
  │       ├── mock.storage.io/remote-address-<pvc>: <address>
  │       └── mock.storage.io/remote-key-secret-<pvc>: <secret>
  │
  └── (Used by operator to configure VolSync resources)
```

## License

Apache 2.0
