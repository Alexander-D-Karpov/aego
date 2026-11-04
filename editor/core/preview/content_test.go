package preview

import (
	"aego/asset"
	"aego/asset/pipeline"
	"aego/core/ids"
	"aego/editor/core/tasks"
	"bytes"
	"image/color"
	"testing"
)

func TestForgottenThumbnailCannotPublishAfterReplacement(t *testing.T) {
	var main []func()
	q := tasks.New(tasks.Deps{Submit: func(fn func()) { fn() }, OnMain: func(fn func()) { main = append(main, fn) }})
	s := New(Deps{Tasks: q, CacheRoot: t.TempDir()})
	q.Tick()
	for _, fn := range main {
		fn()
	}
	main = nil
	id := ids.NewAssetID()
	info := pipeline.AssetInfo{ID: id, Name: "example", Path: "example.custom", Kind: "custom"}
	count := 0
	handler := func(version string, v byte) Handler {
		return Handler{Version: version, Build: func(c *Card) (bool, error) {
			count++
			c.Image.SetNRGBA(0, 0, color.NRGBA{R: v, A: 255})
			return true, nil
		}}
	}
	if err := s.Register(info.Kind, handler("first", 3)); err != nil {
		t.Fatal(err)
	}
	old := s.ensure(info)
	q.Tick()
	if len(main) != 1 {
		t.Fatal("old job never reached main queue")
	}
	s.Forget(id)
	s.Unregister(info.Kind)
	if err := s.Register(info.Kind, handler("second", 9)); err != nil {
		t.Fatal(err)
	}
	fresh := s.ensure(info)
	q.Tick()
	if len(main) != 2 {
		t.Fatal("new generation was not queued")
	}
	main[1]()
	main[0]()
	if old == fresh || old.key == fresh.key || s.items[id] != fresh || fresh.state != ready || fresh.content.buf[0] != 9 {
		t.Fatal("stale completion replaced the active thumbnail")
	}
	s.Forget(id)
	again := s.ensure(info)
	q.Tick()
	main[len(main)-1]()
	if count != 2 || again.state != ready || !bytes.Equal(again.content.buf, fresh.content.buf) {
		t.Fatal("unchanged folder regenerated or lost the cached thumbnail")
	}
}
func TestPreviewRejectsInvalidRegistration(t *testing.T) {
	s := New(Deps{})
	if err := s.Register(asset.ArtMaterial, Handler{Version: "1"}); err == nil {
		t.Fatal("registered a handler without a builder")
	}
}

func TestFallbackCardCannotReplaceRenderedPreset(t *testing.T) {
	s := New(Deps{})
	err := s.Register(asset.ArtPreset, Handler{Version: "v1", Render: func(asset.Entry, int) (Surface, error) { return Surface{}, nil }})
	if err != nil {
		t.Fatal(err)
	}
	e := asset.Entry{ID: ids.NewAssetID(), Source: "smoke.aepreset", Kind: asset.ArtPreset}
	soft, ok := s.Key(e)
	if !ok {
		t.Fatal("missing fallback key")
	}
	s.gpuOK = true
	visual, ok := s.Key(e)
	if !ok || visual == soft {
		t.Fatal("cached fallback would hide GPU rendering")
	}
}

func TestTypedPresetHandlersDispatchAndUnloadIndependently(t *testing.T) {
	s := &Service{}
	h := func(component string, match bool) Handler {
		return Handler{Component: component, Version: "1", Render: func(asset.Entry, int) (Surface, error) {
			if !match {
				return Surface{}, ErrUnsupportedContent
			}
			return Surface{}, nil
		}, Build: func(*Card) (bool, error) { return match, nil }}
	}
	if err := s.Register(asset.ArtPreset, h("a", false)); err != nil {
		t.Fatal(err)
	}
	if err := s.Register(asset.ArtPreset, h("b", true)); err != nil {
		t.Fatal(err)
	}
	if err := s.Register(asset.ArtPreset, h("b", true)); err == nil {
		t.Fatal("duplicate component handler")
	}
	old := s.handlers[asset.ArtPreset]
	if _, err := old.Render(asset.Entry{}, 32); err != nil {
		t.Fatal(err)
	}
	if handled, err := old.Build(nil); err != nil || !handled {
		t.Fatal(handled, err)
	}
	s.UnregisterComponent(asset.ArtPreset, "b")
	if _, err := s.handlers[asset.ArtPreset].Render(asset.Entry{}, 32); err != ErrUnsupportedContent {
		t.Fatal(err)
	}
	if _, err := old.Render(asset.Entry{}, 32); err != nil {
		t.Fatal("mutated an in-flight handler", err)
	}
	s.UnregisterComponent(asset.ArtPreset, "a")
	if _, ok := s.handlers[asset.ArtPreset]; ok {
		t.Fatal("unloaded handlers retained")
	}
}
