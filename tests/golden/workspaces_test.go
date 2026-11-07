//go:build aego_golden

package golden

import (
	"testing"

	"aego/core/mathx"
	"aego/core/trace"
	"aego/editor/core/services"
	"aego/editor/core/workspace"
	"aego/editor/panels/events"
	"aego/editor/panels/watch"
	"aego/editor/shell"
	"aego/ui/core"
)

type panelStub string

func (s panelStub) ID() string    { return string(s) }
func (s panelStub) Title() string { return string(s) }

func (s panelStub) Draw(f *services.Frame, r mathx.Rect) {
	f.UI.TextIn(r, string(s), f.UI.Theme.TextDim, core.AlignCenter)
}

// Dynamic art documents use a shared scope; their AssetIDs are not panel IDs
// baked into the factory workspace presets.
type artDocumentStub struct{ panelStub }

func (artDocumentStub) Scope() string { return "render2d.art" }

func goldenWorkspace(t *testing.T, authoring bool) *workspace.Workspace {
	t.Helper()
	ws := workspace.New(t.TempDir(), nil)
	ws.Tracer = trace.New(trace.Options{})
	descs := []workspace.Desc{
		{Panel: panelStub(workspace.Hierarchy), Slot: workspace.SlotLeft, Order: 100},
		{Panel: panelStub(workspace.Assets), Slot: workspace.SlotBrowser, Order: 110},
		{Panel: panelStub(workspace.Scene), Slot: workspace.SlotCenter, Order: 120},
		{Panel: panelStub(workspace.Game), Slot: workspace.SlotCenter, Order: 130},
		{Panel: panelStub(workspace.Inspector), Slot: workspace.SlotRight, Order: 140},
		{Panel: panelStub(workspace.Console), Slot: workspace.SlotBottom, Order: 150},
		{Panel: panelStub(workspace.Problems), Slot: workspace.SlotBottom, Order: 160},
		{Panel: panelStub(workspace.Profiler), Slot: workspace.SlotBottom, Order: 170},
		{Panel: panelStub(events.ID), Slot: workspace.SlotBottom, Order: 175},
		{Panel: panelStub(watch.ID), Slot: workspace.SlotBottom, Order: 176},
	}
	if authoring {
		// Mirror the registered Art docks and an open art document when
		// checking the new factory layouts; empty placeholder workspaces would
		// not exercise the panels or the dynamic document scope at all.
		descs = append(descs,
			workspace.Desc{Panel: artDocumentStub{panelStub("render2d.art/fixture")}, Slot: workspace.SlotCenter},
			workspace.Desc{Panel: panelStub("art.properties"), Slot: workspace.SlotRight},
			workspace.Desc{Panel: panelStub("art.timeline"), Slot: workspace.SlotBottom},
			workspace.Desc{Panel: panelStub("art.preview"), Slot: workspace.SlotCenter},
		)
	}
	for _, d := range descs {
		if err := ws.Register(d); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range shell.BuiltinWorkspaces() {
		if err := ws.RegisterPreset(p); err != nil {
			t.Fatal(err)
		}
	}
	ws.LoadLayout()
	return ws
}

func TestWorkspaceGolden(t *testing.T) {
	env := newEditorEnv(t)
	shot := func(name string, ws *workspace.Workspace) {
		img, err := env.draw(func(full mathx.Rect) { ws.Draw(services.Frame{UI: env.ui}, full) })
		t.Run(name, func(t *testing.T) { checkGolden(t, name, img, err) })
	}
	for _, p := range shell.BuiltinWorkspaces() {
		authoring := p.ID == workspace.Animation || p.ID == workspace.VFX || p.ID == workspace.Default
		ws := goldenWorkspace(t, authoring)
		ws.Switch(p.ID)
		shot("editor_workspace_"+p.ID, ws)
	}
	ws := goldenWorkspace(t, false)
	ws.ToggleMaximize(workspace.Scene)
	shot("editor_workspace_maximized", ws)
}
