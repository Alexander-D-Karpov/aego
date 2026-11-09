//go:build js && wasm

// The lazy GPU presentation runtime consumes immutable snapshots. The main
// aegoArt runtime remains the only editable document and history owner.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"syscall/js"

	"aego/tools/artstudio/gpupreview"
)

var app *gpupreview.App
var callbacks []js.Func

func reply(err error) any {
	v := map[string]any{}
	if app != nil {
		v["status"] = app.Status()
	}
	if err != nil {
		v["error"] = err.Error()
	}
	b, _ := json.Marshal(v)
	return string(b)
}
func integer(v js.Value) (uint64, error) {
	if v.Type() != js.TypeNumber {
		return 0, fmt.Errorf("preview expects a revision number")
	}
	n := v.Float()
	if n <= 0 || n > 9007199254740991 || math.IsNaN(n) || math.Trunc(n) != n {
		return 0, fmt.Errorf("invalid preview revision")
	}
	return uint64(n), nil
}
func start(_ js.Value, _ []js.Value) any {
	if app != nil {
		return reply(nil)
	}
	a, err := gpupreview.New(false)
	if err != nil {
		return reply(err)
	}
	app = a
	go func() {
		if err := a.Run(); err != nil {
			js.Global().Get("console").Call("error", "Asset Studio GPU initialization failed: "+err.Error())
		}
	}()
	return reply(nil)
}
func publish(_ js.Value, args []js.Value) any {
	if app == nil {
		return reply(fmt.Errorf("GPU preview is not started"))
	}
	if len(args) != 3 {
		return reply(fmt.Errorf("preview requires sequence, revision and snapshot bytes"))
	}
	seq, err := integer(args[0])
	if err != nil {
		return reply(err)
	}
	rev, err := integer(args[1])
	if err != nil {
		return reply(err)
	}
	v := args[2]
	if v.Type() != js.TypeObject || v.IsNull() || !v.InstanceOf(js.Global().Get("Uint8Array")) {
		return reply(fmt.Errorf("preview requires a Uint8Array snapshot"))
	}
	n := v.Get("byteLength").Int()
	if n == 0 || n > gpupreview.MaxSnapshotBytes {
		return reply(fmt.Errorf("preview snapshot exceeds limit"))
	}
	b := make([]byte, n)
	js.CopyBytesToGo(b, v)
	return reply(app.Publish(seq, rev, b))
}
func configure(_ js.Value, args []js.Value) any {
	if app == nil {
		return reply(fmt.Errorf("GPU preview is not started"))
	}
	if len(args) != 1 || args[0].Type() != js.TypeString || len(args[0].String()) > 4096 {
		return reply(fmt.Errorf("invalid preview configuration"))
	}
	o := gpupreview.DefaultOptions()
	d := json.NewDecoder(strings.NewReader(args[0].String()))
	d.DisallowUnknownFields()
	if err := d.Decode(&o); err != nil {
		return reply(err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return reply(fmt.Errorf("trailing preview configuration"))
	}
	return reply(app.Configure(o))
}
func main() {
	api := js.Global().Get("Object").New()
	for _, f := range []struct {
		name string
		run  func(js.Value, []js.Value) any
	}{
		{"start", start}, {"publish", publish}, {"configure", configure},
		{"restartParticles", func(js.Value, []js.Value) any {
			if app == nil {
				return reply(fmt.Errorf("GPU preview is not started"))
			}
			return reply(app.RestartParticles())
		}},
		{"status", func(js.Value, []js.Value) any { return reply(nil) }},
		{"resources", func(js.Value, []js.Value) any {
			if app == nil {
				return "{}"
			}
			b, _ := json.Marshal(app.Resources())
			return string(b)
		}},
		{"contextLoss", func(_ js.Value, args []js.Value) any {
			if app == nil || len(args) != 1 || args[0].Type() != js.TypeBoolean {
				return false
			}
			return app.ContextLoss(args[0].Bool())
		}},
		{"close", func(js.Value, []js.Value) any {
			if app != nil {
				app.Close()
				app = nil
			}
			return reply(nil)
		}},
	} {
		fn := js.FuncOf(f.run)
		callbacks = append(callbacks, fn)
		api.Set(f.name, fn)
	}
	js.Global().Set("aegoArtPreview", api)
	select {}
}
