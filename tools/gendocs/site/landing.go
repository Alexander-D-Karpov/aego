package site

import (
	"strings"
)

// landingText is the home page in one language; the author writes en, ru follows docs/glossary.md.
type landingText struct {
	lang, tagline, lead       string
	entries                   [3][2]string
	insideTitle               string
	cards                     [6][2]string
	demo, demoPlay, unrelated string
	switchLabel               string
}

var landingCardIcons = [6]string{"panel.inspector", "file.script", "cat.gameplay", "play.step_back", "panel.agent", "file.package"}

var landingEntryIcons = [3]string{"file.download", "help.book", "panel.find"}

var landingEN = landingText{
	lang:    "en",
	tagline: homeLead,
	lead:    "Build scenes in the editor, script them in a small Go-like language, extend the engine with Go when you need to, and ship the same project to Linux, Windows, macOS and the browser. Deterministic simulation, rewind, physics, animation, audio and cameras come built in.",
	entries: [3][2]string{
		{"Install", "Download for your OS, make a project, play it in two minutes."},
		{"Learn", "Guides from the first scene to a finished platformer."},
		{"Reference", "Every component, every script function, every file format, with a live example."},
	},
	insideTitle: "What's inside",
	cards: [6][2]string{
		{"Editor", "Scenes, prefabs, Play mode with rewind, multi-object editing, workspaces, light and dark themes, English and Russian."},
		{"Aego Script", "Statically typed, hot-reloaded, with coroutines and signals. Every example in these docs compiles and runs."},
		{"Gameplay kit", "Physics, a character controller, cameras, audio, animation, health and hitboxes. A platformer needs about fifty lines of script."},
		{"Determinism", "Fixed-step simulation, replay from recorded input, rewind to any frame. If it happened once, it happens again."},
		{"Agents", "`aego mcp` gives an AI agent the same scene operations the editor uses: read, change, validate, run headless, undo."},
		{"Export", "Linux, Windows, macOS and the browser. The demo on this page is the platformer example, exported as-is."},
	},
	demo:        "This is `examples/platformer`, exported to the web. Arrows or WASD, Space to jump.",
	demoPlay:    "Play the demo · runs in the browser",
	unrelated:   "Aego is unrelated to eGo2D and Ebitengine. It is a standalone engine with its own editor, scripting language and file formats.",
	switchLabel: "Русский",
}

var landingRU = landingText{
	lang:    "ru",
	tagline: "Делайте 2D-игры на Go. Один редактор, один проект, компьютер и веб.",
	lead:    "Собирайте сцены в редакторе, пишите для них скрипты на небольшом языке в духе Go, расширяйте движок на Go, когда это нужно, и выпускайте один и тот же проект для Linux, Windows, macOS и браузера. Детерминированная симуляция, перемотка, физика, анимация, звук и камеры уже встроены.",
	entries: [3][2]string{
		{"Установить", "Скачайте для своей системы, создайте проект и запустите его за две минуты."},
		{"Учиться", "Руководства от первой сцены до готового платформера."},
		{"Справочник", "Каждый компонент, каждая функция скриптов, каждый формат файлов — с живым примером."},
	},
	insideTitle: "Что внутри",
	cards: [6][2]string{
		{"Редактор", "Сцены, префабы, режим игры с перемоткой, правка нескольких объектов, раскладки, светлая и тёмная темы, английский и русский."},
		{"Aego Script", "Статическая типизация, горячая перезагрузка, корутины и сигналы. Каждый пример в этой документации компилируется и запускается."},
		{"Набор для геймплея", "Физика, контроллер персонажа, камеры, звук, анимация, здоровье и хитбоксы. Платформеру хватает примерно пятидесяти строк скрипта."},
		{"Детерминизм", "Симуляция с фиксированным шагом, повтор по записанному вводу, перемотка к любому кадру. Если что-то случилось один раз, это случится снова."},
		{"Агенты", "`aego mcp` даёт ИИ-агенту те же операции над сценой, что и редактору: прочитать, изменить, проверить, запустить без окна, отменить."},
		{"Экспорт", "Linux, Windows, macOS и браузер. Демо на этой странице — пример платформера, экспортированный как есть."},
	},
	demo:        "Это `examples/platformer`, экспортированный для веба. Стрелки или WASD, пробел — прыжок.",
	demoPlay:    "Играть в демо · работает в браузере",
	unrelated:   "Aego не связан с eGo2D и Ebitengine. Это самостоятельный движок со своим редактором, языком скриптов и форматами файлов.",
	switchLabel: "English",
}

// home is the landing page: hero, three ways in, what is inside, the live demo (D347).
func (b *builder) home(t landingText) string {
	root := ""
	if t.lang != "en" {
		root = "../"
	}
	var c, e strings.Builder
	e.WriteString(`<ul class="entries">`)
	start := b.urls[installPage]
	if start == "" {
		start = b.firstOf(SectionStart)
	}
	targets := [3]string{start, b.firstOf(SectionGuides), b.firstOf(SectionReference)}
	for i, en := range t.entries {
		if targets[i] == "" {
			continue
		}
		e.WriteString(`<li><a href="` + esc(root+targets[i]) + `">` + b.icon(root, landingEntryIcons[i], "") + `<strong>` + esc(en[0]) + `</strong><span>` + inlineMD(en[1]) + "</span></a></li>")
	}
	e.WriteString("</ul>")
	c.WriteString(`<section class="hero"><div class="hero-text"><h1>` + esc(b.exp.Title) + "</h1>")
	c.WriteString(`<p class="tagline">` + inlineMD(t.tagline) + "</p>")
	c.WriteString(`<p class="lead">` + inlineMD(t.lead) + "</p>")
	c.WriteString(b.downloadBlock(root))
	other := "ru/"
	if t.lang != "en" {
		other = "../"
	}
	c.WriteString(`<p class="lang-switch"><a href="` + other + `" hreflang="` + map[bool]string{true: "ru", false: "en"}[t.lang == "en"] + `">` + esc(t.switchLabel) + "</a></p>")
	c.WriteString("</div>")
	switch {
	case b.opt.Demo:
		style := ""
		if b.poster {
			style = ` style="background-image:url(` + root + posterAsset + `)"`
		}
		c.WriteString(`<div class="hero-demo"><div class="demo" data-src="` + root + `demo/"` + style + `><button type="button" class="demo-play" aria-label="` + esc(t.demoPlay) + `"><span class="disc">` + b.icon(root, "play.start", "") + `</span><small>` + esc(t.demoPlay) + `</small></button></div>`)
		c.WriteString(`<p class="demo-caption">` + inlineMD(t.demo) + "</p></div>")
	case b.hero:
		c.WriteString(`<img class="hero-shot" src="` + root + heroAsset + `" alt="Aego">`)
	}
	c.WriteString("</section>\n")
	c.WriteString(e.String())
	c.WriteString(`<section class="inside"><h2 id="inside">` + esc(t.insideTitle) + `</h2><ul class="cards">`)
	for i, card := range t.cards {
		c.WriteString(`<li>` + b.icon(root, landingCardIcons[i], "card-ic") + `<h3>` + esc(card[0]) + `</h3><p>` + inlineMD(card[1]) + "</p></li>")
	}
	c.WriteString(`</ul><p class="unrelated">` + inlineMD(t.unrelated) + "</p></section>\n")
	return c.String()
}
