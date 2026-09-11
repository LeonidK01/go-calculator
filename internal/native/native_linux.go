//go:build linux && cgo

package native

/*
#cgo LDFLAGS: -ldl
#include <dlfcn.h>
#include <stdint.h>
#include <stdlib.h>
#include <stdio.h>

typedef int64_t (*operation)(int64_t, int64_t);
typedef struct { void *c; void *rust; operation add; operation sub; } libraries;

static libraries *open_libraries(const char *c, const char *rust, char *err, size_t n) {
    libraries *p = calloc(1, sizeof(libraries));
    if (!p) { snprintf(err, n, "out of memory"); return NULL; }
    p->c = dlopen(c, RTLD_NOW | RTLD_LOCAL);
    if (!p->c) goto fail;
    p->rust = dlopen(rust, RTLD_NOW | RTLD_LOCAL);
    if (!p->rust) goto fail;
    dlerror();
    p->add = (operation)dlsym(p->c, "add");
    if (!p->add) goto fail;
    p->sub = (operation)dlsym(p->rust, "sub");
    if (!p->sub) goto fail;
    return p;
fail:
    { const char *detail = dlerror(); snprintf(err, n, "%s", detail ? detail : "missing native symbol"); }
    if (p->rust) dlclose(p->rust);
    if (p->c) dlclose(p->c);
    free(p);
    return NULL;
}
static int64_t call_add(libraries *p, int64_t a, int64_t b) { return p->add(a,b); }
static int64_t call_sub(libraries *p, int64_t a, int64_t b) { return p->sub(a,b); }
static int close_libraries(libraries *p) {
    int r = dlclose(p->rust);
    int c = dlclose(p->c);
    free(p);
    return r || c;
}
*/
import "C"

import (
	"fmt"
	"path/filepath"
	"unsafe"
)

type library struct{ ptr *C.libraries }

// Open resolves libraries once, outside the HTTP request path.
func Open(cPath, rustPath string) (Library, error) {
	cAbs, err := filepath.Abs(cPath)
	if err != nil {
		return nil, fmt.Errorf("c library path: %w", err)
	}
	rAbs, err := filepath.Abs(rustPath)
	if err != nil {
		return nil, fmt.Errorf("rust library path: %w", err)
	}
	c, r := C.CString(cAbs), C.CString(rAbs)
	defer C.free(unsafe.Pointer(c))
	defer C.free(unsafe.Pointer(r))
	var detail [1024]C.char
	p := C.open_libraries(c, r, &detail[0], C.size_t(len(detail)))
	if p == nil {
		return nil, fmt.Errorf("load native libraries: %s", C.GoString(&detail[0]))
	}
	return &library{ptr: p}, nil
}

func (l *library) Add(a, b int64) int64 { return int64(C.call_add(l.ptr, C.int64_t(a), C.int64_t(b))) }
func (l *library) Sub(a, b int64) int64 { return int64(C.call_sub(l.ptr, C.int64_t(a), C.int64_t(b))) }
func (l *library) Close() error {
	if l.ptr == nil {
		return nil
	}
	rc := C.close_libraries(l.ptr)
	l.ptr = nil
	if rc != 0 {
		return fmt.Errorf("unload native libraries failed")
	}
	return nil
}
