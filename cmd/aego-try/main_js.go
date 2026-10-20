//go:build js && aego_editor

package main

import (
	"syscall/js"

	"aego/app/docsview"
)

func main() {
	g := js.Global()
	api := g.Get("Object").New()
	app, err := docsview.New(docsview.Options{})
	if err != nil {
		api.Set("error", err.Error())
	} else {
		app.OnChange = func(text string) {
			if f := g.Get("aegoDocsChanged"); f.Type() == js.TypeFunction {
				f.Invoke(text)
			}
		}
		api.Set("call", js.FuncOf(func(_ js.Value, args []js.Value) any {
			method, payload := "", ""
			if len(args) > 0 {
				method = args[0].String()
			}
			if len(args) > 1 {
				payload = args[1].String()
			}
			return app.Reply(method, payload)
		}))
		go app.Run()
	}
	g.Set("aegoDocs", api)
	if ready := g.Get("aegoDocsReady"); ready.Type() == js.TypeFunction {
		ready.Invoke()
	}
	select {}
}
