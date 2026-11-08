package shell

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aego/core/log"
	"aego/core/mathx"
	"aego/editor/core/command"
	"aego/editor/core/context"
	"aego/editor/core/document"
	"aego/editor/shell/launcher"
	"aego/engine"
	"aego/engine/i18n"
	"aego/platform/input"
	"aego/project"
	"aego/ui/core"
	"aego/ui/widgets"
)

func TestRecentNamesDisambiguateProjects(t *testing.T) {
	entries := []launcher.Entry{
		{Name: "Game", Path: "/first/Game"},
		{Name: "Game", Path: "/second/Game"},
		{Name: "Other", Path: "/third/Other"},
	}
	names := recentNames(entries)
	if names[0] == names[1] || !strings.Contains(names[0], entries[0].Path) || !strings.Contains(names[1], entries[1].Path) || names[2] != "Other" {
		t.Fatalf("ambiguous recent names: %q", names)
	}
}

func TestRecentElidedNamesStayDistinct(t *testing.T) {
	ui := core.New(nil)
	common := strings.Repeat("long-directory/", 20)
	names := []string{"Game — /" + common + "first/" + common, "Game — /" + common + "second/" + common}
	labels := recentMenuLabels(ui, names, 180)
	if labels[0] == labels[1] {
		t.Fatalf("elided paths are indistinguishable: %q", labels)
	}
	for _, label := range labels {
		if ui.TextSize(label).X > 180 {
			t.Fatalf("label does not fit: %q", label)
		}
	}
}

func TestRecentMenuSelectsDuplicateAndScrolledProjects(t *testing.T) {
	for _, tc := range []struct {
		name        string
		count       int
		selectIndex int
		height      float32
	}{
		{"second project with same name", 2, 1, 600},
		{"last project after scrolling", launcher.MaxRecent, launcher.MaxRecent - 1, 240},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			entries := make([]launcher.Entry, tc.count)
			for i := range entries {
				dir := filepath.Join(root, fmt.Sprintf("project-%02d", i))
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, project.FileName), nil, 0644); err != nil {
					t.Fatal(err)
				}
				entries[i] = launcher.Entry{Path: dir, Name: "Shared"}
			}
			recentFile := filepath.Join(root, "recent")
			if err := launcher.SaveRecent(recentFile, entries); err != nil {
				t.Fatal(err)
			}
			e, err := engine.NewBuilder(engine.Config{Headless: true, ResRoot: t.TempDir(), UserRoot: t.TempDir()}).WithLogger(log.Nop()).Build()
			if err != nil {
				t.Fatal(err)
			}
			defer e.Close()
			ui, in := core.New(nil), &input.State{}
			var opened string
			a := &App{
				eng: e, ui: ui,
				ctx: &context.Editor{Commands: command.New(), Docs: document.NewManager(document.Options{Root: root})},
				opt: Options{Project: &project.Project{Root: root}, Recent: recentFile, Relaunch: func(target, _ string) error { opened = target; return nil }},
			}
			if err := a.registerProject(); err != nil {
				t.Fatal(err)
			}
			step := func(at mathx.Vec2, down, up bool, wheel float32) {
				in.BeginFrame()
				in.ApplyMouseMove(at, mathx.Vec2{})
				if down {
					in.ApplyMouseDown(at, input.MouseLeft)
				}
				if up {
					in.ApplyMouseUp(at, input.MouseLeft)
				}
				if wheel != 0 {
					in.ApplyWheel(at, mathx.Vec2{Y: wheel})
				}
				ui.Begin(in, mathx.Vec2{X: 1000, Y: tc.height}, 1)
				bar := widgets.BeginMenuBar(ui, mathx.Rect{W: 1000, H: 28})
				if bar.Menu("File", 280) {
					a.commandItems("File")
					bar.EndMenu()
				}
				ui.End()
			}
			step(mathx.Vec2{X: 12, Y: 10}, true, false, 0)
			step(mathx.Vec2{X: 12, Y: 10}, false, true, 0)
			rowH := ui.Theme.RowHeight + ui.Theme.Spacing
			parent := mathx.Vec2{X: 60, Y: 28 + ui.Theme.Padding + 2*rowH + ui.Theme.RowHeight/2}
			step(parent, false, false, 0)
			step(parent, false, false, 0)
			subID := ui.ID("sub/" + i18n.Tr("editor.shell.open_recent", "Open Recent"))
			if !ui.PopupOpen(subID) {
				t.Fatal("hovering Open Recent did not open its submenu")
			}
			areaH := min(float32(tc.count)*rowH, max(rowH, tc.height-2*ui.Theme.Padding-4))
			subY := 28 + 2*rowH
			if subY+areaH+2*ui.Theme.Padding > tc.height {
				subY = tc.height - areaH - 2*ui.Theme.Padding
			}
			at := mathx.Vec2{X: 320, Y: subY + ui.Theme.Padding + ui.Theme.RowHeight/2}
			if tc.selectIndex >= 5 {
				step(at, false, false, -100)
				if a.recentScroll <= 0 {
					t.Fatal("long recent-project list did not scroll")
				}
			}
			at.Y += float32(tc.selectIndex)*rowH - a.recentScroll
			step(at, true, false, 0)
			step(at, false, true, 0)
			if opened != entries[tc.selectIndex].Path {
				t.Fatalf("selected %q, want %q", opened, entries[tc.selectIndex].Path)
			}
		})
	}
}
