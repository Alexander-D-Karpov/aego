package palette

import "testing"

func TestKeywordsFindTechnicalAndFormerNamesWithoutInvalidHighlights(t *testing.T) {
	p := New(0)
	p.SetItems([]Item{
		{Key: "camera.CameraBrain2D", Label: "Выбор выхода камеры 2D", Keywords: []string{"camera.CameraBrain2D", "Camera Output Selector 2D", "Camera Brain 2D", "Мозг камеры 2D"}},
		{Key: "other", Label: "Other", Keywords: []string{"Camera helper"}},
	})
	for _, query := range []string{"camera.CameraBrain2D", "CameraBrain2D", "Camera Output Selector 2D", "Camera Brain 2D", "Мозг камеры 2D", "Выбор выхода камеры 2D"} {
		p.SetQuery(query)
		if got := p.Selected(); got != 0 {
			t.Fatalf("%q selected %d instead of the stable component ID", query, got)
		}
		for _, h := range p.hits {
			for _, offset := range p.pos[h.pos[0]:h.pos[1]] {
				if offset < 0 || offset >= len(p.items[h.item].Label) {
					t.Fatalf("alias match supplied a non-label highlight offset %d", offset)
				}
			}
		}
	}
	// Alias ties must not index an empty highlight slice while sorting.
	p.SetQuery("Camera")
	if len(p.hits) != 2 {
		t.Fatalf("alias results = %d, want 2", len(p.hits))
	}
}
