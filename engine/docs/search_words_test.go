package docs

import "testing"

// A query that names a component and one of its signals finds the signal.
func TestSearchWordsFindsAMemberByItsParent(t *testing.T) {
	r := &Registry{byID: map[string]int{}}
	r.add(Entry{ID: "gameplay.Pickup2D", Kind: Type, Title: "Pickup 2D"})
	r.add(Entry{ID: "gameplay.Pickup2D.Collected", Parent: "gameplay.Pickup2D", Kind: Signal, Title: "Collected"})
	r.add(Entry{ID: "gameplay.Health.Died", Parent: "gameplay.Health", Kind: Signal, Title: "Died"})
	got := r.Search("Pickup2D Collected", 5)
	if len(got) == 0 || got[0].ID != "gameplay.Pickup2D.Collected" {
		t.Fatalf("Search = %v", got)
	}
	if len(r.Search("Health Collected", 5)) != 0 {
		t.Fatal("a member of another component matched")
	}
}
