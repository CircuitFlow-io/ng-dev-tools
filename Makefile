BINARY  := ngt
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
INSTALL_DIR := $(or $(shell go env GOBIN),$(shell go env GOPATH)/bin)

.PHONY: build install test lint fmt tidy clean

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/$(BINARY)
	@case ":$$PATH:" in *":$(INSTALL_DIR):"*) ;; *) \
		echo "Installed to $(INSTALL_DIR), which is not on your PATH. Add this to ~/.zshrc:"; \
		echo '  export PATH="$(INSTALL_DIR):$$PATH"';; \
	esac

test:
	go test -race ./...

lint:
	golangci-lint run

fmt:
	golangci-lint fmt

tidy:
	go mod tidy

clean:
	rm -rf bin
