#include <dlfcn.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

typedef struct {
    void *ptr;
    size_t len;
} cliproxy_buffer;

typedef int (*cliproxy_host_call_fn)(void *, const char *, const uint8_t *, size_t, cliproxy_buffer *);
typedef void (*cliproxy_host_free_fn)(void *, size_t);

typedef struct {
    uint32_t abi_version;
    void *host_ctx;
    cliproxy_host_call_fn call;
    cliproxy_host_free_fn free_buffer;
} cliproxy_host_api;

typedef int (*cliproxy_plugin_call_fn)(char *, uint8_t *, size_t, cliproxy_buffer *);
typedef void (*cliproxy_plugin_free_fn)(void *, size_t);
typedef void (*cliproxy_plugin_shutdown_fn)(void);

typedef struct {
    uint32_t abi_version;
    cliproxy_plugin_call_fn call;
    cliproxy_plugin_free_fn free_buffer;
    cliproxy_plugin_shutdown_fn shutdown;
} cliproxy_plugin_api;

typedef int (*cliproxy_plugin_init_fn)(cliproxy_host_api *, cliproxy_plugin_api *);

int main(int argc, char **argv) {
    if (argc != 2 && argc != 3) {
        fprintf(stderr, "usage: %s <plugin-library> [expected-version]\n", argv[0]);
        return 2;
    }
    void *library = dlopen(argv[1], RTLD_NOW | RTLD_LOCAL);
    if (library == NULL) {
        fprintf(stderr, "dlopen: %s\n", dlerror());
        return 1;
    }
    cliproxy_plugin_init_fn init = (cliproxy_plugin_init_fn)dlsym(library, "cliproxy_plugin_init");
    if (init == NULL) {
        fprintf(stderr, "missing cliproxy_plugin_init\n");
        dlclose(library);
        return 1;
    }
    cliproxy_host_api host = {0};
    host.abi_version = 1;
    cliproxy_plugin_api plugin = {0};
    if (init(&host, &plugin) != 0 || plugin.abi_version != 1 || plugin.call == NULL || plugin.free_buffer == NULL) {
        fprintf(stderr, "plugin initialization failed\n");
        dlclose(library);
        return 1;
    }
    cliproxy_buffer response = {0};
    char method[] = "plugin.register";
    if (plugin.call(method, NULL, 0, &response) != 0 || response.ptr == NULL || response.len == 0) {
        fprintf(stderr, "plugin.register failed\n");
        dlclose(library);
        return 1;
    }
    char *json = calloc(response.len + 1, 1);
    if (json == NULL) {
        plugin.free_buffer(response.ptr, response.len);
        dlclose(library);
        return 1;
    }
    memcpy(json, response.ptr, response.len);
    plugin.free_buffer(response.ptr, response.len);
    int ok = strstr(json, "\"usage_plugin\":true") != NULL && strstr(json, "\"management_api\":true") != NULL;
    if (ok && argc == 3) {
        char version_needle[256];
        int written = snprintf(version_needle, sizeof(version_needle), "\"Version\":\"%s\"", argv[2]);
        ok = written > 0 && (size_t)written < sizeof(version_needle) && strstr(json, version_needle) != NULL;
    }
    free(json);
    if (plugin.shutdown != NULL) {
        plugin.shutdown();
    }
    dlclose(library);
    if (!ok) {
        fprintf(stderr, "registration is missing expected capabilities or version\n");
        return 1;
    }
    puts("ABI smoke test passed");
    return 0;
}
