package assembly

import (
	"slices"
	"strings"
	"testing"

	"aego/engine/module"
	"aego/script/bind"
)

type fake struct {
	id  string
	req []string
}

func (f fake) Info() module.Info {
	return module.Info{ID: f.id, Requires: f.req}
}

func (fake) Register(*module.Registry) error {
	return nil
}

func entry(id string, req ...string) Entry {
	return Entry{ID: id, Requires: req, New: func(Options) module.Module { return fake{id: id, req: req} }}
}

func load(t *testing.T, c Catalog) (*module.Manifest, error) {
	t.Helper()
	types, err := NewTypes()
	if err != nil {
		t.Fatal(err)
	}
	return c.Load(types, bind.New(), Options{})
}

func order(m *module.Manifest) []string {
	out := make([]string, len(m.Modules))
	for i, info := range m.Modules {
		out[i] = info.ID
	}
	return out
}

func TestDuplicateID(t *testing.T) {
	_, err := load(t, Catalog{entry("a"), entry("a")})
	if err == nil || !strings.Contains(err.Error(), "listed twice") {
		t.Fatalf("want a duplicate error, got %v", err)
	}
}

func TestMissingRequirementNamed(t *testing.T) {
	_, err := load(t, Catalog{entry("a", "ghost")})
	if err == nil || !strings.Contains(err.Error(), "requires ghost") {
		t.Fatalf("want the missing module named, got %v", err)
	}
}

func TestCycleChain(t *testing.T) {
	_, err := load(t, Catalog{entry("a", "b"), entry("b", "c"), entry("c", "a")})
	if err == nil || !strings.Contains(err.Error(), "a -> b -> c -> a") {
		t.Fatalf("want the cycle chain, got %v", err)
	}
}

func permutations(c Catalog) []Catalog {
	if len(c) <= 1 {
		return []Catalog{slices.Clone(c)}
	}
	var out []Catalog
	for i := range c {
		rest := append(slices.Clone(c[:i]), c[i+1:]...)
		for _, p := range permutations(rest) {
			out = append(out, append(Catalog{c[i]}, p...))
		}
	}
	return out
}

func TestOrderRespectsRequiresUnderPermutation(t *testing.T) {
	base := Catalog{entry("x", "y"), entry("y"), entry("z"), entry("w", "x")}
	for _, c := range permutations(base) {
		m, err := load(t, c)
		if err != nil {
			t.Fatal(err)
		}
		got := order(m)
		if !(slices.Index(got, "y") < slices.Index(got, "x") && slices.Index(got, "x") < slices.Index(got, "w")) {
			t.Fatalf("catalog %v sorted to %v", c.IDs(), got)
		}
		again, err := load(t, c)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, order(again)) {
			t.Fatalf("catalog %v is not sorted deterministically", c.IDs())
		}
	}
}
