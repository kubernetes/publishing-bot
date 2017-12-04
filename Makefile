all: build
.PHONY: all

REPO ?= k8s-publisher-bot
BUILD_CMD = mkdir -p _output && GOOS=linux go build -o _output/$(1) ./cmd/$(1)
NAMESPACE ?= k8s-publisher-bot
TOKEN ?=

build:
	$(call BUILD_CMD,collapsed-kube-commit-mapper)
	$(call BUILD_CMD,publisher-bot)
	$(call BUILD_CMD,sync-tags)
.PHONY: build

build-image: build
	docker build -t $(REPO)
.PHONY: build-image

push-image:
	docker push $(REPO):latest

clean:
	rm -rf _output
.PHONY: clean

update-deps:
	glide update --strip-vendor
.PHONY: update-deps

deploy:
	kubectl delete -namespace $(NAMESPACE) -f cronjob
	kubectl apply -namespace $(NAMESPACE) -f artifacts/manifests/storage-class.yaml || true
	kubectl get StorageClass ssd
	sed 's/TOKEN/$(TOKEN)/g' artifacts/manifests/secret.yaml | kubectl apply -namespace $(NAMESPACE) -f -
	kubectl apply -namespace $(NAMESPACE) -f artifacts/manifests/configmap.yaml
	kubectl apply -namespace $(NAMESPACE) -f artifacts/manifests/pvc.yaml
	sed 's/DOCKER_IMAGE/$(REPO)/g' artifacts/manifests/cronjob.yaml | kubectl apply -namespace $(NAMESPACE) -f -
