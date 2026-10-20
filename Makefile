.DEFAULT_GOAL := build
.PHONY: lint audit test build ci run doctor fmt

BINARY_NAME ?= yt-cli
BUILD_OUTPUT ?= bin/$(BINARY_NAME)
DEV_BUILD_OUTPUT ?= $(HOME)/.local/$(BUILD_OUTPUT)
TEST_DIR ?= ./...
TEST_CASE ?= .
GOLANGCI_LINT_VERSION ?= v2.14.0
ARGS ?=

lint:
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run --allow-parallel-runners

audit:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

test:
	go test -mod=readonly -count=1 -p 1 -failfast -race -run '$(TEST_CASE)' $(TEST_DIR)

build:
	mkdir -p "$(dir $(BUILD_OUTPUT))"
	go build -mod=readonly -o "$(BUILD_OUTPUT)" .

dev-build: build
	mkdir -p "$(dir $(DEV_BUILD_OUTPUT))"
	ln -sf "$(PWD)/$(BUILD_OUTPUT)" "$(DEV_BUILD_OUTPUT)"

ci: lint audit test

run: build
	"$(abspath $(BUILD_OUTPUT))" $(ARGS)

doctor: build
	"$(abspath $(BUILD_OUTPUT))" --doctor

fmt:
	gofmt -w $$(find . -name '*.go' -not -path './.git/*')
