all: build
.PHONY: all

-include $(CONFIG)

DOCKER_REPO ?= k8s-publisher-bot
NAMESPACE ?=
TOKEN ?=
KUBECTL ?= kubectl
DRYRUN ?= true
TARGET_ORG ?= $(whoami)

build_cmd = mkdir -p _output && GOOS=linux go build -o _output/$(1) ./cmd/$(1)
prepare_job = sed 's,DOCKER_IMAGE,$(DOCKER_REPO),g;s/-dry-run=true/-dry-run=$(DRYRUN)/g;s/TARGET_ORG/$(TARGET_ORG)/g'

build:
	$(call build_cmd,collapsed-kube-commit-mapper)
	$(call build_cmd,publisher-bot)
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
	$(KUBECTL) delete -n "$(NAMESPACE)" cronjob publisher || true
	$(KUBECTL) delete -n "$(NAMESPACE)" job publisher || true
	$(KUBECTL) apply -n "$(NAMESPACE)" -f artifacts/manifests/storage-class.yaml || true
	$(KUBECTL) get StorageClass ssd
	sed 's/TOKEN/$(TOKEN)/g' artifacts/manifests/secret.yaml | $(KUBECTL) apply -n "$(NAMESPACE)" -f -
	sed 's,TARGET_ORG,$(TARGET_ORG),g' artifacts/manifests/configmap.yaml | $(KUBECTL) apply -n "$(NAMESPACE)" -f -
	$(KUBECTL) apply -n "$(NAMESPACE)" -f artifacts/manifests/pvc.yaml

run: init-deploy
	{ cat artifacts/manifests/job.yaml && sed 's/^/    /' artifacts/manifests/jobtemplate.yaml; } | \
	$(call prepare_job) | \
	$(KUBECTL) apply -n "$(NAMESPACE)" -f -

deploy: init-deploy
	{ cat artifacts/manifests/cronjob.yaml && sed 's/^/      /' artifacts/manifests/jobtemplate.yaml; } | \
	$(call prepare_job) | \
	$(KUBECTL) apply -n "$(NAMESPACE)" -f -
