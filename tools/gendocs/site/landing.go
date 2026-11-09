package site

import (
	"strings"
)

// landingText is the home page in one language; the author writes en, ru follows docs/glossary.md.
type landingText struct {
	lang, tagline, lead       string
	entries                   [3][2]string
	insideTitle               string
	cards                     [landingCardCount][2]string
	demo, demoPlay, unrelated string
	switchLabel               string
}

const landingCardCount = 14

var landingCardIcons = [landingCardCount]string{
	"panel.inspector", "file.script", "cat.gameplay", "play.step_back", "panel.agent", "file.export",
	"file.art", "render2d.point_light", "cat.camera", "animation.player", "particles.system", "cat.audio", "panel.assets", "help.book",
}

var landingEntryIcons = [3]string{"file.download", "help.book", "panel.find"}

var landingEN = landingText{
	lang:    "en",
	tagline: homeLead,
	lead:    "Build games visually, write gameplay logic in Aego Script, and extend the engine with native Go.\n\nA complete development environment with a visual editor, hot reload, deterministic simulation, frame-by-frame rewind, and cross-platform export. From your first scene to a playable game on Windows, Linux, macOS, and the web.",
	entries: [3][2]string{
		{"Install", "Download for your OS, make a project, play it in two minutes."},
		{"Learn", "Guides from the first scene to a finished platformer."},
		{"Reference", "Every component, every script function, every file format, with a live example."},
	},
	insideTitle: "What's inside",
	cards: [landingCardCount][2]string{
		{"Editor", "Build scenes from nodes, components and prefabs. Edit several objects together, arrange dockable panels into workspaces, and switch between light and dark themes in English or Russian."},
		{"Aego Script", "Statically typed scripts with hot reload during Play, coroutines and signals. Write gameplay in the editor's code workspace and extend the engine with native Go modules when needed."},
		{"Gameplay kit", "2D bodies and colliders, character motion, input actions, health, hitboxes, checkpoints and respawns. Combine reusable components for common interactions instead of rebuilding each system from scratch."},
		{"Determinism", "Fixed-step gameplay, recorded input and frame-by-frame rewind. Pause, step and revisit a recorded moment to inspect movement, animation and events instead of restarting every experiment."},
		{"Agents", "`aego mcp` gives an AI agent the same scene operations the editor uses: read, change, validate, run headless, undo."},
		{"Export", "Build saved projects for Linux, Windows, macOS and the browser. Run a web export through a local server, then deploy its complete output folder; native cross-builds use the target toolchain."},
		{"Art Studio", "Pixel-art source documents with layers, palettes, frames, onion skin and surface channels. Desktop authoring and browser-based Art Studio have separate workflows, with source saving distinct from preparing game assets."},
		{"2D lighting and materials", "Point, spot and directional lights, shadows, normal maps and emission. Combine sprites with built-in or custom shader materials, and change per-instance parameters without editing the shared material."},
		{"Cameras and post-processing", "Camera follow, zones, shake and named virtual-camera outputs for separate views. Add Bloom, colour grading, vignette, pixelation, CRT or a custom post-process material to a camera output."},
		{"Animation", "Sprite-frame and property clips, timeline editing, animation events and Animator state machines. Use curves and tweens for transitions, and animate transforms, colours, script fields or material parameters."},
		{"Particles and VFX", "Seeded CPU particles with bursts, continuous emission, local or world space and sprite flipbooks. Shape size, velocity and alpha over life with editable curves; start effects from gameplay or animation events."},
		{"Audio", "Clip waveforms and previews, streamed music, spatial sources and listeners, mixer buses and audio zones. Audio events add clip variants, pitch and volume variation, cooldowns and voice limits."},
		{"Assets and data", "A dependency-aware import pipeline keeps editable sources separate from runtime artifacts. Stable AssetIDs, reference searches, missing-reference checks and reusable typed data assets help keep a growing project organized."},
		{"Interactive documentation", "Run and reset component examples, edit script snippets in the playground, and search English or Russian guides and API reference. Browse source file formats separately from the artifacts generated for the runtime."},
	},
	demo:        "This is `examples/platformer`, exported to the web. Arrows or WASD, Space to jump, J to attack.",
	demoPlay:    "Play the demo · runs in the browser",
	unrelated:   "Aego is unrelated to eGo2D and Ebitengine. It is a standalone engine with its own editor, scripting language and file formats.",
	switchLabel: "Русский",
}

var landingRU = landingText{
	lang:    "ru",
	tagline: "Игровой 2D-движок на Go.",
	lead:    "Собирайте игры визуально, пишите игровую логику на Aego Script и расширяйте движок нативным кодом на Go.\n\nПолноценная среда разработки: визуальный редактор, горячая перезагрузка, детерминированная симуляция, покадровая перемотка и экспорт на разные платформы. От первой сцены до готовой игры для Windows, Linux, macOS и веба.",
	entries: [3][2]string{
		{"Установить", "Скачайте для своей системы, создайте проект и запустите его за две минуты."},
		{"Учиться", "Руководства от первой сцены до готового платформера."},
		{"Справочник", "Каждый компонент, каждая функция скриптов, каждый формат файлов — с живым примером."},
	},
	insideTitle: "Что внутри",
	cards: [landingCardCount][2]string{
		{"Редактор", "Собирайте сцены из узлов, компонентов и префабов. Правьте несколько объектов вместе, размещайте стыкуемые панели в рабочих раскладках и выбирайте светлую или тёмную тему, английский или русский язык."},
		{"Aego Script", "Статическая типизация, горячая перезагрузка во время игры, корутины и сигналы. Пишите игровую логику в рабочем пространстве для кода, а при необходимости расширяйте движок нативными модулями на Go."},
		{"Набор для геймплея", "2D-тела и коллайдеры, движение персонажа, действия ввода, здоровье, хитбоксы, контрольные точки и возрождение. Собирайте типичные взаимодействия из готовых компонентов, не создавая каждую систему заново."},
		{"Детерминизм", "Игровая симуляция с фиксированным шагом, запись ввода и покадровая перемотка. Ставьте игру на паузу, выполняйте отдельные шаги и возвращайтесь к записанному моменту, чтобы изучить движение, анимацию и события."},
		{"Агенты", "`aego mcp` даёт ИИ-агенту те же операции над сценой, что и редактору: прочитать, изменить, проверить, запустить без окна, отменить."},
		{"Экспорт", "Собирайте сохранённые проекты для Linux, Windows, macOS и браузера. Проверяйте веб-экспорт через локальный сервер и переносите всю выходную папку; нативная кросс-сборка использует инструменты целевой платформы."},
		{"Art Studio", "Исходные документы пиксельной графики: слои, палитры, кадры, луковая калька и каналы поверхности. Настольный редактор и браузерная Art Studio работают раздельно; сохранение исходника отличается от подготовки игровых ресурсов."},
		{"2D-свет и материалы", "Точечный, направленный свет и прожекторы, тени, карты нормалей и свечение поверхностей. Используйте встроенные или собственные шейдерные материалы и меняйте параметры отдельного объекта, не правя общий материал."},
		{"Камеры и постобработка", "Слежение, зоны, тряска и именованные выходы виртуальных камер для отдельных видов. Добавляйте к выходу камеры Bloom, цветокоррекцию, виньетку, пикселизацию, CRT или собственный материал постобработки."},
		{"Анимация", "Клипы кадров и свойств, редактирование на временной шкале, события анимации и машины состояний Animator. Настраивайте переходы кривыми и твинами, анимируйте трансформации, цвета, поля скриптов и параметры материалов."},
		{"Частицы и VFX", "CPU-частицы с собственным seed: всплески, непрерывное испускание, локальные и мировые координаты, покадровые спрайты. Редактируйте кривые размера, скорости и прозрачности; запускайте эффекты из игровой логики или событий анимации."},
		{"Звук", "Форма волны и предпросмотр клипов, потоковая музыка, пространственные источники и слушатели, шины микшера и звуковые зоны. Аудиособытия задают варианты клипов, разброс высоты тона и громкости, интервалы и ограничения голосов."},
		{"Ассеты и данные", "Импорт с учётом зависимостей отделяет редактируемые исходники от файлов для выполнения игры. Стабильные AssetID, поиск использований, проверка потерянных ссылок и типизированные ресурсы данных помогают поддерживать порядок в растущем проекте."},
		{"Интерактивная документация", "Запускайте и сбрасывайте примеры компонентов, редактируйте скрипты в песочнице, ищите руководства и API на русском или английском. Изучайте исходные форматы отдельно от файлов, которые генерируются для выполнения игры."},
	},
	demo:        "Это `examples/platformer`, экспортированный для веба. Стрелки или WASD, пробел — прыжок, J — атака.",
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
	for _, para := range strings.Split(t.lead, "\n\n") {
		c.WriteString(`<p class="lead">` + inlineMD(para) + "</p>")
	}
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
