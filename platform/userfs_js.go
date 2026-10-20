//go:build js

package platform

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall/js"
	"time"
)

const (
	storagePrefix = "aego:"
	storageBase   = "/local"
)

type webFiles struct {
	store js.Value
	mem   map[string]string
}

var (
	webOnce  sync.Once
	webStore *webFiles
)

func UserFiles() UserFS {
	webOnce.Do(func() { webStore = newWebFiles() })
	return webStore
}

func configBase() (string, error) {
	return storageBase, nil
}

func newWebFiles() *webFiles {
	w := &webFiles{mem: map[string]string{}}
	func() {
		defer func() { recover() }()
		if s := js.Global().Get("localStorage"); s.Type() == js.TypeObject {
			w.store = s
		}
	}()
	return w
}

func (w *webFiles) persistent() bool {
	return w.store.Type() == js.TypeObject
}

func storageKey(p string) string {
	return storagePrefix + path.Clean("/"+p)
}

func notExist(op, p string) error {
	return &fs.PathError{Op: op, Path: p, Err: fs.ErrNotExist}
}

func (w *webFiles) get(k string) (string, bool) {
	if !w.persistent() {
		v, ok := w.mem[k]
		return v, ok
	}
	v := w.store.Call("getItem", k)
	if v.Type() != js.TypeString {
		return "", false
	}
	return v.String(), true
}

func (w *webFiles) put(k, v string) (err error) {
	if !w.persistent() {
		w.mem[k] = v
		return nil
	}
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("browser storage is full or blocked: " + fmt.Sprint(r))
		}
	}()
	w.store.Call("setItem", k, v)
	return nil
}

func (w *webFiles) keys() []string {
	if !w.persistent() {
		out := make([]string, 0, len(w.mem))
		for k := range w.mem {
			out = append(out, k)
		}
		return out
	}
	n := w.store.Get("length").Int()
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		if k := w.store.Call("key", i); k.Type() == js.TypeString {
			out = append(out, k.String())
		}
	}
	return out
}

func encodeItem(data []byte) string {
	return strconv.FormatInt(time.Now().UnixMilli(), 10) + "|" + base64.StdEncoding.EncodeToString(data)
}

func decodeItem(v string) (time.Time, []byte, error) {
	ms, b64, ok := strings.Cut(v, "|")
	if !ok {
		return time.Time{}, nil, errors.New("browser storage item is malformed")
	}
	n, err := strconv.ParseInt(ms, 10, 64)
	if err != nil {
		return time.Time{}, nil, err
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	return time.UnixMilli(n), data, err
}

func (w *webFiles) ReadFile(p string) ([]byte, error) {
	v, ok := w.get(storageKey(p))
	if !ok {
		return nil, notExist("read", p)
	}
	_, data, err := decodeItem(v)
	return data, err
}

func (w *webFiles) WriteFile(p string, data []byte) error {
	return w.put(storageKey(p), encodeItem(data))
}

func (w *webFiles) Remove(p string) error {
	k := storageKey(p)
	if _, ok := w.get(k); !ok {
		return notExist("remove", p)
	}
	if w.persistent() {
		w.store.Call("removeItem", k)
	} else {
		delete(w.mem, k)
	}
	return nil
}

func (w *webFiles) Stat(p string) (UserEntry, error) {
	v, ok := w.get(storageKey(p))
	if !ok {
		return UserEntry{}, notExist("stat", p)
	}
	mod, data, err := decodeItem(v)
	if err != nil {
		return UserEntry{}, err
	}
	return UserEntry{Name: path.Base(p), Size: int64(len(data)), Mod: mod}, nil
}

func (w *webFiles) List(dir string) ([]UserEntry, error) {
	prefix := storageKey(dir) + "/"
	var out []UserEntry
	for _, k := range w.keys() {
		rest, ok := strings.CutPrefix(k, prefix)
		if !ok || rest == "" || strings.Contains(rest, "/") {
			continue
		}
		if e, err := w.Stat(strings.TrimPrefix(k, storagePrefix)); err == nil {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
