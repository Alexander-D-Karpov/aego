package shell

import (
	"image/color"
	"strings"

	"aego/core/ids"
	"aego/editor/core/assetops"
	"aego/editor/core/command"
	"aego/editor/core/workspace"
	"aego/editor/panels/health"
	"aego/editor/panels/references"
	"aego/editor/panels/vcsdiff"
	"aego/engine/module"
	"aego/engine/problems"
	"aego/modules/art"
)

const CommandBatchRenameNodes = "edit.batch_rename"

func (a *App) cameraTypes() []string {
	var out []string
	for _, td := range a.opt.Types.Schema().Components() {
		if strings.HasSuffix(td.FullName, "Camera2D") || strings.HasSuffix(td.FullName, "Camera3D") {
			out = append(out, td.FullName)
		}
	}
	return out
}

func (a *App) moveAssets(ops []assetops.FileOp) error {
	return a.files.History().Apply(a.ctx, ops)
}

func (a *App) batchRenameAssets(list []ids.AssetID) {
	a.renameDlg.OpenAssets(a.ui, list)
}

func (a *App) compare(rel string) {
	a.diffPanel.Show(rel)
	a.ws.Show(vcsdiff.ID)
}

func (a *App) healthDeps() health.Deps {
	return health.Deps{
		Pipeline: a.opt.Pipeline,
		Project:  a.opt.Project,
		Docs:     a.ctx,
		Tasks:    a.tasks,
		Problems: func() []problems.Diagnostic {
			if a.opt.Problems == nil {
				return nil
			}
			list, _ := a.opt.Problems.Collect()
			return list
		},
		Missing: a.missing.Count,
		Cameras: a.cameraTypes,
		Palette: func() []color.NRGBA {
			if v, ok := module.SectionValue[art.Art](a.ctx.Sections, art.SectionID); ok {
				return v.Palette
			}
			return nil
		},
		ShowMissing:  func() { a.ws.Show(references.MissingID) },
		ShowProblems: func() { a.ws.Show(workspace.Problems) },
		Reveal:       a.ctx.RevealPath,
		Export:       a.exportWork,
	}
}

func (a *App) registerAssetTools() error {
	return a.ctx.Commands.Register(command.Desc{
		ID: CommandBatchRenameNodes, Name: "Batch Rename Nodes…", Category: "Edit", Order: 405,
		Enabled: func() bool { return len(a.ctx.Selected()) > 1 },
		Run: func() error {
			a.renameDlg.OpenNodes(a.ui, a.ctx.Doc(), append([]ids.NodeID(nil), a.ctx.Selected()...))
			return nil
		},
	})
}
