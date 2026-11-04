package document

import (
	"aego/core/ids"
	"aego/render/material"
	"aego/scene/mutate"
)

// ApplyMaterialMutation edits the existing source document. Inspector and the
// code panel share this buffer and its undo history; callers supply the current
// checked shader schema, never mutate the loader's shared runtime material.
func ApplyMaterialMutation(d *ScriptDocument, resolve func(ids.AssetID) (*material.Shader, bool), tx mutate.Tx, continuing bool) (*mutate.Result, error) {
	m, err := mutate.LoadMaterial(d.Code.Bytes(), d.Path, resolve)
	if err != nil {
		return nil, err
	}
	result, err := m.Apply(tx)
	if err != nil {
		return nil, err
	}
	d.Code.ReplaceSnapshot(m.Bytes(), continuing)
	return result, nil
}
