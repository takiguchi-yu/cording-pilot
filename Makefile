.PHONY: build run lint test

# Go packages excluding paths managed by Nix/direnv
GO_PKGS := $(shell go list ./... | grep -v '\.direnv')

## build: compile all packages
build:
	go build $(GO_PKGS)

## run: execute the orchestrator with a sample requirement
run:
	go run ./cmd/orchestrator "文字列を逆順にする関数"

## test: run the unit tests
test:
	go test $(GO_PKGS)

## lint: run golangci-lint (requires golangci-lint to be installed)
lint:
	golangci-lint run ./...
