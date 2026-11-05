package agentui

import (
	"strings"
	"testing"
)

func TestEstimateReadsJSONDenserThanProse(t *testing.T) {
	prose := strings.Repeat("The player runs faster when the ground is flat. ", 20)
	json := strings.Repeat(`{"id":"#c0000000000000000000000000000003","name":"Player"},`, 16)
	if p, j := Estimate(prose), Estimate(json); float64(len(prose))/float64(p) <= float64(len(json))/float64(j) {
		t.Fatalf("prose %d chars -> %d tokens, JSON %d chars -> %d tokens", len(prose), p, len(json), j)
	}
}

func TestCategoryOfTools(t *testing.T) {
	for tool, want := range map[string]Category{
		"scene_tree": CatScene, "scene_get": CatScene, "script_read": CatCode, "docs_search": CatDocs, "docs_read": CatDocs,
		"project_get": CatProject, "schema_show": CatProject, "asset_list": CatProject, "scene_run": CatResults, "scene_validate": CatResults,
		"scene_apply": CatResults, "ask_user": CatMessages,
	} {
		if got := CategoryOf(tool); got != want {
			t.Errorf("%s goes to %d, want %d", tool, got, want)
		}
	}
}

func session() *Meter {
	m := NewMeter(strings.Repeat("guide ", 300), strings.Repeat(`{"name":"scene_tree"}`, 50))
	m.User("add a ledge")
	m.Usage(20_000)
	m.Call("scene_tree", "1", `{"file":"scenes/main.aescene"}`, "Read scene main.aescene")
	m.Result("", "1", strings.Repeat(`{"name":"Node","id":"#c0000000000000000000000000000003"},`, 60), "48 nodes", false)
	m.Call("docs_search", "2", `{"query":"platform"}`, "Search docs")
	m.Result("", "2", strings.Repeat("Moving platforms carry the body. ", 30), "10 results", false)
	m.Text("I will add the ledge now.")
	return m
}

// The usage is spread over the categories: what the editor measured, its items scaled to the CLI's count, nothing lost.
func TestLayoutAddsUpToUsage(t *testing.T) {
	m := session()
	m.Usage(26_000)
	m.SetWindow(200_000)
	var l Layout
	m.Layout(&l)
	sum := 0
	for _, r := range l.Rows {
		sum += r.Tokens
	}
	if d := sum - 26_000; d < -20 || d > 20 {
		t.Fatalf("the categories add up to %d, the CLI counted 26000", sum)
	}
	if l.Rows[CatScene].Tokens == 0 || l.Rows[CatDocs].Tokens == 0 || l.Rows[CatSystem].Tokens == 0 {
		t.Fatalf("a category is empty: %+v", l.Rows)
	}
	if l.Free != 200_000-26_000-CompactBuffer || !l.Measured {
		t.Fatalf("free %d, measured %v", l.Free, l.Measured)
	}
	if l.NearCompaction() {
		t.Fatal("13 % full is near compaction")
	}
}

func TestMeasuredLayoutAfterCompactionDoesNotInventTokens(t *testing.T) {
	m := session()
	m.SetWindow(400_000)
	m.Usage(250_000)
	var l Layout
	m.Layout(&l)
	if l.Window != 400_000 || l.Used != 250_000 || l.Fraction() != 0.625 {
		t.Fatalf("provider window or usage was not preserved: %+v", l)
	}
	m.Compacted()
	m.Usage(100)
	m.Layout(&l)
	total := 0
	for _, r := range l.Rows {
		total += r.Tokens
	}
	if total != 100 || l.Used != 100 {
		t.Fatalf("categories add to %d for measured usage %d", total, l.Used)
	}
	m.SetWindow(16_000)
	m.Layout(&l)
	if l.Buffer > l.Window/4 || l.NearCompaction() {
		t.Fatalf("small model starts near compaction: %+v", l)
	}
}

// A dropped result is left out of the rebuilt history, a pinned one goes in full, the rest as summaries.
func TestProjectionDropsAndPins(t *testing.T) {
	m := session()
	tree, docs := m.Items()[1], m.Items()[2]
	m.Drop(tree.ID, true)
	if n, tok := m.Dropped(); n != 1 || tok != tree.Est {
		t.Fatalf("dropped %d (%d tokens)", n, tok)
	}
	p := m.Projection()
	if strings.Contains(p, "48 nodes") || strings.Contains(p, "#c000") {
		t.Fatal("the dropped tree is in the projection")
	}
	if !strings.Contains(p, "User: add a ledge") || !strings.Contains(p, "Result of Search docs: 10 results") || strings.Contains(p, "Moving platforms carry") {
		t.Fatalf("the projection misses the messages or the summary:\n%s", p)
	}
	m.Pin(docs.ID, true)
	if p = m.Projection(); !strings.Contains(p, "Moving platforms carry") {
		t.Fatal("the pinned result is not in full")
	}
	m.Rebuilt(p)
	if n, _ := m.Dropped(); n != 0 || len(m.Items()) != 2 {
		t.Fatalf("after the rebuild the meter has %d items, %d dropped", len(m.Items()), n)
	}
}

func TestContextMeterReleasesDiscardedPayloads(t *testing.T) {
	m := NewMeter("", "")
	m.User("first")
	m.Call("docs_read", "1", "{}", "Read documentation")
	m.Result("", "1", "keep this result", "pinned", false)
	for range 5 {
		m.User("discard this message")
		m.Text("discard this answer")
	}
	m.Pin(m.Items()[1].ID, true)
	m.Rebuilt("new summary")
	if len(m.items) != 2 || m.items[0].Text != "keep this result" || m.items[1].Text != "new summary" {
		t.Fatalf("rebuilding lost active context: %+v", m.items)
	}
	for _, unused := range m.items[len(m.items):cap(m.items)] {
		if unused != (Item{}) {
			t.Fatal("shortened context retains a discarded item")
		}
	}
	m.Reset()
	if cap(m.items) != 0 {
		t.Fatal("new session still references its predecessor's item array")
	}
}
