//go:build darwin

package sdl

/*
#include <stdlib.h>
void aego_menu_begin(void);
void aego_menu_push(const char* title);
void aego_menu_pop(void);
void aego_menu_item(int tag, const char* label, const char* key, unsigned long long mods, int enabled, int checked);
void aego_menu_separator(void);
void aego_menu_role(int role, const char* label);
void aego_menu_commit(void);
int aego_menu_take(int* out, int max);
*/
import "C"

import (
	"strconv"
	"strings"
	"unicode/utf8"
	"unsafe"

	"aego/platform"
)

const (
	modShift   uint64 = 1 << 17
	modControl uint64 = 1 << 18
	modOption  uint64 = 1 << 19
	modCommand uint64 = 1 << 20
)

var glyphMods = map[rune]uint64{'⌘': modCommand, '⇧': modShift, '⌥': modOption, '⌃': modControl}

var namedKeys = map[string]rune{
	"space": ' ', "enter": '\r', "return": '\r', "tab": '\t', "esc": 0x1b, "escape": 0x1b,
	"backspace": 0x7f, "delete": 0xF728, "del": 0xF728,
	"left": 0xF702, "right": 0xF703, "up": 0xF700, "down": 0xF701,
	"home": 0xF729, "end": 0xF72B, "pageup": 0xF72C, "pagedown": 0xF72D,
	"←": 0xF702, "→": 0xF703, "↑": 0xF700, "↓": 0xF701,
}

func keyName(k string) string {
	k = strings.TrimSpace(k)
	low := strings.ToLower(k)
	if r, ok := namedKeys[low]; ok {
		return string(r)
	}
	if len(low) >= 2 && low[0] == 'f' {
		if n, err := strconv.Atoi(low[1:]); err == nil && n >= 1 && n <= 24 {
			return string(rune(0xF704 + n - 1))
		}
	}
	if utf8.RuneCountInString(k) == 1 {
		return low
	}
	return ""
}

func keyEquivalent(s string) (string, uint64) {
	s = strings.TrimSpace(s)
	var mods uint64
	for len(s) > 0 {
		r, n := utf8.DecodeRuneInString(s)
		m, ok := glyphMods[r]
		if !ok {
			break
		}
		mods |= m
		s = s[n:]
	}
	if s == "" {
		return "", 0
	}
	parts := strings.Split(s, "+")
	for _, f := range parts[:len(parts)-1] {
		switch strings.ToLower(strings.TrimSpace(f)) {
		case "cmd", "command", "primary", "super", "meta":
			mods |= modCommand
		case "ctrl", "control":
			mods |= modControl
		case "shift":
			mods |= modShift
		case "alt", "option", "opt":
			mods |= modOption
		}
	}
	k := keyName(parts[len(parts)-1])
	if k == "" {
		return "", 0
	}
	return k, mods
}

func cbool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

func pushMenu(title string, items []platform.MenuItem) {
	ct := C.CString(title)
	C.aego_menu_push(ct)
	C.free(unsafe.Pointer(ct))
	for _, it := range items {
		switch {
		case it.Separator:
			C.aego_menu_separator()
		case it.Role != platform.RoleNone:
			cl := C.CString(it.Label)
			C.aego_menu_role(C.int(it.Role), cl)
			C.free(unsafe.Pointer(cl))
		case len(it.Items) > 0:
			pushMenu(it.Label, it.Items)
		default:
			key, mods := keyEquivalent(it.Shortcut)
			cl, ck := C.CString(it.Label), C.CString(key)
			C.aego_menu_item(C.int(it.ID), cl, ck, C.ulonglong(mods), cbool(it.Enabled), cbool(it.Checked))
			C.free(unsafe.Pointer(cl))
			C.free(unsafe.Pointer(ck))
		}
	}
	C.aego_menu_pop()
}

func (w *Window) SetMenuBar(app string, menus []platform.Menu) {
	C.aego_menu_begin()
	for i, m := range menus {
		title := m.Title
		if i == 0 && title == "" {
			title = app
		}
		pushMenu(title, m.Items)
	}
	C.aego_menu_commit()
}

func (w *Window) MenuClicks(dst []int) []int {
	var buf [64]C.int
	n := int(C.aego_menu_take(&buf[0], 64))
	for i := 0; i < n; i++ {
		dst = append(dst, int(buf[i]))
	}
	return dst
}

var _ platform.MenuBar = (*Window)(nil)
