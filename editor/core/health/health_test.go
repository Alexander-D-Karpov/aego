package health

import "testing"

func TestExcluded(t *testing.T) {
	list := []string{"assets/raw", "assets/keep.png"}
	for rel, want := range map[string]bool{
		"assets/raw/a.png":  true,
		"assets/rawx/a.png": false,
		"assets/keep.png":   true,
		"assets/other.png":  false,
	} {
		if excluded(rel, list) != want {
			t.Fatalf("excluded(%q) != %v", rel, want)
		}
	}
	if PathOf("assets/big.png  8192×8192") != "assets/big.png" {
		t.Fatal("PathOf must strip the size suffix")
	}
}
