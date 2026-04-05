.PHONY: build docker-build docker-push quay-build quay-push minikube-load deploy undeploy install uninstall

# Image URL to use for building/pushing image targets
IMG ?= quay.io/bmekhiss/mock-storage-operator:latest
VERSION ?= latest
MINIKUBE_PROFILE ?= dr2

# Container tool to use (podman or docker)
CONTAINER_TOOL ?= podman

build:
	go build -o bin/manager ./cmd/main.go

docker-build:
	$(CONTAINER_TOOL) build -t $(IMG) .

docker-push:
	$(CONTAINER_TOOL) push $(IMG)

# Build and push AMD64 image to Quay.io
quay-push:
	@echo "Building AMD64 image..."
	$(CONTAINER_TOOL) build --platform linux/amd64 \
		-t quay.io/bmekhiss/mock-storage-operator:$(VERSION) .
	@echo "Pushing image..."
	$(CONTAINER_TOOL) push quay.io/bmekhiss/mock-storage-operator:$(VERSION)
	@if [ "$(VERSION)" != "latest" ]; then \
		echo "Tagging and pushing as latest..."; \
		$(CONTAINER_TOOL) tag quay.io/bmekhiss/mock-storage-operator:$(VERSION) \
			quay.io/bmekhiss/mock-storage-operator:latest; \
		$(CONTAINER_TOOL) push quay.io/bmekhiss/mock-storage-operator:latest; \
	fi
	@echo "Image pushed successfully!"

minikube-load:
	@echo "Saving container image $(IMG) to tar file..."
	$(CONTAINER_TOOL) save $(IMG) -o /tmp/mock-storage-operator.tar
	@echo "Loading image into Minikube profile $(MINIKUBE_PROFILE)..."
	minikube -p $(MINIKUBE_PROFILE) image load /tmp/mock-storage-operator.tar
	@echo "Cleaning up tar file..."
	rm -f /tmp/mock-storage-operator.tar
	@echo "Image loaded successfully!"

install:
	@echo "Installing VolumeGroupReplication CRDs from kubernetes-csi-addons..."
	@echo "Run: kubectl apply -k 'github.com/csi-addons/kubernetes-csi-addons/config/crd?ref=v0.14.0'"
	@echo "Or see config/crd/bases/install.yaml for manual installation instructions"

uninstall:
	@echo "Uninstalling VolumeGroupReplication CRDs..."
	@echo "Run: kubectl delete -k 'github.com/csi-addons/kubernetes-csi-addons/config/crd?ref=v0.14.0'"

deploy:
	@echo "Deploying mock-storage-operator using Kustomize..."
	@echo "Note: Ensure VolumeGroupReplication CRDs are installed first"
	@echo "Run: kubectl apply -k 'github.com/csi-addons/kubernetes-csi-addons/config/crd?ref=v0.14.0'"
	kubectl apply -k config/default

undeploy:
	kubectl delete -k config/default --ignore-not-found

# Deploy from GitHub using Kustomize
deploy-github:
	@echo "Deploying from GitHub using Kustomize..."
	kubectl apply -k https://github.com/BenamarMk/mock-storage-operator/config/default?ref=main

run:
	go run ./cmd/main.go

fmt:
	go fmt ./...

vet:
	go vet ./...
