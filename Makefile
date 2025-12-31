.PHONY: build rust-build rust-test test test-unit test-integration test-e2e admin-deps admin-test-unit admin-test-e2e fmt tidy clean

TESTFLAGS ?=

ifneq ($(JUNO_TEST_LOG),)
TESTFLAGS += -v
endif

BIN_DIR := bin
BIN := $(BIN_DIR)/juno-pay-server
RUST_MANIFEST := rust/keys/Cargo.toml
ADMIN_DIR := admin-dashboard
ADMIN_STAMP := $(ADMIN_DIR)/node_modules/.install-stamp

build: rust-build
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN) ./cmd/juno-pay-server

rust-build:
	cargo build --release --manifest-path $(RUST_MANIFEST)

rust-test:
	cargo test --manifest-path $(RUST_MANIFEST)

test-unit:
	CGO_ENABLED=0 go test $(TESTFLAGS) ./...

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

admin-test-unit: admin-deps
	cd $(ADMIN_DIR) && npm test

admin-test-e2e: admin-deps
	cd $(ADMIN_DIR) && npm run test:e2e

test: rust-test test-unit admin-test-unit test-integration test-e2e admin-test-e2e

fmt:
	gofmt -w .

tidy:
	go mod tidy

clean:
	rm -rf $(BIN_DIR)
	rm -rf rust/keys/target
