# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=v0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=v0.0.2)
VERSION ?= v0.0.0-${USER}-dev

# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
# This variable is used to construct full image tags for bundle and catalog images.
#
# For example, running 'make bundle-build bundle-push catalog-build catalog-push' will build and push both
# puregw.io/puregw-bundle:$VERSION and puregw.io/puregw-catalog:$VERSION.
IMAGE_TAG_BASE ?= quay.io/epic-gateway

# Image URL to use all building/pushing image targets
IMG ?= ${IMAGE_TAG_BASE}/pure-gateway:${VERSION}
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.22

# Tools that we use.
CONTROLLER_GEN = go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.7.0
KUSTOMIZE = go run sigs.k8s.io/kustomize/kustomize/v4@v4.5.2
ENVTEST = go run sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

generate: CACHE != mktemp -d
generate: ## Generate code and manifests
# Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases
# cache kustomization.yaml because "kustomize edit" modifies it
	cp config/agent/kustomization.yaml ${CACHE}/kustomization-agent.yaml
	cp config/manager/kustomization.yaml ${CACHE}/kustomization-manager.yaml
	cd config/agent && $(KUSTOMIZE) edit set image controller=${IMG}
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > deploy/pure-gateway.yaml
	$(KUSTOMIZE) build config/development > deploy/pure-gateway-development.yaml
	cp deploy/pure-gateway.yaml deploy/pure-gateway-${VERSION}.yaml
	cp deploy/pure-gateway-development.yaml deploy/pure-gateway-development-${VERSION}.yaml
# restore kustomization.yaml
	cp ${CACHE}/kustomization-agent.yaml config/agent/kustomization.yaml
	cp ${CACHE}/kustomization-manager.yaml config/manager/kustomization.yaml
# generate deepcopy code
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

fmt: ## Run go fmt against code.
	go fmt ./...

vet: ## Run go vet against code.
	go vet ./...

unit-test: generate fmt vet ## Run unit tests (no external resources needed).
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./... -short -coverprofile cover.out

check: generate fmt vet ## Run "short" tests only.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./... -short -coverprofile cover.out

test: generate fmt vet ## Run all tests, even ones that need external resources.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./... -coverprofile cover.out

##@ Build

build: generate fmt vet ## Build manager binary.
	go build -o bin/manager ./cmd/manager/...
	go build -o bin/agent ./cmd/agent/...

run: generate fmt vet ## Run a controller from your host.
	go run ./cmd/manager/...

run-agent: generate fmt vet ## Run an agent from your host.
	EPIC_NODE_NAME=mk8s1 EPIC_HOST_IP=192.168.254.101 go run ./cmd/agent/...

image-build: ## Build the container image.
	docker build -t ${IMG} .

image-push: ## Push the container image to the registry.
	docker push ${IMG}

##@ Deployment

install: ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

uninstall: ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

deploy: generate ## Deploy to the K8s cluster specified in ~/.kube/config.
	kubectl apply -f deploy/pure-gateway.yaml

undeploy: generate ## Undeploy from the K8s cluster specified in ~/.kube/config.
	kubectl delete -f deploy/pure-gateway.yaml
