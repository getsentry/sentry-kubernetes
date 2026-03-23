# syntax=docker/dockerfile:1

# Build the application
FROM golang:1.24-alpine AS build-stage

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/

ARG TARGETOS TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -trimpath -o /bin/sentry-kubernetes ./cmd/agent

# Run the tests in the container
FROM build-stage AS test-stage
RUN go test -v ./...

# Use a slim container
FROM gcr.io/distroless/static-debian12 AS build-slim-stage

USER nonroot:nonroot

WORKDIR /

COPY --from=build-stage /bin/sentry-kubernetes /bin/sentry-kubernetes

ENTRYPOINT ["/bin/sentry-kubernetes"]
