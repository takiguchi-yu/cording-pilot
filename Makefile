.PHONY: build run lint test ollama-serve ollama-pull

# Go packages excluding paths managed by Nix/direnv
GO_PKGS := $(shell go list ./... | grep -v '\.direnv')
OLLAMA_MODEL ?= gemma3:4b
OLLAMA_LOG ?= /tmp/ollama-serve.log

## build: compile all packages
build:
	go build $(GO_PKGS)

## run: execute the orchestrator with a sample requirement
run:
	go run ./cmd/orchestrator "文字列を逆順にする関数をpkgに追加してください"

## test: run the unit tests
test:
	go test $(GO_PKGS)

## lint: run golangci-lint (requires golangci-lint to be installed)
lint:
	golangci-lint run ./...

## ollama-serve: start Ollama server in background if not running
ollama-serve:
	@if pgrep -f "ollama serve" >/dev/null; then \
		echo "Ollama server is already running."; \
	else \
		OLLAMA_NUM_PARALLEL=1 OLLAMA_MAX_QUEUE=1 nohup ollama serve >$(OLLAMA_LOG) 2>&1 & \
		echo "Ollama server started in background. log=$(OLLAMA_LOG)"; \
	fi

## ollama-stop: stop Ollama server
ollama-stop:
	@if pgrep -f "ollama serve" >/dev/null; then \
		pkill -f "ollama serve"; \
		echo "Ollama server stopped."; \
	else \
		echo "Ollama server is not running."; \
	fi

## ollama-pull: ensure server is up and pull the recommended coding model
ollama-pull: ollama-serve
	@echo "Pulling model: $(OLLAMA_MODEL)"
	@ollama pull $(OLLAMA_MODEL)
