//go:build aego_golden

package golden

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"aego/app/projects"
	"aego/core/mathx"
	"aego/editor/shell/launcher"
	"aego/engine/i18n"
	"aego/ui/theme"
)

type overflow struct {
	key, text  string
	scale      float32
	need, have float32
	where      string
}

type keyMatcher struct {
	key string
	re  *regexp.Regexp
}

// matchers turn every ru text into a pattern with its placeholders as wildcards, longest first.
func matchers() []keyMatcher {
	c := i18n.Default().Catalog(i18n.Russian)
	var out []keyMatcher
	add := func(k, text string) {
		if strings.TrimSpace(text) == "" {
			return
		}
		pat := regexp.QuoteMeta(text)
		pat = regexp.MustCompile(`\\\{[A-Za-z_][A-Za-z0-9_]*\\\}`).ReplaceAllString(pat, `.*`)
		out = append(out, keyMatcher{k, regexp.MustCompile("^" + pat + "$")})
	}
	for k, m := range c.Msgs {
		add(k, m.Text)
		for _, t := range m.Plural {
			add(k, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return len(out[i].re.String()) > len(out[j].re.String()) })
	return out
}

func (o *overflow) find(ms []keyMatcher) bool {
	s := strings.TrimSpace(o.text)
	for _, m := range ms {
		if m.re.MatchString(s) {
			o.key = m.key
			return true
		}
	}
	return false
}

// Every Russian string drawn by the key panels fits its widget at UI scale 1 and 2; the list goes to .cache/i18n-overflow.txt (D348).
func TestRussianOverflow(t *testing.T) {
	russian(t)
	ms := matchers()
	seen := map[string]*overflow{}
	record := func(env *editorEnv, where string) {
		env.ui.Overflow = func(text string, need, have float32) {
			o := &overflow{text: text, scale: max(1, env.scale), need: need, have: have, where: where}
			if !o.find(ms) {
				return
			}
			id := fmt.Sprintf("%s@%g", o.key, o.scale)
			if old, ok := seen[id]; !ok || need-have > old.need-old.have {
				seen[id] = o
			}
		}
	}
	for _, scale := range []float32{1, 2} {
		for _, sc := range []struct {
			name string
			w, h int
			shot func(t *testing.T, env *editorEnv)
		}{
			{"inspector", editorW, editorH, func(t *testing.T, env *editorEnv) { shotInspector(t, env) }},
			{"hierarchy", 320, 240, func(t *testing.T, env *editorEnv) { shotHierarchyPrefab(t, env) }},
			{"problems", 640, 200, func(t *testing.T, env *editorEnv) { shotProblems(t, env) }},
			{"launcher", editorW, editorH, func(t *testing.T, env *editorEnv) { shotLauncher(t, env) }},
			{"launcher.new", editorW, editorH, func(t *testing.T, env *editorEnv) { drawLauncherTab(env, launcher.TabNew) }},
			{"launcher.examples", editorW, editorH, func(t *testing.T, env *editorEnv) { drawLauncherTab(env, launcher.TabExamples) }},
			{"menu", editorW, editorH, func(t *testing.T, env *editorEnv) { shotMenu(t, env) }},
		} {
			env := newEditorEnvSize(t, sc.w, sc.h)
			env.scale = scale
			record(env, sc.name)
			sc.shot(t, env)
		}
	}
	s := paletteSession(t)
	ui := s.App.Context().UI
	for _, scale := range []float32{1, 2} {
		s.App.SetDensity(scale)
		sess := &editorEnv{ui: ui, scale: scale}
		record(sess, "editor")
		for range 4 {
			s.Step()
		}
		record(sess, "palette")
		s.App.AddPalette().AddComponent(s.App.Context().Selected(), nil)
		for range 4 {
			s.Step()
		}
		ui.Overflow = nil
	}

	list := make([]*overflow, 0, len(seen))
	for _, o := range seen {
		list = append(list, o)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].key != list[j].key {
			return list[i].key < list[j].key
		}
		return list[i].scale < list[j].scale
	})
	var b strings.Builder
	fmt.Fprintf(&b, "%-50s %-5s %-6s %-6s %-12s %s\n", "KEY", "SCALE", "NEED", "HAVE", "WHERE", "TEXT")
	for _, o := range list {
		fmt.Fprintf(&b, "%-50s %-5g %-6.0f %-6.0f %-12s %s\n", o.key, o.scale, o.need, o.have, o.where, o.text)
	}
	fmt.Fprintf(&b, "i18n overflow: %d string(s) wider than their widget\n", len(list))
	out := filepath.Join("..", "..", ".cache", "i18n-overflow.txt")
	os.MkdirAll(filepath.Dir(out), 0o755)
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + b.String())
}

func drawLauncherTab(env *editorEnv, tab launcher.Tab) {
	var tmpl []launcher.Template
	for _, t := range projects.Templates {
		tmpl = append(tmpl, launcher.Template{ID: t.ID, Name: t.Name, Description: t.Description, Icon: t.Icon, Files: projects.TemplateFiles(t.ID)})
	}
	l := launcher.New(launcher.Options{
		Templates:   tmpl,
		Workspaces:  []launcher.Choice{{ID: "design", Title: "Design"}, {ID: "programming", Title: "Programming"}},
		Home:        "/home/dev/games",
		Theme:       func() theme.Mode { return theme.ModeSystem },
		SetTheme:    func(theme.Mode) {},
		Language:    func() string { return i18n.Russian },
		SetLanguage: func(string) {},
	})
	l.SetTab(tab)
	env.draw(func(full mathx.Rect) { l.Draw(env.ui, full) })
}
