.PHONY: build test test-unit fmt tidy clean

TESTFLAGS ?=

ifneq ($(JUNO_TEST_LOG),)
TESTFLAGS += -v
endif

BIN_DIR := bin
BIN := $(BIN_DIR)/juno-pay-server

build:
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN) ./cmd/juno-pay-server

test-unit:
	go test $(TESTFLAGS) ./...

test: test-unit

fmt:
	gofmt -w .

tidy:
	go mod tidy

clean:
	rm -rf $(BIN_DIR)
