BINARY  := subrouterctl
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X github.com/Apothic-AI/subrouterctl/internal/cmd.Version=$(VERSION)

.PHONY: all build install test smoke fmt vet lint clean help

all: build ## Build the binary

build: ## Build ./subrouterctl for the current platform
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/subrouterctl

install: ## Install into $(GOBIN)
	go install -trimpath -ldflags '$(LDFLAGS)' ./cmd/subrouterctl

test: ## Run the unit tests
	go test ./...

smoke: build ## Run the live smoke test against the configured account
	./scripts/smoke.sh ./$(BINARY)

fmt: ## Format the source
	gofmt -w .

vet: ## Run go vet
	go vet ./...

lint: fmt vet ## Format and vet

clean: ## Remove build artefacts
	rm -f $(BINARY)

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'
