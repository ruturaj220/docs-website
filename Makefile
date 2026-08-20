IMAGE ?= mojrodevops.azurecr.io/docs-platform
TAG ?= 0.1.0
NAMESPACE ?= docs-platform
RELEASE ?= docs

.PHONY: build test image push run helm-lint helm-template deploy uninstall status logs

build:
	go build ./...

test:
	go test ./...

image:
	podman build -t $(IMAGE):$(TAG) .

push:
	podman push $(IMAGE):$(TAG)

# Run locally against a scratch data dir; needs mkdocs on PATH.
run:
	DATA_DIR=$${DATA_DIR:-/tmp/docs-platform-data} \
	LISTEN_ADDR=:8080 \
	WEBHOOK_SECRET=$${WEBHOOK_SECRET:-dev-secret} \
	SHARED_DOCS_DIR=$(PWD)/docs \
	MKDOCS_CONFIG=$(PWD)/mkdocs.yml \
	go run ./cmd/server

helm-lint:
	helm lint helm/docs-platform --set webhook.secret=lint

helm-template:
	helm template $(RELEASE) helm/docs-platform --set webhook.secret=lint

deploy:
	helm upgrade --install $(RELEASE) helm/docs-platform \
	  --namespace $(NAMESPACE) --create-namespace \
	  --set image.repository=$(IMAGE) --set image.tag=$(TAG)

uninstall:
	helm uninstall $(RELEASE) --namespace $(NAMESPACE)

status:
	kubectl -n $(NAMESPACE) get pods,pvc,svc,httproute

logs:
	kubectl -n $(NAMESPACE) logs -l app.kubernetes.io/name=docs-platform -f
