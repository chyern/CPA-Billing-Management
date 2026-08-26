package main

/*
#cgo linux CFLAGS: -D_GNU_SOURCE
#if defined(__linux__)
#define _GNU_SOURCE
#endif

#include <stdint.h>
#include <stdlib.h>
#include <dlfcn.h>

#cgo linux LDFLAGS: -ldl

typedef struct {
	void* ptr;
	size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_host_free_fn)(void*, size_t);

typedef struct {
	uint32_t abi_version;
	void* host_ctx;
	cliproxy_host_call_fn call;
	cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*cliproxy_plugin_free_fn)(void*, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
	uint32_t abi_version;
	cliproxy_plugin_call_fn call;
	cliproxy_plugin_free_fn free_buffer;
	cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);

static const char* cliproxyPluginLibraryPath(void) {
	Dl_info info;
	if (dladdr((void*)&cliproxyPluginCall, &info) != 0 && info.dli_fname != NULL) {
		return info.dli_fname;
	}
	return NULL;
}
*/
import "C"

import (
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"github.com/chyern/CPA-Billing-Management/internal/abi"
	"github.com/chyern/CPA-Billing-Management/internal/billing"
)

const (
	pluginID     = "cpa-billing-management"
	resourcePath = "/v0/resource/plugins/" + pluginID + "/billing"
	pricingPath  = "/v0/resource/plugins/" + pluginID + "/pricing"
)

// pluginVersion is injected by the build with the release tag. Keeping a
// development fallback makes local builds explicit instead of letting a
// hand-maintained source constant drift from the published artifact.
var pluginVersion = "dev"

var (
	storeMu      sync.Mutex
	store        *billing.Store
	storeErr     error
	storeDataDir string
)

func getBillingStore(configuredDataDir string) (*billing.Store, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	// Tests may inject a store directly; keep that store until a path is
	// explicitly configured.
	if store != nil && storeDataDir == "" && strings.TrimSpace(configuredDataDir) == "" {
		return store, storeErr
	}
	dataDir := billing.ResolveDataDir(configuredDataDir, pluginInstallDir())
	if store != nil && storeDataDir == dataDir {
		return store, storeErr
	}
	if store != nil {
		_ = store.Close()
	}
	store, storeErr = billing.NewStore(dataDir)
	storeDataDir = dataDir
	return store, storeErr
}

func pluginInstallDir() string {
	path := C.GoString(C.cliproxyPluginLibraryPath())
	if strings.TrimSpace(path) == "" {
		return ""
	}
	return filepath.Dir(path)
}

func configuredDataDir(raw []byte) string {
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		const key = "cpa_billing_data_dir:"
		if strings.HasPrefix(line, key) {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, key)), "\"'")
		}
	}
	return ""
}

func main() {}

//export cliproxy_plugin_init
func cliproxy_plugin_init(_ *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if plugin == nil {
		return 1
	}
	plugin.abi_version = C.uint32_t(abi.ABIVersion)
	plugin.call = C.cliproxy_plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.cliproxy_plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.cliproxy_plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, requestLen C.size_t, response *C.cliproxy_buffer) C.int {
	if response != nil {
		response.ptr = nil
		response.len = 0
	}
	if method == nil {
		writeResponse(response, errorEnvelope("invalid_method", "method is required"))
		return 1
	}
	requestBytes := []byte(nil)
	if request != nil && requestLen > 0 {
		if uint64(requestLen) > uint64(1<<31-1) {
			writeResponse(response, errorEnvelope("invalid_request", "request is too large"))
			return 1
		}
		requestBytes = C.GoBytes(unsafe.Pointer(request), C.int(requestLen))
	}
	raw, err := handleMethod(C.GoString(method), requestBytes)
	if err != nil {
		writeResponse(response, errorEnvelope("plugin_error", err.Error()))
		return 1
	}
	writeResponse(response, raw)
	return 0
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, _ C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() {}

func writeResponse(response *C.cliproxy_buffer, raw []byte) {
	if response == nil || len(raw) == 0 {
		return
	}
	ptr := C.CBytes(raw)
	if ptr == nil {
		return
	}
	response.ptr = ptr
	response.len = C.size_t(len(raw))
}
