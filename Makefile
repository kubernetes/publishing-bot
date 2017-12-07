all: build
.PHONY: all

-include $(CONFIG)

DOCKER_REPO ?= k8s-publishing-bot
NAMESPACE ?=
TOKEN ?=
KUBECTL ?= kubectl
SCHEDULE ?= 0 5 * * *
INTERVAL ?= 86400

build_cmd = mkdir -p _output && GOOS=linux go build -o _output/$(1) ./cmd/$(1)
prepare_spec = sed 's,DOCKER_IMAGE,$(DOCKER_REPO),g'

build:
	$(call build_cmd,collapsed-kube-commit-mapper)
	$(call build_cmd,publishing-bot)
	$(call build_cmd,sync-tags)
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

init-deploy:
	$(KUBECTL) delete -n "$(NAMESPACE)" rc publisher || true
	$(KUBECTL) delete -n "$(NAMESPACE)" pod publisher || true
	$(KUBECTL) apply -n "$(NAMESPACE)" -f artifacts/manifests/storage-class.yaml || true
	$(KUBECTL) get StorageClass ssd
	sed 's/TOKEN/$(shell echo "$(TOKEN)" | base64 | tr -d '\n')/g' artifacts/manifests/secret.yaml | $(KUBECTL) apply -n "$(NAMESPACE)" -f -
	$(KUBECTL) apply -n "$(NAMESPACE)" -f $(CONFIG)-configmap.yaml
	$(KUBECTL) apply -n "$(NAMESPACE)" -f artifacts/manifests/pvc.yaml

run: init-deploy
	{ cat artifacts/manifests/pod.yaml && sed 's/^/  /' artifacts/manifests/podspec.yaml; } | \
	$(call prepare_spec) | $(KUBECTL) apply -n "$(NAMESPACE)" -f -

deploy: init-deploy
	{ cat artifacts/manifests/rc.yaml && sed 's/^/      /' artifacts/manifests/podspec.yaml; } | \
	$(call prepare_spec) | sed 's/-interval=0/-interval=$(INTERVAL)/g' | \
	$(KUBECTL) apply -n "$(NAMESPACE)" -f -
