# Mock Storage Operator Documentation

Welcome to the Mock Storage Operator documentation. This directory contains comprehensive guides for deploying, configuring, and using the operator.

## 📖 Documentation Index

### Getting Started

1. **[Deployment Steps](DEPLOYMENT_STEPS.md)** ⭐ **START HERE**
   - Complete step-by-step deployment guide
   - Prerequisites installation
   - Operator deployment
   - Configuration and testing
   - Verification procedures
   - **Best for:** First-time users setting up the operator

2. **[VGR Creation Guide](VGR_CREATION_GUIDE.md)** ⭐ **CREATING VGR RESOURCES**
   - Detailed guide for creating VolumeGroupReplication resources
   - Understanding ConfigMap-based PVC configuration
   - Step-by-step ConfigMap creation from primary PVCs
   - VGR resource creation and verification
   - **Best for:** Users who need to create VGR resources with ConfigMap

### Reference Guides

3. **[User Guide](USER_GUIDE.md)**
   - Comprehensive user documentation
   - Detailed parameter configuration
   - Multiple deployment scenarios
   - Monitoring and troubleshooting
   - Common issues and solutions
   - Best practices
   - **Best for:** Understanding all features and configurations

4. **[VGR Quick Reference](VGR_QUICK_REFERENCE.md)**
   - Quick reference for VolumeGroupReplication resources
   - Field descriptions and examples
   - Label selector patterns
   - Status field reference
   - Common commands
   - Deployment scripts
   - **Best for:** Quick lookups and creating VGR resources

### Examples

5. **[Examples Directory](../examples/)**
   - `volumegroupreplicationclass.yaml` - VGRClass configuration
   - `primary-vgr.yaml` - Primary cluster VGR
   - `secondary-vgr.yaml` - Secondary cluster VGR
   - **Best for:** Copy-paste ready YAML configurations

---

## 🚀 Quick Start Path

Follow this path for the fastest setup:

1. **Read**: [Deployment Steps](DEPLOYMENT_STEPS.md) - Follow all steps in order
2. **Reference**: [VGR Quick Reference](VGR_QUICK_REFERENCE.md) - When creating VGR resources
3. **Troubleshoot**: [User Guide](USER_GUIDE.md) - If you encounter issues

---

## 📋 Documentation Overview

### Deployment Steps (DEPLOYMENT_STEPS.md)

**Purpose:** Step-by-step guide from zero to working replication

**Contents:**
- Environment setup requirements
- Prerequisites installation (CRDs, VolSync, Submariner)
- Operator deployment on both clusters
- Configuration (namespaces, secrets, VGRClass)
- Testing replication with sample data
- Verification procedures
- Troubleshooting common issues
- Cleanup instructions

**When to use:**
- First time deploying the operator
- Setting up a new environment
- Need detailed step-by-step instructions
- Want to verify each step before proceeding

---

### User Guide (USER_GUIDE.md)

**Purpose:** Comprehensive reference for all operator features

**Contents:**
- Overview and architecture
- Prerequisites with verification commands
- Installation options (Kustomize, local, Minikube)
- Parameter configuration explained
- Multiple deployment scenarios
- Monitoring and troubleshooting
- Common issues with solutions
- Best practices
- Complete working examples

**When to use:**
- Need detailed explanation of features
- Want to understand parameter options
- Looking for specific deployment scenarios
- Troubleshooting complex issues
- Learning best practices

---

### VGR Quick Reference (VGR_QUICK_REFERENCE.md)

**Purpose:** Fast reference for creating and managing VGR resources

**Contents:**
- VGR resource structure and templates
- Field descriptions with examples
- Label selector patterns
- Status field reference
- Common patterns and commands
- Quick deployment scripts
- Validation checklist

**When to use:**
- Creating new VGR resources
- Need quick syntax reference
- Looking for label selector examples
- Want to check VGR status
- Need troubleshooting commands

---

## 🎯 Use Case Guide

### "I'm new and want to deploy the operator"
→ Start with [Deployment Steps](DEPLOYMENT_STEPS.md)

### "I need to create a VGR resource"
→ Use [VGR Quick Reference](VGR_QUICK_REFERENCE.md)

### "I want to understand all configuration options"
→ Read [User Guide](USER_GUIDE.md)

### "I need example YAML files"
→ Check [Examples Directory](../examples/)

### "Something isn't working"
→ Check troubleshooting in [User Guide](USER_GUIDE.md) or [Deployment Steps](DEPLOYMENT_STEPS.md)

### "I want to understand the architecture"
→ Read the architecture section in [User Guide](USER_GUIDE.md)

### "I need to configure per-PVC settings"
→ See parameter configuration in [User Guide](USER_GUIDE.md)

### "I want quick commands for monitoring"
→ Use [VGR Quick Reference](VGR_QUICK_REFERENCE.md)

---

## 🔑 Key Concepts

### VolumeGroupReplication (VGR)
A Kubernetes custom resource that defines replication for a group of PVCs. It has three states:
- **Primary**: Source cluster that pushes data
- **Secondary**: Destination cluster that receives data
- **Resync**: Not implemented in mock operator

### VolumeGroupReplicationClass (VGRClass)
Defines configuration for VGR resources, including:
- Provisioner name (`mock.storage.io`)
- Per-PVC parameters (scheduling, storage classes)
- Global settings (capacity, PSK secret name)

### VolSync
The underlying replication engine that performs actual data transfer using rsync-tls protocol.

### Submariner
Optional multi-cluster networking solution that enables automatic service discovery across clusters.

---

## 📊 Documentation Comparison

| Feature | Deployment Steps | User Guide | VGR Quick Reference |
|---------|-----------------|------------|---------------------|
| Step-by-step instructions | ✅ Detailed | ⚠️ Partial | ❌ No |
| Prerequisites | ✅ Complete | ✅ Complete | ❌ No |
| Configuration examples | ✅ Basic | ✅ Advanced | ✅ Many |
| Troubleshooting | ✅ Common issues | ✅ Comprehensive | ✅ Commands only |
| VGR syntax reference | ⚠️ Basic | ⚠️ Partial | ✅ Complete |
| Quick commands | ⚠️ Some | ⚠️ Some | ✅ Many |
| Best practices | ❌ No | ✅ Yes | ⚠️ Some |
| Architecture | ❌ No | ✅ Yes | ❌ No |

---

## 🆘 Getting Help

If you can't find what you need in the documentation:

1. **Check the examples**: [../examples/](../examples/)
2. **Review operator logs**: 
   ```bash
   kubectl logs -n mock-storage-operator-system -l app=mock-storage-operator -f
   ```
3. **Check VGR status**:
   ```bash
   kubectl get vgr <name> -n <namespace> -o yaml
   ```
4. **Visit the GitHub repository**: https://github.com/BenamarMk/mock-storage-operator
5. **Check VolSync documentation**: https://volsync.readthedocs.io/

---

## 📝 Documentation Maintenance

**Last Updated:** 2026-04-05  
**Operator Version:** latest  
**Documentation Version:** 1.0

### Document Status

| Document | Status | Last Updated |
|----------|--------|--------------|
| DEPLOYMENT_STEPS.md | ✅ Current | 2026-04-05 |
| USER_GUIDE.md | ✅ Current | 2026-04-05 |
| VGR_QUICK_REFERENCE.md | ✅ Current | 2026-04-05 |
| README.md (this file) | ✅ Current | 2026-04-05 |

---

## 🔄 Related Resources

- **Main README**: [../README.md](../README.md)
- **Examples**: [../examples/](../examples/)
- **Source Code**: [../internal/](../internal/)
- **Configuration**: [../config/](../config/)
- **GitHub Repository**: https://github.com/BenamarMk/mock-storage-operator
- **Container Registry**: https://quay.io/repository/bmekhiss/mock-storage-operator

---

**Happy Replicating! 🚀**