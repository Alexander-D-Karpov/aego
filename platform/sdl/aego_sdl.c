#include <string.h>

#include "aego_sdl.h"

static char aego_title_buf[1024];

static Uint32 aego_mods(SDL_Keymod m) {
	Uint32 r = 0;
	if (m & SDL_KMOD_SHIFT) r |= AEGO_MOD_SHIFT;
	if (m & SDL_KMOD_CTRL)  r |= AEGO_MOD_CTRL;
	if (m & SDL_KMOD_ALT)   r |= AEGO_MOD_ALT;
	if (m & SDL_KMOD_GUI)   r |= AEGO_MOD_SUPER;
	if (m & SDL_KMOD_CAPS)  r |= AEGO_MOD_CAPS;
	if (m & SDL_KMOD_NUM)   r |= AEGO_MOD_NUM;
	return r;
}

static int aego_button(Uint8 b) {
	switch (b) {
	case SDL_BUTTON_LEFT:   return AEGO_BTN_LEFT;
	case SDL_BUTTON_RIGHT:  return AEGO_BTN_RIGHT;
	case SDL_BUTTON_MIDDLE: return AEGO_BTN_MIDDLE;
	case SDL_BUTTON_X1:     return AEGO_BTN_X1;
	case SDL_BUTTON_X2:     return AEGO_BTN_X2;
	}
	return AEGO_BTN_OTHER;
}

bool aego_init(void) {
	return SDL_Init(SDL_INIT_VIDEO | SDL_INIT_EVENTS);
}

void aego_quit(void) {
	SDL_Quit();
}

const char* aego_error(void) {
	return SDL_GetError();
}

const char* aego_video_driver(void) {
	const char* d = SDL_GetCurrentVideoDriver();
	return d ? d : "unknown";
}

Uint64 aego_ticks_ns(void) {
	return SDL_GetTicksNS();
}

int aego_version(void) { return SDL_GetVersion(); }
int aego_version_major(int v) { return SDL_VERSIONNUM_MAJOR(v); }
int aego_version_minor(int v) { return SDL_VERSIONNUM_MINOR(v); }
int aego_version_micro(int v) { return SDL_VERSIONNUM_MICRO(v); }

bool aego_gl_setup(int debug, int srgb) {
	bool ok = true;
	int flags = SDL_GL_CONTEXT_FORWARD_COMPATIBLE_FLAG;
	if (debug) flags |= SDL_GL_CONTEXT_DEBUG_FLAG;
	ok = SDL_GL_SetAttribute(SDL_GL_CONTEXT_MAJOR_VERSION, 3) && ok;
	ok = SDL_GL_SetAttribute(SDL_GL_CONTEXT_MINOR_VERSION, 3) && ok;
	ok = SDL_GL_SetAttribute(SDL_GL_CONTEXT_PROFILE_MASK, SDL_GL_CONTEXT_PROFILE_CORE) && ok;
	ok = SDL_GL_SetAttribute(SDL_GL_CONTEXT_FLAGS, flags) && ok;
	ok = SDL_GL_SetAttribute(SDL_GL_FRAMEBUFFER_SRGB_CAPABLE, srgb ? 1 : 0) && ok;
	ok = SDL_GL_SetAttribute(SDL_GL_DOUBLEBUFFER, 1) && ok;
	ok = SDL_GL_SetAttribute(SDL_GL_RED_SIZE, 8) && ok;
	ok = SDL_GL_SetAttribute(SDL_GL_GREEN_SIZE, 8) && ok;
	ok = SDL_GL_SetAttribute(SDL_GL_BLUE_SIZE, 8) && ok;
	ok = SDL_GL_SetAttribute(SDL_GL_ALPHA_SIZE, 8) && ok;
	ok = SDL_GL_SetAttribute(SDL_GL_DEPTH_SIZE, 24) && ok;
	ok = SDL_GL_SetAttribute(SDL_GL_STENCIL_SIZE, 8) && ok;
	return ok;
}

SDL_Window* aego_create_window(const char* title, int w, int h, int resizable, int fullscreen, int highdpi) {
	SDL_WindowFlags flags = SDL_WINDOW_OPENGL | SDL_WINDOW_HIDDEN;
	if (resizable) flags |= SDL_WINDOW_RESIZABLE;
	if (fullscreen) flags |= SDL_WINDOW_FULLSCREEN;
	if (highdpi) flags |= SDL_WINDOW_HIGH_PIXEL_DENSITY;
	return SDL_CreateWindow(title, w, h, flags);
}

aego_metrics aego_window_metrics(SDL_Window* w) {
	aego_metrics m;
	memset(&m, 0, sizeof(m));
	SDL_GetWindowSize(w, &m.lw, &m.lh);
	SDL_GetWindowSizeInPixels(w, &m.pw, &m.ph);
	m.density = SDL_GetWindowPixelDensity(w);
	m.scale = SDL_GetWindowDisplayScale(w);
	return m;
}

void aego_set_title(SDL_Window* w, const char* s, size_t n) {
	if (n >= sizeof(aego_title_buf)) n = sizeof(aego_title_buf) - 1;
	if (n > 0) memcpy(aego_title_buf, s, n);
	aego_title_buf[n] = 0;
	SDL_SetWindowTitle(w, aego_title_buf);
}

int aego_set_swap_interval(int interval) {
	return SDL_GL_SetSwapInterval(interval) ? 1 : 0;
}

int aego_get_swap_interval(void) {
	int v = 0;
	SDL_GL_GetSwapInterval(&v);
	return v;
}

void* aego_gl_proc(const char* name) {
	return (void*)SDL_GL_GetProcAddress(name);
}

int aego_poll(aego_event* e) {
	SDL_Event ev;
	if (!SDL_PollEvent(&ev)) return 0;
	memset(e, 0, sizeof(*e));
	e->kind = AEGO_EV_NONE;
	switch (ev.type) {
	case SDL_EVENT_QUIT:
	case SDL_EVENT_WINDOW_CLOSE_REQUESTED:
		e->kind = AEGO_EV_QUIT;
		break;
	case SDL_EVENT_WINDOW_RESIZED:
	case SDL_EVENT_WINDOW_PIXEL_SIZE_CHANGED:
	case SDL_EVENT_WINDOW_DISPLAY_SCALE_CHANGED:
	case SDL_EVENT_WINDOW_ENTER_FULLSCREEN:
	case SDL_EVENT_WINDOW_LEAVE_FULLSCREEN:
		e->kind = AEGO_EV_RESIZED;
		break;
	case SDL_EVENT_WINDOW_FOCUS_GAINED:
		e->kind = AEGO_EV_FOCUS_GAINED;
		break;
	case SDL_EVENT_WINDOW_FOCUS_LOST:
		e->kind = AEGO_EV_FOCUS_LOST;
		break;
	case SDL_EVENT_WINDOW_MINIMIZED:
		e->kind = AEGO_EV_MINIMIZED;
		break;
	case SDL_EVENT_WINDOW_RESTORED:
		e->kind = AEGO_EV_RESTORED;
		break;
	case SDL_EVENT_KEY_DOWN:
	case SDL_EVENT_KEY_UP:
		e->kind = ev.type == SDL_EVENT_KEY_DOWN ? AEGO_EV_KEY_DOWN : AEGO_EV_KEY_UP;
		e->scancode = (Uint32)ev.key.scancode;
		e->key = (Uint32)ev.key.key;
		e->mod = aego_mods(ev.key.mod);
		e->repeat = ev.key.repeat ? 1 : 0;
		break;
	case SDL_EVENT_TEXT_INPUT:
		e->kind = AEGO_EV_TEXT;
		if (ev.text.text) {
			size_t n = strlen(ev.text.text);
			if (n > AEGO_TEXT_CAPACITY) n = AEGO_TEXT_CAPACITY;
			memcpy(e->text, ev.text.text, n);
			e->textn = (int)n;
		}
		break;
	case SDL_EVENT_MOUSE_MOTION:
		e->kind = AEGO_EV_MOUSE_MOVE;
		e->x = ev.motion.x;
		e->y = ev.motion.y;
		e->dx = ev.motion.xrel;
		e->dy = ev.motion.yrel;
		break;
	case SDL_EVENT_MOUSE_BUTTON_DOWN:
	case SDL_EVENT_MOUSE_BUTTON_UP:
		e->kind = ev.type == SDL_EVENT_MOUSE_BUTTON_DOWN ? AEGO_EV_MOUSE_DOWN : AEGO_EV_MOUSE_UP;
		e->x = ev.button.x;
		e->y = ev.button.y;
		e->button = aego_button(ev.button.button);
		break;
	case SDL_EVENT_MOUSE_WHEEL:
		e->kind = AEGO_EV_WHEEL;
		e->wx = ev.wheel.x;
		e->wy = ev.wheel.y;
		if (ev.wheel.direction == SDL_MOUSEWHEEL_FLIPPED) {
			e->wx = -e->wx;
			e->wy = -e->wy;
		}
		e->x = ev.wheel.mouse_x;
		e->y = ev.wheel.mouse_y;
		break;
	}
	return 1;
}