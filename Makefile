# Copyright 2017 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

all: build
.PHONY: all

-include $(CONFIG)
-include $(CONFIG)-token

GIT_TAG ?= $(shell git describe --tags --always --dirty)

# Image variables
IMG_REGISTRY ?= gcr.io/k8s-staging-publishing-bot
IMG_NAME = k8s-publishing-bot

IMG_VERSION ?= v0.0.0-1

# TODO(image): Consider renaming this variable
DOCKER_REPO ?= $(IMG_REGISTRY)/$(IMG_NAME)
NAMESPACE ?=
TOKEN ?=
KUBECTL ?= kubectl
SCHEDULE ?= 0 5 * * *
INTERVAL ?= 86400
CPU_LIMITS ?= 2
CPU_REQUESTS ?= 300m
MEMORY_REQUESTS ?= 200Mi
MEMORY_LIMITS ?= 1639Mi
GOOS ?= linux

build_cmd = mkdir -p _output && GOOS=$(GOOS) CGO_ENABLED=0 go build -o _output/$(1) ./cmd/$(1)
prepare_spec = sed 's,DOCKER_IMAGE,$(DOCKER_REPO),g;s,CPU_LIMITS,$(CPU_LIMITS),g;s,CPU_REQUESTS,$(CPU_REQUESTS),g;s,MEMORY_REQUESTS,$(MEMORY_REQUESTS),g;s,MEMORY_LIMITS,$(MEMORY_LIMITS),g'

SHELL := /bin/bash

build:
	$(call build_cmd,collapsed-kube-commit-mapper)
	$(call build_cmd,publishing-bot)
	$(call build_cmd,sync-tags)
	$(call build_cmd,init-repo)
	$(call build_cmd,gomod-zip)
	$(call build_cmd,update-rules)
.PHONY: build

build-image: build
	docker build \
		-t $(DOCKER_REPO):$(GIT_TAG) \
		-t $(DOCKER_REPO):$(IMG_VERSION) \
		-t $(DOCKER_REPO):latest \
		.
.PHONY: build-image

push-image:
	docker push $(DOCKER_REPO):$(GIT_TAG)
	docker push $(DOCKER_REPO):$(IMG_VERSION)
	docker push $(DOCKER_REPO):latest

build-and-push-image: build-image push-image
.PHONY: build-and-push-image

clean:
	rm -rf _output
.PHONY: clean

update-deps:
	go mod tidy
.PHONY: update-deps

validate:
	if [ -f $(CONFIG)-rules-configmap.yaml ]; then \
		go run ./cmd/validate-rules <(sed '1,/config: /d;s/^    //' $(CONFIG)-rules-configmap.yaml); \
	else \
		go run ./cmd/validate-rules $$(grep "rules-file: " $(CONFIG)-configmap.yaml | sed 's/.*rules-file: //'); \
	fi
.PHONY: validate

init-deploy: validate
	$(KUBECTL) delete -n "$(NAMESPACE)" --ignore-not-found=true replicaset publisher
	$(KUBECTL) delete -n "$(NAMESPACE)" --ignore-not-found=true pod publisher
	while $(KUBECTL) get pod -n "$(NAMESPACE)" publisher -a &>/dev/null; do echo -n .; sleep 1; done
	$(KUBECTL) apply -n "$(NAMESPACE)" -f artifacts/manifests/storage-class.yaml || true
	$(KUBECTL) get StorageClass ssd
	$(KUBECTL) apply -n "$(NAMESPACE)" -f $(CONFIG)-configmap.yaml
	$(KUBECTL) apply -n "$(NAMESPACE)" -f $(CONFIG)-rules-configmap.yaml; \
	$(KUBECTL) apply -n "$(NAMESPACE)" -f artifacts/manifests/pvc.yaml

run: init-deploy
	{ cat artifacts/manifests/pod.yaml && sed 's/^/  /' artifacts/manifests/podspec.yaml; } | \
	$(call prepare_spec) | $(KUBECTL) apply -n "$(NAMESPACE)" -f -

deploy: init-deploy
	$(KUBECTL) apply -n "$(NAMESPACE)" -f artifacts/manifests/service.yaml
	{ cat artifacts/manifests/rs.yaml && sed 's/^/      /' artifacts/manifests/podspec.yaml; } | \
	$(call prepare_spec) | sed 's/-interval=0/-interval=$(INTERVAL)/g' | \
	$(KUBECTL) apply -n "$(NAMESPACE)" -f -

test: ## Run go tests
	go test -v -coverprofile=coverage.out ./...
