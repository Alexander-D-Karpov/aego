//go:build js && wasm

package main

import (
	"syscall/js"

	"aego/app/playground"
)

func main() {
	api := js.Global().Get("Object").New()
	pg, err := playground.New()
	if err != nil {
		api.Set("error", err.Error())
	} else {
		api.Set("call", js.FuncOf(func(_ js.Value, args []js.Value) any {
			method, payload := "", ""
			if len(args) > 0 {
				method = args[0].String()
			}
			if len(args) > 1 {
				payload = args[1].String()
			}
			return pg.Reply(method, payload)
		}))
	}
	js.Global().Set("aego", api)
	if ready := js.Global().Get("aegoReady"); ready.Type() == js.TypeFunction {
		ready.Invoke()
	}
	select {}
}
