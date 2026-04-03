FROM registry.ci.openshift.org/ocp/builder:rhel-9-golang-1.24-openshift-4.22 AS builder

WORKDIR /build
ENV GOFLAGS=""
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go run github.com/onsi/ginkgo/v2/ginkgo build --tags E2Etests ./test/e2e/

FROM registry.ci.openshift.org/ci/rosa-aws-cli:latest

COPY --from=builder /build/test/e2e/e2e.test /usr/local/bin/e2e.test

ENTRYPOINT ["/usr/local/bin/e2e.test"]
