package agentbridge

import (
	"bytes"
	"context"
	"fmt"
	"image/png"
	"path"
	"strconv"
	"strings"

	"aego/app/agent"
	"aego/art"
	"aego/editor/core/artedit"
	"aego/scene/mutate"
)

var _ agent.ArtHost = (*Bridge)(nil)

func (b *Bridge) artDocument(file string) (*artedit.Document, error) {
	rel, err := b.Rel(file)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(path.Ext(rel), ".aeart") {
		return nil, fmt.Errorf("art: an .aeart source is required")
	}
	if b.ctx.Docs == nil || b.ctx.Pipeline == nil {
		return nil, fmt.Errorf("art: project documents are unavailable")
	}
	id, ok := b.ctx.Pipeline.Lookup(rel)
	if !ok {
		return nil, fmt.Errorf("art: %s is not in the project asset index", rel)
	}
	d, err := b.ctx.DocumentForAsset(id)
	if err != nil {
		return nil, err
	}
	return artedit.Bind(b.ctx.Docs, d)
}

// All ArtHost methods are reached through Bridge's existing main-thread queue.
func (b *Bridge) ReadArt(file, section string, offset, limit int) (*agent.Output, error) {
	d, err := b.artDocument(file)
	if err != nil {
		return nil, err
	}
	i := d.Session.Info()
	value := map[string]any{
		"file": d.Asset.Path, "documentID": i.Document.ID, "revision": strconv.FormatUint(i.Document.Revision, 10),
		"canvas": i.Document.Canvas, "dirty": i.Dirty, "gesture": i.Gesture,
		"activeLayer": i.Layer, "activeFrame": i.Frame, "activeChannel": i.Channel,
		"undo": i.Undo, "redo": i.Redo,
		"counts": map[string]int{"layers": len(i.Document.Layers), "frames": len(i.Document.Frames), "cels": len(i.Document.Cels), "palette": len(i.Document.Palette), "tags": len(i.Document.Tags), "slices": len(i.Document.Slices)},
	}
	if offset < 0 || limit < 1 || limit > 1024 {
		return nil, fmt.Errorf("art: invalid page")
	}
	var total, next int
	var items any
	switch section {
	case "", "summary":
		return &agent.Output{Value: value}, nil
	case "layers":
		items, total, next = artPage(i.Document.Layers, offset, limit)
	case "frames":
		items, total, next = artPage(i.Document.Frames, offset, limit)
	case "cels":
		items, total, next = artPage(i.Document.Cels, offset, limit)
	case "palette":
		items, total, next = artPage(i.Document.Palette, offset, limit)
	case "tags":
		items, total, next = artPage(i.Document.Tags, offset, limit)
	case "slices":
		items, total, next = artPage(i.Document.Slices, offset, limit)
	default:
		return nil, fmt.Errorf("art: unknown section %q", section)
	}
	value["section"], value["items"], value["total"], value["nextOffset"] = section, items, total, next
	return &agent.Output{Value: value}, nil
}
func artPage[T any](values []T, offset, limit int) ([]T, int, int) {
	start := min(offset, len(values))
	end := min(start+limit, len(values))
	next := -1
	if end < len(values) {
		next = end
	}
	return values[start:end], len(values), next
}
func (b *Bridge) CaptureArt(file string, frame art.ID, channel art.Channel) (*agent.Output, error) {
	d, err := b.artDocument(file)
	if err != nil {
		return nil, err
	}
	if frame == "" {
		frame = d.Session.Frame
	}
	if channel == "" {
		channel = d.Session.Channel
	}
	pixels, revision, err := d.Capture(frame, channel)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err = png.Encode(&buf, pixels); err != nil {
		return nil, err
	}
	if buf.Len() > 16<<20 {
		return nil, fmt.Errorf("art: PNG exceeds agent capture budget")
	}
	return &agent.Output{
		Value:  map[string]any{"file": d.Asset.Path, "documentID": d.Session.Document.Info().ID, "revision": strconv.FormatUint(revision, 10), "frame": frame, "channel": channel, "width": pixels.Bounds().Dx(), "height": pixels.Bounds().Dy(), "dirty": d.Session.Info().Dirty},
		Images: []agent.Image{{Mime: "image/png", Data: buf.Bytes()}},
	}, nil
}

type preparedArt struct {
	bridge   *Bridge
	owner    *artedit.Document
	proposal *artedit.Proposal
	label    string
	used     bool
}

func (p *preparedArt) RetainedBytes() int64 { return p.proposal.Bytes() }
func (p *preparedArt) Commit() (*agent.Output, error) {
	if p.used {
		return nil, fmt.Errorf("art: proposal has already been consumed")
	}
	p.used = true
	b, d := p.bridge, p.owner
	if b.PlanOnly() {
		return nil, errPlanOnly
	}
	if b.ctx.Docs.AssetDocument(d.Asset.Asset) != d.Asset || d.Asset.Binding.Model != d {
		return nil, fmt.Errorf("art: document was closed or replaced while awaiting approval")
	}
	if err := d.Accept(p.proposal); err != nil {
		return nil, err
	}
	b.last = &undoRef{doc: d.Asset.ID, label: p.label, art: &artUndo{owner: d, revision: d.Session.Document.Revision()}}
	return &agent.Output{OK: true, Value: map[string]any{
		"file": d.Asset.Path, "documentID": d.Session.Document.Info().ID, "revision": strconv.FormatUint(d.Session.Document.Revision(), 10), "results": p.proposal.Results, "dirty": d.Session.Info().Dirty,
	}}, nil
}
func (b *Bridge) PrepareArt(file string, revision uint64, label string, ops []art.Operation) (*agent.Output, error) {
	if b.PlanOnly() {
		return nil, errPlanOnly
	}
	d, err := b.artDocument(file)
	if err != nil {
		return nil, err
	}
	label = b.stepName(label)
	p, err := d.Propose(context.Background(), revision, label, ops)
	if err != nil {
		return nil, err
	}
	var held int64
	for _, pending := range b.proposals {
		if pending.Preview != nil && pending.Preview.Art != nil {
			held += pending.Preview.Art.RetainedBytes()
		}
	}
	if p.Bytes() > (64<<20)-held {
		return nil, fmt.Errorf("art: pending proposals exceed memory budget; resolve existing proposals first")
	}
	return &agent.Output{
		DryRun:  true,
		Changes: []mutate.Change{{Op: "art.edit", Node: d.Asset.Path, Path: "pixels/layers/frames", Before: strconv.FormatUint(revision, 10), After: fmt.Sprintf("%d operations: %s", len(ops), label)}},
		Value:   map[string]any{"file": d.Asset.Path, "documentID": d.Session.Document.Info().ID, "revision": strconv.FormatUint(revision, 10), "results": p.Results},
		Art:     &preparedArt{bridge: b, owner: d, proposal: p, label: label},
	}, nil
}

type artUndo struct {
	owner    *artedit.Document
	revision uint64
}

func (b *Bridge) canUndoArt(u *artUndo) bool {
	if u == nil || u.owner == nil || b.ctx.Docs == nil {
		return false
	}
	d := u.owner
	return b.ctx.Docs.AssetDocument(d.Asset.Asset) == d.Asset && d.Asset.Binding.Model == d && !d.Session.Info().Gesture && d.Session.Document.Revision() == u.revision && d.History.CanUndo()
}
func (b *Bridge) undoArt(u *artUndo) error {
	if !b.canUndoArt(u) {
		return fmt.Errorf("art: proposal is no longer the latest document edit")
	}
	return u.owner.SessionUndo()
}
