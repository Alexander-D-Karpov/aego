package site

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// ruChrome is the site's own text in Russian, by its English text; page texts come from the export's translations (D348).
var ruChrome = map[string]string{
	"Getting started":           "Начало работы",
	"Guides":                    "Руководства",
	"Reference":                 "Справочник",
	"Other":                     "Другое",
	"Example":                   "Пример",
	"See the":                   "Посмотрите",
	"module example":            "пример модуля",
	"for this function in use.": "с этой функцией в работе.",
	"One per scene.":            "Один на сцену.",
	"Usage":                     "Использование",
	"Fields":                    "Поля",
	"Runtime state":             "Состояние во время игры",
	"Values the component has while the game runs. They are not saved with the scene.": "Значения, которые есть у компонента во время игры. Они не сохраняются в сцене.",
	"Name":        "Имя",
	"Type":        "Тип",
	"Default":     "По умолчанию",
	"Description": "Описание",
	"Sound":       "Звук",
	"Try it":      "Попробовать",
	"Runs this scene in the browser, with the Inspector next to it": "Запускает эту сцену в браузере, рядом с Инспектором",
	"Script":                                "Скрипт",
	"Script events":                         "События скриптов",
	"Related guides":                        "Связанные руководства",
	"On this page":                          "На этой странице",
	"Previous":                              "Назад",
	"Next":                                  "Далее",
	"Edit this page":                        "Править эту страницу",
	"Edit the English page":                 "Править английскую страницу",
	"Playground":                            "Песочница",
	"Functions":                             "Функции",
	"Filter":                                "Найти среди",
	"members of":                            "членов",
	"Filter members":                        "Найти член",
	"Methods":                               "Методы",
	"Signals":                               "Сигналы",
	"No example":                            "Без примера",
	"Fix:":                                  "Как исправить:",
	"Broken":                                "С ошибкой",
	"Fixed":                                 "Исправлено",
	"Download":                              "Скачать",
	"Docs":                                  "Документация",
	"Demo":                                  "Демо",
	"Source":                                "Исходники",
	"Source code":                           "Исходный код",
	"Search the docs":                       "Поиск по документации",
	"Search":                                "Поиск",
	"Navigation":                            "Навигация",
	"Documentation":                         "Документация",
	"Site":                                  "Сайт",
	"Theme":                                 "Тема",
	"Copy":                                  "Копировать",
	"Copied":                                "Скопировано",
	"Run":                                   "Запустить",
	"Components":                            "Компоненты",
	"Assets":                                "Ассеты",
	"built":                                 "собрано",
	"Source on":                             "Исходники на",
	"These docs as Markdown, for AI agents": "Эта документация в Markdown, для ИИ-агентов",
	"During Play":                           "Во время игры",
	"untranslated":                          "Эта страница ещё не переведена, ниже её английский текст.",
	"partly":                                "Часть этой страницы ещё не переведена и показана по-английски.",
	"Lexer":                                 "Лексер",
	"Parser":                                "Парсер",
	"Types":                                 "Типы",
	"Coroutines, signals and events":        "Корутины, сигналы и события",
	"Attributes":                            "Атрибуты",
	"Packages and imports":                  "Пакеты и импорт",
	"Engine bindings":                       "Привязки движка",
	"Runtime faults":                        "Ошибки во время выполнения",
	"Compiled code":                         "Скомпилированный код",
	"Needs %s on the same node.":            "Нужен %s на том же узле.",
	"Add it with %s.":                       "Добавляется через %s.",
	"Scripts on a node with this component receive these events from the %s module.": "Скрипты на узле с этим компонентом получают эти события от модуля «%s».",
	"player setting":          "настройка игрока",
	"project default":         "значение проекта",
	"applies after a restart": "после перезапуска",
	"asks to keep the change": "просит подтвердить",
	"not saved":               "не сохраняется",
	"accessibility option":    "доступность",
	"control binding":         "назначение управления",
	"preset":                  "пресет",
	"read-only":               "только чтение",
	"read-only in scripts":    "в скриптах только чтение",
	"scripts only":            "только в скриптах",
	"not in scripts":          "нет в скриптах",
	"hidden in the Inspector": "скрыто в Инспекторе",
	"saved with the game":     "сохраняется с игрой",
	"live tuning":             "настройка на лету",
	"mask over %s":            "маска по %s",
	"gizmo: %s":               "гизмо: %s",
	"step %s":                 "шаг %s",
	"%d constants and enum values are listed below.": "Ниже перечислены константы и значения перечислений: %d.",
	"one of: %s":  "одно из: %s",
	"one of %s":   "одно из %s",
	"category %s": "категория %s",
	homeLead:      "Игровой движок на Go, прежде всего для 2D, со встроенным редактором, своим языком скриптов и нативными расширениями на Go.",
	llmsNote:      "Каждая страница ниже — Markdown; /ru/llms-full.txt содержит их все в одном файле.",
}

// tr is the site's own text in the builder's language.
func (b *builder) tr(en string) string {
	if b.lang == "ru" {
		if t, ok := ruChrome[en]; ok {
			return t
		}
	}
	return en
}

// forLang is a builder for lang: the same pages and URLs with every entry and group in that language where a translation exists.
func (b *builder) forLang(lang string) *builder {
	rb := *b
	rb.lang = lang
	rb.en = b
	rb.byID = make(map[string]*Entry, len(b.byID))
	rb.has = map[string]bool{}
	for id, e := range b.byID {
		cp := *e
		if t, ok := e.Translations[lang]; ok {
			rb.has[id] = true
			if t.Title != "" {
				if cp.Field != nil {
					f := *cp.Field
					f.Label = t.Title
					cp.Field = &f
				} else {
					cp.Title = t.Title
				}
			}
			if t.Summary != "" {
				cp.Summary = t.Summary
			}
			if t.Body != "" {
				cp.Body = t.Body
			}
			if (t.Fix != "" || t.Why != "") && cp.Diag != nil {
				d := *cp.Diag
				if t.Fix != "" {
					d.Fix = t.Fix
				}
				if t.Why != "" {
					d.Why = t.Why
				}
				cp.Diag = &d
			}
			if t.Source != "" {
				cp.Location.Source, cp.Location.Line = t.Source, t.Line
			} else {
				rb.has[id] = cp.Kind != "page" || t.Body != ""
			}
		}
		rb.byID[id] = &cp
	}
	rb.groups = make([]navGroup, len(b.groups))
	for i, g := range b.groups {
		rb.groups[i] = g
		if t := g.Titles[lang]; t != "" {
			rb.groups[i].Title = t
		} else {
			rb.groups[i].Title = rb.tr(g.Title)
		}
	}
	return &rb
}

var (
	tagWithHref = regexp.MustCompile(`<[a-z]+\b[^>]*\bhref="[^"]*"[^>]*>`)
	pageLink    = regexp.MustCompile(`href="((?:\.\./)*)([^"#?:]*)([#?][^"]*)?"`)
)

// langLinks points the links of a page in the builder's language at the pages in that language; links marked hreflang="en" stay English.
func (b *builder) langLinks(page string) string {
	pages := make(map[string]bool, len(b.urls)+1)
	for _, u := range b.urls {
		pages[u] = true
	}
	pages["index.html"] = true
	prefix := b.lang + "/"
	return tagWithHref.ReplaceAllStringFunc(page, func(tag string) string {
		if strings.Contains(tag, `hreflang="en"`) {
			return tag
		}
		return pageLink.ReplaceAllStringFunc(tag, func(m string) string {
			sub := pageLink.FindStringSubmatch(m)
			target := sub[2]
			if target == "" || strings.HasPrefix(target, prefix) || !pages[target] && !pages[target+".html"] {
				return m
			}
			return `href="` + sub[1] + prefix + target + sub[3] + `"`
		})
	})
}

// writeLang writes the pages of the builder's language under <lang>/, with their search index, Markdown copies and llms.txt.
func (b *builder) writeLang() error {
	var search []searchItem
	for _, id := range b.order {
		e := b.byID[id]
		rel := b.lang + "/" + b.urls[id]
		content := b.content(e, rootOf(rel))
		if b.has[id] && b.en != nil {
			content = sameIDs(content, b.en.content(b.en.byID[id], rootOf(b.urls[id])))
		}
		if !b.has[id] && e.Kind == "page" {
			content = `<p class="untranslated">` + esc(b.tr("untranslated")) + "</p>\n" + content
		}
		if err := b.emit(rel, e.Title, id, content, e); err != nil {
			return err
		}
	}
	for i := range b.exp.Entries {
		id := b.exp.Entries[i].ID
		u, ok := b.urls[id]
		if !ok {
			continue
		}
		e := b.byID[id]
		item := searchItem{Title: e.Title, URL: cleanURL(b.lang + "/" + u), Summary: plainMD(e.Summary), Kind: e.Kind, ID: e.ID, Icon: b.entryIcon(e)}
		if gi, ok := b.groupOf[id]; ok {
			item.Group, item.Section = b.groups[gi].Title, b.tr(sectionTitle(b.groups[gi].Section))
		}
		search = append(search, item)
	}
	data, err := json.Marshal(search)
	if err != nil {
		return err
	}
	dir := filepath.Join(b.out, b.lang)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "search.json"), data, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "llms.txt"), []byte(b.llms()), 0o644); err != nil {
		return err
	}
	return b.writeMarkdown()
}

// langPath is a site path in the builder's language.
func (b *builder) langPath(rel string) string {
	if b.lang == "" || b.lang == "en" {
		return rel
	}
	return path.Join(b.lang, rel)
}

// pageLang is the html lang of the builder's pages.
func (b *builder) pageLang() string {
	if b.lang == "" {
		return "en"
	}
	return b.lang
}

var headingID = regexp.MustCompile(`(<h[1-6][^>]* id=")([^"]*)(")`)

// sameIDs gives the headings of a translated page the ids of the English page, so links to a section work in both languages.
func sameIDs(page, en string) string {
	ids := headingID.FindAllStringSubmatch(en, -1)
	if len(headingID.FindAllStringIndex(page, -1)) != len(ids) {
		return page
	}
	i := 0
	return headingID.ReplaceAllStringFunc(page, func(m string) string {
		sub := headingID.FindStringSubmatch(m)
		out := sub[1] + ids[i][2] + sub[3]
		i++
		return out
	})
}

// note is a field note in the builder's language.
func (b *builder) note(n string) string {
	for _, p := range []string{"one of: ", "one of ", "category ", "mask over ", "gizmo: ", "step "} {
		if rest, ok := strings.CutPrefix(n, p); ok {
			return strings.Replace(b.tr(p+"%s"), "%s", rest, 1)
		}
	}
	return b.tr(n)
}

// moduleName is a module's title in the builder's language, from its component page.
func (b *builder) moduleName(title string) string {
	for _, e := range b.byID {
		if e.Kind == "module" && strings.HasPrefix(e.ID, "module/") && b.en != nil && b.en.byID[e.ID].Title == title {
			return e.Title
		}
	}
	return title
}
