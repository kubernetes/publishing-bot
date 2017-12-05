all: build
.PHONY: all

REPO ?= k8s-publisher-bot
BUILD_CMD = mkdir -p _output && GOOS=linux go build -o _output/$(1) ./cmd/$(1)
NAMESPACE ?=
TOKEN ?=
KUBECTL ?= kubectl

build:
	$(call BUILD_CMD,collapsed-kube-commit-mapper)
	$(call BUILD_CMD,publisher-bot)
	$(call BUILD_CMD,sync-tags)
.PHONY: build

build-image: build
	docker build -t $(REPO) .
.PHONY: build-image

push-image:
	docker push $(REPO):latest

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
	$(KUBECTL) apply -n "$(NAMESPACE)" -f artifacts/manifests/configmap.yaml
	$(KUBECTL) apply -n "$(NAMESPACE)" -f artifacts/manifests/pvc.yaml

run: init-deploy
	{ cat artifacts/manifests/job.yaml && sed 's/^/    /' artifacts/manifests/jobtemplate.yaml; } | \
	sed 's,DOCKER_IMAGE,$(REPO),g' | $(KUBECTL) apply -n "$(NAMESPACE)" -f -

deploy: init-deploy
	{ cat artifacts/manifests/cronjob.yaml && sed 's/^/      /' artifacts/manifests/jobtemplate.yaml; } | \
	sed 's,DOCKER_IMAGE,$(REPO),g' | $(KUBECTL) apply -n "$(NAMESPACE)" -f -
