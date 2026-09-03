GO ?= go
BIN ?= build/ts-pro

.PHONY: fmt test test-linux-amd64 vet build pure-go-build check doctor clean

fmt:
	$(GO) fmt ./...

test:
	CGO_ENABLED=0 $(GO) test ./...

test-linux-amd64:
	./scripts/test-linux-amd64.sh

vet:
	CGO_ENABLED=0 $(GO) vet ./...

build:
	mkdir -p build
	CGO_ENABLED=0 $(GO) build -o $(BIN) ./cmd/ts-pro

pure-go-build: build

check: fmt vet test build

doctor: build
	./$(BIN) doctor

clean:
	rm -rf build
