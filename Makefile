.PHONY: fmt-check vet test-unit test-e2e test verify build install clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILT_AT ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS = -X github.com/cmtonkinson/governator/internal/buildinfo.Version=$(VERSION) \
	-X github.com/cmtonkinson/governator/internal/buildinfo.Commit=$(COMMIT) \
	-X github.com/cmtonkinson/governator/internal/buildinfo.BuiltAt=$(BUILT_AT)

fmt-check:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "The following files are not formatted:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

vet:
	go vet ./...

test-unit:
	go test -v $$(go list ./... | grep -v '/tests/e2e$$')

test-e2e:
	go test -v ./tests/e2e

test: test-unit test-e2e

verify: fmt-check vet test

build:
	go build -ldflags "$(LDFLAGS)" -o governator .

install:
	go install -ldflags "$(LDFLAGS)" .

clean:
	rm -f governator
