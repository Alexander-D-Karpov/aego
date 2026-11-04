package mutate

import (
	"fmt"
	"slices"

	"aego/asset"
	"aego/core/errs"
	"aego/core/ids"
	"aego/format/aetext"
	"aego/render/material"
	"aego/render/shader"
)

const (
	KindMaterialSet      = "material-set"
	KindMaterialReset    = "material-reset"
	KindMaterialShader   = "material-shader"
	KindMaterialVariants = "material-variants"
	KindMaterialBlend    = "material-blend"
	KindMaterialRestore  = "material-restore"
)

var materialKinds = map[string]bool{
	KindMaterialSet: true, KindMaterialReset: true, KindMaterialShader: true,
	KindMaterialVariants: true, KindMaterialBlend: true, KindMaterialRestore: true,
}

func MaterialSet(name, text string) Op {
	return Op{Kind: KindMaterialSet, Path: name, Text: text}
}

func MaterialSetValue(name string, value aetext.Value) Op {
	return Op{Kind: KindMaterialSet, Path: name, Value: value.Clone(), HasValue: true}
}

func MaterialReset(name string) Op {
	return Op{Kind: KindMaterialReset, Path: name}
}

func MaterialShader(id string) Op {
	return Op{Kind: KindMaterialShader, Asset: id}
}

func MaterialVariants(keywords []string) Op {
	values := make([]aetext.Value, len(keywords))
	for i, keyword := range keywords {
		values[i] = aetext.String(keyword)
	}
	return Op{Kind: KindMaterialVariants, Value: aetext.Value{Kind: aetext.VList, List: values}, HasValue: true}
}

// MaterialBlend accepts an empty name to restore the shader's blend default.
func MaterialBlend(name string) Op {
	return Op{Kind: KindMaterialBlend, Name: name}
}

// MaterialDocument is the common semantic mutation target for Inspector,
// CLI and MCP. It owns source values, never a shared GPU material or play state.
type MaterialDocument struct {
	Name    string
	doc     *asset.MaterialDoc
	resolve func(ids.AssetID) (*material.Shader, bool)
}

func LoadMaterial(src []byte, name string, resolve func(ids.AssetID) (*material.Shader, bool)) (*MaterialDocument, error) {
	doc, err := asset.ParseMaterial(src, name)
	if err != nil {
		return nil, err
	}
	return &MaterialDocument{Name: name, doc: doc, resolve: resolve}, nil
}

func (d *MaterialDocument) Doc() *asset.MaterialDoc { return d.doc }

func (d *MaterialDocument) Bytes() []byte { return asset.MarshalMaterial(d.doc) }

func (d *MaterialDocument) Shader() (*material.Shader, bool) {
	if d.resolve == nil {
		return nil, false
	}
	return d.resolve(d.doc.Shader)
}

func (d *MaterialDocument) Problems() []Problem {
	sh, ok := d.Shader()
	if !ok || sh == nil {
		return []Problem{{Error: true, Message: "MAT004: the shader is missing or its current source is invalid: " + d.doc.Shader.String()}}
	}
	var out []Problem
	for _, p := range d.doc.Validate(sh) {
		out = append(out, Problem{Error: !p.Warn, Message: p.Code + ": " + p.Msg})
	}
	return out
}

func (d *MaterialDocument) Apply(tx Tx) (*Result, error) {
	before := d.doc
	res, err := run(d, tx)
	if err != nil {
		d.doc = before
		return nil, err
	}
	res.Problems = d.Problems()
	// History can restore a formerly valid document after its shader changed.
	// Preserve that source and show diagnostics; undo must never delete values.
	restore := len(tx.Ops) > 0
	for _, op := range tx.Ops {
		restore = restore && op.Kind == KindMaterialRestore
	}
	if !restore {
		for _, p := range res.Problems {
			if p.Error {
				d.doc = before
				return nil, errs.New(errs.EditorProps, p.Message)
			}
		}
	}
	return res, nil
}

func (d *MaterialDocument) DryRun(tx Tx) (*Result, error) {
	before := d.doc
	res, err := d.Apply(tx)
	d.doc = before
	if res != nil {
		res.Inverse, res.Redo, res.DryRun = Tx{}, Tx{}, true
	}
	return res, err
}

func (d *MaterialDocument) do(op Op, _ *Result) (step, error) {
	before := d.Bytes()
	next, err := asset.ParseMaterial(before, d.Name)
	if err != nil {
		return step{}, err
	}
	change := Change{Op: op.Kind, Path: op.Path}
	switch op.Kind {
	case KindMaterialSet:
		value, err := op.value()
		if err != nil {
			return step{}, err
		}
		if sh, ok := d.Shader(); ok {
			if p, exists := sh.Schema.Param(op.Path); !exists {
				return step{}, errs.New(errs.EditorProps, "the shader has no parameter "+op.Path)
			} else {
				// Aliases name the same semantic property. Keeping both would
				// make effective value depend on serialization order.
				op.Path = p.Name
				for _, former := range p.Former {
					next.Unset(former)
				}
			}
		}
		if old, ok := next.Value(op.Path); ok {
			change.Before = FormatValue(old)
		}
		if err := next.SetValue(op.Path, value); err != nil {
			return step{}, err
		}
		change.After = FormatValue(value)
	case KindMaterialReset:
		if old, ok := next.Value(op.Path); ok {
			change.Before = FormatValue(old)
		}
		next.Unset(op.Path)
		if sh, ok := d.Shader(); ok {
			if p, exists := sh.Schema.Param(op.Path); exists {
				next.Unset(p.Name)
				for _, former := range p.Former {
					next.Unset(former)
				}
			}
		}
	case KindMaterialShader:
		id, err := ids.ParseAssetID(op.Asset)
		if err != nil || id.IsZero() {
			return step{}, errs.New(errs.EditorProps, "shader needs a nonzero asset ID")
		}
		change.Path, change.Before, change.After = "shader", next.Shader.String(), id.String()
		next.Shader = id
	case KindMaterialVariants:
		value, err := op.value()
		if err != nil {
			return step{}, err
		}
		if value.Kind != aetext.VList {
			return step{}, errs.New(errs.EditorProps, "variants must be a list of keyword names")
		}
		keywords := make([]string, 0, len(value.List))
		for _, v := range value.List {
			keyword := wordOf(v)
			if (v.Kind != aetext.VString && v.Kind != aetext.VIdent) || keyword == "" || slices.Contains(keywords, keyword) {
				return step{}, errs.New(errs.EditorProps, "variants must contain distinct, nonempty keyword names")
			}
			keywords = append(keywords, keyword)
		}
		slices.Sort(keywords)
		change.Path, change.Before, change.After = "variants", fmt.Sprint(next.Variants), fmt.Sprint(keywords)
		next.Variants = keywords
	case KindMaterialBlend:
		change.Path = "blend"
		if next.HasBlend {
			change.Before = next.Blend.String()
		}
		next.HasBlend = op.Name != ""
		if next.HasBlend {
			blend, ok := shader.ParseBlend(op.Name)
			if !ok {
				return step{}, errs.New(errs.EditorProps, "blend is premultiplied, opaque or additive")
			}
			next.Blend = blend
			change.After = op.Name
		}
	case KindMaterialRestore:
		next, err = asset.ParseMaterial([]byte(op.Fragment), d.Name)
		if err != nil {
			return step{}, err
		}
	default:
		return step{}, errs.New(errs.EditorCommand, op.Kind+" does not apply to material assets")
	}
	d.doc = next
	return step{
		inv:    Op{Kind: KindMaterialRestore, Fragment: string(before)},
		redo:   Op{Kind: KindMaterialRestore, Fragment: string(d.Bytes())},
		change: change,
	}, nil
}
