//go:build js

package web

import (
	_ "embed"
	"encoding/json"
	"math"
	"strconv"
	"syscall/js"
	"unsafe"

	"aego/core/errs"
	"aego/platform"
)

const (
	audioProcessor = "aego-out"
	audioReport    = 2048
	muteKey        = "aego-muted"
	soundDefault   = "aegoSoundDefault"
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
	Out       float64 `json:"out"`
	Consumed  int64   `json:"consumed"`
	Rate      int     `json:"rate"`
	Error     string  `json:"error,omitempty"`
}

type Audio struct {
	g         js.Value
	muteKey   string
	pageCtx   bool
	chunk     js.Value
	chunkU8   js.Value
	chunkLen  int
	ctx       js.Value
	node      js.Value
	port      js.Value
	f32       js.Value
	u8        js.Value
	atomics   js.Value
	data      js.Value
	views     map[[2]int]js.Value
	index     js.Value
	url       js.Value
	spec      platform.AudioSpec
	shared    bool
	target    int
	ready     bool
	state     string
	unlocked  bool
	hidden    bool
	paused    bool
	closed    bool
	loading   bool
	tail      int32
	count     sendCount
	refill    func() int
	underruns int
	sum       float64
	samples   float64
	failed    string
	listeners []listener
	funcs     []js.Func
	gain      js.Value
	muted     bool
	prime     []float32
	primeMax  int
	priming   bool
	sentAudio bool
	// A claimed worklet must never receive PCM from the main-thread pump.
	// The owner stops the Worker before cancelling an uncommitted claim.
	workerReserved bool
	workerAttached bool
}

func (p *Platform) OpenAudio(want platform.AudioSpec) (platform.AudioOutput, error) {
	return OpenAudio(want)
}

// sharedContextGlobal is an AudioContext the page created inside a click, so it starts running; the docs Try it button makes one before the runtime boots.
const (
	sharedContextGlobal = "aegoAudioContext"
	workletLoaded       = "aegoWorkletLoaded"
)

func sharedContext(g js.Value) (js.Value, bool) {
	c := g.Get(sharedContextGlobal)
	if !c.Truthy() || c.Get("state").String() == "closed" {
		return js.Value{}, false
	}
	return c, true
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
	ctx, pageCtx := sharedContext(g)
	if !pageCtx {
		var err error
		ctx, err = newContext(ctor, want.SampleRate)
		if err != nil {
			ctx, err = newContext(ctor, 0)
		}
		if err != nil {
			return nil, err
		}
	}
	if !ctx.Get("audioWorklet").Truthy() {
		try(ctx, "close")
		return nil, errs.New(errs.PlatformInit, "Web Audio has no AudioWorklet here; it needs a secure context")
	}
	a := &Audio{
		pageCtx: pageCtx,
		g:       g, ctx: ctx, f32: g.Get("Float32Array"), u8: g.Get("Uint8Array"), atomics: g.Get("Atomics"),
		state: ctx.Get("state").String(),
	}
	a.spec = platform.AudioSpec{SampleRate: ctx.Get("sampleRate").Int(), Channels: 2, Frames: want.Frames}
	a.shared = g.Get("crossOriginIsolated").Truthy() && g.Get("SharedArrayBuffer").Truthy() && a.atomics.Truthy()
	latency := 0.0
	if v := ctx.Get("baseLatency"); v.Type() == js.TypeNumber {
		latency = v.Float()
	}
	if a.shared {
		a.target = sharedTargetBlocks(latency, a.spec.SampleRate, a.spec.Frames)
	} else {
		a.target = messageTargetBlocks(latency, a.spec.SampleRate, a.spec.Frames)
	}
	a.hook(ctx, "statechange", func(js.Value) {
		a.state = ctx.Get("state").String()
		a.fillRunning()
	})
	if doc := g.Get("document"); doc.Truthy() {
		for _, name := range []string{"pointerdown", "keydown", "touchend"} {
			a.hook(doc, name, a.gesture)
		}
		a.hook(doc, "visibilitychange", func(js.Value) { a.visibility(doc.Get("hidden").Truthy()) })
		a.hidden = doc.Get("hidden").Truthy()
	}
	a.muteKey = muteKey
	if d := g.Get(soundDefault); d.Type() == js.TypeBoolean {
		a.muteKey, a.muted = muteKey+"-"+strconv.FormatBool(d.Bool()), !d.Bool()
	}
	if v, ok := storageGet(g, a.muteKey); ok {
		a.muted = v == "1"
	}
	if activated(g) {
		a.gesture(js.Undefined())
	}
	a.publish()
	blob := g.Get("Blob").New([]any{workletSource}, map[string]any{"type": "text/javascript"})
	a.url = g.Get("URL").Call("createObjectURL", blob)
	if ctx.Get(workletLoaded).Truthy() {
		a.attach()
		return a, nil
	}
	a.loading = true
	loaded := a.fn(func(js.Value, []js.Value) any {
		defer a.loadSettled()
		ctx.Set(workletLoaded, true)
		a.attach()
		return nil
	})
	bad := a.fn(func(_ js.Value, args []js.Value) any {
		defer a.loadSettled()
		if !a.closed {
			a.failed = "the audio worklet did not load"
			if len(args) > 0 {
				a.failed += ": " + args[0].Call("toString").String()
			}
		}
		return nil
	})
	ctx.Get("audioWorklet").Call("addModule", a.url).Call("then", loaded, bad)
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
		// Native JS callback can outlive this Go output (notably Close).
		v.Call("catch", a.g.Get("Boolean"))
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
	po := map[string]any{"channels": a.spec.Channels, "report": audioReport, "refillAt": a.refillThreshold()}
	if a.shared {
		sab := a.g.Get("SharedArrayBuffer")
		data := sab.New(ringFrames * a.spec.Channels * 4)
		index := sab.New(16)
		a.data = a.u8.New(data)
		a.views = nil
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
	a.fillRunning()
}

// suspend() is asynchronous. Continue to feed actual running quanta until
// statechange confirms suspension; service the restored device immediately.
func (a *Audio) fillRunning() {
	if a.refill != nil && a.Running() && !a.muted {
		a.refill()
	}
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
		a.fillRunning()
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
		return a.target
	}
	if n := a.g.Get("aegoAudioBlocks"); n.Type() == js.TypeNumber && n.Int() > 0 {
		return n.Int()
	}
	return max(messageMinimumBlocks, a.target)
}

func (a *Audio) Spec() platform.AudioSpec {
	return a.spec
}

func floatBytes(s []float32) []byte {
	return unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(s))), len(s)*4)
}

// BeginAudioPrime buffers only the first pump fill. The worklet still counts
// every shortage after playback begins; no silence is reclassified as audio.
func (a *Audio) BeginAudioPrime(frames int) {
	if a.workerReserved || a.sentAudio || a.priming || !a.Running() || frames <= 0 || a.spec.Channels <= 0 {
		return
	}
	a.primeMax = min(frames, ringFrames)
	a.prime = make([]float32, 0, a.primeMax*a.spec.Channels)
	a.priming = true
}

// EndAudioPrime publishes the initial PCM in one message or one shared-ring
// tail update. A later stall is not re-primed or ignored by the worklet.
func (a *Audio) EndAudioPrime() {
	if !a.priming {
		return
	}
	a.priming = false
	frames := len(a.prime) / a.spec.Channels
	if frames > 0 && (!a.Running() || a.writePCM(a.prime) != frames) {
		a.failed = "the initial audio buffer could not be committed"
	}
	a.prime = nil
	a.primeMax = 0
}

func (a *Audio) Write(s []float32) int {
	ch := a.spec.Channels
	frames := len(s) / ch
	if frames == 0 || a.closed || a.workerReserved {
		return 0
	}
	if !a.Running() {
		return frames
	}
	if a.priming {
		frames = min(frames, a.primeMax-len(a.prime)/ch)
		a.prime = append(a.prime, s[:frames*ch]...)
		return frames
	}
	return a.writePCM(s[:frames*ch])
}

func (a *Audio) writePCM(s []float32) int {
	ch := a.spec.Channels
	frames := len(s) / ch
	if a.shared {
		written := a.writeShared(s[:frames*ch])
		a.sentAudio = a.sentAudio || written > 0
		return written
	}
	if a.chunkLen != frames*ch {
		a.chunkLen = frames * ch
		a.chunk = a.f32.New(a.chunkLen)
		a.chunkU8 = a.u8.New(a.chunk.Get("buffer"))
	}
	js.CopyBytesToJS(a.chunkU8, floatBytes(s[:frames*ch]))
	a.port.Call("postMessage", a.chunk)
	a.count.sent += int64(frames)
	a.sentAudio = true
	return frames
}

const maxViews = 64

// SetRefill permits the worklet's existing consumption reports to service the
// same mixer between visual frames. No additional browser timer is installed.
func (a *Audio) SetRefill(fill func() int) {
	if a.workerReserved && fill != nil {
		a.failed = "the audio output is reserved by a worker"
		return
	}
	a.refill = fill
	if a.port.Truthy() {
		a.port.Call("postMessage", map[string]any{"refillAt": a.refillThreshold()})
	}
}

func (a *Audio) refillThreshold() int {
	if a.refill == nil && !a.workerReserved {
		return 0
	}
	return min(3*a.spec.Frames, (a.spec.SampleRate*24+999)/1000)
}

func (a *Audio) FrameDriven() bool {
	return true
}

func (a *Audio) view(from, to int) js.Value {
	k := [2]int{from, to}
	if v, ok := a.views[k]; ok {
		return v
	}
	if a.views == nil || len(a.views) >= maxViews {
		a.views = make(map[[2]int]js.Value, maxViews)
	}
	v := a.data.Call("subarray", from, to)
	a.views[k] = v
	return v
}

func (a *Audio) writeShared(s []float32) int {
	ch := a.spec.Channels
	head := int32(a.atomics.Call("load", a.index, 0).Int())
	n := min(len(s)/ch, ringFree(head, a.tail, ringFrames))
	if n <= 0 {
		return 0
	}
	pos, first := ringSpan(a.tail, ringFrames, n)
	js.CopyBytesToJS(a.view(pos*ch*4, (pos+first)*ch*4), floatBytes(s[:first*ch]))
	if n > first {
		js.CopyBytesToJS(a.view(0, (n-first)*ch*4), floatBytes(s[first*ch:n*ch]))
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
		a.fillRunning()
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
	s.Out = s.RMS * a.level()
	return s
}

func (a *Audio) Close() {
	if a.closed {
		return
	}
	a.closed = true
	a.refill = nil
	if a.node.Truthy() {
		a.port.Set("onmessage", js.Null())
		a.node.Call("disconnect")
	}
	if a.gain.Truthy() {
		a.gain.Call("disconnect")
	}
	if !a.pageCtx {
		a.quiet(try(a.ctx, "close"))
	}
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
	if !a.loading {
		a.releaseFunctions()
	}
}

func (a *Audio) loadSettled() {
	a.loading = false
	// Promise handlers cannot be detached. Release them only after success
	// or failure, including if Close happened during addModule.
	if a.closed {
		a.releaseFunctions()
	}
}

func (a *Audio) releaseFunctions() {
	for _, fn := range a.funcs {
		fn.Release()
	}
	a.funcs = nil
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
	a.fillRunning()
	if a.gain.Truthy() {
		a.gain.Get("gain").Call("setTargetAtTime", a.level(), a.ctx.Get("currentTime"), 0.01)
	}
	v := "0"
	if on {
		v = "1"
	}
	storageSet(a.g, a.muteKey, v)
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
