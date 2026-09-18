#ifndef AEGO_SDL_H
#define AEGO_SDL_H

#include <SDL3/SDL.h>

#define AEGO_EV_NONE          -1
#define AEGO_EV_QUIT           0
#define AEGO_EV_RESIZED        1
#define AEGO_EV_FOCUS_GAINED   2
#define AEGO_EV_FOCUS_LOST     3
#define AEGO_EV_MINIMIZED      4
#define AEGO_EV_RESTORED       5
#define AEGO_EV_KEY_DOWN       6
#define AEGO_EV_KEY_UP         7
#define AEGO_EV_TEXT           8
#define AEGO_EV_MOUSE_MOVE     9
#define AEGO_EV_MOUSE_DOWN    10
#define AEGO_EV_MOUSE_UP      11
#define AEGO_EV_WHEEL         12

#define AEGO_MOD_SHIFT  1
#define AEGO_MOD_CTRL   2
#define AEGO_MOD_ALT    4
#define AEGO_MOD_SUPER  8
#define AEGO_MOD_CAPS  16
#define AEGO_MOD_NUM   32

#define AEGO_BTN_LEFT   0
#define AEGO_BTN_RIGHT  1
#define AEGO_BTN_MIDDLE 2
#define AEGO_BTN_X1     3
#define AEGO_BTN_X2     4
#define AEGO_BTN_OTHER  7

#define AEGO_TEXT_CAPACITY 32

typedef struct {
	int kind;
	Uint32 scancode;
	Uint32 key;
	Uint32 mod;
	int repeat;
	int button;
	float x, y, dx, dy, wx, wy;
	int textn;
	char text[AEGO_TEXT_CAPACITY];
} aego_event;

typedef struct {
	int lw, lh, pw, ph;
	float density, scale;
} aego_metrics;

bool aego_init(void);
void aego_quit(void);
const char* aego_error(void);
const char* aego_video_driver(void);
Uint64 aego_ticks_ns(void);

int aego_version(void);
int aego_version_major(int v);
int aego_version_minor(int v);
int aego_version_micro(int v);

bool aego_gl_setup(int debug, int srgb);
SDL_Window* aego_create_window(const char* title, int w, int h, int resizable, int fullscreen, int highdpi);
aego_metrics aego_window_metrics(SDL_Window* w);
void aego_set_title(SDL_Window* w, const char* s, size_t n);

int aego_set_swap_interval(int interval);
int aego_get_swap_interval(void);
void* aego_gl_proc(const char* name);

int aego_poll(aego_event* e);

#endif