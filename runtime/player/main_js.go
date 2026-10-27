//go:build js

package player

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path"
	"runtime"
	"runtime/debug"
	"strconv"
	"syscall/js"

	"aego/core/trace"
	"aego/modules/audio"
	"aego/modules/settings"
	"aego/platform"
	"aego/platform/input"
	"aego/sdk"
)

const (
	dataGlobal    = "aegoData"
	hookGlobal    = "aegoGame"
	failGlobal    = "aegoFail"
	startedGlobal = "aegoStarted"
	replayGlobal  = "aegoReplay"
)

func Main(mods ...sdk.Module) {
	g := js.Global()
	q := g.Get("URLSearchParams").New(g.Get("location").Get("search"))
	param := func(k string) string {
		if v := q.Call("get", k); v.Type() == js.TypeString {
			return v.String()
		}
		return ""
	}
	memprof := param("memprofile") != ""
	if memprof {
		runtime.MemProfileRate = 1
	}
	b, err := takeBundle(g.Get(dataGlobal))
	g.Set(dataGlobal, js.Undefined())
	if err != nil {
		fail(err.Error())
		return
	}
	if v, err := strconv.Atoi(param("gogc")); err == nil {
		debug.SetGCPercent(v)
	}
	if v, err := strconv.Atoi(param("memlimit")); err == nil && v > 0 {
		debug.SetMemoryLimit(int64(v) << 20)
	}
	replay, err := replayData(g)
	if err != nil {
		fail(err.Error())
		return
	}
	seed, _ := strconv.ParseUint(param("seed"), 10, 64)
	cfg := Config{
		Bundle: b, Profile: param("profile"), Scene: param("scene"), Debug: param("debug") != "", Seed: seed,
		Tracing: param("trace") != "", Probe: param("bench") != "", ReplayData: replay, MemProfile: memprof,
	}
	p, err := Open(cfg, sdk.AdaptAll(mods)...)
	if err != nil {
		fail(err.Error())
		return
	}
	expose(p)
	if f := g.Get(startedGlobal); f.Type() == js.TypeFunction {
		f.Invoke()
	}
	if err := p.Run(); err != nil {
		fail(err.Error())
	}
}

func replayData(g js.Value) ([]byte, error) {
	v := g.Get(replayGlobal)
	if v.Type() != js.TypeString {
		return nil, nil
	}
	g.Set(replayGlobal, js.Undefined())
	data, err := base64.StdEncoding.DecodeString(v.String())
	if err != nil {
		return nil, errors.New("the page input recording is not valid base64")
	}
	return data, nil
}

func fail(msg string) {
	if f := js.Global().Get(failGlobal); f.Type() == js.TypeFunction {
		f.Invoke(msg)
		return
	}
	js.Global().Get("console").Call("error", "game: "+msg)
}

func bytesOf(u js.Value) []byte {
	if u.Type() != js.TypeObject {
		return nil
	}
	out := make([]byte, u.Get("byteLength").Int())
	js.CopyBytesToGo(out, u)
	return out
}

func takeBundle(v js.Value) (*Bundle, error) {
	if v.Type() != js.TypeObject {
		return nil, errors.New("the page did not load the game data")
	}
	b := &Bundle{Manifest: bytesOf(v.Get("manifest")), Files: map[string][]byte{}}
	files := v.Get("files")
	keys := js.Global().Get("Object").Call("keys", files)
	for i := 0; i < keys.Length(); i++ {
		k := keys.Index(i).String()
		b.Files[k] = bytesOf(files.Get(k))
	}
	return b, nil
}

func (p *Player) userPath(name string) (string, bool) {
	if p.profileDir == "" {
		return "", false
	}
	return path.Join(p.profileDir, path.Clean("/"+name)), true
}

type contextLoser interface {
	SimulateLoss(lose bool) bool
}

func expose(p *Player) {
	api := js.Global().Get("Object").New()
	api.Set("menuOpen", js.FuncOf(func(js.Value, []js.Value) any {
		v := settings.Of(p.Engine.MainWorld())
		return v != nil && v.Opened()
	}))
	api.Set("frames", js.FuncOf(func(js.Value, []js.Value) any {
		return float64(p.Engine.MainWorld().Time.Frame())
	}))
	api.Set("stats", js.FuncOf(func(js.Value, []js.Value) any {
		data, err := json.Marshal(p.Stats())
		if err != nil {
			return js.Null()
		}
		return string(data)
	}))
	api.Set("frameLimit", js.FuncOf(func(_ js.Value, a []js.Value) any {
		if len(a) > 0 {
			p.Engine.SetFrameLimit(a[0].Float())
		}
		return nil
	}))
	api.Set("pixelRatio", js.FuncOf(func(_ js.Value, a []js.Value) any {
		if l, ok := p.Engine.Window().(platform.PixelRatioLimiter); ok && len(a) > 0 {
			l.SetMaxPixelRatio(a[0].Float())
			return true
		}
		return false
	}))
	api.Set("allocs", js.FuncOf(func(js.Value, []js.Value) any {
		r := p.Allocs()
		if r == nil {
			return js.Null()
		}
		data, err := json.Marshal(r)
		if err != nil {
			return js.Null()
		}
		return string(data)
	}))
	api.Set("lockstep", js.FuncOf(func(_ js.Value, a []js.Value) any {
		p.Engine.SetLockstep(len(a) > 0 && a[0].Truthy())
		return nil
	}))
	api.Set("window", js.FuncOf(func(_ js.Value, a []js.Value) any {
		if len(a) < 4 {
			return false
		}
		opened, closed := a[2], a[3]
		return p.Window(uint64(a[0].Int()), uint64(a[1].Int()), func() { opened.Invoke() }, func(s, e Stats) {
			sj, _ := json.Marshal(s)
			ej, _ := json.Marshal(e)
			closed.Invoke(string(sj), string(ej))
		})
	}))
	api.Set("resetMax", js.FuncOf(func(js.Value, []js.Value) any {
		p.ResetMaxReal()
		return nil
	}))
	api.Set("trace", js.FuncOf(func(js.Value, []js.Value) any {
		t := p.Engine.Trace()
		if !t.Enabled() {
			return js.Null()
		}
		var b bytes.Buffer
		if err := trace.WriteChrome(&b, t, t.Snapshot(t.Frames())); err != nil {
			return js.Null()
		}
		return b.String()
	}))
	api.Set("loseContext", js.FuncOf(func(_ js.Value, a []js.Value) any {
		l, ok := p.Engine.GPU().(contextLoser)
		return ok && len(a) > 0 && l.SimulateLoss(a[0].Truthy())
	}))
	api.Set("pads", js.FuncOf(func(js.Value, []js.Value) any {
		in := p.Engine.Input()
		out := js.Global().Get("Array").New()
		for i := 0; i < input.MaxPads; i++ {
			if in.PadConnected(i) {
				o := js.Global().Get("Object").New()
				o.Set("name", in.PadName(i))
				o.Set("leftX", float64(in.PadAxisValue(i, input.PadLeftX)))
				out.Call("push", o)
			}
		}
		return out
	}))
	api.Set("userRead", js.FuncOf(func(_ js.Value, a []js.Value) any {
		if len(a) == 0 {
			return js.Null()
		}
		at, ok := p.userPath(a[0].String())
		if !ok {
			return js.Null()
		}
		data, err := platform.UserFiles().ReadFile(at)
		if err != nil {
			return js.Null()
		}
		return string(data)
	}))
	api.Set("userWrite", js.FuncOf(func(_ js.Value, a []js.Value) any {
		if len(a) < 2 {
			return "userWrite needs a name and a text"
		}
		at, ok := p.userPath(a[0].String())
		if !ok {
			return "there is no player profile"
		}
		if err := platform.UserFiles().WriteFile(at, []byte(a[1].String())); err != nil {
			return err.Error()
		}
		return js.Null()
	}))
	api.Set("mixStats", js.FuncOf(func(js.Value, []js.Value) any {
		m := audio.ModuleOf(p.Engine)
		if m == nil {
			return js.Null()
		}
		st := m.Stats()
		o := js.Global().Get("Object").New()
		o.Set("mixMaxUs", float64(st.MixMax.Microseconds()))
		o.Set("mixAvgUs", float64(st.MixAvg.Microseconds()))
		o.Set("voices", st.Voices)
		o.Set("blocks", float64(st.Blocks))
		return o
	}))
	js.Global().Set(hookGlobal, api)
}
