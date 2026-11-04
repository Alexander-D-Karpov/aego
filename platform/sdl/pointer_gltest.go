//go:build aego_gltest && linux && cgo && !aego_nonative

package sdl

/*
#include <SDL3/SDL.h>

// SDL_Event is a union. Clear the whole object, including fields of the
// selected event variant, before pushing a synthetic event.
static bool push_pointer(float x,float y,int kind,int button) {
	SDL_Event e;
	SDL_zero(e);
	if (kind == 0) {
		e.type = SDL_EVENT_MOUSE_MOTION;
		e.motion.x = x;
		e.motion.y = y;
	} else {
		e.type = kind == 1 ? SDL_EVENT_MOUSE_BUTTON_DOWN : SDL_EVENT_MOUSE_BUTTON_UP;
		e.button.x = x;
		e.button.y = y;
		e.button.button = button;
		e.button.down = kind == 1;
		e.button.clicks = 1;
	}
	return SDL_PushEvent(&e);
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
*/
import "C"

import "aego/platform/input"

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
