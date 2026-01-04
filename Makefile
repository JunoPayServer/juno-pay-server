.PHONY: build rust-build rust-test test test-unit test-integration test-e2e admin-deps admin-build admin-embed admin-test-unit admin-test-e2e demo-deps demo-build demo-test-unit demo-test-e2e fmt tidy clean

TESTFLAGS ?=

ifneq ($(JUNO_TEST_LOG),)
TESTFLAGS += -v
endif

BIN_DIR := bin
BIN := $(BIN_DIR)/juno-pay-server
RUST_MANIFEST := rust/keys/Cargo.toml
ADMIN_DIR := admin-dashboard
ADMIN_STAMP := $(ADMIN_DIR)/node_modules/.install-stamp
ADMIN_EMBED_DIR := internal/api/adminui_dist
DEMO_DIR := demo-app
DEMO_STAMP := $(DEMO_DIR)/node_modules/.install-stamp

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

test-unit:
	CGO_ENABLED=0 go test $(TESTFLAGS) -tags=adminui ./...

test-integration:
	$(MAKE) rust-build
	go test $(TESTFLAGS) -tags=integration ./...

test-e2e:
	$(MAKE) build
	go test $(TESTFLAGS) -tags=e2e ./...

$(ADMIN_STAMP): $(ADMIN_DIR)/package.json $(ADMIN_DIR)/package-lock.json
	cd $(ADMIN_DIR) && npm ci
	@mkdir -p $(@D)
	@touch $(ADMIN_STAMP)

admin-deps: $(ADMIN_STAMP)

admin-build: admin-deps
	cd $(ADMIN_DIR) && npm run build

admin-test-unit: admin-deps
	cd $(ADMIN_DIR) && npm test

admin-test-e2e: admin-deps
	cd $(ADMIN_DIR) && npm run test:e2e

$(DEMO_STAMP): $(DEMO_DIR)/package.json $(DEMO_DIR)/package-lock.json
	cd $(DEMO_DIR) && npm ci
	@mkdir -p $(@D)
	@touch $(DEMO_STAMP)

demo-deps: $(DEMO_STAMP)

demo-build: demo-deps
	cd $(DEMO_DIR) && npm run build

demo-test-unit: demo-deps
	cd $(DEMO_DIR) && npm test

demo-test-e2e: demo-deps
	cd $(DEMO_DIR) && npm run test:e2e

test: rust-test test-unit admin-test-unit demo-test-unit test-integration test-e2e admin-test-e2e demo-test-e2e

fmt:
	gofmt -w .

tidy:
	go mod tidy

clean:
	rm -rf $(BIN_DIR)
	rm -rf rust/keys/target
