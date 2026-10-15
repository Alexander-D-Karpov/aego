package integration

import (
	"testing"

	"aego/app/edit"
	"aego/core/log"
	"aego/editor/core/vectors"
	"aego/editor/panels/gameview"
	"aego/project"
)

func BenchmarkEditorFrame(b *testing.B) {
	for _, mode := range []string{"Edit", "Play"} {
		b.Run(mode, func(b *testing.B) {
			root := copyExample(b, "physics-sandbox")
			p, err := project.Load(root)
			if err != nil {
				b.Fatal(err)
			}
			s, err := edit.Open(p, edit.Config{Headless: true, NoWatch: true, Logger: log.Nop()})
			if err != nil {
				b.Fatal(err)
			}
			defer s.Close()
			s.Step()
			ctx := s.App.Context()
			d, err := ctx.Docs.Open("scenes/main.aescene")
			if err != nil {
				b.Fatal(err)
			}
			ctx.Overlay.Set(gameview.CommandOverlays, true)
			ctx.Overlay.Set(vectors.CommandToggle, true)
			if mode == "Play" {
				if err := s.App.Play().Start(d); err != nil {
					b.Fatal(err)
				}
			}
			for i := 0; i < 120; i++ {
				s.Step()
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				s.Step()
			}
		})
	}
}
