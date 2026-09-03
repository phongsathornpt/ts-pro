GO ?= go
BIN ?= build/ts-pro

.PHONY: fmt test vet build pure-go-build check doctor clean

fmt:
	$(GO) fmt ./...

test:
	CGO_ENABLED=0 $(GO) test ./...

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
