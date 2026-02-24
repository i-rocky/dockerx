IMAGE ?= wpkpda/dockerx
VERSION ?= dev
GO ?= go

build:
	docker build -t $(IMAGE):$(VERSION) .

build-test-image:
	docker build -t $(IMAGE):test .

UID ?= $(shell id -u)
GID ?= $(shell id -g)

run:
	docker run -it --privileged -u $(UID):$(GID) -v .:/app -w /app $(IMAGE):$(VERSION) zsh

publish:
	docker buildx build --platform linux/amd64,linux/arm64 \
		-t $(IMAGE):$(VERSION) \
		-t $(IMAGE):latest \
		--push .

launch: build run

test:
	$(GO) test ./...

build-cli:
	CGO_ENABLED=0 $(GO) build -o dockerx .

cli: build-cli build-test-image
	./dockerx --image $(IMAGE):test --no-pull

build-win:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -ldflags "-X main.version=$(VERSION)" -o dockerx.exe .

.PHONY: build build-test-image run publish launch test build-cli cli build-win
