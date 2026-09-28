IMAGE   ?= f1mcp
VERSION ?= dev

.PHONY: build test lint docker-build docker-run tracks-fetch tracks-render

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

# Download missing circuit layouts and fix up stored ones.
tracks-fetch:
	go run ./cmd/trackgen fetch

# Draw track maps and corner images into assets/tracks (not committed).
tracks-render:
	go run ./cmd/trackgen render
