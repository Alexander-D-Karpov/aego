package bind

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
)

type Info struct {
	Doc     string
	Params  []string
	Example string
	Loc     string
}

type Option func(*Info)

func Doc(s string) Option {
	return func(i *Info) { i.Doc = s }
}

func Params(names ...string) Option {
	return func(i *Info) { i.Params = append([]string(nil), names...) }
}

func Example(s string) Option {
	return func(i *Info) { i.Example = s }
}

func NewManifest() *Registry {
	r := New()
	r.locate = true
	return r
}

func (r *Registry) Locate(on bool) {
	r.locate = on
}

func (r *Registry) Info(module, name string) Info {
	return r.docs[key(module, name)]
}

func Describe(m *Module, name string, opts ...Option) {
	k := key(m.path, name)
	info := m.reg.docs[k]
	for _, o := range opts {
		o(&info)
	}
	m.reg.docs[k] = info
}

func (r *Registry) describe(module, name string, opts []Option, params int) error {
	var info Info
	for _, o := range opts {
		o(&info)
	}
	if params >= 0 && len(info.Params) > 0 && len(info.Params) != params {
		return fmt.Errorf("Params names %d parameters, the function has %d", len(info.Params), params)
	}
	if r.locate {
		info.Loc = callerLoc()
	}
	if info.Doc != "" || info.Loc != "" || info.Example != "" || len(info.Params) > 0 {
		r.docs[key(module, name)] = info
	}
	return nil
}

func callerLoc() string {
	var pcs [24]uintptr
	n := runtime.Callers(3, pcs[:])
	frames := runtime.CallersFrames(pcs[:n])
	for {
		f, more := frames.Next()
		if !strings.HasPrefix(f.Function, "aego/script/bind.") && !strings.HasPrefix(f.Function, "aego/sdk.") {
			return f.File + ":" + strconv.Itoa(f.Line)
		}
		if !more {
			return ""
		}
	}
}
