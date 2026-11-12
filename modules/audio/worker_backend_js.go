//go:build js && wasm

package audio

import (
	"fmt"
	"syscall/js"
)

// A page must compile the reactor and supply a same-origin Worker URL before
// Init. Absence or an unsupported browser keeps the ordinary local pump.
func (m *Module) newWorkerBackend() (workerBackend, error) {
	g := js.Global()
	config := g.Get("aegoAudioWorkerConfig")
	if config.Type() != js.TypeObject || !config.Truthy() || g.Get("AegoAudioWorkerHost").Type() != js.TypeFunction ||
		g.Get("Worker").Type() != js.TypeFunction {
		return nil, nil
	}
	reactor, url := config.Get("module"), config.Get("url")
	if !reactor.Truthy() || url.Type() != js.TypeString || url.String() == "" {
		return nil, nil
	}
	workerURL, ok := sameOriginWorkerURL(g, url.String())
	if !ok {
		return nil, nil
	}
	out, ok := m.out.(browserWorkerOutput)
	if !ok {
		return nil, nil
	}
	create := func(url string) (worker js.Value, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("audio worker: %v", r)
			}
		}()
		return g.Get("Worker").New(url), nil
	}
	r, err := newBrowserWorker(m, out, reactor, workerURL, create)
	if r == nil {
		return nil, err
	}
	return r, err
}

func sameOriginWorkerURL(g js.Value, raw string) (url string, ok bool) {
	defer func() {
		if recover() != nil {
			url, ok = "", false
		}
	}()
	location, ctor := g.Get("location"), g.Get("URL")
	if !location.Truthy() || ctor.Type() != js.TypeFunction {
		return "", false
	}
	v := ctor.New(raw, location.Get("href"))
	protocol := v.Get("protocol").String()
	if (protocol != "https:" && protocol != "http:") || v.Get("origin").String() != location.Get("origin").String() ||
		v.Get("username").String() != "" || v.Get("password").String() != "" {
		return "", false
	}
	return v.Get("href").String(), true
}
