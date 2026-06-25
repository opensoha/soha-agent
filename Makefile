IMAGE_TAG ?= local
IMAGE_PLATFORMS ?= linux/amd64,linux/arm64
GOPROXY ?= https://proxy.golang.org,direct
PUSH_LATEST ?= 0
SOHA_CONTRACTS_DIR ?= $(abspath ../soha-contracts)
AGENT_IMAGE_REPOSITORY ?= yshanchui/soha-agent
HERMES_IMAGE_REPOSITORY ?= yshanchui/soha-hermes-agent

AGENT_IMAGE_TAGS = -t $(AGENT_IMAGE_REPOSITORY):$(IMAGE_TAG)
HERMES_IMAGE_TAGS = -t $(HERMES_IMAGE_REPOSITORY):$(IMAGE_TAG)
ifeq ($(PUSH_LATEST),1)
AGENT_IMAGE_TAGS += -t $(AGENT_IMAGE_REPOSITORY):latest
HERMES_IMAGE_TAGS += -t $(HERMES_IMAGE_REPOSITORY):latest
endif

.PHONY: help build deploy-agent-image deploy-hermes-image deploy-images deploy-agent-image-push deploy-hermes-image-push deploy-images-push

help:
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_.-]+:.*##/ {printf "  %-28s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the soha-agent binary.
	CGO_ENABLED=0 go build -o bin/soha-agent ./cmd/agent

deploy-agent-image: ## Build the generic soha-agent image.
	docker build --build-arg GOPROXY=$(GOPROXY) --build-context contracts=$(SOHA_CONTRACTS_DIR) -f deploy/Dockerfile $(AGENT_IMAGE_TAGS) .

deploy-hermes-image: ## Build the Hermes runner image.
	docker build --build-arg GOPROXY=$(GOPROXY) --build-context contracts=$(SOHA_CONTRACTS_DIR) -f deploy/Dockerfile.hermes-agent-runner $(HERMES_IMAGE_TAGS) .

deploy-images: deploy-agent-image deploy-hermes-image ## Build both agent images.

deploy-agent-image-push: ## Build and push the generic soha-agent image.
	@test "$(IMAGE_TAG)" != "local" || (echo "Set IMAGE_TAG to a release version before pushing." >&2; exit 1)
	docker buildx build --platform $(IMAGE_PLATFORMS) --build-arg GOPROXY=$(GOPROXY) --build-context contracts=$(SOHA_CONTRACTS_DIR) -f deploy/Dockerfile $(AGENT_IMAGE_TAGS) --push .

deploy-hermes-image-push: ## Build and push the Hermes runner image.
	@test "$(IMAGE_TAG)" != "local" || (echo "Set IMAGE_TAG to a release version before pushing." >&2; exit 1)
	docker buildx build --platform $(IMAGE_PLATFORMS) --build-arg GOPROXY=$(GOPROXY) --build-context contracts=$(SOHA_CONTRACTS_DIR) -f deploy/Dockerfile.hermes-agent-runner $(HERMES_IMAGE_TAGS) --push .

deploy-images-push: deploy-agent-image-push deploy-hermes-image-push ## Build and push both agent images.
