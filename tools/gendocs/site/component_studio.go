package site

// componentStudioActions connects graphic component pages to the same versioned
// standalone application and lessons used by the documentation embed. It never
// creates another canvas, runtime or copy of the drawing implementation.
func (b *builder) componentStudioActions(e *Entry, root string) string {
	if e == nil || e.Component == nil || (!b.opt.Studio && b.opt.StudioURL == "") {
		return ""
	}
	var lesson, title string
	switch e.ID {
	case "engine.SpriteRenderer":
		lesson, title = "first-sprite", "Draw your first sprite"
	case "animation.AnimationPlayer":
		lesson, title = "animation", "Frame-by-frame animation"
	case "animation.Animator":
		lesson, title = "character", "Character animation"
	case "particles.ParticleSystem2D":
		lesson, title = "explosion", "Explosion flipbook"
	case "engine.MaterialOverride", "engine.Light2D", "engine.LightOccluder2D":
		lesson, title = "surface-channels", "Emission and normal channels"
	default:
		return ""
	}
	lang := b.pageLang()
	// The development-only legacy shell has no versioned example routes.
	// Link only its existing entry point; release builds use StudioURL.
	editor := root + "studio/"
	guide := ""
	if b.opt.StudioURL != "" {
		editor = b.opt.StudioURL + "examples/" + lesson + "/?lang=" + lang
		guide = b.opt.StudioURL + "docs/" + lang + "/" + lesson + "/"
		if b.opt.StudioGuides {
			guide = root + "asset-studio/" + lang + "/" + lesson + "/"
		}
	}
	links := `<section class="component-studio" data-studio-example="` + esc(lesson) + `"><h2 id="asset-studio">` + esc(b.tr("Art Studio")) + `</h2><p><a class="button" href="` + esc(editor) + `">` + esc(b.tr(title)) + `</a>`
	if guide != "" {
		links += ` <a href="` + esc(guide) + `">` + esc(b.tr("Art Studio lessons")) + `</a>`
	}
	return links + "</p></section>\n"
}
