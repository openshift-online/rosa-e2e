FROM docker.io/library/golang:1.23 AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go run github.com/onsi/ginkgo/v2/ginkgo build --tags E2Etests ./test/e2e/

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

COPY --from=builder /build/test/e2e/e2e.test /usr/local/bin/e2e.test

ENTRYPOINT ["/usr/local/bin/e2e.test"]
