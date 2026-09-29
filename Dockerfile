# Builds on the build machine's platform and cross-compiles for the target,
# so multi-platform images don't need emulation.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG VERSION=dev
ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/f1mcp ./cmd/f1mcp
# Where transcripts are kept (F1MCP_CACHE_DIR), owned by the nonroot user so
# a volume mounted there is writable.
RUN mkdir -p /out/cache

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/f1mcp /f1mcp
COPY --from=build --chown=65532:65532 /out/cache /cache
EXPOSE 8080
ENTRYPOINT ["/f1mcp"]
