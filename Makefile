IMAGE   ?= f1mcp
VERSION ?= dev

.PHONY: build test lint docker-build docker-run

build:
	go build -ldflags="-X main.version=$(VERSION)" -o bin/f1mcp ./cmd/f1mcp

test:
	go test ./...

lint:
	go vet ./...

docker-build:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):$(VERSION) .

docker-run:
	docker run --rm -i $(IMAGE):$(VERSION)
