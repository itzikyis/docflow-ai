# Recipes must run under both cmd.exe (GNU make on Windows without sh on PATH)
# and POSIX sh (Linux CI), so keep them to plain commands without shell syntax.

SERVICES := api
BIN_DIR  := bin

ifeq ($(OS),Windows_NT)
  EXE     := .exe
  NULLDEV := nul
  RM_BIN  := if exist $(BIN_DIR) rmdir /s /q $(BIN_DIR)
else
  EXE     :=
  NULLDEV := /dev/null
  RM_BIN  := rm -rf $(BIN_DIR)
endif

VERSION ?= $(shell git describe --tags --always --dirty 2>$(NULLDEV) || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.DEFAULT_GOAL := help

.PHONY: help
help:
	@echo Usage: make TARGET
	@echo   build     Build all service binaries into $(BIN_DIR)/
	@echo   run       Run the API locally
	@echo   test      Run unit tests
	@echo   cover     Run unit tests with a coverage report
	@echo   fmt       Format code with gofmt and goimports
	@echo   vet       Run go vet
	@echo   lint      Run golangci-lint
	@echo   check     Run vet, lint and tests - the pre-push gate
	@echo   tidy      Tidy go.mod and go.sum
	@echo   clean     Remove build output

.PHONY: build
build: $(addprefix build-,$(SERVICES))

build-%:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$*$(EXE) ./cmd/$*

.PHONY: run
run:
	go run -ldflags "$(LDFLAGS)" ./cmd/api

.PHONY: test
test:
	go test ./...

.PHONY: cover
cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

.PHONY: fmt
fmt:
	golangci-lint fmt

.PHONY: vet
vet:
	go vet ./...

.PHONY: lint
lint:
	golangci-lint run

.PHONY: check
check: vet lint test

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: clean
clean:
	$(RM_BIN)
