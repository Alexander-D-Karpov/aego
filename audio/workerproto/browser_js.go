//go:build js && wasm

package workerproto

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"syscall/js"
	"time"
	"unsafe"

	"aego/audio/mix"
)

var ErrBrowserTransport = errors.New("audio worker: browser transport failed")

// BrowserOptions supplies already-created browser objects. Worker is dedicated
// to this transport; Module is a compiled WASI reactor; Node is the Web Audio
// output. The caller owns compilation, deadlines, output gain and lifecycle.
type BrowserOptions struct {
	Worker, Module, Node js.Value
	Data, Index          js.Value
	Init                 Request
	Block, Target        int
	RequireStats         bool
	OnReady              func()
	OnRebind             func()
	OnQuiesced           func()
	OnResume             func()
	OnBatch              func()
	OnIdle               func()
	OnFailure            func(error, bool)
}

// BrowserStats are the Worker's queue-empty estimates and measured mixer time.
// Underruns is an estimate based on the latest published active-voice batch;
// actual AudioWorklet underruns are reported separately by the output.
type BrowserStats struct {
	Blocks, Underruns uint64
	MixMax, MixTotal  time.Duration
}

// BrowserOutput describes an already-created replacement output. The caller
// must disconnect the previous AudioWorklet before initiating the rebind.
type BrowserOutput struct {
	Node, Data, Index js.Value
	Block, Target     int
	Rate              int
	// Quiet starts the replacement worklet quiesced, before it can consume PCM.
	Quiet bool
}

// Browser connects Controller to AegoAudioWorkerHost in a js/wasm page. It is
// explicitly driven by its owner; it does not select the backend, open a
// device, fetch a reactor, install timers, or recover uncertain playback.
type Browser struct {
	controller  *Controller
	host        js.Value
	callback    js.Func
	options     BrowserOptions
	pending     uint64
	rebinding   bool
	rebindQuiet bool
	quiesceReq  bool
	quiescing   bool
	quiesced    bool
	quietToken  uint64
	resuming    bool
	notifyIdle  bool
	ready       bool
	attached    bool
	terminated  bool
	err         error
	stats       BrowserStats
	hasStats    bool
}

// NewBrowser attaches a fresh producer-side mixer before it renders. The host
// script must already be installed and the worker/module supplied by the
// caller. A startup failure before output attachment permits CancelUnsent;
// after attachment the output must not silently fall back to local playback.
// If it returns both a browser and an error, the caller must terminate that
// browser and explicitly resolve ownership using CancelUnsent or
// AbortAfterTermination. If the browser is nil, the caller still owns Worker.
func NewBrowser(m *mix.Mixer, options BrowserOptions) (b *Browser, err error) {
	controller, err := NewController(m)
	if err != nil {
		return nil, err
	}
	b = &Browser{controller: controller, options: options}
	b.callback = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) != 1 {
			b.fail(ErrBrowserTransport, b.attached)
			return nil
		}
		b.event(args[0])
		return nil
	})
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", ErrBrowserTransport, r)
			if b.host.Truthy() {
				// Once a host exists, preserve the controller for explicit
				// termination and recovery; attachment may be uncertain.
				b.fail(err, b.attached)
				return
			}
			// The caller still owns the worker if host construction failed.
			_ = controller.CancelUnsent()
			b.callback.Release()
			b = nil
		}
	}()
	ctor := js.Global().Get("AegoAudioWorkerHost")
	if ctor.Type() != js.TypeFunction {
		_ = controller.CancelUnsent()
		b.callback.Release()
		return nil, ErrBrowserTransport
	}
	init, err := json.Marshal(options.Init)
	if err != nil {
		_ = controller.CancelUnsent()
		b.callback.Release()
		return nil, err
	}
	b.host = ctor.New(options.Worker, b.callback)
	b.host.Call("start", map[string]any{"module": options.Module, "node": options.Node,
		"input": string(init), "block": options.Block, "target": options.Target,
		"data": options.Data, "index": options.Index})
	return b, b.err
}

func (b *Browser) fail(err error, attached bool) {
	if b == nil || b.terminated || b.err != nil {
		return
	}
	// Attachment is irreversible: an inconsistent later failure must not make
	// an already-used output eligible for automatic local fallback.
	attached = b.attached || attached
	b.ready, b.attached, b.err = false, attached, err
	if b.host.Truthy() {
		b.host.Call("close")
	}
	if b.options.OnFailure != nil {
		b.options.OnFailure(err, attached)
	}
}

func (b *Browser) event(event js.Value) {
	if b == nil || b.terminated || b.err != nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			b.fail(fmt.Errorf("%w: %v", ErrBrowserTransport, r), b.attached)
		}
	}()
	switch event.Get("type").String() {
	case "ready":
		if b.ready || b.attached {
			b.fail(ErrBrowserTransport, b.attached)
			return
		}
		b.ready, b.attached = true, true
		if b.options.OnReady != nil {
			b.options.OnReady()
		}
		_ = b.Pump()
	case "reply":
		if !b.ready || event.Get("result").Type() != js.TypeString {
			b.fail(ErrBrowserTransport, b.attached)
			return
		}
		if b.rebinding {
			if event.Get("result").String() != "" {
				b.fail(ErrBrowserTransport, b.attached)
				return
			}
			b.rebinding, b.quiesced = false, b.rebindQuiet
			b.rebindQuiet = false
			b.quietToken = 0
			if b.options.OnRebind != nil {
				b.options.OnRebind()
			}
			_ = b.Pump()
			return
		}
		if b.quiescing {
			if event.Get("result").String() != "" {
				b.fail(ErrBrowserTransport, b.attached)
				return
			}
			b.quiescing, b.quiesced = false, true
			if b.options.OnQuiesced != nil {
				b.options.OnQuiesced()
			}
			_ = b.Pump()
			return
		}
		if b.resuming {
			if event.Get("result").String() != "" {
				b.fail(ErrBrowserTransport, b.attached)
				return
			}
			b.resuming, b.quiesced = false, false
			if b.options.OnResume != nil {
				b.options.OnResume()
			}
			_ = b.Pump()
			return
		}
		if b.pending == 0 {
			b.fail(ErrBrowserTransport, b.attached)
			return
		}
		if err := b.controller.Ack(b.pending, []byte(event.Get("result").String())); err != nil {
			b.fail(err, b.attached)
			return
		}
		b.pending = 0
		_ = b.Pump()
	case "batch":
		if !b.ready || event.Get("batch").Type() != js.TypeString {
			b.fail(ErrBrowserTransport, b.attached)
			return
		}
		value := event.Get("stats")
		present := value.Type() != js.TypeUndefined && value.Type() != js.TypeNull
		next := b.stats
		var err error
		if present {
			next, err = browserStats(value, b.stats)
		} else if b.options.RequireStats || b.hasStats {
			err = ErrBrowserTransport
		}
		var batch Batch
		if err == nil {
			batch, err = DecodeBatch([]byte(event.Get("batch").String()))
		}
		if err == nil {
			err = b.controller.ApplyBatch(batch.Mix())
		}
		if err != nil {
			b.fail(err, b.attached)
			return
		}
		if present {
			b.stats, b.hasStats = next, true
		}
		b.host.Call("ackBatch", strconv.FormatUint(batch.Serial, 10))
		if b.err != nil {
			return
		}
		if b.options.OnBatch != nil {
			b.options.OnBatch()
		}
		_ = b.Pump()
	case "fatal":
		attached := event.Get("attached").Truthy()
		b.fail(fmt.Errorf("%w: %s", ErrBrowserTransport, event.Get("error").String()), attached)
	default:
		b.fail(ErrBrowserTransport, b.attached)
	}
}

func browserStats(value js.Value, old BrowserStats) (BrowserStats, error) {
	if value.Type() != js.TypeObject || !value.Truthy() {
		return BrowserStats{}, ErrBrowserTransport
	}
	read := func(name string) (float64, bool) {
		v := value.Get(name)
		if v.Type() != js.TypeNumber {
			return 0, false
		}
		n := v.Float()
		return n, !math.IsNaN(n) && !math.IsInf(n, 0) && n >= 0
	}
	blocks, bok := read("blocks")
	underruns, uok := read("underruns")
	maxMS, mok := read("maxMixMS")
	totalMS, tok := read("totalMixMS")
	const maxSafeInteger = float64(1<<53 - 1)
	maxDurationMS := float64(math.MaxInt64) / float64(time.Millisecond)
	if !bok || !uok || !mok || !tok || blocks > maxSafeInteger || underruns > maxSafeInteger ||
		math.Trunc(blocks) != blocks || math.Trunc(underruns) != underruns || maxMS > totalMS ||
		maxMS >= maxDurationMS || totalMS >= maxDurationMS {
		return BrowserStats{}, ErrBrowserTransport
	}
	next := BrowserStats{Blocks: uint64(blocks), Underruns: uint64(underruns),
		MixMax: time.Duration(maxMS * float64(time.Millisecond)), MixTotal: time.Duration(totalMS * float64(time.Millisecond))}
	if next.Blocks < old.Blocks || next.Underruns < old.Underruns || next.MixMax < old.MixMax || next.MixTotal < old.MixTotal {
		return BrowserStats{}, ErrBrowserTransport
	}
	return next, nil
}

// Stats returns the last validated Worker measurements, if the transport has
// received them. They never replace the output's real underrun measurements.
func (b *Browser) Stats() (BrowserStats, bool) {
	if b == nil || b.terminated || b.err != nil {
		return BrowserStats{}, false
	}
	return b.stats, b.hasStats
}

// Pump sends at most one operation. Further steps follow successful replies;
// call it again after commands are queued or a producer finishes a write.
func (b *Browser) Pump() (err error) {
	if b == nil || b.terminated {
		return ErrBrowserTransport
	}
	if b.err != nil {
		return b.err
	}
	if !b.ready || b.pending != 0 || b.rebinding || b.quiescing || b.resuming {
		return nil
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", ErrBrowserTransport, r)
			b.fail(err, b.attached)
		}
	}()
	if b.quiesceReq {
		b.quiesceReq, b.quiescing = false, true
		id := b.host.Call("quiesce")
		if id.Type() != js.TypeNumber || id.Float() < 1 || id.Float() > 1<<53-1 || math.Trunc(id.Float()) != id.Float() {
			b.fail(ErrBrowserTransport, b.attached)
			return ErrBrowserTransport
		}
		b.quietToken = uint64(id.Float())
		return b.err
	}
	step, ok, err := b.controller.Next()
	if err != nil {
		b.fail(err, b.attached)
		return err
	}
	if !ok {
		if b.controller.Idle() && b.options.OnIdle != nil && !b.notifyIdle {
			b.notifyIdle = true
			defer func() { b.notifyIdle = false }()
			b.options.OnIdle()
		}
		return nil
	}
	if step.Op == Skip && !step.Chunk {
		// Skip is the hot path while output is quiet. Its only input is the
		// bounded frame count; serialize it in the host instead of building a
		// Go JSON object and string on every engine frame. The host still uses
		// the normal single-flight RPC and the Worker validates the request.
		b.pending = step.Token
		b.host.Call("skip", step.Request.Frames)
		return b.err
	}
	raw, err := json.Marshal(step.Request)
	if err != nil {
		b.fail(err, b.attached)
		return err
	}
	var pcm any = js.Null()
	if len(step.PCM) > 0 {
		array := js.Global().Get("Float32Array").New(len(step.PCM))
		bytes := unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(step.PCM))), len(step.PCM)*4)
		if js.CopyBytesToJS(js.Global().Get("Uint8Array").New(array.Get("buffer")), bytes) != len(bytes) {
			b.fail(ErrBrowserTransport, b.attached)
			return ErrBrowserTransport
		}
		pcm = array
	}
	var op any = step.Op
	if step.Chunk {
		op = "memory-chunk"
	}
	b.pending = step.Token
	b.host.Call("send", op, string(raw), pcm, step.Offset)
	return b.err
}

// Idle is a snapshot of an open, acknowledged transport and controller. It
// does not prevent a producer from publishing another command afterwards.
func (b *Browser) Idle() bool {
	return b != nil && !b.terminated && b.err == nil && b.ready && b.pending == 0 && !b.rebinding &&
		!b.quiesceReq && !b.quiescing && !b.resuming && b.controller.Idle()
}

// QuietToken identifies the acknowledged worklet flush. A quiet replacement
// starts at token zero; the page must match its final report before rebind.
func (b *Browser) QuietToken() (uint64, bool) {
	if b == nil || b.terminated || b.err != nil || !b.ready || !b.quiesced || b.rebinding || b.quiescing || b.resuming {
		return 0, false
	}
	return b.quietToken, true
}

// Rebind changes the output while retaining the Worker and its live mixer. It
// is asynchronous and requires an idle RPC channel; OnRebind runs only after
// the new worklet has been attached. The caller owns the previous output's
// disconnection and the producer-side mixer sample-rate bookkeeping.
func (b *Browser) Rebind(output BrowserOutput) (err error) {
	if b == nil || b.terminated || b.err != nil || !b.ready || b.pending != 0 || b.rebinding || b.quiesceReq || b.quiescing || b.resuming || !b.controller.Idle() {
		return ErrBrowserTransport
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", ErrBrowserTransport, r)
			b.fail(err, b.attached)
		}
	}()
	b.rebinding = true
	b.rebindQuiet = output.Quiet
	b.host.Call("rebind", map[string]any{"node": output.Node, "data": output.Data, "index": output.Index,
		"block": output.Block, "target": output.Target, "rate": output.Rate, "quiet": output.Quiet})
	return b.err
}

// Quiesce stops the Worker and discards previously queued PCM in the worklet
// after any in-flight RPC, including a pending Resume. The owner must suppress
// audible output while the asynchronous transition is pending, because old
// PCM can play before the acknowledgement. Completion is reported through
// OnQuiesced. Commands continue once the Worker is quiesced.
func (b *Browser) Quiesce() error {
	if b == nil || b.terminated || b.err != nil || !b.ready || b.rebinding {
		return ErrBrowserTransport
	}
	if b.resuming {
		b.quiesceReq = true
		return nil
	}
	if b.quiesced || b.quiesceReq || b.quiescing {
		return nil
	}
	b.quiesceReq = true
	return b.Pump()
}

// QueueSkip requests silent mixer advancement after Quiesce has been requested.
// It can queue work while quiescence is in flight but sends no skip until the
// worklet acknowledges its flush. Commands retain their publication order.
func (b *Browser) QueueSkip(frames int64) error {
	if b == nil || b.terminated || b.err != nil || !b.ready || (!b.quiesced && !b.quiesceReq && !b.quiescing) || b.rebinding || (b.resuming && !b.quiesceReq) {
		return ErrBrowserTransport
	}
	if err := b.controller.QueueSkip(frames); err != nil {
		return err
	}
	return b.Pump()
}

// Resume requires all queued skips, commands and stream reconciliation to have
// completed. The Worker primes new PCM before the worklet resumes consumption;
// the owner must wait for OnResume before making the output audible again.
func (b *Browser) Resume() (err error) {
	if b == nil || b.terminated || b.err != nil || !b.ready || !b.quiesced || b.rebinding || b.quiesceReq || b.quiescing || b.resuming || b.pending != 0 || !b.controller.Idle() {
		return ErrBrowserTransport
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", ErrBrowserTransport, r)
			b.fail(err, b.attached)
		}
	}()
	b.resuming = true
	b.host.Call("resume")
	return b.err
}

// PollStreams requests a bounded cycle for live streams and starts it when the
// current operation completes. The caller supplies the cadence and wakeups.
func (b *Browser) PollStreams() error {
	if b == nil || b.terminated || b.err != nil {
		return ErrBrowserTransport
	}
	if err := b.controller.PollStreams(); err != nil {
		b.fail(err, b.attached)
		return err
	}
	return b.Pump()
}

// Terminate detaches the host's event handlers and stops its dedicated Worker.
// It must precede either CancelUnsent or AbortAfterTermination.
func (b *Browser) Terminate() {
	if b == nil || b.terminated {
		return
	}
	b.terminated, b.ready = true, false
	if b.host.Truthy() {
		b.host.Call("close")
	}
	b.callback.Release()
}

// CancelUnsent restores the local mixer only if the Worker was terminated
// before attaching the output and before any command or batch crossed.
func (b *Browser) CancelUnsent() error {
	if b == nil || !b.terminated || b.attached {
		return ErrBrowserTransport
	}
	return b.controller.CancelUnsent()
}

// AbortAfterTermination relinquishes stream claims after the owner has stopped
// or otherwise reconciled decoders. It never resumes uncertain playback.
func (b *Browser) AbortAfterTermination() error {
	if b == nil || !b.terminated {
		return ErrBrowserTransport
	}
	return b.controller.AbortAfterTermination()
}
