BINARY := codex-usage
PKG    := ./cmd/codex-usage
GOBIN  := $(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(shell go env GOPATH)/bin
endif

.PHONY: all build install uninstall test vet fmt run reconcile clean

all: build

## build: compile the binary into ./bin
build:
	@mkdir -p bin
	go build -o bin/$(BINARY) $(PKG)

## install: build and install the binary into $GOBIN (or $GOPATH/bin)
install:
	go install $(PKG)
	@echo "Installed $(BINARY) -> $(GOBIN)/$(BINARY)"
	@echo "Assicurati che $(GOBIN) sia nel PATH."

## uninstall: remove the installed binary
uninstall:
	rm -f $(GOBIN)/$(BINARY)
	@echo "Removed $(GOBIN)/$(BINARY)"

## test: run the test suite
test:
	go test ./...

## vet: run go vet
vet:
	go vet ./...

## fmt: format the source
fmt:
	gofmt -w .

## run: run the default report
run:
	go run $(PKG)

## reconcile: run the local-vs-server reconciliation
reconcile:
	go run $(PKG) --reconcile

## clean: remove build artifacts
clean:
	rm -rf bin