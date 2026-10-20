//go:build aego_golden

package golden

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"aego/app/projects"
	"aego/core/mathx"
	"aego/editor/shell/launcher"
	"aego/project"
)

func TestEditorLauncher(t *testing.T) {
	env := newEditorEnv(t)
	local := time.Local
	time.Local = time.UTC
	defer func() { time.Local = local }()
	alive := filepath.Join(os.TempDir(), "aego-golden-recent")
	os.MkdirAll(alive, 0o755)
	defer os.RemoveAll(alive)
	os.WriteFile(filepath.Join(alive, project.FileName), []byte("project version=1 {\n}\n"), 0o644)
	recent := filepath.Join(t.TempDir(), "recent")
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	launcher.Touch(recent, "/nonexistent/old-game", "Old Game", at)
	launcher.Touch(recent, alive, "Live Game", at.Add(time.Hour))
	var tmpl []launcher.Template
	for _, t := range projects.Templates {
		tmpl = append(tmpl, launcher.Template{ID: t.ID, Name: t.Name, Description: t.Description, Icon: t.Icon})
	}
	l := launcher.New(launcher.Options{Templates: tmpl, RecentPath: recent, Home: "/home/dev/games"})
	img, err := env.draw(func(full mathx.Rect) { l.Draw(env.ui, full) })
	checkGolden(t, "editor_launcher", img, err)
}

func TestEditorLauncherNewProject(t *testing.T) {
	env := newEditorEnv(t)
	var tmpl []launcher.Template
	for _, t := range projects.Templates {
		id := t.ID
		tmpl = append(tmpl, launcher.Template{ID: t.ID, Name: t.Name, Description: t.Description, Icon: t.Icon,
			Files: projects.TemplateFiles(id)})
	}
	l := launcher.New(launcher.Options{
		Templates:  tmpl,
		Workspaces: []launcher.Choice{{ID: "design", Title: "Design"}, {ID: "programming", Title: "Programming"}},
		Home:       "/home/dev/games",
	})
	l.SetTab(launcher.TabNew)
	img, err := env.draw(func(full mathx.Rect) { l.Draw(env.ui, full) })
	checkGolden(t, "editor_launcher_new", img, err)
}
