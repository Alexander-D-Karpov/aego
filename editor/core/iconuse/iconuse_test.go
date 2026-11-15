package iconuse

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestEditorIconsHaveNoDuplicates(t *testing.T) {
	listed := map[string]bool{}
	for _, u := range Editor {
		if listed[u.ID] {
			t.Errorf("icon %s is listed twice", u.ID)
		}
		listed[u.ID] = true
	}
}

func TestReservationsNotExpired(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "ARCHITECTURE.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range Reserved {
		done := regexp.MustCompile(`(?m)^### [0-9.]+ Stage ` + regexp.QuoteMeta(r.Until) + `([^0-9]|$)`)
		if done.Match(data) {
			t.Errorf("icon %s was reserved until Stage %s (%s); that stage is in ARCHITECTURE.md, use the icon or drop the reservation", r.ID, r.Until, r.Reason)
		}
	}
}
