package mutate

import (
	"fmt"
	"slices"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/format/aetext"
)

const (
	KindTextureSetting      = "texture-setting"
	KindTextureSettingReset = "texture-setting-reset"
	KindTextureChannel      = "texture-channel"
	KindTextureChannelReset = "texture-channel-reset"
	KindTextureFrames       = "texture-frames"
	KindTextureFrameAdd     = "texture-frame-add"
	KindTextureFrameRemove  = "texture-frame-remove"
	KindTextureFrameRename  = "texture-frame-rename"
	KindTextureFrameRect    = "texture-frame-rect"
	KindTextureFrameOrder   = "texture-frame-order"
	KindTexturePivot        = "texture-pivot"
	KindTextureRestore      = "texture-restore"
)

var textureKinds = map[string]bool{KindTextureSetting: true, KindTextureSettingReset: true, KindTextureChannel: true, KindTextureChannelReset: true, KindTextureFrames: true, KindTextureFrameAdd: true, KindTextureFrameRemove: true, KindTextureFrameRename: true, KindTextureFrameRect: true, KindTextureFrameOrder: true, KindTexturePivot: true, KindTextureRestore: true}

func TextureSetting(name string, value aetext.Value) Op {
	return Op{Kind: KindTextureSetting, Path: name, Value: value.Clone(), HasValue: true}
}
func TextureSettingReset(name string) Op { return Op{Kind: KindTextureSettingReset, Path: name} }
func TextureChannel(frame ids.AssetID, channel string, target ids.AssetID) Op {
	return Op{Kind: KindTextureChannel, ID: frame.String(), Path: channel, Asset: target.String()}
}
func TextureChannelReset(frame ids.AssetID, channel string) Op {
	return Op{Kind: KindTextureChannelReset, ID: frame.String(), Path: channel}
}
func TextureFrameRename(id ids.AssetID, name string) Op {
	return Op{Kind: KindTextureFrameRename, ID: id.String(), Name: name}
}
func TextureFrameRect(id ids.AssetID, rect [4]int) Op {
	return Op{Kind: KindTextureFrameRect, ID: id.String(), Value: rectValue(rect), HasValue: true}
}
func TextureFrameOrder(id ids.AssetID, index int) Op {
	return Op{Kind: KindTextureFrameOrder, ID: id.String(), Index: index, HasIndex: true}
}
func TextureFrameAdd(frame asset.SourceFrame) Op {
	return Op{Kind: KindTextureFrameAdd, Value: frameValue(frame), HasValue: true}
}
func TextureFrameRemove(id ids.AssetID) Op { return Op{Kind: KindTextureFrameRemove, ID: id.String()} }
func TexturePivot(id ids.AssetID, pivot [2]float32) Op {
	return Op{Kind: KindTexturePivot, ID: id.String(), Value: aetext.List(aetext.Float32(pivot[0]), aetext.Float32(pivot[1])), HasValue: true}
}
func TextureFramesReplace(frames []asset.SourceFrame) Op {
	list := make([]aetext.Value, len(frames))
	for i, f := range frames {
		list[i] = frameValue(f)
	}
	return Op{Kind: KindTextureFrames, Value: aetext.List(list...), HasValue: true}
}
func rectValue(r [4]int) aetext.Value {
	return aetext.List(aetext.Int(int64(r[0])), aetext.Int(int64(r[1])), aetext.Int(int64(r[2])), aetext.Int(int64(r[3])))
}
func frameValue(f asset.SourceFrame) aetext.Value {
	fields := []aetext.Field{{Name: "id", Value: aetext.AssetRef(f.ID.String())}, {Name: "name", Value: aetext.String(f.Name)}, {Name: "rect", Value: rectValue(f.Rect)}}
	if f.HasPivot {
		fields = append(fields, aetext.Field{Name: "pivot", Value: aetext.List(aetext.Float32(f.Pivot[0]), aetext.Float32(f.Pivot[1]))})
	}
	return aetext.Struct(fields...)
}

type TextureEnv struct {
	Width, Height int
	Resolve       func(ids.AssetID) (asset.TextureSourceInfo, bool)
}

// TextureDocument is source metadata, shared by CLI/MCP and the editor's existing
// Code journal. Neither mutable GPU resources nor pipeline locks belong here.
type TextureDocument struct {
	Name string
	doc  *asset.SourceMeta
	env  TextureEnv
}

func LoadTexture(src []byte, name string, env TextureEnv) (*TextureDocument, error) {
	doc, err := asset.ParseSourceMeta(src, name)
	if err != nil {
		return nil, err
	}
	if doc.Importer != "texture" {
		return nil, errs.New(errs.AssetBadMeta, "texture mutations require texture metadata")
	}
	return &TextureDocument{Name: name, doc: doc, env: env}, nil
}
func (d *TextureDocument) Doc() *asset.SourceMeta { return d.doc }
func (d *TextureDocument) Bytes() []byte          { return asset.MarshalSourceMeta(d.doc) }
func (d *TextureDocument) SourceProblems() []asset.TextureSourceProblem {
	return asset.ValidateTextureSource(d.doc, d.Name, d.env.Width, d.env.Height, d.env.Resolve)
}
func (d *TextureDocument) Problems() []Problem {
	var out []Problem
	for _, p := range d.SourceProblems() {
		out = append(out, Problem{Error: p.Error, Message: p.Code + ": " + p.Field + ": " + p.Message})
	}
	return out
}
func (d *TextureDocument) Apply(tx Tx) (*Result, error) {
	before := d.doc
	res, err := run(d, tx)
	if err != nil {
		d.doc = before
		return nil, err
	}
	restore := len(tx.Ops) > 0
	for _, op := range tx.Ops {
		restore = restore && op.Kind == KindTextureRestore
	}
	if !restore {
		for _, p := range d.SourceProblems() {
			if p.Error && !p.Missing {
				d.doc = before
				return nil, errs.New(errs.EditorProps, p.Code+": "+p.Field+": "+p.Message)
			}
		}
	}
	res.Problems = d.Problems()
	return res, nil
}
func (d *TextureDocument) DryRun(tx Tx) (*Result, error) {
	before := d.doc
	res, err := d.Apply(tx)
	d.doc = before
	if res != nil {
		res.DryRun, res.Inverse, res.Redo = true, Tx{}, Tx{}
	}
	return res, err
}
func (d *TextureDocument) do(op Op, _ *Result) (step, error) {
	before := d.Bytes()
	next, err := asset.ParseSourceMeta(before, d.Name)
	if err != nil {
		return step{}, err
	}
	change := Change{Op: op.Kind, Node: op.ID, Path: op.Path}
	frameID := ids.AssetID{}
	if op.ID != "" {
		frameID, err = ids.ParseAssetID(op.ID)
		if err != nil {
			return step{}, err
		}
	}
	frame := func() (*asset.SourceFrame, error) {
		f, ok := next.Frame(frameID)
		if !ok {
			return nil, fmt.Errorf("frame %s is absent", frameID)
		}
		return f, nil
	}
	switch op.Kind {
	case KindTextureSetting:
		if _, ok := asset.TextureSetting(op.Path); !ok {
			return step{}, fmt.Errorf("unregistered texture setting %q", op.Path)
		}
		v, err := op.value()
		if err != nil {
			return step{}, err
		}
		change.Before = FormatValue(next.Settings[op.Path])
		next.Settings[op.Path] = v.Clone()
		change.After = FormatValue(v)
	case KindTextureSettingReset:
		if _, ok := asset.TextureSetting(op.Path); !ok {
			return step{}, fmt.Errorf("unregistered texture setting %q", op.Path)
		}
		delete(next.Settings, op.Path)
	case KindTextureChannel, KindTextureChannelReset:
		target := ids.AssetID{}
		if op.Kind == KindTextureChannel {
			target, err = ids.ParseAssetID(op.Asset)
			if err != nil {
				return step{}, err
			}
		}
		if err = asset.SetTextureChannel(next, frameID, op.Path, target, op.Kind == KindTextureChannel); err != nil {
			return step{}, err
		}
	case KindTextureFrames:
		v, err := op.value()
		if err != nil {
			return step{}, err
		}
		if v.Kind != aetext.VList {
			return step{}, fmt.Errorf("texture-frames requires a list")
		}
		frames := make([]asset.SourceFrame, 0, len(v.List))
		for _, value := range v.List {
			f, err := decodeFrame(value, next)
			if err != nil {
				return step{}, err
			}
			frames = append(frames, f)
		}
		next.Subs = frames
	case KindTextureFrameAdd:
		v, err := op.value()
		if err != nil {
			return step{}, err
		}
		f, err := decodeFrame(v, next)
		if err != nil {
			return step{}, err
		}
		next.Subs = append(next.Subs, f)
	case KindTextureFrameRemove:
		if _, err := frame(); err != nil {
			return step{}, err
		}
		next.Subs = slices.DeleteFunc(next.Subs, func(f asset.SourceFrame) bool { return f.ID == frameID })
		// Retain associations from removed frame IDs so undo/restoration is lossless.
	case KindTextureFrameRename:
		f, err := frame()
		if err != nil {
			return step{}, err
		}
		change.Before, change.After = f.Name, op.Name
		f.Name = op.Name
	case KindTextureFrameRect:
		f, err := frame()
		if err != nil {
			return step{}, err
		}
		v, err := op.value()
		if err != nil {
			return step{}, err
		}
		f.Rect, err = decodeRect(v)
		if err != nil {
			return step{}, err
		}
	case KindTextureFrameOrder:
		f, err := frame()
		if err != nil {
			return step{}, err
		}
		if !op.HasIndex || op.Index < 0 || op.Index >= len(next.Subs) {
			return step{}, fmt.Errorf("frame index is outside the source")
		}
		copyFrame := *f
		next.Subs = slices.DeleteFunc(next.Subs, func(f asset.SourceFrame) bool { return f.ID == frameID })
		next.Subs = slices.Insert(next.Subs, op.Index, copyFrame)
	case KindTexturePivot:
		v, err := op.value()
		if err != nil {
			return step{}, err
		}
		if v.Kind != aetext.VList || len(v.List) != 2 {
			return step{}, fmt.Errorf("pivot requires two normalized coordinates")
		}
		p := [2]float32{}
		for i, x := range v.List {
			if x.Kind != aetext.VInt && x.Kind != aetext.VFloat {
				return step{}, fmt.Errorf("pivot coordinates must be numeric")
			}
			p[i] = float32(x.AsFloat())
			if !(p[i] >= 0 && p[i] <= 1) {
				return step{}, fmt.Errorf("pivot coordinates must be finite and in [0,1]")
			}
		}
		if frameID.IsZero() || frameID == next.ID {
			next.Pivot, next.HasPivot = p, true
		} else {
			f, err := frame()
			if err != nil {
				return step{}, err
			}
			f.Pivot, f.HasPivot = p, true
		}
	case KindTextureRestore:
		next, err = asset.ParseSourceMeta([]byte(op.Fragment), d.Name)
		if err != nil {
			return step{}, err
		}
		if next.ID != d.doc.ID || next.Importer != d.doc.Importer {
			return step{}, fmt.Errorf("restore cannot change source identity or importer")
		}
	default:
		return step{}, errs.New(errs.EditorCommand, op.Kind+" does not apply to texture metadata")
	}
	d.doc = next
	return step{inv: Op{Kind: KindTextureRestore, Fragment: string(before)}, redo: Op{Kind: KindTextureRestore, Fragment: string(d.Bytes())}, change: change}, nil
}
func decodeRect(v aetext.Value) ([4]int, error) {
	r := [4]int{}
	if v.Kind != aetext.VList || len(v.List) != 4 {
		return r, fmt.Errorf("frame rect needs four integer source pixel coordinates")
	}
	for i, x := range v.List {
		if x.Kind != aetext.VInt {
			return r, fmt.Errorf("frame rect coordinates must be integers")
		}
		r[i] = int(x.Int)
	}
	return r, nil
}
func decodeFrame(v aetext.Value, m *asset.SourceMeta) (asset.SourceFrame, error) {
	if v.Kind != aetext.VStruct {
		return asset.SourceFrame{}, fmt.Errorf("frame must be a structured value")
	}
	fields := asset.ImportSettings{}
	for _, f := range v.Obj {
		fields[f.Name] = f.Value
	}
	idv := fields["id"]
	id, err := ids.ParseAssetID(idv.Str)
	if err != nil || id.IsZero() {
		return asset.SourceFrame{}, fmt.Errorf("frame needs a stable nonzero AssetID")
	}
	f := asset.SourceFrame{ID: id}
	if old, ok := m.Frame(id); ok {
		f = *old
	}
	name := fields["name"]
	if name.Kind != aetext.VString {
		return f, fmt.Errorf("frame name must be a string")
	}
	f.Name = name.Str
	f.Rect, err = decodeRect(fields["rect"])
	if err != nil {
		return f, err
	}
	if p, ok := fields["pivot"]; ok {
		if p.Kind != aetext.VList || len(p.List) != 2 {
			return f, fmt.Errorf("frame pivot needs two coordinates")
		}
		for i, x := range p.List {
			f.Pivot[i] = float32(x.AsFloat())
			if (x.Kind != aetext.VInt && x.Kind != aetext.VFloat) || !(f.Pivot[i] >= 0 && f.Pivot[i] <= 1) {
				return f, fmt.Errorf("frame pivot must be finite and in [0,1]")
			}
		}
		f.HasPivot = true
	}
	return f, nil
}
