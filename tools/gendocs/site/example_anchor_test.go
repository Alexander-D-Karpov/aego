package site

import (
	"strings"
	"testing"
)

func TestModuleExampleLinksResolveWithoutEnglishHeading(t *testing.T) {
	for _, flag := range []string{"run", "check"} {
		t.Run(flag, func(t *testing.T) {
			exp := fixture()
			for i := range exp.Entries {
				e := &exp.Entries[i]
				switch e.ID {
				case "aego/math":
					e.Body = "Use this module.\n\n```aes " + flag + "\npackage demo\n```\n\n## More examples\n\n```aes " + flag + "\npackage second\n```\n"
					e.Snippets = []Snippet{{Lang: "aes", Run: flag == "run", Check: flag == "check"}, {Lang: "aes", Run: flag == "run", Check: flag == "check"}}
					e.Translations = map[string]Translation{"ru": {Body: "Используйте этот модуль.\n\n```aes " + flag + "\npackage demo\n```\n\n## Ещё примеры\n\n```aes " + flag + "\npackage second\n```\n"}}
				case "aego/math.Clamp":
					e.Example = ""
				}
			}
			_, _, pages := build(t, exp, Options{})
			for _, path := range []string{"ref/aego/math.html", "ru/ref/aego/math.html"} {
				page := pages[path]
				for _, anchor := range []string{"aego/math-example-1", "aego/math-example-2"} {
					if n := strings.Count(page, `id="`+anchor+`"`); n != 1 {
						t.Fatalf("%s: expected one stable example anchor %s, got %d", path, anchor, n)
					}
				}
				if !strings.Contains(page, `math#aego/math-example-1"`) {
					t.Fatalf("%s: function does not link to the first checked example", path)
				}
				if strings.Contains(page, `math#example"`) {
					t.Fatalf("%s: still assumes an English Example heading", path)
				}
			}
		})
	}
}
