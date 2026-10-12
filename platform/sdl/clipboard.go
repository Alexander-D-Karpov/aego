package sdl

/*
#include <stdlib.h>
#include "aego_sdl.h"

bool aego_set_clipboard_items(char **mimes, void **data, size_t *lens, int n);
*/
import "C"

import (
	"unsafe"

	"aego/platform"
)

func (w *Window) SetClipboardItems(items []platform.ClipItem) bool {
	n := 0
	for _, it := range items {
		if it.Mime != "" && len(it.Data) > 0 {
			n++
		}
	}
	if n == 0 {
		return false
	}
	ptr := C.size_t(unsafe.Sizeof(uintptr(0)))
	mimes := (**C.char)(C.malloc(C.size_t(n) * ptr))
	datas := (*unsafe.Pointer)(C.malloc(C.size_t(n) * ptr))
	lens := (*C.size_t)(C.malloc(C.size_t(n) * C.size_t(unsafe.Sizeof(C.size_t(0)))))
	ms, ds, ls := unsafe.Slice(mimes, n), unsafe.Slice(datas, n), unsafe.Slice(lens, n)
	i := 0
	for _, it := range items {
		if it.Mime == "" || len(it.Data) == 0 {
			continue
		}
		ms[i] = C.CString(it.Mime)
		ds[i] = C.CBytes(it.Data)
		ls[i] = C.size_t(len(it.Data))
		i++
	}
	return bool(C.aego_set_clipboard_items(mimes, datas, lens, C.int(n)))
}

func (w *Window) ClipboardMimes() []string {
	var n C.size_t
	p := C.SDL_GetClipboardMimeTypes(&n)
	if p == nil {
		return nil
	}
	defer C.SDL_free(unsafe.Pointer(p))
	list := unsafe.Slice(p, int(n))
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = C.GoString(s)
	}
	return out
}

var _ platform.ClipboardItems = (*Window)(nil)
