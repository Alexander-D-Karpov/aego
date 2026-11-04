package agentbridge

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"strings"
	"testing"
	"time"

	"aego/app/agent"
	ed "aego/editor/core/context"
	"aego/project"
)

type recordingGenerator struct {
	calls int
	req   ed.ImageGeneration
}

func (*recordingGenerator) Name() string { return "test" }
func (g *recordingGenerator) Generate(_ context.Context, r ed.ImageGeneration) ([][]byte, error) {
	g.calls++
	g.req = r
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 3, 4))); err != nil {
		return nil, err
	}
	return [][]byte{buf.Bytes()}, nil
}

func TestGeneratorNeedsSeparateConsentAndScopesVariants(t *testing.T) {
	q := make(chan func(), 4)
	g := &recordingGenerator{}
	c := &ed.Editor{Project: &project.Project{Root: t.TempDir()}, ImageGenerator: g}
	b := &Bridge{ctx: c, cfg: Config{OnMain: func(fn func()) { q <- fn }, Timeout: 5 * time.Second}}
	b.panelGeneration.Store(1)
	b.SetAutoApply(true)
	run := func(choice int) (*agent.Output, error) {
		t.Helper()
		var result *agent.Output
		var err error
		done := make(chan struct{})
		go func() {
			result, err = b.generateVariants(map[string]any{"prompt": "a tile", "count": 1, "maxCents": 7})
			close(done)
		}()
		timeout := time.After(8 * time.Second)
		for {
			select {
			case <-timeout:
				t.Fatal("request hung")
			case <-done:
				return result, err
			case f := <-q:
				f()
				for _, ask := range c.TakeQuestions() {
					if !strings.Contains(ask.Message, "7") || !strings.Contains(ask.Message, "test") {
						t.Fatal("consent does not disclose cost or generator")
					}
					ask.Then(choice)
				}
			}
		}
	}
	if _, err := run(1); err == nil || g.calls != 0 {
		t.Fatal("Auto-apply or rejection submitted the request")
	}
	out, err := run(0)
	if err != nil || g.calls != 1 || g.req.MaxCents != 7 || len(out.Images) != 1 {
		t.Fatalf("generation: %v, %v", out, err)
	}
	for id := range b.variants.bytes {
		if _, ok := b.GeneratedImage(id); !ok {
			t.Fatal("cannot select variant")
		}
		b.panelGeneration.Add(1)
		if _, ok := b.GeneratedImage(id); ok {
			t.Fatal("old conversation retained variant")
		}
	}
}
