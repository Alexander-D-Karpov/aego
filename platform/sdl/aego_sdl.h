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
#define AEGO_EV_TEXT_EDITING  13
#define AEGO_EV_DROP_BEGIN    14
#define AEGO_EV_DROP_FILE     15
#define AEGO_EV_DROP_TEXT     16
#define AEGO_EV_DROP_COMPLETE 17
#define AEGO_EV_PAD_ADDED     18
#define AEGO_EV_PAD_REMOVED   19
#define AEGO_EV_PAD_DOWN      20
#define AEGO_EV_PAD_UP        21
#define AEGO_EV_PAD_AXIS      22
#define AEGO_EV_AUDIO_ADDED   23
#define AEGO_EV_AUDIO_REMOVED 24
#define AEGO_EV_AUDIO_FORMAT  25
#define AEGO_EV_DIALOG        26
#define AEGO_PAD_BUTTONS      15
#define AEGO_PAD_AXES         6

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
#define AEGO_CURSOR_COUNT  16

typedef struct {
	int kind;
	Uint32 scancode;
	Uint32 key;
	Uint32 mod;
	int repeat;
	int button;
	float x, y, dx, dy, wx, wy;
	int flipped;
	int pad;
	int textn;
	char text[AEGO_TEXT_CAPACITY];
	const char* data;
	int start;
	int length;
	Uint32 device;
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
bool aego_dialog(int id, int kind, SDL_Window* w, const char* start, const char* name, const char* pattern);
bool aego_alert(const char* title, const char* msg);

void aego_text_input_area(SDL_Window* w, int x, int y, int ww, int hh, int cursor);
bool aego_set_clipboard(const char* mime, const void* data, size_t len, const char* text, size_t textlen);
void* aego_get_clipboard(const char* mime, size_t* len);
void aego_set_cursor(int shape);
void aego_free_cursors(void);

typedef struct {
	int w, h;
	float hz;
} aego_mode;

int aego_displays(SDL_DisplayID* out, int max);
bool aego_display_current(SDL_DisplayID id, aego_mode* out);
int aego_display_modes(SDL_DisplayID id, aego_mode* out, int max);
bool aego_set_window_mode(SDL_Window* w, int mode, SDL_DisplayID id, int width, int height, float hz);

SDL_AudioStream* aego_audio_open(int freq, int ch, int frames, int* got_freq, int* got_frames);
bool aego_audio_put(SDL_AudioStream* s, const float* data, int bytes);
int aego_audio_queued(SDL_AudioStream* s);
void aego_audio_pause(SDL_AudioStream* s, int on);
void aego_audio_close(SDL_AudioStream* s);
Uint32 aego_audio_device(SDL_AudioStream* s);

SDL_Surface* aego_icon_surface(int w, int h, const void* rgba);
bool aego_icon_alternate(SDL_Surface* base, SDL_Surface* alt);
bool aego_set_icon(SDL_Window* w, SDL_Surface* s);

const char* aego_gl_string(unsigned int name);
const char* aego_gl_probe(SDL_Window* w);

#endif
