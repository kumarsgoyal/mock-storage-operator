# Deployment Fix for PVC Update Permission Error

## Error
```
persistentvolumeclaims "mock-pvc-test" is forbidden: User "system:serviceaccount:mock-storage-operator-system:mock-storage-operator-controller-manager" cannot update resource "persistentvolumeclaims"
```

## Root Cause
The deployed operator is using an older version of the RBAC configuration that didn't include the `update` verb for PVCs. The current code requires PVC update permissions to manage ownership references.

## Current RBAC Configuration (Correct)
The `config/rbac/role.yaml` file already has the correct permissions:
```yaml
- apiGroups: [""]
  resources: [persistentvolumeclaims]
  verbs: [get, list, watch, create, update, patch]
```

## Solution
You need to redeploy the operator with the updated RBAC configuration:

### Step 1: Push commits to GitHub
```bash
git push origin main
```

### Step 2: Rebuild and push container image
```bash
make quay-push VERSION=latest
```

### Step 3: Redeploy the operator
```bash
# Delete the old deployment
kubectl delete -k config/default

# Wait a few seconds for cleanup
sleep 5

# Deploy the new version (includes updated RBAC)
kubectl apply -k config/default
```

### Alternative: Apply RBAC only
If you can't rebuild the image immediately, you can apply just the RBAC:
```bash
kubectl apply -f config/rbac/role.yaml
kubectl apply -f config/rbac/role_binding.yaml

# Restart the operator pod to pick up new permissions
kubectl rollout restart deployment mock-storage-operator-controller-manager -n mock-storage-operator-system
```

## Verification
After redeployment, verify the permissions:
```bash
# Check if the ClusterRole has update permission
kubectl get clusterrole mock-storage-operator-manager-role -o yaml | grep -A 5 persistentvolumeclaims

# Check operator logs
kubectl logs -n mock-storage-operator-system deployment/mock-storage-operator-controller-manager -f
```

## What Changed
The recent commits added optimized PVC ownership management that requires the ability to update PVCs to set/clear owner references during primary/secondary transitions.