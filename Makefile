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

# A tagged checkout gets the exact release version; all other local builds are
# intentionally marked as dev. Release CI passes the tag explicitly.
PLUGIN_VERSION ?= $(or $(shell git describe --tags --exact-match --match 'v[0-9]*.[0-9]*.[0-9]*' 2>/dev/null | sed 's/^v//'),dev)
GO_LDFLAGS := -X main.pluginVersion=$(PLUGIN_VERSION)

.PHONY: test build smoke preview clean

test:
	$(GO) test ./...

build:
	mkdir -p $(BIN_DIR)
	$(GO) build -ldflags "$(GO_LDFLAGS)" -buildmode=c-shared -o $(BIN_DIR)/$(PLUGIN_ID).$(PLUGIN_EXT) ./cmd/plugin

smoke: build
	$(CC) -Wall -Wextra -Werror -o $(BIN_DIR)/abi-smoke tests/abi_smoke.c $(DL_LIBS)
	$(BIN_DIR)/abi-smoke $(BIN_DIR)/$(PLUGIN_ID).$(PLUGIN_EXT) $(PLUGIN_VERSION)

preview:
	$(GO) run ./cmd/preview

clean:
	rm -rf $(BIN_DIR)
