package langsvc

import (
	"strings"
	"testing"

	"aego/script/abi"
	"aego/script/bind"
)

// Hover and go to definition follow the D93 rule: the project Health, not the engine one.
func TestShortNameHoverAndDefinition(t *testing.T) {
	s, w, _ := setup(t, func(r *bind.Registry) {
		r.Component(&abi.Component{Name: "gameplay.Health", Builtin: true, Fields: []abi.Field{{Name: "Max", Type: abi.T(abi.Int)}}})
		r.Component(&abi.Component{Name: "game.Health", Fields: []abi.Field{{Name: "Hits", Type: abi.T(abi.Int)}}})
	})
	src := strings.Replace(scriptTmpl, "CURSOR", "\t\th := node.Get<Health>()\n\t\th.Hits = 1", 1)
	id, _ := overlay(t, s, w, src+"|")
	if h, ok := s.Hover(id, uint32(strings.Index(src, "Get<")+1)); !ok || h.Ref != "game.Health" {
		t.Fatalf("hover = %+v", h)
	}
	def, ok := s.Definition(id, uint32(strings.Index(src, "Health>")+1))
	if !ok || def.Symbol != "component:game.Health" {
		t.Fatalf("definition = %+v", def)
	}
}
