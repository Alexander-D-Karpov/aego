package agentbridge

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"

	"aego/app/agent/mcp"
	"aego/core/ids"
	"aego/editor/core/document"
)

// materialText reads only authored shader/material text inside the project.
// The shared source buffer covers both the code editor and structured material inspector.
func materialText(docs *document.Manager, name string) (string, error) {
	const limit = 256 << 10
	if !fs.ValidPath(name) || strings.ContainsAny(name, "\\:") || (path.Ext(name) != ".aeshader" && path.Ext(name) != ".aemat") {
		return "", errors.New("expected a project-relative .aeshader or .aemat")
	}
	if docs == nil {
		return "", errors.New("editor documents unavailable")
	}
	var source []byte
	if d := docs.Script(name); d != nil {
		source = d.Code.Bytes()
	} else {
		f, err := docs.OpenSource(name)
		if err != nil {
			return "", err
		}
		defer f.Close()
		source, err = io.ReadAll(io.LimitReader(f, limit+1))
		if err != nil {
			return "", err
		}
	}
	if len(source) > limit {
		return "", fmt.Errorf("asset source exceeds %d bytes", limit)
	}
	return string(source), nil
}

func (b *Bridge) materialSourceTool() mcp.Tool {
	return mcp.Tool{Name: "editor_material_source", ReadOnly: true,
		Description: "Read the current project .aeshader or .aemat source by relative path or AssetID, including unsaved edits.",
		Schema:      visualSchema([]string{"asset"}, map[string]any{"asset": map[string]any{"type": "string"}}),
		Run: func(args map[string]any) (string, bool) {
			name, _ := args["asset"].(string)
			var text string
			var err error
			generation := b.panelGeneration.Load()
			if e := b.main(func() {
				if b.cancelled(generation) {
					err = errCancelled
					return
				}
				if id, e := ids.ParseAssetID(name); e == nil {
					if b.ctx.Pipeline == nil {
						err = errors.New("asset index unavailable")
						return
					}
					var ok bool
					if name, ok = b.ctx.Pipeline.PathOf(id); !ok {
						err = errors.New("asset not found")
						return
					}
				}
				text, err = materialText(b.ctx.Docs, name)
			}); e != nil {
				return e.Error(), true
			}
			if err != nil {
				return err.Error(), true
			}
			return text, false
		}}
}
