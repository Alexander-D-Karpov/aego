package pipeline

import (
	"errors"
	"os"
	"testing"

	"aego/asset"
	"aego/format/aetext"
)

type savedDependencyImporter struct {
	calls         map[string]int
	failDependent bool
}

func (*savedDependencyImporter) ID() string           { return "saved-dependency" }
func (*savedDependencyImporter) Version() int         { return 1 }
func (*savedDependencyImporter) Extensions() []string { return []string{".txt"} }
func (*savedDependencyImporter) Defaults() Settings   { return Settings{"mode": aetext.Ident("plain")} }
func (imp *savedDependencyImporter) Import(ctx *ImportContext) (*Result, error) {
	imp.calls[ctx.Path]++
	if string(ctx.Source) == "bad" || (ctx.Path == "assets/b.txt" && imp.failDependent) {
		return nil, errors.New("deliberate importer failure")
	}
	result := &Result{Kind: asset.ArtScene, Artifacts: []Produced{{Kind: asset.ArtScene, Data: ctx.Source}}}
	if ctx.Path == "assets/b.txt" {
		id, ok := ctx.Lookup("assets/a.txt")
		if !ok {
			return nil, errors.New("dependency missing")
		}
		source, err := ctx.Read("assets/a.txt")
		if err != nil {
			return nil, err
		}
		result.Deps = append(result.Deps, id)
		result.Artifacts[0].Data = append([]byte("dependency:"), source...)
	}
	return result, nil
}

func TestReimportWithDependentsReportsFailuresAndRetries(t *testing.T) {
	p := newProject(t)
	pathA := write(t, p, "assets/a.txt", "initial")
	write(t, p, "assets/b.txt", "consumer")
	imp := &savedDependencyImporter{calls: map[string]int{}}
	pl := openPipeline(t, p, imp)
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	a, okA := pl.Lookup("assets/a.txt")
	b, okB := pl.Lookup("assets/b.txt")
	if !okA || !okB {
		t.Fatal("source or dependent not indexed")
	}
	initial, err := pl.ImportAll()
	if err != nil || initial == nil || len(initial.Failed) != 0 || len(initial.Retry) != 0 {
		t.Fatalf("initial import: %+v, %v", initial, err)
	}
	pixels := func() string {
		t.Helper()
		entry, ok := pl.Entry(b)
		if !ok {
			t.Fatal("dependent disappeared")
		}
		a, ok := entry.Artifact(asset.ArtScene)
		if !ok {
			t.Fatal("dependent artifact disappeared")
		}
		data, err := pl.Artifacts().ReadFile(a.Path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if got := pixels(); got != "dependency:initial" {
		t.Fatal("initial dependent bytes:", got)
	}
	if err := os.WriteFile(pathA, []byte("updated"), 0644); err != nil {
		t.Fatal(err)
	}
	batch, err := pl.ReimportWithDependents(a)
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 || len(batch.Imported) != 2 || pixels() != "dependency:updated" {
		t.Fatalf("dependent import: %+v, %v", batch, err)
	}
	t.Logf("success: imported=%d Failed=%v Retry=%v", len(batch.Imported), batch.Failed, batch.Retry)
	for _, tc := range []struct {
		name  string
		data  []byte
		retry bool
	}{{"source importer failure", []byte("bad"), false}, {"unstable empty source", nil, true}} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(pathA, tc.data, 0644); err != nil {
				t.Fatal(err)
			}
			calls := imp.calls["assets/b.txt"]
			batch, err := pl.ReimportWithDependents(a)
			if err != nil || batch == nil {
				t.Fatalf("source batch: %+v, %v", batch, err)
			}
			if tc.retry {
				if len(batch.Retry) != 1 || batch.Retry[0] != a || len(batch.Failed) != 0 {
					t.Fatalf("retry was lost: %+v", batch)
				}
			} else if len(batch.Failed) != 1 || batch.Failed[a] == nil || len(batch.Retry) != 0 {
				t.Fatalf("source failure was lost: %+v", batch)
			}
			if imp.calls["assets/b.txt"] != calls || pixels() != "dependency:updated" {
				t.Fatal("failed or unstable source published a dependent")
			}
			t.Logf("Failed=%v Retry=%v", batch.Failed, batch.Retry)
		})
	}
	if err := os.WriteFile(pathA, []byte("later"), 0644); err != nil {
		t.Fatal(err)
	}
	imp.failDependent = true
	batch, err = pl.ReimportWithDependents(a)
	if err != nil || batch == nil || len(batch.Failed) != 1 || batch.Failed[b] == nil || len(batch.Retry) != 0 || len(batch.Imported) != 1 {
		t.Fatalf("dependent failure was lost: %+v, %v", batch, err)
	}
	if pixels() != "dependency:updated" || pl.VerifyCurrent(b) == nil {
		t.Fatal("failed dependent was published or reported current")
	}
	t.Logf("dependent failure: imported=%d Failed=%v Retry=%v", len(batch.Imported), batch.Failed, batch.Retry)
}
