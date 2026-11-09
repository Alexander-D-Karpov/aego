package editor

import (
	"bytes"
	stdctx "context"
	"encoding/binary"
	"os"
	"reflect"
	"testing"

	"aego/art"
)

// This exercises the editor's guarded interchange path, not compatibility
// with the external Aseprite application or the canonical Save workflow.
func TestAsepriteInterchangeRequiresConsentAndPreservesSource(t *testing.T) {
	p, _, _, _ := artDurationFixture(t)
	pl := p.ctx.Pipeline
	beforeSource, err := pl.SourceBytes(p.doc.Path)
	if err != nil {
		t.Fatal(err)
	}
	beforeMeta, err := pl.SourceBytes(p.doc.Path + ".meta")
	if err != nil {
		t.Fatal(err)
	}
	before, cursor, dirty := p.model.Session.Document.Info(), p.model.History.Cursor(), p.doc.Dirty()
	confirm := func(accept bool) {
		t.Helper()
		p.askAsepriteExport()
		questions := p.ctx.TakeQuestions()
		if len(questions) != 1 || questions[0].Then == nil {
			t.Fatalf("missing interchange warning: %+v, %v", questions, p.err)
		}
		choice := 1
		if accept {
			choice = 0
		}
		questions[0].Then(choice)
	}
	confirm(false)
	if len(p.ctx.TakeQuestions()) != 0 {
		t.Fatal("cancelling the warning opened an export path")
	}
	confirm(true)
	questions := p.ctx.TakeQuestions()
	if len(questions) != 1 || questions[0].Submit == nil {
		t.Fatal("confirmed export did not request a destination")
	}
	const target = "assets/interchange.aseprite"
	questions[0].Submit([]string{target})
	if p.err != nil {
		t.Fatal(p.err)
	}
	data, err := pl.SourceBytes(target)
	if err != nil || len(data) < 128 || binary.LittleEndian.Uint16(data[4:6]) != 0xa5e0 || bytes.HasPrefix(data, []byte("PK")) {
		t.Fatalf("export is not an Aseprite-format copy: %v", err)
	}
	if _, indexed := pl.Lookup(target); indexed {
		t.Fatal("interchange export silently created a new project identity")
	}
	afterSource, err := pl.SourceBytes(p.doc.Path)
	if err != nil {
		t.Fatal(err)
	}
	afterMeta, err := pl.SourceBytes(p.doc.Path + ".meta")
	if err != nil || !bytes.Equal(beforeSource, afterSource) || !bytes.Equal(beforeMeta, afterMeta) || !reflect.DeepEqual(before, p.model.Session.Document.Info()) || p.model.History.Cursor() != cursor || p.doc.Dirty() != dirty {
		t.Fatal("interchange export modified the original source, model or history", err)
	}
}

func TestAsepriteInterchangeRejectsStaleAndOccupiedDestinations(t *testing.T) {
	p, _, _, _ := artDurationFixture(t)
	p.askAsepriteExport()
	questions := p.ctx.TakeQuestions()
	if len(questions) != 1 || questions[0].Then == nil {
		t.Fatal("missing warning")
	}
	if _, err := p.model.Execute(stdctx.Background(), art.Operation{Kind: "duration", DurationMS: 321}); err != nil {
		t.Fatal(err)
	}
	questions[0].Then(0)
	if p.err == nil || len(p.ctx.TakeQuestions()) != 0 {
		t.Fatal("a stale export snapshot reached the path prompt")
	}
	p.err = nil
	const target = "assets/occupied.aseprite"
	const sidecar = "external metadata"
	if err := os.WriteFile(p.ctx.Docs.Abs(target+".meta"), []byte(sidecar), 0644); err != nil {
		t.Fatal(err)
	}
	p.askAsepriteExport()
	questions = p.ctx.TakeQuestions()
	if len(questions) != 1 || questions[0].Then == nil {
		t.Fatal("missing second warning")
	}
	questions[0].Then(0)
	questions = p.ctx.TakeQuestions()
	if len(questions) != 1 || questions[0].Submit == nil {
		t.Fatal("missing destination prompt")
	}
	questions[0].Submit([]string{target})
	if p.err == nil {
		t.Fatal("orphan destination metadata was silently claimed")
	}
	if data, err := p.ctx.Pipeline.SourceBytes(target); err != nil || data != nil {
		t.Fatal("failed export created the destination", err)
	}
	if data, err := os.ReadFile(p.ctx.Docs.Abs(target + ".meta")); err != nil || string(data) != sidecar {
		t.Fatal("failed export changed the destination sidecar", err)
	}
}

func TestAsepriteInterchangeRejectsUnsupportedSurfaceBeforePrompt(t *testing.T) {
	p, _, _, _ := artDurationFixture(t)
	if _, err := p.model.Execute(stdctx.Background(), art.Operation{Kind: "ensure-cel", Channel: art.Normal}); err != nil {
		t.Fatal(err)
	}
	before, cursor := p.model.Session.Document.Info(), p.model.History.Cursor()
	p.askAsepriteExport()
	if p.err == nil || len(p.ctx.TakeQuestions()) != 0 || p.model.History.Cursor() != cursor || !reflect.DeepEqual(before, p.model.Session.Document.Info()) {
		t.Fatal("unsupported surface reached export or changed the drawing")
	}
}

func TestAsepriteInterchangeProtectsOpenDirtyDestination(t *testing.T) {
	p, _, _, _ := artDurationFixture(t)
	const target = "assets/open.aseprite"
	metaPath := target + ".meta"
	if err := os.WriteFile(p.ctx.Docs.Abs(metaPath), []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	opened, err := p.ctx.Docs.OpenScript(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	opened.Code.ReplaceSnapshot([]byte("unsaved changes"), false)
	if err := os.Remove(p.ctx.Docs.Abs(metaPath)); err != nil {
		t.Fatal(err)
	}
	p.askAsepriteExport()
	questions := p.ctx.TakeQuestions()
	if len(questions) != 1 || questions[0].Then == nil {
		t.Fatal("missing interchange warning")
	}
	questions[0].Then(0)
	questions = p.ctx.TakeQuestions()
	if len(questions) != 1 || questions[0].Submit == nil {
		t.Fatal("missing destination prompt")
	}
	questions[0].Submit([]string{target})
	if p.err == nil || !opened.Dirty() || string(opened.Code.Bytes()) != "unsaved changes" {
		t.Fatal("export did not protect the open dirty metadata")
	}
	if data, err := p.ctx.Pipeline.SourceBytes(target); err != nil || data != nil {
		t.Fatal("export created a destination while metadata was open", err)
	}
}
