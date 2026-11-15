//go:build linux

package pipeline

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"aego/anim"
	"aego/art"
	"aego/core/ids"
	"aego/core/log"
	"aego/format/aetext"
	"aego/project"
)

// This importer validates real source encodings but deliberately does not
// replace the production Art/animation importers. It isolates the filesystem
// journal from renderer and texture preparation in these crash tests.
type artJournalImporter struct{ fakeImporter }

func (*artJournalImporter) Extensions() []string { return []string{".aeart", ".aeanim", ".txt"} }

func (*artJournalImporter) ResolveMetadata(ctx *ImportContext) (Settings, error) {
	if !strings.HasSuffix(ctx.Path, ".aeart") {
		return nil, nil
	}
	d, err := art.Decode(ctx.Source, art.DefaultLimits())
	if err != nil {
		return nil, err
	}
	defer d.Close()
	settings := ctx.Meta.Settings.Clone()
	settings["art_revision"] = aetext.Int(int64(d.Revision()))
	return settings, nil
}

func (imp *artJournalImporter) Import(ctx *ImportContext) (*Result, error) {
	if strings.HasSuffix(ctx.Path, ".aeart") {
		d, err := art.Decode(ctx.Source, art.DefaultLimits())
		if err != nil {
			return nil, err
		}
		d.Close()
	}
	if strings.HasSuffix(ctx.Path, ".aeanim") {
		n, err := aetext.Parse(ctx.Source, ctx.Path)
		if err != nil {
			return nil, err
		}
		c, err := anim.Decode(n)
		if err != nil {
			return nil, err
		}
		if issues := c.Check(nil); len(issues) != 0 {
			return nil, fmt.Errorf("invalid animation fixture: %+v", issues)
		}
	}
	return imp.fakeImporter.Import(ctx)
}

func artJournalPipeline(t *testing.T, p *project.Project) *Pipeline {
	t.Helper()
	reg := NewImporterRegistry()
	if err := reg.Register(&artJournalImporter{fakeImporter{version: 1}}); err != nil {
		t.Fatal(err)
	}
	pl, err := Open(p, reg, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := pl.Scan(); err != nil {
		pl.Close()
		t.Fatal(err)
	}
	batch, err := pl.ImportAll()
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		pl.Close()
		t.Fatalf("fixture import: %+v, %v", batch, err)
	}
	return pl
}

func artJournalImages(t *testing.T) ([]byte, []byte) {
	t.Helper()
	d, err := art.New(8, 8, art.RGBA, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	info := d.Info()
	e, err := d.Begin(d.Revision(), "linked frames")
	if err != nil {
		t.Fatal(err)
	}
	second, err := e.DuplicateFrame(info.Frames[0].ID, true)
	if err != nil {
		t.Fatal(err)
	}
	change, err := e.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	if err := change.Apply(); err != nil {
		t.Fatal(err)
	}
	before, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	e, err = d.Begin(d.Revision(), "paint and duration")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.SetPixel(info.Cels[0].ID, 1, 1, art.Paint{Color: art.Color{R: 255, A: 255}}, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := e.SetDuration(second, 275); err != nil {
		t.Fatal(err)
	}
	change, err = e.Prepare()
	if err != nil {
		t.Fatal(err)
	}
	if err := change.Apply(); err != nil {
		t.Fatal(err)
	}
	after, err := d.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return before, after
}

const (
	artJournalPlan   = ".aego/test-art-journal-plan.json"
	artJournalBefore = "anim version=1 length=1 loop=loop {\n  event t=0.2 name=\"Footstep\" {}\n}\n"
	artJournalAfter  = "anim version=1 length=2 loop=loop {\n  event t=0.4 name=\"Footstep\" {}\n}\n"
)

type artJournalFixture struct {
	p      *project.Project
	before map[string][]byte
	after  map[string][]byte
	ids    map[string]ids.AssetID
	edits  []SourceEdit
}

func newArtJournalFixture(t *testing.T) artJournalFixture {
	t.Helper()
	p := newProject(t)
	beforeArt, afterArt := artJournalImages(t)
	write(t, p, "assets/hero.aeart", string(beforeArt))
	write(t, p, "assets/hero.aeanim", artJournalBefore)
	for i := range 37 {
		write(t, p, fmt.Sprintf("assets/source-%02d.txt", i), fmt.Sprintf("before-%02d", i))
	}
	pl := artJournalPipeline(t, p)
	f := artJournalFixture{p: p, before: map[string][]byte{}, after: map[string][]byte{}, ids: map[string]ids.AssetID{}}
	paths := []string{"assets/hero.aeart", "assets/hero.aeanim"}
	for i := range 37 {
		paths = append(paths, fmt.Sprintf("assets/source-%02d.txt", i))
	}
	for _, rel := range paths {
		id, ok := pl.Lookup(rel)
		if !ok {
			pl.Close()
			t.Fatalf("fixture is not indexed: %s", rel)
		}
		f.ids[rel] = id
		for _, name := range []string{rel, rel + MetaSuffix} {
			data, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(name)))
			if err != nil {
				pl.Close()
				t.Fatal(err)
			}
			f.before[name] = data
		}
	}
	pl.Close()
	f.after[paths[0]], f.after[paths[1]] = afterArt, []byte(artJournalAfter)
	for i := range 37 {
		f.after[paths[i+2]] = []byte(fmt.Sprintf("after-%02d", i))
	}
	for _, rel := range paths[:2] {
		name := rel + MetaSuffix
		meta, err := ParseMeta(f.before[name], name)
		if err != nil {
			t.Fatal(err)
		}
		meta.Settings["journal_probe"] = aetext.Ident("after")
		f.after[name] = WriteMeta(meta)
	}
	// 41 distinct publication points: two real sources, their sidecars, and
	// 37 independently journaled files. Other existing sidecars are guards.
	order := []string{paths[0], paths[1], paths[0] + MetaSuffix, paths[1] + MetaSuffix}
	order = append(order, paths[2:]...)
	for _, rel := range order {
		f.edits = append(f.edits, SourceEdit{Path: rel, Before: f.before[rel], After: f.after[rel]})
	}
	plan, err := json.Marshal(f.edits)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Root, filepath.FromSlash(artJournalPlan)), plan, 0600); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestArtJournalCrashWorker(t *testing.T) {
	dir := os.Getenv("AEGO_TEST_ART_JOURNAL_DIR")
	if dir == "" {
		return
	}
	p, err := project.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	boundary := os.Getenv("AEGO_TEST_ART_JOURNAL_BOUNDARY")
	checkpoint := func(stage, rel string) {
		if stage+":"+rel == boundary {
			fmt.Fprintln(os.Stdout, "ART_JOURNAL_BOUNDARY")
			_, _ = io.Copy(io.Discard, os.Stdin)
		}
	}
	if os.Getenv("AEGO_TEST_ART_JOURNAL_MODE") == "restore" {
		if err := restoreSourceRecovery(p, checkpoint); err != nil {
			t.Fatal(err)
		}
		t.Fatal("requested recovery boundary was not reached")
	}
	pl := artJournalPipeline(t, p)
	defer pl.Close()
	raw, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(artJournalPlan)))
	if err != nil {
		t.Fatal(err)
	}
	var edits []SourceEdit
	if err := json.Unmarshal(raw, &edits); err != nil {
		t.Fatal(err)
	}
	pl.sourceTransactionCheckpoint = checkpoint
	if _, err := pl.ApplySources(edits, false); err != nil {
		t.Fatal(err)
	}
	t.Fatal("requested boundary was not reached")
}

func killArtJournalWorker(t *testing.T, p *project.Project, boundary string) {
	killArtJournalWorkerMode(t, p, "write", boundary)
}

func killArtJournalWorkerMode(t *testing.T, p *project.Project, mode, boundary string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestArtJournalCrashWorker$", "-test.count=1")
	cmd.Env = append(os.Environ(), "AEGO_TEST_ART_JOURNAL_DIR="+p.Root, "AEGO_TEST_ART_JOURNAL_MODE="+mode, "AEGO_TEST_ART_JOURNAL_BOUNDARY="+boundary)
	cmd.Stderr = os.Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		_ = stdin.Close()
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	lines := make(chan bool, 1)
	go func() {
		s := bufio.NewScanner(stdout)
		for s.Scan() {
			if s.Text() == "ART_JOURNAL_BOUNDARY" {
				lines <- true
				return
			}
		}
		lines <- false
	}()
	select {
	case ready := <-lines:
		if !ready {
			t.Fatalf("worker did not reach %s", boundary)
		}
	case <-ctx.Done():
		t.Fatalf("worker timed out before %s", boundary)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	err = cmd.Wait()
	waited = true
	exit, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("worker did not die from SIGKILL: %v", err)
	}
	status, ok := exit.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
		t.Fatalf("worker did not die from SIGKILL: %v", err)
	}
	t.Logf("actual SIGKILL at %s", boundary)
}

func verifyArtJournalGeneration(t *testing.T, f artJournalFixture, committed bool) {
	t.Helper()
	for rel, want := range f.before {
		if committed {
			if after, ok := f.after[rel]; ok {
				want = after
			}
			// The source resolver canonically updates this sidecar's revision;
			// validate the resulting structured metadata below instead.
			if rel == "assets/hero.aeart"+MetaSuffix {
				continue
			}
		}
		got, err := os.ReadFile(filepath.Join(f.p.Root, filepath.FromSlash(rel)))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("recovery left a mixed generation at %s: %v", rel, err)
		}
	}
	artBytes, err := os.ReadFile(filepath.Join(f.p.Root, "assets", "hero.aeart"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := art.Decode(artBytes, art.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	info := d.Info()
	wantDuration, wantAlpha := 100, uint8(0)
	wantLength, wantEventTime := float32(1), float32(0.2)
	if committed {
		wantDuration, wantAlpha = 275, 255
		wantLength, wantEventTime = 2, 0.4
	}
	if len(info.Frames) != 2 || len(info.Cels) != 2 || info.Cels[0].Image != info.Cels[1].Image || info.Frames[1].DurationMS != wantDuration {
		t.Fatalf("restored Art frames or links differ: %+v", info)
	}
	pixel, err := d.Pixel(info.Cels[0].ID, 1, 1)
	if err != nil || pixel.A != wantAlpha {
		t.Fatalf("restored Art pixel differs: %+v, %v", pixel, err)
	}
	clipBytes, err := os.ReadFile(filepath.Join(f.p.Root, "assets", "hero.aeanim"))
	if err != nil {
		t.Fatal(err)
	}
	node, err := aetext.Parse(clipBytes, "hero.aeanim")
	if err != nil {
		t.Fatal(err)
	}
	clip, err := anim.Decode(node)
	if err != nil || clip.Length != wantLength || len(clip.Events) != 1 || clip.Events[0].Name != "Footstep" || clip.Events[0].T != wantEventTime {
		t.Fatalf("restored animation metadata differs: %+v, %v", clip, err)
	}
	metaBytes, err := os.ReadFile(filepath.Join(f.p.Root, "assets", "hero.aeart"+MetaSuffix))
	if err != nil {
		t.Fatal(err)
	}
	meta, err := ParseMeta(metaBytes, "assets/hero.aeart"+MetaSuffix)
	if err != nil || meta.ID != f.ids["assets/hero.aeart"] || meta.Settings.Int("art_revision", -1) != int(d.Revision()) {
		t.Fatalf("Art metadata does not match its source: %+v, %v", meta, err)
	}
	if committed && meta.Settings.String("journal_probe", "") != "after" {
		t.Fatal("committed authored Art metadata was lost")
	}
	pl := artJournalPipeline(t, f.p)
	defer pl.Close()
	for rel, want := range f.ids {
		got, ok := pl.Lookup(rel)
		if !ok || got != want {
			t.Fatalf("AssetID changed for %s: %s -> %s", rel, want, got)
		}
		if err := pl.VerifyCurrent(got); err != nil {
			t.Fatalf("recovered asset is stale %s: %v", rel, err)
		}
	}
}

// Each subtest kills a separate process after a different actual file
// publication. These 41 points cover one phase, not every journal phase or a
// power loss, and are deliberately not counted as complete crash acceptance.
func TestArtMultiFileJournalSIGKILLPublicationPoints(t *testing.T) {
	for i := range 41 {
		t.Run(fmt.Sprintf("file-%02d", i+1), func(t *testing.T) {
			f := newArtJournalFixture(t)
			target := f.edits[i].Path
			killArtJournalWorker(t, f.p, "published:"+target)
			interrupted := make(map[string][]byte, len(f.before))
			for rel := range f.before {
				data, err := os.ReadFile(filepath.Join(f.p.Root, filepath.FromSlash(rel)))
				if err != nil {
					t.Fatal(err)
				}
				interrupted[rel] = data
			}
			for j, edit := range f.edits {
				want := edit.Before
				if j <= i {
					want = edit.After
				}
				if !bytes.Equal(interrupted[edit.Path], want) {
					t.Fatalf("worker did not stop at the expected publication: %s", edit.Path)
				}
			}
			if status, err := SourceRecoveryStatus(f.p); err != nil || status == nil || status.State != "prepared" {
				t.Fatalf("interrupted journal not visible: %+v, %v", status, err)
			}
			if pl, err := Open(f.p, NewImporterRegistry(), log.Nop()); err == nil {
				pl.Close()
				t.Fatal("Open silently accepted pending recovery")
			}
			for rel, want := range interrupted {
				got, err := os.ReadFile(filepath.Join(f.p.Root, filepath.FromSlash(rel)))
				if err != nil || !bytes.Equal(got, want) {
					t.Fatalf("Open changed interrupted source or metadata %s: %v", rel, err)
				}
			}
			if err := RestoreSourceRecovery(f.p); err != nil {
				t.Fatalf("explicit recovery: %v", err)
			}
			if status, err := SourceRecoveryStatus(f.p); err != nil || status != nil {
				t.Fatalf("recovery still pending: %+v, %v", status, err)
			}
			verifyArtJournalGeneration(t, f, false)
		})
	}
}

func TestArtMultiFileJournalSIGKILLAfterCommit(t *testing.T) {
	f := newArtJournalFixture(t)
	killArtJournalWorker(t, f.p, "committed:")
	if status, err := SourceRecoveryStatus(f.p); err != nil || status == nil || status.State != "committed" {
		t.Fatalf("committed journal not visible: %+v, %v", status, err)
	}
	beforeCleanup := make(map[string][]byte, len(f.before))
	for rel := range f.before {
		data, err := os.ReadFile(filepath.Join(f.p.Root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		beforeCleanup[rel] = data
	}
	if pl, err := Open(f.p, NewImporterRegistry(), log.Nop()); err == nil {
		pl.Close()
		t.Fatal("Open silently cleaned the committed journal")
	}
	if err := RestoreSourceRecovery(f.p); err != nil {
		t.Fatalf("explicit committed-journal cleanup: %v", err)
	}
	for rel, want := range beforeCleanup {
		data, err := os.ReadFile(filepath.Join(f.p.Root, filepath.FromSlash(rel)))
		if err != nil || !bytes.Equal(data, want) {
			t.Fatalf("committed generation changed during cleanup for %s: %v", rel, err)
		}
	}
	if status, err := SourceRecoveryStatus(f.p); err != nil || status != nil {
		t.Fatalf("committed cleanup remains pending: %+v, %v", status, err)
	}
	verifyArtJournalGeneration(t, f, true)
}

func TestArtMultiFileExplicitRecoveryCanBeInterruptedAgain(t *testing.T) {
	for _, tc := range []struct {
		boundary, state string
	}{
		{"recovery-validated:", "prepared"},
		{"recovery-restored:assets/source-36.txt", "prepared"},
		{"recovery-restored:assets/hero.aeanim", "prepared"},
		{"restored:", "restored"},
	} {
		t.Run(tc.boundary, func(t *testing.T) {
			f := newArtJournalFixture(t)
			killArtJournalWorker(t, f.p, "published:assets/source-36.txt")
			killArtJournalWorkerMode(t, f.p, "restore", tc.boundary)
			status, err := SourceRecoveryStatus(f.p)
			if err != nil || status == nil || status.State != tc.state {
				t.Fatalf("second interruption was not retained: %+v, %v", status, err)
			}
			if pl, err := Open(f.p, NewImporterRegistry(), log.Nop()); err == nil {
				pl.Close()
				t.Fatal("Open silently resumed interrupted recovery")
			}
			if err := RestoreSourceRecovery(f.p); err != nil {
				t.Fatalf("explicit recovery after second SIGKILL: %v", err)
			}
			if status, err := SourceRecoveryStatus(f.p); err != nil || status != nil {
				t.Fatalf("recovery remains pending: %+v, %v", status, err)
			}
			verifyArtJournalGeneration(t, f, false)
		})
	}
}
