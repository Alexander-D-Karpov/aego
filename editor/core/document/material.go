package document

import (
	"aego/core/ids"
	"aego/render/material"
	"aego/scene/mutate"
)

// ApplyMaterialMutation edits the Inspector/code panel's shared buffer and history using the caller's current checked shader schema, never the loader's shared runtime material.
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
