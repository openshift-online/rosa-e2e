FROM registry.access.redhat.com/ubi9/go-toolset:latest AS builder

USER root
WORKDIR /build
ENV GOFLAGS=""
ENV GOGC=50
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go run github.com/onsi/ginkgo/v2/ginkgo build --tags E2Etests ./test/e2e/

FROM registry.access.redhat.com/ubi9/ubi:latest

LABEL com.redhat.component="rosa-e2e" \
      name="rosa-e2e" \
      version="0.1" \
      release="1" \
      description="ROSA E2E test suite for ROSA and OSD cluster validation" \
      summary="Unified E2E test binary for ROSA HCP, ROSA Classic STS, and OSD GCP topologies" \
      url="https://github.com/openshift-online/rosa-e2e" \
      vendor="Red Hat, Inc." \
      distribution-scope="public" \
      maintainer="ROSA SRE <rosa-sre@redhat.com>"

RUN yum -y install --setopt=skip_missing_names_on_install=False \
    jq && yum clean all

RUN curl -sL https://github.com/openshift-online/ocm-cli/releases/latest/download/ocm-linux-amd64 -o /usr/local/bin/ocm && \
    chmod +x /usr/local/bin/ocm

RUN curl -sL https://mirror.openshift.com/pub/openshift-v4/clients/ocp/stable/openshift-client-linux.tar.gz | \
    tar -C /usr/local/bin -xzf - oc kubectl

COPY --from=builder /build/test/e2e/e2e.test /usr/local/bin/e2e.test

RUN useradd -r -s /sbin/nologin -M e2e
USER e2e
WORKDIR /tmp

ENTRYPOINT ["/usr/local/bin/e2e.test"]
