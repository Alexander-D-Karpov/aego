package site

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIconCatalogUsesTheActualSpriteInHTMLAndMarkdown(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"z.last", "a.first"} {
		data := `<svg viewBox="0 0 16 16"><path fill="#fff" d="M0 0h4v4z"/></svg>`
		if err := os.WriteFile(filepath.Join(dir, name+".svg"), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	b := &builder{icons: loadIcons([]string{dir}), missing: map[string]bool{}}
	html := b.iconCatalog("../")
	md := b.pageMD(&Entry{ID: "editor/icons", Title: "Icon catalog"})
	for _, name := range []string{"a.first", "z.last"} {
		if !strings.Contains(html, "#"+name) || !strings.Contains(md, "`"+name+"`") {
			t.Fatalf("icon %s missing from catalog", name)
		}
	}
	if strings.Index(html, "a.first") > strings.Index(html, "z.last") || !strings.Contains(b.icons.sprite, `fill="currentColor"`) {
		t.Fatal("catalog order or theme-aware sprite is incorrect")
	}
}

func TestDocumentationTaskIconsAreSemanticAndExist(t *testing.T) {
	dirs, err := filepath.Glob("../../../modules/*/res/icons/src")
	if err != nil {
		t.Fatal(err)
	}
	icons := loadIcons(append(dirs, "../../../engine/res/icons/src"))
	b := &builder{}
	for id, want := range map[string]string{
		"platformer": "doc.tutorial", "platformer-effects": "render2d.point_light", "credits": "help.about",
		"quickstart": "doc.project", "curves": "ui.curve", "render2d/normal-mapping": "render2d.normals",
		"render2d.PostProcess": "view.overlays", "camera.CameraBrain2D": "view.split",
	} {
		if got := b.entryIcon(&Entry{ID: id, Kind: KindPage, Icon: "doc.tutorial"}); got != want {
			t.Errorf("%s: got %s, want %s", id, got, want)
		}
	}
	for id, icon := range taskIcons {
		if !icons.names[icon] {
			t.Errorf("%s uses missing task icon %s", id, icon)
		}
	}
}
