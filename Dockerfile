# syntax=docker/dockerfile:1

# Build stage
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/

ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -trimpath -o /bin/sentry-kubernetes ./cmd/agent

# Runtime stage - use distroless for minimal attack surface
FROM gcr.io/distroless/static:nonroot

COPY --from=builder /bin/sentry-kubernetes /bin/sentry-kubernetes

ENTRYPOINT ["/bin/sentry-kubernetes"]
