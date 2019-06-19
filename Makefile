all: build
.PHONY: all

-include $(CONFIG)
-include $(CONFIG)-token

DOCKER_REPO ?= k8s-publishing-bot
NAMESPACE ?=
TOKEN ?=
KUBECTL ?= kubectl
SCHEDULE ?= 0 5 * * *
INTERVAL ?= 86400
MEMORY_REQUESTS ?= 200Mi
MEMORY_LIMITS ?= 1.6Gi
GOOS ?= linux

build_cmd = mkdir -p _output && GOOS=$(GOOS) go build -o _output/$(1) ./cmd/$(1)
prepare_spec = sed 's,DOCKER_IMAGE,$(DOCKER_REPO),g;s,MEMORY_REQUESTS,$(MEMORY_REQUESTS),g;s,MEMORY_LIMITS,$(MEMORY_LIMITS),g'

SHELL := /bin/bash

build:
	$(call build_cmd,collapsed-kube-commit-mapper)
	$(call build_cmd,publishing-bot)
	$(call build_cmd,sync-tags)
	$(call build_cmd,init-repo)
	$(call build_cmd,godeps-gen)
	$(call build_cmd,gomod-zip)
.PHONY: build

build-image: build
	docker build -t $(DOCKER_REPO) .
.PHONY: build-image

push-image:
	docker push $(DOCKER_REPO):latest

clean:
	rm -rf _output
.PHONY: clean

update-deps:
	glide update --strip-vendor
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
	sed 's/TOKEN/$(shell echo "$(TOKEN)" | base64 | tr -d '\n')/g' artifacts/manifests/secret.yaml | $(KUBECTL) apply -n "$(NAMESPACE)" -f -
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
