package site

import (
	"path/filepath"
	"strings"
)

// These are cross-links from real component pages to the relevant authoring
// lessons, not executable component demos or claims of runtime support.
type studioLesson struct {
	ID    string
	Title string
}

var componentStudioLessons = map[string][]studioLesson{
	"engine.SpriteRenderer":      {{"first-sprite", "Draw your first sprite"}, {"export", "Export and use in Aego"}},
	"animation.AnimationPlayer":  {{"animation", "Frame-by-frame animation"}},
	"animation.Animator":         {{"character", "Character animation"}},
	"particles.ParticleSystem2D": {{"explosion", "Explosion flipbook"}},
	"engine.MaterialOverride":    {{"surface-channels", "Emission and normal channels"}},
	"engine.Light2D":             {{"surface-channels", "Emission and normal channels"}},
	"engine.LightOccluder2D":     {{"surface-channels", "Emission and normal channels"}},
}

func (b *builder) componentStudioLinks(e *Entry, root string) string {
	if actions := b.componentStudioActions(e, root); actions != "" {
		return actions
	}
	if !b.opt.StudioGuides || e == nil || e.Component == nil {
		return ""
	}
	var links []string
	lang := b.pageLang()
	for _, lesson := range componentStudioLessons[e.ID] {
		if !isFile(filepath.Join(b.out, "asset-studio", lang, lesson.ID, "index.html")) {
			continue
		}
		href := root + "asset-studio/" + lang + "/" + lesson.ID + "/"
		links = append(links, `<li><a href="`+esc(href)+`">`+esc(b.tr(lesson.Title))+`</a></li>`)
	}
	if len(links) == 0 {
		return ""
	}
	return `<h2 id="` + esc(e.ID) + `-art-studio">` + esc(b.tr("Art Studio lessons")) + "</h2>\n<ul>" + strings.Join(links, "") + "</ul>\n"
}
