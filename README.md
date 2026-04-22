# mock-storage-operator

A Kubernetes operator that acts as a **mock storage vendor** implementing the VolumeGroupReplication API for DR testing with Ramen. It uses VolSync internally for actual data replication while presenting a storage-vendor-like interface to Ramen.

## 📚 Documentation

- **[Deployment Steps](docs/DEPLOYMENT_STEPS.md)** - Step-by-step deployment guide from prerequisites to testing
- **[VGR Creation Guide](docs/VGR_CREATION_GUIDE.md)** - Detailed guide for creating VGR resources
- **[User Guide](docs/USER_GUIDE.md)** - Complete guide with installation, configuration, and troubleshooting
- **[VGR Quick Reference](docs/VGR_QUICK_REFERENCE.md)** - Quick reference for creating VolumeGroupReplication resources
- **[Examples](examples/)** - YAML examples for VGRClass and VGR resources

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

### Quick Start: Deploy from Quay.io (Recommended)

The operator is available as a multi-architecture container image on Quay.io and can be deployed with a single command using Kustomize:

```bash
# Deploy on both clusters (primary and secondary)
kubectl apply -k https://github.com/BenamarMk/mock-storage-operator/config/default?ref=main
```

**What this does:**
- ✅ Creates `mock-storage-operator-system` namespace
- ✅ Deploys RBAC resources (ServiceAccount, ClusterRole, ClusterRoleBinding)
- ✅ Deploys the operator using `quay.io/bmekhiss/mock-storage-operator:latest`
- ✅ Supports both AMD64 (x86_64) and ARM64 architectures

**Verify deployment:**
```bash
# Check operator is running
kubectl get pods -n mock-storage-operator-system

# Check logs
kubectl logs -n mock-storage-operator-system -l app=mock-storage-operator -f
```

### Alternative Deployment Options

<details>
<summary><b>Option 1: Deploy Components Separately</b></summary>

```bash
# Deploy only RBAC
kubectl apply -k https://github.com/BenamarMk/mock-storage-operator/config/rbac?ref=main

# Deploy only manager
kubectl apply -k https://github.com/BenamarMk/mock-storage-operator/config/manager?ref=main
```
</details>

<details>
<summary><b>Option 2: Deploy from Local Clone</b></summary>

```bash
# Clone the repository
git clone https://github.com/BenamarMk/mock-storage-operator.git
cd mock-storage-operator

# Deploy using local Kustomize configs
kubectl apply -k config/default
```
</details>

<details>
<summary><b>Option 3: Build and Push Your Own Image</b></summary>

```bash
# Login to Quay.io
podman login quay.io

# Clean up any existing local images (important!)
podman rmi quay.io/bmekhiss/mock-storage-operator:v0.1.0 2>/dev/null || true
podman rmi quay.io/bmekhiss/mock-storage-operator:latest 2>/dev/null || true

# Build and push multi-architecture image (AMD64 + ARM64)
make quay-push VERSION=v0.1.0
```

This creates:
- `quay.io/bmekhiss/mock-storage-operator:v0.1.0` (multi-arch manifest)
- `quay.io/bmekhiss/mock-storage-operator:latest` (multi-arch manifest)
- Architecture-specific images: `v0.1.0-amd64` and `v0.1.0-arm64`
</details>

<details>
<summary><b>Option 4: Local Development</b></summary>

```bash
# Build locally
make build

# Run locally (requires kubeconfig)
make run

# Or build container image for local testing
make docker-build IMG=localhost/mock-storage-operator:dev
```
</details>

<details>
<summary><b>Option 5: Deploy to Minikube</b></summary>

```bash
# Build and load into Minikube
make docker-build IMG=mock-storage-operator:latest
make minikube-load MINIKUBE_PROFILE=dr1

# Deploy using Kustomize
kubectl apply -k config/default
```
</details>

### Uninstall

```bash
# Remove the operator
kubectl delete -k https://github.com/BenamarMk/mock-storage-operator/config/default?ref=main

# Or using make
make undeploy
```

## Setup Order

### 1. Create VolumeGroupReplicationClass

First, create the VolumeGroupReplicationClass that defines the mock provisioner:

```bash
kubectl apply -f examples/volumegroupreplicationclass.yaml
```

This class specifies:
- `provisioner: mock.storage.io` - tells the operator to handle VGRs using this class
- Default parameters (capacity, storageClassName, schedule, pskSecretName, etc.)

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

### 3. Copy Secrets to Primary

Copy the rsync-tls key secrets from secondary to primary:
```bash
kubectl get secret volsync-rsync-tls-dst-mockdr-mysql-data -n myapp --context secondary -o yaml \
  | kubectl apply --context primary -f -
```

The operator automatically discovers ReplicationDestination addresses using Submariner's clusterset.local DNS.

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
  │       ├── schedulingInterval: "5m" (or cron format)
  │       ├── capacity: "10Gi"
  │       ├── storageClassName: "standard"
  │       ├── pskSecretName: "volsync-rsync-tls-secret"
  │       └── volumeSnapshotClassName: "csi-snapclass" (optional)
  │
  └── (Used by operator to configure VolSync resources)
```

## License

Apache 2.0
