GO ?= go
BIN_DIR ?= bin
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X silent-devops/internal/app.Version=$(VERSION) -X silent-devops/internal/app.Commit=$(COMMIT) -X silent-devops/internal/app.Date=$(DATE)
COMMANDS := agent validator client
PROTOC_GEN_GO_VERSION := v1.36.11
PROTOC_GEN_GO_GRPC_VERSION := v1.6.1
TOOLS_DIR := $(CURDIR)/.tools

.PHONY: build build-linux fmt fmt-check vet test test-race test-e2e test-e2e-easypanel tools generate generate-check clean
build:
	@mkdir -p $(BIN_DIR)
	@for command in $(COMMANDS); do $(GO) build -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$$command ./cmd/$$command; done
build-linux:
	@mkdir -p $(BIN_DIR)
	@for arch in amd64 arm64; do for command in $(COMMANDS); do GOOS=linux GOARCH=$$arch $(GO) build -ldflags '$(LDFLAGS)' -o $(BIN_DIR)/$$command-linux-$$arch ./cmd/$$command; done; done
fmt:
	gofmt -w $$(find . -name '*.go' -type f)
fmt-check:
	@test -z "$$(gofmt -l $$(find . -name '*.go' -type f))"
vet:
	$(GO) vet ./...
test:
	$(GO) test ./...
test-race:
	$(GO) test -race ./...
test-e2e:
	./integration/run.sh
test-e2e-easypanel:
	./integration/easypanel/run.sh
tools:
	@mkdir -p $(TOOLS_DIR)
	@test -x $(TOOLS_DIR)/protoc-gen-go || GOBIN=$(TOOLS_DIR) $(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	@test -x $(TOOLS_DIR)/protoc-gen-go-grpc || GOBIN=$(TOOLS_DIR) $(GO) install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)
generate: tools
	PATH="$(TOOLS_DIR):$$PATH" protoc -I . --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative api/devops/v1/devops.proto
generate-check:
	@tmp=$$(mktemp -d); trap 'rm -rf $$tmp' EXIT; cp api/devops/v1/*.pb.go $$tmp/; $(MAKE) generate >/dev/null; diff -u $$tmp/devops.pb.go api/devops/v1/devops.pb.go; diff -u $$tmp/devops_grpc.pb.go api/devops/v1/devops_grpc.pb.go
clean:
	rm -rf $(BIN_DIR) $(TOOLS_DIR)
