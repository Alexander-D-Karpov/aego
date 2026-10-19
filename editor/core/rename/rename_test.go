package rename

import (
	"testing"
)

func TestPatternNumbersAndCase(t *testing.T) {
	items := []Item{{Name: "enemy run", Ext: ".png", Parent: "art"}, {Name: "enemy jump", Ext: ".png", Parent: "art"}}
	rows, err := Plan(Rule{Pattern: "{parent}_{name}_{n:03}", Find: "enemy ", Replace: "", Case: CaseTitle, Start: 1}, items, nil, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].To != "Art_Run_001" || rows[1].To != "Art_Jump_002" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestRegexAndConflicts(t *testing.T) {
	items := []Item{{Name: "a1", Ext: ".png", Parent: "x"}, {Name: "a2", Ext: ".png", Parent: "x"}, {Name: "b", Ext: ".png", Parent: "x"}}
	taken := func(parent, name string) bool { return name == "c.png" }
	rows, err := Plan(Rule{Find: `^a\d$`, Replace: "c", Regex: true}, items, nil, taken, true)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0].Conflict == "" || rows[1].Conflict == "" {
		t.Fatal("two items renamed to the same name must conflict")
	}
	if rows[2].Conflict != "" || rows[2].Changed() {
		t.Fatal("an untouched item has no conflict")
	}
	if !Blocked(rows) {
		t.Fatal("conflicts block the rename")
	}
}

func TestChainInsideBatchIsAllowed(t *testing.T) {
	items := []Item{{Name: "a", Ext: ".png", Parent: "x"}, {Name: "b", Ext: ".png", Parent: "x"}}
	taken := func(parent, name string) bool { return name == "a.png" || name == "b.png" }
	rows, err := Plan(Rule{Pattern: "{name}{name}"}, []Item{items[0]}, nil, taken, true)
	if err != nil || rows[0].Conflict != "" {
		t.Fatalf("aa is free: %+v %v", rows, err)
	}
	rows, _ = Plan(Rule{Find: "a", Replace: "b"}, items[:1], nil, taken, true)
	if rows[0].Conflict != "already exists" {
		t.Fatal("renaming onto a file outside the batch is a conflict")
	}
	swap, _ := Plan(Rule{Find: "x", Replace: "x"}, items, nil, taken, true)
	if Blocked(swap) {
		t.Fatal("unchanged names never conflict")
	}
}

func TestBadRegex(t *testing.T) {
	if _, err := Plan(Rule{Find: "(", Regex: true}, []Item{{Name: "a"}}, nil, nil, false); err == nil {
		t.Fatal("an invalid regex is an error")
	}
}
