.PHONY: build docker-build minikube-load deploy undeploy install uninstall

IMG ?= mock-storage-operator:latest
MINIKUBE_PROFILE ?= dr2

build:
	go build -o bin/manager ./cmd/main.go

docker-build:
	podman build -t $(IMG) .

minikube-load:
	@echo "Saving Podman image $(IMG) to tar file..."
	podman save $(IMG) -o /tmp/mock-storage-operator.tar
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
