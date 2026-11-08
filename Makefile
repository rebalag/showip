# Override on the command line, e.g.:
#   make images REGISTRY=myregistry.example.com/team TAG=v1.0.0
REGISTRY ?= docker.io/rebala
IMAGE    ?= $(REGISTRY)/showip
TAG      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo latest)
PLATFORMS ?= linux/amd64,linux/arm64

.PHONY: local test image images run clean

local:
	CGO_ENABLED=0 go build -o bin/showip .

test:
	go test -race ./...

# Single-arch image loaded into the local Docker daemon
image:
	docker build --target showip-image -t $(IMAGE):$(TAG) .

# Multi-arch image pushed to the registry (requires `docker login` first)
images:
	docker buildx build --platform $(PLATFORMS) --target showip-image \
		-t $(IMAGE):$(TAG) --push .

run: image
	docker run --rm -p 8080:8080 $(IMAGE):$(TAG)

clean:
	rm -rf bin/
