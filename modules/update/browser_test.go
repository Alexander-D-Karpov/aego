package update

import (
	"slices"
	"testing"

	"aego/engine/module"
	"aego/scene"
	"aego/schema"
	"aego/script/bind"
)

func registered(t *testing.T, m *Module) (*module.Manifest, *bind.Registry) {
	t.Helper()
	b := bind.NewManifest()
	r := module.New(scene.NewTypes(schema.NewRegistry()), b)
	if err := r.Add(m); err != nil {
		t.Fatal(err)
	}
	man, err := r.Build()
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Freeze(); err != nil {
		t.Fatal(err)
	}
	return man, b
}

// The browser build registers what the native one does: the project section, the settings and the script API.
func TestBrowserRegistersLikeNative(t *testing.T) {
	nat, nb := registered(t, New())
	web, wb := registered(t, &Module{b: browser{}})
	ids := func(m *module.Manifest) (s []string) {
		for _, sec := range m.Sections {
			s = append(s, sec.ID)
		}
		for _, d := range m.Settings {
			s = append(s, d.ID)
		}
		return s
	}
	if a, b := ids(nat), ids(web); !slices.Equal(a, b) || !slices.Contains(a, SectionID) {
		t.Fatalf("sections and settings differ: native %v, browser %v", a, b)
	}
	if nb.ABIHash() != wb.ABIHash() {
		t.Fatal("the browser build's aego/update script API differs from the native one")
	}
}

func TestBrowserHasNothingToUpdate(t *testing.T) {
	m := &Module{b: browser{}, sec: Section{URL: "https://example.com/updates", PublicKey: "k"}}
	if err := m.Check(); err != errBrowser {
		t.Fatalf("Check: %v", err)
	}
	m.Download()
	if err := m.Apply(); err != errBrowser {
		t.Fatalf("Apply: %v", err)
	}
	if p, v, _, msg := m.State(); p != Idle || v != "" || msg != "" {
		t.Fatalf("state %v %q %q", p, v, msg)
	}
}
