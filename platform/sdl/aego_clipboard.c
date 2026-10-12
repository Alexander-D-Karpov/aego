#include <stdlib.h>
#include "aego_sdl.h"

typedef struct {
	int n;
	char **mimes;
	void **data;
	size_t *lens;
} aego_clip;

static void aego_clip_release(char **mimes, void **data, size_t *lens, int n) {
	for (int i = 0; i < n; i++) {
		free(mimes[i]);
		free(data[i]);
	}
	free(mimes);
	free(data);
	free(lens);
}

static const void *SDLCALL aego_clip_data(void *userdata, const char *mime, size_t *size) {
	aego_clip *c = (aego_clip *)userdata;
	for (int i = 0; i < c->n; i++) {
		if (SDL_strcmp(c->mimes[i], mime) == 0) {
			*size = c->lens[i];
			return c->data[i];
		}
	}
	*size = 0;
	return NULL;
}

static void SDLCALL aego_clip_cleanup(void *userdata) {
	aego_clip *c = (aego_clip *)userdata;
	aego_clip_release(c->mimes, c->data, c->lens, c->n);
	free(c);
}

bool aego_set_clipboard_items(char **mimes, void **data, size_t *lens, int n) {
	aego_clip *c = (aego_clip *)malloc(sizeof(aego_clip));
	if (c == NULL) {
		aego_clip_release(mimes, data, lens, n);
		return false;
	}
	c->n = n;
	c->mimes = mimes;
	c->data = data;
	c->lens = lens;
	return SDL_SetClipboardData(aego_clip_data, aego_clip_cleanup, c, (const char **)mimes, (size_t)n);
}
