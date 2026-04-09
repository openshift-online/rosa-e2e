.PHONY: build test dry-run unit-test lint image clean

# Clear GOFLAGS to avoid -mod=vendor when no vendor dir exists
export GOFLAGS=
GINKGO = go run github.com/onsi/ginkgo/v2/ginkgo
LABEL_FILTER ?=
GOLANGCI_LINT = $(shell go env GOPATH)/bin/golangci-lint

build:
	$(GINKGO) build --tags E2Etests ./test/e2e/

test:
ifdef LABEL_FILTER
	$(GINKGO) run --tags E2Etests --label-filter="$(LABEL_FILTER)" --junit-report junit-report.xml -v ./test/e2e/
else
	$(GINKGO) run --tags E2Etests --junit-report junit-report.xml -v ./test/e2e/
endif

dry-run:
ifdef LABEL_FILTER
	$(GINKGO) run --tags E2Etests --label-filter="$(LABEL_FILTER)" --dry-run -v ./test/e2e/
else
	$(GINKGO) run --tags E2Etests --dry-run -v ./test/e2e/
endif

unit-test:
	go test -mod=mod ./pkg/...

lint:
	@which golangci-lint > /dev/null || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go vet --tags E2Etests ./...
	GOLANGCI_LINT_CACHE=/tmp/.golangci-lint-cache $(GOLANGCI_LINT) run --build-tags E2Etests --concurrency=1 --timeout=15m ./...

image:
	podman build -t rosa-e2e:latest -f Containerfile .

clean:
	rm -f test/e2e/e2e.test
	rm -f junit-report.xml
