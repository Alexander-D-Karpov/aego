//go:build !aego_runtime

package docs

import "testing"

func TestLocalizeValueTypeKeepsAuthoredProseWithoutDuplication(t *testing.T) {
	for _, tc := range []struct {
		name, catalog, file, want string
	}{
		{"same body", "Тип материала.", "Тип материала.", "Тип материала."},
		{"same body with whitespace", "  Тип материала.\n", "Тип материала.", "Тип материала."},
		{"complementary bodies", "Тип материала.", "Дополнительное описание.", "Тип материала.\n\nДополнительное описание."},
		{"file only", "", "Описание из файла.", "Описание из файла."},
		{"catalog only", "Описание из каталога.", "", "Описание из каталога."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &Registry{entries: []Entry{{
				ID: "engine.MaterialParameter", Kind: Type, Body: "Material parameter.",
				Translations: map[string]Translation{"ru": {Body: tc.file}},
			}}}
			r.Localize("ru", func(key, en string) string {
				if key == APIKey("engine.MaterialParameter") {
					return tc.catalog
				}
				return en
			})
			if got := r.entries[0].Translations["ru"].Body; got != tc.want {
				t.Fatalf("localized body = %q, want %q", got, tc.want)
			}
		})
	}
}
