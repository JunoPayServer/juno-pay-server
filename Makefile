.PHONY: build rust-build rust-test test test-unit test-integration test-e2e admin-deps admin-build admin-embed admin-test-unit admin-test-e2e demo-deps demo-build demo-test-unit demo-test-e2e fmt tidy clean

TESTFLAGS ?=
GO_PACKAGES := $(shell go list ./... | grep -v '/node_modules/')

ifneq ($(JUNO_TEST_LOG),)
TESTFLAGS += -v
endif

BIN_DIR := bin
BIN := $(BIN_DIR)/juno-pay-server
RUST_MANIFEST := rust/keys/Cargo.toml
ADMIN_DIR := admin-dashboard
NODE_STAMP := node_modules/.install-stamp
ADMIN_EMBED_DIR := internal/api/adminui_dist
DEMO_DIR := demo-app

admin-embed: admin-build
	rm -rf $(ADMIN_EMBED_DIR)
	mkdir -p $(ADMIN_EMBED_DIR)
	cp -R $(ADMIN_DIR)/out/* $(ADMIN_EMBED_DIR)/

build: rust-build admin-embed
	@mkdir -p $(BIN_DIR)
	go build -tags=adminui -o $(BIN) ./cmd/juno-pay-server

rust-build:
	cargo build --release --manifest-path $(RUST_MANIFEST)

rust-test:
	cargo test --manifest-path $(RUST_MANIFEST)

test-unit: admin-embed
	CGO_ENABLED=0 go test $(TESTFLAGS) -tags=adminui $(GO_PACKAGES)

test-integration:
	$(MAKE) rust-build
	go test $(TESTFLAGS) -tags=integration ./...

test-e2e:
	$(MAKE) build
	go test $(TESTFLAGS) -tags=e2e ./...

$(NODE_STAMP): package.json package-lock.json $(ADMIN_DIR)/package.json $(DEMO_DIR)/package.json packages/ui/package.json
	npm ci
	@mkdir -p $(@D)
	@touch $(NODE_STAMP)

admin-deps: $(NODE_STAMP)

admin-build: admin-deps
	npm --workspace $(ADMIN_DIR) run build

admin-test-unit: admin-deps
	npm --workspace $(ADMIN_DIR) test

admin-test-e2e: admin-deps
	npm --workspace $(ADMIN_DIR) run test:e2e

demo-deps: $(NODE_STAMP)

demo-build: demo-deps
	npm --workspace $(DEMO_DIR) run build

demo-test-unit: demo-deps
	npm --workspace $(DEMO_DIR) test

demo-test-e2e: demo-deps
	npm --workspace $(DEMO_DIR) run test:e2e

test: rust-test test-unit admin-test-unit demo-test-unit test-integration test-e2e admin-test-e2e demo-test-e2e

fmt:
	gofmt -w .

tidy:
	go mod tidy

clean:
	rm -rf $(BIN_DIR)
	rm -rf rust/keys/target
