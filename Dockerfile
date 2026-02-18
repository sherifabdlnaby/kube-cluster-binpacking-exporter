FROM golang:1.26-alpine AS builder

WORKDIR /build

# Install make and git for Makefile — cached unless base image changes
RUN apk add --no-cache make git

# Copy dependency files first — cached unless go.mod/go.sum changes
COPY go.mod go.sum Makefile ./
RUN go mod download

# Copy source code — cached unless .go files change
COPY *.go ./

# ARGs declared here so they don't bust the dependency/apk cache above.
# VERSION/COMMIT/DATE change on every release — only the final build layer is invalidated.
ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown

# Build using Makefile (centralized ldflags)
RUN CGO_ENABLED=0 make build \
    VERSION=${VERSION} \
    COMMIT=${COMMIT} \
    DATE=${DATE} && \
    mv kube-binpacking-exporter /kube-binpacking-exporter

FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.source="https://github.com/sherifabdlnaby/kube-binpacking-exporter"
LABEL org.opencontainers.image.description="Prometheus exporter for Kubernetes binpacking efficiency"
LABEL org.opencontainers.image.licenses="MIT"

COPY --from=builder /kube-binpacking-exporter /kube-binpacking-exporter

ENTRYPOINT ["/kube-binpacking-exporter"]
