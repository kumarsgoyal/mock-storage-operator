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

# Build and push to Quay.io
quay-build:
	$(CONTAINER_TOOL) build -t quay.io/bmekhiss/mock-storage-operator:$(VERSION) .

quay-push: quay-build
	$(CONTAINER_TOOL) push quay.io/bmekhiss/mock-storage-operator:$(VERSION)
	@if [ "$(VERSION)" != "latest" ]; then \
		$(CONTAINER_TOOL) tag quay.io/bmekhiss/mock-storage-operator:$(VERSION) quay.io/bmekhiss/mock-storage-operator:latest; \
		$(CONTAINER_TOOL) push quay.io/bmekhiss/mock-storage-operator:latest; \
	fi

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
	@echo "Deploying mock-storage-operator..."
	@echo "Note: Ensure VolumeGroupReplication CRDs are installed first"
	@echo "Run: kubectl apply -k 'github.com/csi-addons/kubernetes-csi-addons/config/crd?ref=v0.14.0'"
	kubectl apply -f config/rbac/
	kubectl apply -f config/manager/

undeploy:
	kubectl delete -f config/manager/ --ignore-not-found
	kubectl delete -f config/rbac/ --ignore-not-found

run:
	go run ./cmd/main.go

fmt:
	go fmt ./...

vet:
	go vet ./...
