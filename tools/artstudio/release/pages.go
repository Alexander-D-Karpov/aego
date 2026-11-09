package main

import (
	"fmt"
	"html"
	"path/filepath"
	"strings"
)

func esc(v string) string { return html.EscapeString(v) }
func writeEditorPage(out, route, original string, names map[string]string, id string) error {
	prefix := "/r/" + id + "/"
	h := original
	for _, n := range []string{"studio.css", "wasm_exec.js", "recovery.js", "gpu-preview.js", "studio.js"} {
		h = strings.ReplaceAll(h, `"`+n+`"`, `"`+prefix+names[n]+`"`)
	}
	h = strings.Replace(h, "</head>", `<link rel="stylesheet" href="`+prefix+names["surface.css"]+`"></head>`, 1)
	marker := `<script src="` + prefix + names["wasm_exec.js"] + `"></script>`
	h = strings.Replace(h, marker, `<script src="`+prefix+names["config.js"]+`"></script><script src="`+prefix+names["protocol.js"]+`"></script>`+marker, 1)
	h = strings.Replace(h, "</body>", `<script src="`+prefix+names["adapter.js"]+`"></script><script src="`+prefix+names["surface.js"]+`"></script></body>`, 1)
	return writeNew(filepath.Join(out, filepath.FromSlash(route), "index.html"), []byte(h))
}
func writeLanding(out string, names map[string]string, examples []example, templates []effectTemplate, id string, snapshot bool) error {
	var cards strings.Builder
	for _, e := range examples {
		fmt.Fprintf(&cards, `<a class="example-card" href="examples/%s/"><img src="%s" width="96" height="96" alt=""><strong>%s</strong><span>%s</span></a>`, esc(e.ID), esc(e.Thumbnail), esc(e.Title["ru"]), esc(e.Title["en"]))
	}
	var templateCards strings.Builder
	for _, item := range templates {
		fmt.Fprintf(&templateCards, `<a class="example-card" href="editor/?new=effect&amp;template=%s"><img src="%s" width="96" height="96" alt=""><strong>%s</strong><span>%s · %s</span></a>`, esc(item.ID), esc(item.Thumbnail), esc(item.Title["ru"]), esc(item.Title["en"]), esc(item.ParticlePreset))
	}
	badge := id
	if snapshot {
		badge += " · snapshot / не production"
	}
	h := `<!doctype html><html lang="ru"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Aego Asset Studio</title><link rel="stylesheet" href="` + names["studio.css"] + `"><link rel="stylesheet" href="` + names["surface.css"] + `"></head><body class="landing"><header class="appbar"><div class="brand"><span class="brand-mark">A</span><strong>AEGO</strong><span>Asset Studio</span></div><span class="build-badge">` + esc(badge) + `</span></header><main class="welcome"><p class="eyebrow">PIXEL ART · FRAME ANIMATION</p><h1>Нарисуйте. Оживите. Сохраните.</h1><p>Draw, animate and export. Без регистрации и загрузки рисунков на сервер.</p><div class="welcome-actions"><a href="editor/?new=sprite">Новый спрайт / New Sprite</a><a href="editor/?new=animation">Новая анимация / New Animation</a><a href="editor/?new=effect">Новый эффект / New Effect</a><a href="editor/?open=1">Открыть файл / Open File</a><a href="editor/?recent=1">Локальные черновики / Local Drafts</a></div><p><a href="docs/ru/first-sprite/">Руководство RU</a> · <a href="docs/en/first-sprite/">Guide EN</a></p><h2>Шаблоны эффектов / Effect templates</h2><div class="example-grid">` + templateCards.String() + `</div><h2>Примеры и уроки / Examples & tutorials</h2><div class="example-grid">` + cards.String() + `</div><p class="release-note">В GPU Preview доступны Lit, normal/emission, alpha-тени, Flash/Dissolve, Bloom и ParticleSystem2D. Они используют изолированную сцену Aego. Назначение объекту проекта выполняется в native Aego.</p></main></body></html>`
	for _, attr := range []string{"href", "src"} {
		h = strings.ReplaceAll(h, attr+`="assets/`, attr+`="/r/`+id+`/assets/`)
		for _, route := range []string{"editor/", "examples/", "templates/", "docs/"} {
			h = strings.ReplaceAll(h, attr+`="`+route, attr+`="/r/`+id+`/`+route)
		}
	}
	return writeNew(filepath.Join(out, "index.html"), []byte(h))
}
func writeGuides(out string, names map[string]string, examples []example, o buildOptions) error {
	for _, lang := range []string{"ru", "en"} {
		for _, l := range lessons {
			title, text, action := l.EN, l.ENText, l.ENAction
			labels := map[string]string{"start": "Try it", "reset": "Reset example", "full": "Open Full Editor", "download": "Download .aeart", "check": "Check action", "manual": "Requires manual verification; not an automated acceptance result.", "limits": "Import, storage and troubleshooting", "intro": "Files stay on your device. Click Try it to start the real editor. A second active example is not loaded automatically.", "trouble": "WASM load failure: check MIME application/wasm and CSP. Storage unavailable: export .aeart; browser recovery is optional. Unsupported import: read its diagnostics; Aseprite round-trip is not guaranteed. Web GPU Preview uses an isolated Aego scene for Lit, Bloom and particles. To preview the asset in your project's Scene or Game View, open the source in native Aego."}
			if lang == "ru" {
				title, text, action = l.RU, l.RUText, l.RUAction
				labels = map[string]string{"start": "Попробовать", "reset": "Сбросить пример", "full": "Открыть полный редактор", "download": "Скачать .aeart", "check": "Проверить действие", "manual": "Нужна ручная проверка; это не автоматический результат приёмки.", "limits": "Импорт, хранение и диагностика", "intro": "Файлы остаются на устройстве. Нажмите «Попробовать», чтобы запустить настоящий редактор. Второй пример не загружается автоматически.", "trouble": "Ошибка WASM: проверьте MIME application/wasm и CSP. Хранилище недоступно: экспортируйте .aeart; recovery зависит от браузера. Ошибка импорта: прочитайте диагностику, lossless Aseprite не гарантируется. GPU-предпросмотр в браузере использует изолированную сцену Aego для освещения, Bloom и частиц. Для проверки ассета в Scene View или Game View вашего проекта откройте исходник в native Aego."}
			}
			var nav strings.Builder
			fmt.Fprintf(&nav, `<a href="../guide/getting-started/">%s</a>`, map[string]string{"en": "Asset Studio manual", "ru": "Руководство Asset Studio"}[lang])
			for _, n := range lessons {
				label := n.EN
				if lang == "ru" {
					label = n.RU
				}
				fmt.Fprintf(&nav, `<a href="../%s/">%s</a>`, esc(n.ID), esc(label))
			}
			// Relative URL resolves on the actual app origin, including a pinned /r/<id>/ prefix.
			editor := o.Origin + "/r/" + o.ID + "/"
			h := `<!doctype html><html lang="` + lang + `"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>` + esc(title) + ` — Aego Asset Studio</title><link rel="stylesheet" href="../../../` + names["docs.css"] + `"></head><body><header><a href="` + esc(editor) + `">AEGO · Asset Studio</a><a href="../../` + map[string]string{"ru": "en", "en": "ru"}[lang] + `/` + l.ID + `/">` + map[string]string{"ru": "EN", "en": "RU"}[lang] + `</a></header><main><nav>` + nav.String() + `</nav><article><h1>` + esc(title) + `</h1><p>` + esc(text) + `</p><p>` + esc(labels["intro"]) + `</p><section class="aego-studio-embed" data-editor="` + esc(editor) + `" data-build="` + esc(o.ID) + `" data-example="` + esc(l.ID) + `" data-lang="` + lang + `" data-check="` + esc(l.Check) + `"><div class="embed-actions"><button data-action="start">` + esc(labels["start"]) + `</button><button data-action="check" disabled>` + esc(labels["check"]) + `</button><button data-action="reset" disabled>` + esc(labels["reset"]) + `</button><button data-action="download" disabled>` + esc(labels["download"]) + `</button></div><p class="exercise">` + esc(action) + `</p><p class="embed-status" role="status"></p><div class="embed-mount"></div><p>` + esc(labels["full"]) + ` — ` + map[string]string{"ru": "кнопка внутри редактора переносит несохранённый документ в новую вкладку.", "en": "the button inside the editor transfers your unsaved document to a new tab."}[lang] + `</p></section><h2>` + esc(labels["limits"]) + `</h2><p>` + esc(labels["trouble"]) + `</p><p class="scope-note">` + esc(labels["manual"]) + `</p></article></main><script src="../../../` + names["protocol.js"] + `"></script><script src="../../../` + names["docs-embed.js"] + `"></script></body></html>`
			if l.ID == "aseprite" {
				sample := o.Origin + "/r/" + o.ID + "/examples/aseprite/"
				links := `<p><a download href="` + esc(sample+"point4frames.aseprite") + `">point4frames.aseprite</a> · <a href="` + esc(sample+"LICENSE.txt") + `">License</a> · <a href="` + esc(sample+"PROVENANCE.md") + `">Provenance</a></p>`
				h = strings.Replace(h, `<section class="aego-studio-embed"`, links+`<section class="aego-studio-embed"`, 1)
			}
			if err := writeNew(filepath.Join(out, "docs", lang, l.ID, "index.html"), []byte(h)); err != nil {
				return err
			}
		}
	}
	if err := writeGuidebook(out, names, o); err != nil {
		return err
	}
	return writeNew(filepath.Join(out, "docs", "integration.json"), jsonBytes(map[string]any{"buildID": o.ID, "protocol": embedProtocolVersion, "editorURL": o.Origin + "/r/" + o.ID + "/", "embedScript": names["docs-embed.js"], "protocolScript": names["protocol.js"], "note": "Copy guide pages and their referenced scripts/styles into the existing docs generator; do not copy or rebuild the editor. Preserve the pinned editorURL."}))
}
