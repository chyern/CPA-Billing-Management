PLUGIN_ID := cpa-billing-management
BIN_DIR := bin
GO ?= go
CC ?= cc
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
PLUGIN_EXT := dylib
DL_LIBS :=
else
PLUGIN_EXT := so
DL_LIBS := -ldl
endif

.PHONY: test build smoke preview clean

test:
	$(GO) test ./...

build:
	mkdir -p $(BIN_DIR)
	$(GO) build -buildmode=c-shared -o $(BIN_DIR)/$(PLUGIN_ID).$(PLUGIN_EXT) ./cmd/plugin

smoke: build
	$(CC) -Wall -Wextra -Werror -o $(BIN_DIR)/abi-smoke tests/abi_smoke.c $(DL_LIBS)
	CPA_BILLING_DATA_DIR="$(CURDIR)/$(BIN_DIR)/smoke-data" $(BIN_DIR)/abi-smoke "$(CURDIR)/$(BIN_DIR)/$(PLUGIN_ID).$(PLUGIN_EXT)"

preview:
	$(GO) run ./cmd/preview

clean:
	rm -rf $(BIN_DIR)
