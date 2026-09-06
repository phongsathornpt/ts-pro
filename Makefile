GO ?= go
BIN ?= build/ts-pro

.PHONY: fmt test test-linux-amd64 vet build pure-go-build check doctor bench-performance clean

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

bench-performance:
	$(GO) test ./pkg/tspro -run '^$$' -bench 'Benchmark(CompileSource|DynamicProperty|ArrayGrowth|StringConcat)' -benchmem -benchtime=250ms -count=1
	$(GO) test ./internal/backend/regalloc -run '^$$' -bench BenchmarkAllocateLoopHeavy -benchmem -benchtime=250ms -count=1
	$(GO) test ./internal/midend/opt -run '^$$' -bench BenchmarkOptimizeLinearIR -benchmem -benchtime=250ms -count=1
	$(GO) test ./internal/backend/lower -run '^$$' -bench BenchmarkAMD64GCStringAllocationChurn -benchmem -benchtime=250ms -count=1

clean:
	rm -rf build
