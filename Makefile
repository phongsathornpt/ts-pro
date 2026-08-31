GO ?= go
BIN ?= build/tsnative

.PHONY: fmt test vet build check doctor clean

fmt:
	$(GO) fmt ./...

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

build:
	mkdir -p build
	$(GO) build -o $(BIN) ./cmd/tsnative

check: fmt vet test build

doctor: build
	./$(BIN) doctor

clean:
	rm -rf build
