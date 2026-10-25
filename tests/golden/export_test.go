//go:build aego_golden

package golden

import (
	"testing"

	"aego/core/mathx"
	"aego/editor/core/services"
	exportpanel "aego/editor/panels/export"
	"aego/engine/problems"
	"aego/ui/dnd"
)

type fakeExporter struct{}

func (fakeExporter) Targets() []string { return []string{"host", "linux", "windows", "darwin", "web"} }
func (fakeExporter) Out(target, profile string) string {
	return "/home/dev/Projects/game/exports/" + target + "-amd64-" + profile
}
func (fakeExporter) Prepare() any                            { return nil }
func (fakeExporter) Check(any, string) []problems.Diagnostic { return nil }
func (fakeExporter) Reveal(string) error                     { return nil }
func (fakeExporter) Export(any, services.ExportRequest, func(float32, string)) (services.ExportReport, error) {
	return services.ExportReport{}, nil
}

func TestEditorExportPanel(t *testing.T) {
	env := newEditorEnvSize(t, 960, 210)
	p := exportpanel.New(exportpanel.Deps{Exporter: fakeExporter{}, PickFolder: func(string, func(string)) {}})
	draw := func(name string) {
		t.Helper()
		img, err := env.draw(func(full mathx.Rect) {
			p.Draw(&services.Frame{UI: env.ui, DnD: &dnd.State{}}, full)
		})
		checkGolden(t, name, img, err)
	}
	draw("editor_export_idle")
	p.ShowReport("linux", exportpanel.Release, services.ExportReport{
		Out:    "/home/dev/Projects/game/exports/linux-amd64-release",
		Lines:  [][2]string{{"game", "18.4 MB"}, {"assets.aepak", "2.1 MB"}, {"Scenes", "3"}, {"Scripts", "12"}},
		Issues: []problems.Diagnostic{{Code: "ASSET004", Severity: problems.Warning, Message: "texture is larger than 4096 px", Subject: problems.Subject{File: "assets/bg.png"}}},
	}, "Exported to linux-amd64-release")
	draw("editor_export_done")
}
