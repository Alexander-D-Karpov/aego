//go:build aego_gltest && linux && cgo && !aego_nonative

package sdl

/*
#include <SDL3/SDL.h>
#include <stdlib.h>

// SDL_Event is a union. Clear the whole object, including fields of the
// selected event variant, before pushing a synthetic event.
static float test_pointer_x, test_pointer_y;
static bool push_pointer(float x,float y,int kind,int button) {
	SDL_Event e;
	SDL_zero(e);
	if (kind == 0) {
		e.type = SDL_EVENT_MOUSE_MOTION;
		e.motion.x = x;
		e.motion.y = y;
		e.motion.xrel = x - test_pointer_x;
		e.motion.yrel = y - test_pointer_y;
	} else {
		e.type = kind == 1 ? SDL_EVENT_MOUSE_BUTTON_DOWN : SDL_EVENT_MOUSE_BUTTON_UP;
		e.button.x = x;
		e.button.y = y;
		e.button.button = button;
		e.button.down = kind == 1;
		e.button.clicks = 1;
	}
	bool pushed = SDL_PushEvent(&e);
	if (pushed) {
		test_pointer_x = x;
		test_pointer_y = y;
	}
	return pushed;
}
static bool push_edit_key(int scan, bool down, bool ctrl, bool shift) {
	SDL_Event e;
	SDL_zero(e);
	e.type = down ? SDL_EVENT_KEY_DOWN : SDL_EVENT_KEY_UP;
	e.key.scancode = (SDL_Scancode)scan;
	e.key.down = down;
	e.key.mod = (ctrl ? SDL_KMOD_CTRL : 0) | (shift ? SDL_KMOD_SHIFT : 0);
	return SDL_PushEvent(&e);
}

static bool push_wheel(float x, float y, float steps) {
	SDL_Event e;
	SDL_zero(e);
	e.type = SDL_EVENT_MOUSE_WHEEL;
	e.wheel.mouse_x = x;
	e.wheel.mouse_y = y;
	e.wheel.y = steps;
	return SDL_PushEvent(&e);
}
// SDL3's text event holds a pointer. Keep storage alive until the test drives
// the next event loop step; tests push one text event at a time.
static char test_text[256];
static bool push_text(const char *text) {
	SDL_Event e;
	SDL_zero(e);
	if (SDL_strlen(text) >= sizeof(test_text)) return false;
	SDL_strlcpy(test_text, text, sizeof(test_text));
	e.type = SDL_EVENT_TEXT_INPUT;
	e.text.text = test_text;
	return SDL_PushEvent(&e);
}
// Keep the dropped path alive until SDL has delivered the queued events.
static char test_drop[4096];
static bool push_drop_file(const char *path, float x, float y) {
	SDL_Event e;
	if (SDL_strlen(path) >= sizeof(test_drop)) return false;
	SDL_strlcpy(test_drop, path, sizeof(test_drop));
	SDL_zero(e);
	e.type = SDL_EVENT_DROP_FILE;
	e.drop.x = x;
	e.drop.y = y;
	e.drop.data = test_drop;
	if (!SDL_PushEvent(&e)) return false;
	SDL_zero(e);
	e.type = SDL_EVENT_DROP_COMPLETE;
	e.drop.x = x;
	e.drop.y = y;
	return SDL_PushEvent(&e);
}
*/
import "C"

import (
	"unsafe"

	"aego/platform/input"
)

func TestPointerEvent(x, y float32, kind int) bool {
	return bool(C.push_pointer(C.float(x), C.float(y), C.int(kind), C.SDL_BUTTON_LEFT))
}

func TestRightPointerEvent(x, y float32, kind int) bool {
	return bool(C.push_pointer(C.float(x), C.float(y), C.int(kind), C.SDL_BUTTON_RIGHT))
}

func TestEditKey(k input.Scancode, down, ctrl, shift bool) bool {
	return bool(C.push_edit_key(C.int(k), C.bool(down), C.bool(ctrl), C.bool(shift)))
}

func TestWheelEvent(x, y, steps float32) bool {
	return bool(C.push_wheel(C.float(x), C.float(y), C.float(steps)))
}

func TestTextInput(text string) bool {
	c := C.CString(text)
	defer C.free(unsafe.Pointer(c))
	return bool(C.push_text(c))
}

func TestDropFile(path string, x, y float32) bool {
	c := C.CString(path)
	defer C.free(unsafe.Pointer(c))
	return bool(C.push_drop_file(c, C.float(x), C.float(y)))
}
