//go:build js

package web

import (
	_ "embed"
	"encoding/json"
	"math"
	"syscall/js"
	"unsafe"

	"aego/core/errs"
	"aego/platform"
)

const (
	audioProcessor = "aego-out"
	audioReport    = 2048
	muteKey        = "aego-muted"
)

var currentAudio *Audio

//go:embed worklet.js
var workletSource string

type AudioStats struct {
	Running   bool    `json:"running"`
	Shared    bool    `json:"shared"`
	Muted     bool    `json:"muted"`
	Underruns int     `json:"underruns"`
	RMS       float64 `json:"rms"`
	Consumed  int64   `json:"consumed"`
	Rate      int     `json:"rate"`
	Error     string  `json:"error,omitempty"`
}

type Audio struct {
	g         js.Value
	ctx       js.Value
	node      js.Value
	port      js.Value
	f32       js.Value
	u8        js.Value
	atomics   js.Value
	data      js.Value
	index     js.Value
	url       js.Value
	noop      js.Func
	spec      platform.AudioSpec
	shared    bool
	ready     bool
	state     string
	unlocked  bool
	hidden    bool
	paused    bool
	closed    bool
	tail      int32
	count     sendCount
	underruns int
	sum       float64
	samples   float64
	failed    string
	listeners []listener
	funcs     []js.Func
	gain      js.Value
	muted     bool
}

func (p *Platform) OpenAudio(want platform.AudioSpec) (platform.AudioOutput, error) {
	return OpenAudio(want)
}

func newContext(ctor js.Value, rate int) (ctx js.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errs.New(errs.PlatformInit, "the AudioContext could not be created")
		}
	}()
	opts := map[string]any{"latencyHint": "interactive"}
	if rate > 0 {
		opts["sampleRate"] = rate
	}
	return ctor.New(opts), nil
}

func OpenAudio(want platform.AudioSpec) (*Audio, error) {
	want = want.WithDefaults()
	g := js.Global()
	ctor := g.Get("AudioContext")
	if !ctor.Truthy() {
		ctor = g.Get("webkitAudioContext")
	}
	if !ctor.Truthy() {
		return nil, errs.New(errs.PlatformInit, "this browser has no Web Audio")
	}
	ctx, err := newContext(ctor, want.SampleRate)
	if err != nil {
		ctx, err = newContext(ctor, 0)
	}
	if err != nil {
		return nil, err
	}
	if !ctx.Get("audioWorklet").Truthy() {
		try(ctx, "close")
		return nil, errs.New(errs.PlatformInit, "Web Audio has no AudioWorklet here; it needs a secure context")
	}
	a := &Audio{
		g: g, ctx: ctx, f32: g.Get("Float32Array"), u8: g.Get("Uint8Array"), atomics: g.Get("Atomics"),
		state: ctx.Get("state").String(),
	}
	a.noop = a.fn(func(js.Value, []js.Value) any { return nil })
	a.spec = platform.AudioSpec{SampleRate: ctx.Get("sampleRate").Int(), Channels: 2, Frames: want.Frames}
	a.shared = g.Get("crossOriginIsolated").Truthy() && g.Get("SharedArrayBuffer").Truthy() && a.atomics.Truthy()
	a.hook(ctx, "statechange", func(js.Value) { a.state = ctx.Get("state").String() })
	if doc := g.Get("document"); doc.Truthy() {
		for _, name := range []string{"pointerdown", "keydown", "touchend"} {
			a.hook(doc, name, a.gesture)
		}
		a.hook(doc, "visibilitychange", func(js.Value) { a.visibility(doc.Get("hidden").Truthy()) })
		a.hidden = doc.Get("hidden").Truthy()
	}
	if v, ok := storageGet(g, muteKey); ok {
		a.muted = v == "1"
	}
	if activated(g) {
		a.gesture(js.Undefined())
	}
	a.publish()
	blob := g.Get("Blob").New([]any{workletSource}, map[string]any{"type": "text/javascript"})
	a.url = g.Get("URL").Call("createObjectURL", blob)
	ok := a.fn(func(js.Value, []js.Value) any {
		a.attach()
		return nil
	})
	bad := a.fn(func(_ js.Value, args []js.Value) any {
		a.failed = "the audio worklet did not load"
		if len(args) > 0 {
			a.failed += ": " + args[0].Call("toString").String()
		}
		return nil
	})
	ctx.Get("audioWorklet").Call("addModule", a.url).Call("then", ok, bad)
	return a, nil
}

func (a *Audio) fn(f func(js.Value, []js.Value) any) js.Func {
	jf := js.FuncOf(f)
	a.funcs = append(a.funcs, jf)
	return jf
}

func (a *Audio) hook(target js.Value, name string, fn func(js.Value)) {
	f := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			fn(args[0])
		} else {
			fn(js.Undefined())
		}
		return nil
	})
	target.Call("addEventListener", name, f, map[string]any{"passive": true, "capture": true})
	a.listeners = append(a.listeners, listener{target: target, name: name, fn: f})
}

func (a *Audio) quiet(v js.Value) {
	if v.Truthy() && v.Get("catch").Type() == js.TypeFunction {
		v.Call("catch", a.noop)
	}
}

func newNode(ctx js.Value, po map[string]any) (node js.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errs.New(errs.PlatformInit, "the audio worklet node could not be created")
		}
	}()
	return js.Global().Get("AudioWorkletNode").New(ctx, audioProcessor, map[string]any{
		"numberOfInputs":     0,
		"numberOfOutputs":    1,
		"outputChannelCount": []any{2},
		"processorOptions":   po,
	}), nil
}

func (a *Audio) attach() {
	if a.closed {
		return
	}
	po := map[string]any{"channels": a.spec.Channels, "report": audioReport}
	if a.shared {
		sab := a.g.Get("SharedArrayBuffer")
		data := sab.New(ringFrames * a.spec.Channels * 4)
		index := sab.New(16)
		a.data = a.u8.New(data)
		a.index = a.g.Get("Int32Array").New(index)
		po["data"] = data
		po["index"] = index
	}
	node, err := newNode(a.ctx, po)
	if err != nil {
		a.failed = err.Error()
		return
	}
	a.node = node
	a.port = node.Get("port")
	a.port.Set("onmessage", a.fn(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			a.report(args[0].Get("data"))
		}
		return nil
	}))
	a.gain = a.ctx.Call("createGain")
	a.gain.Get("gain").Set("value", a.level())
	node.Call("connect", a.gain)
	a.gain.Call("connect", a.ctx.Get("destination"))
	a.ready = true
}

func (a *Audio) report(d js.Value) {
	if !d.Truthy() {
		return
	}
	a.count.consumed = int64(d.Get("consumed").Float())
	if t := d.Get("time"); t.Type() == js.TypeNumber {
		a.count.at = t.Float()
	}
	a.underruns = d.Get("underruns").Int()
	a.sum = d.Get("sum").Float()
	a.samples = d.Get("count").Float()
}

func (a *Audio) gesture(js.Value) {
	a.unlocked = true
	if !a.hidden && !a.paused && a.state != "running" {
		a.quiet(try(a.ctx, "resume"))
	}
}

func (a *Audio) visibility(hidden bool) {
	a.hidden = hidden
	switch {
	case hidden:
		a.quiet(try(a.ctx, "suspend"))
	case a.unlocked && !a.paused:
		a.quiet(try(a.ctx, "resume"))
	}
}

func (a *Audio) Running() bool {
	return !a.closed && a.ready && a.state == "running"
}

func (a *Audio) TargetBlocks() int {
	if a.shared {
		return 3
	}
	return 6
}

func (a *Audio) Spec() platform.AudioSpec {
	return a.spec
}

func floatBytes(s []float32) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*4)
}

func (a *Audio) Write(s []float32) int {
	ch := a.spec.Channels
	frames := len(s) / ch
	if frames == 0 || a.closed {
		return 0
	}
	if !a.Running() {
		return frames
	}
	if a.shared {
		return a.writeShared(s[:frames*ch])
	}
	arr := a.f32.New(frames * ch)
	buf := arr.Get("buffer")
	js.CopyBytesToJS(a.u8.New(buf), floatBytes(s[:frames*ch]))
	a.port.Call("postMessage", arr, []any{buf})
	a.count.sent += int64(frames)
	return frames
}

func (a *Audio) writeShared(s []float32) int {
	ch := a.spec.Channels
	head := int32(a.atomics.Call("load", a.index, 0).Int())
	n := min(len(s)/ch, ringFree(head, a.tail, ringFrames))
	if n <= 0 {
		return 0
	}
	pos, first := ringSpan(a.tail, ringFrames, n)
	js.CopyBytesToJS(a.data.Call("subarray", pos*ch*4, (pos+first)*ch*4), floatBytes(s[:first*ch]))
	if n > first {
		js.CopyBytesToJS(a.data.Call("subarray", 0, (n-first)*ch*4), floatBytes(s[first*ch:n*ch]))
	}
	a.tail += int32(n)
	a.atomics.Call("store", a.index, 1, a.tail)
	return n
}

func (a *Audio) Queued() int {
	if !a.Running() {
		return 0
	}
	if a.shared {
		return ringQueued(int32(a.atomics.Call("load", a.index, 0).Int()), a.tail)
	}
	return a.count.queuedAt(a.ctx.Get("currentTime").Float(), a.spec.SampleRate, 2*audioReport)
}

func (a *Audio) Pause(on bool) {
	a.paused = on
	switch {
	case on:
		a.quiet(try(a.ctx, "suspend"))
	case a.unlocked && !a.hidden:
		a.quiet(try(a.ctx, "resume"))
	}
}

func (a *Audio) Stats() AudioStats {
	s := AudioStats{Running: a.Running(), Shared: a.shared, Muted: a.muted, Underruns: a.underruns, Consumed: a.count.consumed, Rate: a.spec.SampleRate, Error: a.failed}
	if a.samples > 0 {
		s.RMS = math.Sqrt(a.sum / a.samples)
	}
	return s
}

func (a *Audio) Close() {
	if a.closed {
		return
	}
	a.closed = true
	if a.node.Truthy() {
		a.port.Set("onmessage", js.Null())
		a.node.Call("disconnect")
	}
	if a.gain.Truthy() {
		a.gain.Call("disconnect")
	}
	a.quiet(try(a.ctx, "close"))
	for _, l := range a.listeners {
		l.target.Call("removeEventListener", l.name, l.fn, map[string]any{"capture": true})
		l.fn.Release()
	}
	a.listeners = nil
	if a.url.Truthy() {
		a.g.Get("URL").Call("revokeObjectURL", a.url)
	}
	if currentAudio == a {
		a.g.Delete("aegoAudioStats")
		a.g.Delete("aegoAudioMute")
		currentAudio = nil
	}
}

func storageGet(g js.Value, key string) (v string, ok bool) {
	defer func() {
		if recover() != nil {
			v, ok = "", false
		}
	}()
	ls := g.Get("localStorage")
	if !ls.Truthy() {
		return "", false
	}
	r := ls.Call("getItem", key)
	if r.Type() != js.TypeString {
		return "", false
	}
	return r.String(), true
}

func storageSet(g js.Value, key, v string) {
	defer func() { _ = recover() }()
	if ls := g.Get("localStorage"); ls.Truthy() {
		ls.Call("setItem", key, v)
	}
}

func activated(g js.Value) (on bool) {
	defer func() {
		if recover() != nil {
			on = false
		}
	}()
	ua := g.Get("navigator").Get("userActivation")
	return ua.Truthy() && ua.Get("hasBeenActive").Truthy()
}

func (a *Audio) level() float64 {
	if a.muted {
		return 0
	}
	return 1
}

func (a *Audio) Muted() bool {
	return a.muted
}

func (a *Audio) SetMuted(on bool) {
	a.muted = on
	if a.gain.Truthy() {
		a.gain.Get("gain").Call("setTargetAtTime", a.level(), a.ctx.Get("currentTime"), 0.01)
	}
	v := "0"
	if on {
		v = "1"
	}
	storageSet(a.g, muteKey, v)
}

func (a *Audio) publish() {
	currentAudio = a
	a.g.Set("aegoAudioStats", a.fn(func(js.Value, []js.Value) any {
		data, _ := json.Marshal(a.Stats())
		return string(data)
	}))
	a.g.Set("aegoAudioMute", a.fn(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 && args[0].Type() == js.TypeBoolean {
			a.SetMuted(args[0].Bool())
		}
		return a.muted
	}))
}

var (
	_ platform.Audioer     = (*Platform)(nil)
	_ platform.AudioOutput = (*Audio)(nil)
)
