package bind

import (
	"runtime"
	"strconv"
	"strings"
)

type Info struct {
	Doc string
	Loc string
}

type Option func(*Info)

func Doc(s string) Option {
	return func(i *Info) { i.Doc = s }
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

func (r *Registry) describe(module, name string, opts []Option) {
	var info Info
	for _, o := range opts {
		o(&info)
	}
	if r.locate {
		info.Loc = callerLoc()
	}
	if info.Doc != "" || info.Loc != "" {
		r.docs[key(module, name)] = info
	}
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
