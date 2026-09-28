FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/f1mcp ./cmd/f1mcp

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/f1mcp /f1mcp
ENTRYPOINT ["/f1mcp"]
