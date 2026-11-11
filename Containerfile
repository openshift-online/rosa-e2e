# The runtime stage is a Red Hat Hardened Image (Project Hummingbird) with no
# package manager. Builder and tools stages are build-time only and are never
# shipped, so they can use any image.

FROM registry.access.redhat.com/ubi9/go-toolset:latest AS builder

USER root
WORKDIR /build
ENV GOFLAGS=""
ENV GOGC=50
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go run github.com/onsi/ginkgo/v2/ginkgo build --tags E2Etests ./test/e2e/

# The CI step scripts that run inside this image call ocm, oc and kubectl, so
# those binaries are staged here from the same upstream sources as before.
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest AS tools

RUN microdnf install -y coreutils-single tar gzip && microdnf clean all

# coreutils-single is a single multicall binary. Hardlink it out as rm and chmod
# so the copy into the runtime stage picks up real files, not symlinks.
RUN mkdir /out \
 && ln /usr/bin/coreutils /out/rm \
 && ln /usr/bin/coreutils /out/chmod

RUN curl -sL https://github.com/openshift-online/ocm-cli/releases/latest/download/ocm-linux-amd64 -o /out/ocm \
 && chmod +x /out/ocm

RUN curl -sL https://mirror.openshift.com/pub/openshift-v4/clients/ocp/stable/openshift-client-linux.tar.gz \
 | tar -C /out -xzf - oc kubectl

# jq is only useful with libjq and libonig, so it is a named stage to keep the
# binary and its libraries on one version.
FROM registry.access.redhat.com/hi/jq:1.8@sha256:6d1c63481a7520d203f6e993c82be4d114cefebcf32b604a405c7d99c1cc314d AS jq

FROM registry.access.redhat.com/hi/core-runtime:2.43@sha256:4163796d8b7f28021178093d31cb97c05336fd3f3a4d7006603a7fef29e997e7

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

# core-runtime already carries the bash, coreutils subset and glibc the step
# scripts rely on, plus the CA bundle. Only the missing pieces are added here.
COPY --from=tools /out/rm /usr/local/bin/rm
COPY --from=tools /out/chmod /usr/local/bin/chmod
COPY --from=tools /out/ocm /usr/local/bin/ocm
COPY --from=tools /out/oc /usr/local/bin/oc
COPY --from=tools /out/kubectl /usr/local/bin/kubectl
COPY --from=jq /usr/bin/jq /usr/local/bin/jq
COPY --from=jq /usr/lib64/libjq.so.1* /usr/lib64/
COPY --from=jq /usr/lib64/libonig.so.5* /usr/lib64/
COPY --from=builder /build/test/e2e/e2e.test /usr/local/bin/e2e.test

# Hardened images run non-root by default; 65532 matches the upstream default.
USER 65532
WORKDIR /tmp

ENTRYPOINT ["/usr/local/bin/e2e.test"]
