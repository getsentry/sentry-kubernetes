# syntax=docker/dockerfile:1

# Build the application
FROM golang:1.20 AS build-stage

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./

ENV CGO_ENABLED=0 GOOS=${TARGETPLATFORM} GOARCH=${TARGETARCH} GO111MODULE=on

RUN go build -o /bin/sentry-kubernetes

# Run the tests in the container
FROM build-stage AS test-stage
RUN go test -v ./...

# Use a slim container
FROM gcr.io/distroless/static-debian11 AS build-slim-stage

USER nonroot:nonroot

WORKDIR /

COPY --from=build-stage /bin/sentry-kubernetes /bin/sentry-kubernetes

ENTRYPOINT ["/bin/sentry-kubernetes"]
