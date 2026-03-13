.PHONY: build test dry-run unit-test lint image clean

GINKGO = go run github.com/onsi/ginkgo/v2/ginkgo
LABEL_FILTER ?=

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
	go test ./pkg/...

lint:
	go vet --tags E2Etests ./...
	golangci-lint run --build-tags E2Etests ./...

image:
	podman build -t rosa-e2e:latest -f Containerfile .

clean:
	rm -f test/e2e/e2e.test
	rm -f junit-report.xml
