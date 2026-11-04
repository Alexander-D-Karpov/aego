package document

import (
	"aego/core/errs"
	"aego/scene/mutate"
)

// ApplyTextureMutation uses the source metadata's existing Code undo journal.
// Sliders pass continuing=true during a drag; only the final Save enters pipeline.
func ApplyTextureMutation(d *ScriptDocument, env mutate.TextureEnv, tx mutate.Tx, continuing bool) (*mutate.Result, error) {
	if d == nil || d.ReadOnly {
		return nil, errs.New(errs.EditorProps, "texture metadata is read-only")
	}
	if d.Conflict {
		return nil, errs.New(errs.EditorProps, "texture metadata changed on disk; resolve the document conflict first")
	}
	m, err := mutate.LoadTexture(d.Code.Bytes(), d.Path, env)
	if err != nil {
		return nil, err
	}
	res, err := m.Apply(tx)
	if err != nil {
		return nil, err
	}
	d.Code.ReplaceSnapshot(m.Bytes(), continuing)
	return res, nil
}
