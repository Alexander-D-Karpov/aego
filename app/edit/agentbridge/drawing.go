package agentbridge

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"aego/app/agent"
	"aego/asset/pipeline"
	"aego/editor/core/context"
	"aego/editor/core/history"
)

type drawingCommand struct {
	label  string
	editor *context.Editor
	edits  []pipeline.SourceEdit
}

func (c *drawingCommand) Label() string             { return c.label }
func (*drawingCommand) Merge(history.Command) bool  { return false }
func (c *drawingCommand) Do(history.Target) error   { return c.apply(false) }
func (c *drawingCommand) Undo(history.Target) error { return c.apply(true) }

func (c *drawingCommand) check(back bool) error {
	for _, e := range c.edits {
		rel := strings.TrimSuffix(e.Path, pipeline.MetaSuffix)
		if id, ok := c.editor.Pipeline.Lookup(rel); ok {
			if d := c.editor.Docs.AssetDocument(id); d != nil && d.Dirty() {
				return fmt.Errorf("%s has unsaved edits; save or discard them first", rel)
			}
		}
		if d := c.editor.Docs.Script(e.Path); d != nil && (d.Conflict || d.Dirty()) {
			return fmt.Errorf("%s has unsaved changes", e.Path)
		}
		if back {
			got, err := c.editor.Pipeline.SourceBytes(e.Path)
			if err != nil {
				return err
			}
			if (got == nil) != (e.After == nil) || !bytes.Equal(got, e.After) {
				return fmt.Errorf("%s changed after the drawing", e.Path)
			}
		}
	}
	return nil
}

func (c *drawingCommand) apply(back bool) error {
	if err := c.check(back); err != nil {
		return err
	}
	b, err := c.editor.Pipeline.ApplySources(c.edits, back)
	if err != nil {
		return err
	}
	c.editor.ApplyBatch(b)
	c.editor.Docs.CheckExternalScripts()
	return nil
}

func (b *Bridge) applyDrawing(p *Proposal) (*agent.Output, error) {
	draw := p.Preview.Drawing
	cmd := &drawingCommand{editor: b.ctx, edits: draw.Sources, label: b.stepName("Draw " + draw.Files[0])}
	if d := b.ctx.Docs.Edit(); d != nil {
		if err := d.History.Exec(cmd); err != nil {
			return nil, err
		}
		b.note(d, cmd.Label())
	} else {
		if err := cmd.Do(nil); err != nil {
			return nil, err
		}
		b.last = &undoRef{label: cmd.Label()}
	}
	// The reference PNG is an input to the approved render. Later changes
	// to it must not prevent undoing or redoing this independent destination.
	cmd.edits = slices.DeleteFunc(slices.Clone(cmd.edits), func(e pipeline.SourceEdit) bool {
		return !slices.Contains(draw.Files, strings.TrimSuffix(e.Path, pipeline.MetaSuffix))
	})
	b.last.drawing = cmd
	o := *p.Preview
	o.DryRun = false
	o.Files = draw.Files
	o.Created = draw.Created
	o.Drawing = nil
	p.Preview.Drawing = nil
	return &o, nil
}
