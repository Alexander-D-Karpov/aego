package pipeline

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"aego/core/log"
	"aego/format/aetext"
	"aego/project"
)

func recoveryTestPipeline(t *testing.T, p *project.Project) *Pipeline {
	t.Helper()
	reg := NewImporterRegistry()
	if err := reg.Register(&canonicalRollbackImporter{fakeImporter: fakeImporter{version: 1}}); err != nil {
		t.Fatal(err)
	}
	pl, err := Open(p, reg, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := pl.Scan(); err != nil {
		t.Fatal(err)
	}
	b, err := pl.ImportAll()
	if err != nil || b == nil || len(b.Failed) != 0 || len(b.Retry) != 0 {
		t.Fatalf("initial import: %+v, %v", b, err)
	}
	return pl
}

// This child executes the actual ApplySources. Its parent kills it at a
// synchronous boundary, not after an estimated delay or a simulated error.
func TestSourceJournalCrashWorker(t *testing.T) {
	dir := os.Getenv("AEGO_TEST_SOURCE_JOURNAL_PROJECT")
	if dir == "" {
		return
	}
	p, err := project.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	pl := recoveryTestPipeline(t, p)
	defer pl.Close()
	name := "assets/a.txt"
	create := os.Getenv("AEGO_TEST_SOURCE_JOURNAL_CREATE") == "1"
	mode := os.Getenv("AEGO_TEST_SOURCE_JOURNAL_MODE")
	if create || mode == "undo-create" {
		name = "assets/new.txt"
	}
	var edits []SourceEdit
	back := false
	if create {
		edits = []SourceEdit{{Path: name, After: []byte("candidate")}}
	} else {
		meta, err := os.ReadFile(filepath.Join(p.Root, filepath.FromSlash(name+MetaSuffix)))
		if err != nil {
			t.Fatal(err)
		}
		manual, err := ParseMeta(meta, name+MetaSuffix)
		if err != nil {
			t.Fatal(err)
		}
		manual.Settings["mode"] = aetext.Ident("manual")
		switch mode {
		case "undo":
			back = true
			edits = []SourceEdit{
				{Path: name, Before: []byte("old"), After: []byte("candidate")},
				{Path: name + MetaSuffix, Before: WriteMeta(manual), After: meta},
			}
		case "undo-create":
			back = true
			edits = []SourceEdit{
				{Path: name, After: []byte("candidate")},
				{Path: name + MetaSuffix, After: meta},
			}
		default:
			edits = []SourceEdit{
				{Path: name, Before: []byte("old"), After: []byte("candidate")},
				{Path: name + MetaSuffix, Before: meta, After: WriteMeta(manual)},
			}
		}
	}
	want := os.Getenv("AEGO_TEST_SOURCE_JOURNAL_BOUNDARY")
	pl.sourceTransactionCheckpoint = func(stage, rel string) {
		if stage+":"+rel != want {
			return
		}
		fmt.Fprintln(os.Stdout, "SOURCE_JOURNAL_BOUNDARY")
		// The parent keeps stdin open until it sends SIGKILL (or the platform's
		// equivalent). No source mutation occurs while this call is blocked.
		_, _ = io.Copy(io.Discard, os.Stdin)
	}
	if _, err := pl.ApplySources(edits, back); err != nil {
		t.Fatal(err)
	}
	t.Fatal("the requested interruption boundary was not reached")
}

func killSourceJournalWorker(t *testing.T, p *project.Project, boundary string, create bool) {
	killSourceJournalWorkerMode(t, p, boundary, create, "")
}

func killSourceJournalWorkerMode(t *testing.T, p *project.Project, boundary string, create bool, mode string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSourceJournalCrashWorker$", "-test.count=1")
	cmd.Env = append(os.Environ(), "AEGO_TEST_SOURCE_JOURNAL_PROJECT="+p.Root, "AEGO_TEST_SOURCE_JOURNAL_BOUNDARY="+boundary)
	if create {
		cmd.Env = append(cmd.Env, "AEGO_TEST_SOURCE_JOURNAL_CREATE=1")
	}
	if mode != "" {
		cmd.Env = append(cmd.Env, "AEGO_TEST_SOURCE_JOURNAL_MODE="+mode)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	lines := make(chan string, 1)
	go func() {
		s := bufio.NewScanner(stdout)
		for s.Scan() {
			if s.Text() == "SOURCE_JOURNAL_BOUNDARY" {
				lines <- s.Text()
				return
			}
		}
		lines <- ""
	}()
	select {
	case line := <-lines:
		if line == "" {
			_ = stdin.Close()
			_ = cmd.Wait()
			t.Fatalf("worker did not reach %s: %s", boundary, stderr.String())
		}
	case <-ctx.Done():
		_ = stdin.Close()
		_ = cmd.Wait()
		t.Fatalf("worker timed out before %s: %s", boundary, stderr.String())
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = stdin.Close()
	waitErr := cmd.Wait()
	if waitErr == nil {
		t.Fatal("worker exited normally instead of being killed")
	}
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		exit, ok := waitErr.(*exec.ExitError)
		if !ok {
			t.Fatalf("worker did not report an exit status: %v", waitErr)
		}
		status, ok := exit.Sys().(syscall.WaitStatus)
		if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
			t.Fatalf("worker was not killed by SIGKILL: %v", waitErr)
		}
	}
	if strings.Contains(stderr.String(), "WARNING: DATA RACE") {
		t.Fatalf("race detector reported an error before interruption: %s", stderr.String())
	}
	t.Logf("worker killed at %s", boundary)
}

func TestSourceJournalSIGKILLDuringUndo(t *testing.T) {
	for _, mode := range []string{"undo", "undo-create"} {
		t.Run(mode, func(t *testing.T) {
			p := newProject(t)
			name := "a.txt"
			if mode == "undo-create" {
				name = "new.txt"
			}
			file := write(t, p, "assets/"+name, "candidate")
			pl := recoveryTestPipeline(t, p)
			pl.Close()
			meta, err := os.ReadFile(file + MetaSuffix)
			if err != nil {
				t.Fatal(err)
			}
			killSourceJournalWorkerMode(t, p, "published:assets/"+name, false, mode)
			if mode == "undo" {
				if got, err := os.ReadFile(file); err != nil || string(got) != "old" {
					t.Fatalf("undo did not reach its first publication: %q, %v", got, err)
				}
			} else if _, err := os.Stat(file); !os.IsNotExist(err) {
				t.Fatalf("undo did not reach source deletion: %v", err)
			}
			if err := RestoreSourceRecovery(p); err != nil {
				t.Fatalf("explicit recovery of interrupted undo: %v", err)
			}
			if got, err := os.ReadFile(file); err != nil || string(got) != "candidate" {
				t.Fatalf("previous source was not recovered: %q, %v", got, err)
			}
			if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, meta) {
				t.Fatalf("previous sidecar was not recovered: %q, %v", got, err)
			}
		})
	}
}

func TestSourceJournalSIGKILLAndExplicitRecovery(t *testing.T) {
	cases := []struct {
		name, boundary string
		create         bool
		committed      bool
	}{
		{"before-first-file", "prepared:", false, false},
		{"after-first-file", "published:assets/a.txt", false, false},
		{"after-sidecar", "published:assets/a.txt.meta", false, false},
		{"after-metadata-resolution", "metadata:assets/a.txt.meta", false, false},
		{"after-commit-before-cleanup", "committed:", false, true},
		{"created-source", "published:assets/new.txt", true, false},
		{"created-sidecar", "published:assets/new.txt.meta", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newProject(t)
			var originalMeta []byte
			if !tc.create {
				write(t, p, "assets/a.txt", "old")
				pl := recoveryTestPipeline(t, p)
				pl.Close()
				var err error
				originalMeta, err = os.ReadFile(filepath.Join(p.AssetsDir(), "a.txt.meta"))
				if err != nil {
					t.Fatal(err)
				}
			}
			killSourceJournalWorker(t, p, tc.boundary, tc.create)
			status, err := SourceRecoveryStatus(p)
			if err != nil || status == nil || status.ID == "" {
				t.Fatalf("missing interrupted transaction: %+v, %v", status, err)
			}
			wantState := "prepared"
			if tc.committed {
				wantState = "committed"
			}
			if status.State != wantState {
				t.Fatalf("recovery state = %s, want %s", status.State, wantState)
			}
			reg := NewImporterRegistry()
			if pl, err := Open(p, reg, log.Nop()); err == nil {
				pl.Close()
				t.Fatal("opening the project silently accepted pending recovery")
			}
			if err := RestoreSourceRecovery(p); err != nil {
				t.Fatalf("explicit recovery: %v", err)
			}
			if status, err := SourceRecoveryStatus(p); err != nil || status != nil {
				t.Fatalf("resolved journal still blocks project: %+v, %v", status, err)
			}
			if tc.create {
				for _, name := range []string{"new.txt", "new.txt.meta"} {
					if _, err := os.Lstat(filepath.Join(p.AssetsDir(), name)); !os.IsNotExist(err) {
						t.Fatalf("created source or sidecar survived rollback: %s: %v", name, err)
					}
				}
			} else {
				source, err := os.ReadFile(filepath.Join(p.AssetsDir(), "a.txt"))
				if err != nil {
					t.Fatal(err)
				}
				meta, err := os.ReadFile(filepath.Join(p.AssetsDir(), "a.txt.meta"))
				if err != nil {
					t.Fatal(err)
				}
				if tc.committed {
					m, err := ParseMeta(meta, "assets/a.txt.meta")
					if err != nil || string(source) != "candidate" || m.Settings.String("mode", "") != "resolved" {
						t.Fatalf("completed transaction was rolled back: source=%q metadata=%q err=%v", source, meta, err)
					}
				} else if string(source) != "old" || !bytes.Equal(meta, originalMeta) {
					t.Fatalf("recovery did not restore exact source and metadata: source=%q metadata=%q", source, meta)
				}
			}
			pl := recoveryTestPipeline(t, p)
			pl.Close()
		})
	}
}

func TestSourceJournalRecoveryRejectsExternalChanges(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "old")
	pl := recoveryTestPipeline(t, p)
	pl.Close()
	killSourceJournalWorker(t, p, "metadata:assets/a.txt.meta", false)
	meta, err := os.ReadFile(file + MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("external"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RestoreSourceRecovery(p); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("external source change was accepted: %v", err)
	}
	if got, _ := os.ReadFile(file); string(got) != "external" {
		t.Fatalf("external source was overwritten: %q", got)
	}
	if got, _ := os.ReadFile(file + MetaSuffix); !bytes.Equal(got, meta) {
		t.Fatal("preflight conflict modified the sidecar")
	}
	// Resolving the conflict is an explicit test-fixture action, not recovery.
	if err := os.WriteFile(file, []byte("candidate"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RestoreSourceRecovery(p); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(file); string(got) != "old" {
		t.Fatalf("the original source was not recovered: %q", got)
	}
}

func TestSourceJournalRecoveryPreservesIncompleteLastEvent(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "old")
	root, err := os.OpenRoot(p.Root)
	if err != nil {
		t.Fatal(err)
	}
	j, err := startSourceJournal(root, []SourceEdit{{Path: "assets/a.txt", Before: []byte("old"), After: []byte("new")}}, false)
	if err != nil || j == nil {
		t.Fatalf("prepare journal: %v", err)
	}
	if err := replaceSource(root, "assets/a.txt", []byte("new")); err != nil {
		t.Fatal(err)
	}
	f, err := root.OpenFile(sourceJournalFile, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte(`{"kind":"allow"`)); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	root.Close()
	journal := filepath.Join(p.Root, filepath.FromSlash(sourceJournalFile))
	before, err := os.ReadFile(journal)
	if err != nil {
		t.Fatal(err)
	}
	if err := RestoreSourceRecovery(p); err == nil {
		t.Error("recovery accepted and truncated an unverifiable journal tail")
	}
	if got, _ := os.ReadFile(file); string(got) != "new" {
		t.Errorf("an incomplete journal changed the source: %q", got)
	}
	if got, err := os.ReadFile(journal); err != nil || !bytes.Equal(got, before) {
		t.Errorf("an incomplete journal was changed or discarded: %q, %v", got, err)
	}
}

func TestSourceJournalRecoveryRefusesChangedBackup(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "old")
	pl := recoveryTestPipeline(t, p)
	pl.Close()
	killSourceJournalWorker(t, p, "published:assets/a.txt", false)
	backup := filepath.Join(p.Root, filepath.FromSlash(sourceJournalDir), "before-0000")
	if err := os.WriteFile(backup, []byte("somebody else's backup"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RestoreSourceRecovery(p); err == nil || !strings.Contains(err.Error(), "backup") {
		t.Fatalf("changed backup was accepted: %v", err)
	}
	if got, _ := os.ReadFile(file); string(got) != "candidate" {
		t.Fatalf("failed recovery changed source: %q", got)
	}
	if got, _ := os.ReadFile(backup); string(got) != "somebody else's backup" {
		t.Fatalf("changed recovery data was overwritten: %q", got)
	}
}

func TestSourceJournalRecoveryPreservesUnexpectedFiles(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "old")
	pl := recoveryTestPipeline(t, p)
	pl.Close()
	killSourceJournalWorker(t, p, "published:assets/a.txt", false)
	other := filepath.Join(p.Root, filepath.FromSlash(sourceJournalDir), "notes.txt")
	if err := os.WriteFile(other, []byte("not a transaction file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RestoreSourceRecovery(p); err == nil || !strings.Contains(err.Error(), "unexpected") {
		t.Fatalf("unexpected recovery data was discarded: %v", err)
	}
	if got, _ := os.ReadFile(file); string(got) != "old" {
		t.Fatalf("source was not restored before cleanup: %q", got)
	}
	if got, _ := os.ReadFile(other); string(got) != "not a transaction file" {
		t.Fatalf("unrelated recovery data changed: %q", got)
	}
	status, err := SourceRecoveryStatus(p)
	if err != nil || status == nil || status.State != "restored" {
		t.Fatalf("incomplete cleanup lost its resolved state: %+v, %v", status, err)
	}
	// Only the test's owner explicitly removes the test file.
	if err := os.Remove(other); err != nil {
		t.Fatal(err)
	}
	if err := RestoreSourceRecovery(p); err != nil {
		t.Fatalf("explicitly retrying cleanup: %v", err)
	}
}

func TestSourceJournalCommittedCleanupDoesNotDiscardUnexpectedFiles(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "old")
	pl := recoveryTestPipeline(t, p)
	other := filepath.Join(p.Root, filepath.FromSlash(sourceJournalDir), "external-note")
	pl.sourceTransactionCheckpoint = func(stage, _ string) {
		if stage == "committed" {
			if err := os.WriteFile(other, []byte("preserve"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	batch, err := pl.ApplySources([]SourceEdit{{Path: "assets/a.txt", Before: []byte("old"), After: []byte("candidate")}}, false)
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("the committed edit was reported as failed: %+v, %v", batch, err)
	}
	if got, err := os.ReadFile(other); err != nil || string(got) != "preserve" {
		t.Fatalf("cleanup discarded an external file: %q, %v", got, err)
	}
	if _, _, err := pl.Scan(); err == nil {
		t.Fatal("pipeline continued writing while cleanup remained unresolved")
	}
	status, err := SourceRecoveryStatus(p)
	if err != nil || status == nil || status.State != "committed" {
		t.Fatalf("committed journal was not retained: %+v, %v", status, err)
	}
	pl.Close()
	// Only the test's owner removes the test file before retrying cleanup.
	if err := os.Remove(other); err != nil {
		t.Fatal(err)
	}
	if err := RestoreSourceRecovery(p); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != "candidate" {
		t.Fatalf("cleanup rolled back a committed source: %q, %v", got, err)
	}
}

func TestSourceJournalLiveCleanupRejectsDirectoryReplacement(t *testing.T) {
	p := newProject(t)
	write(t, p, "assets/a.txt", "old")
	pl := recoveryTestPipeline(t, p)
	defer pl.Close()
	active := filepath.Join(p.Root, filepath.FromSlash(sourceJournalDir))
	moved := filepath.Join(p.Root, ".aego", "source-transactions", "original-active")
	pl.sourceTransactionCheckpoint = func(stage, _ string) {
		if stage != "committed" {
			return
		}
		if err := os.Rename(active, moved); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(active, 0o700); err != nil {
			t.Fatal(err)
		}
		entries, err := os.ReadDir(moved)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			data, err := os.ReadFile(filepath.Join(moved, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(active, entry.Name()), data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	batch, err := pl.ApplySources([]SourceEdit{{Path: "assets/a.txt", Before: []byte("old"), After: []byte("candidate")}}, false)
	if err != nil || batch == nil || len(batch.Failed) != 0 || len(batch.Retry) != 0 {
		t.Fatalf("committed source edit: %+v, %v", batch, err)
	}
	if _, err := os.Stat(filepath.Join(active, "journal.jsonl")); err != nil {
		t.Errorf("cleanup removed the replacement directory's journal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(moved, "journal.jsonl")); err != nil {
		t.Errorf("the original recovery directory was lost: %v", err)
	}
}

func TestSourceJournalRecoveryRejectsSymlink(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "old")
	out := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(out, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	probe := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(out, probe); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	pl := recoveryTestPipeline(t, p)
	pl.Close()
	killSourceJournalWorker(t, p, "published:assets/a.txt", false)
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(out, file); err != nil {
		t.Fatal(err)
	}
	if err := RestoreSourceRecovery(p); err == nil {
		t.Fatal("recovery followed an external symlink")
	}
	if got, _ := os.ReadFile(out); string(got) != "outside" {
		t.Fatalf("symlink target changed: %q", got)
	}
	if info, err := os.Lstat(file); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("recovery replaced the external symlink: %v", err)
	}
}

func TestSourceJournalRecoveryRequiresExclusiveIndex(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "old")
	pl := recoveryTestPipeline(t, p)
	root, err := os.OpenRoot(p.Root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := startSourceJournal(root, []SourceEdit{{Path: "assets/a.txt", Before: []byte("old"), After: []byte("new")}}, false); err != nil {
		t.Fatal(err)
	}
	if err := replaceSource(root, "assets/a.txt", []byte("new")); err != nil {
		t.Fatal(err)
	}
	root.Close()
	id, ok := pl.Lookup("assets/a.txt")
	if !ok {
		t.Fatal("initial asset was not indexed")
	}
	if _, _, err := pl.Scan(); err == nil {
		t.Error("scan accepted pending recovery")
	}
	if _, err := pl.ImportAll(); err == nil {
		t.Error("import accepted pending recovery")
	}
	if err := pl.SetSettings(id, nil); err == nil {
		t.Error("metadata settings accepted pending recovery")
	}
	if err := pl.Move(id, "assets/moved.txt"); err == nil {
		t.Error("move accepted pending recovery")
	}
	if _, err := pl.Delete(id); err == nil {
		t.Error("delete accepted pending recovery")
	}
	if err := pl.Mkdir("assets/unexpected"); err == nil {
		t.Error("folder creation accepted pending recovery")
	}
	if _, err := pl.Repair(nil); err == nil {
		t.Error("metadata repair accepted pending recovery")
	}
	if err := RestoreSourceRecovery(p); err == nil {
		t.Fatal("recovery ran while a pipeline still owned the index")
	}
	if got, _ := os.ReadFile(file); string(got) != "new" {
		t.Fatalf("concurrent recovery changed source: %q", got)
	}
	pl.Close()
	if err := RestoreSourceRecovery(p); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(file); string(got) != "old" {
		t.Fatalf("recovery after closing the pipeline failed: %q", got)
	}
}

func TestIndexLockDoesNotClaimAnIncompleteFile(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, indexLock)
	if err := os.WriteFile(name, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	unlock, owner := acquireIndexLock(dir)
	defer unlock()
	if owner {
		t.Fatal("an incomplete lock was treated as stale")
	}
	if data, err := os.ReadFile(name); err != nil || len(data) != 0 {
		t.Fatalf("the incomplete lock was overwritten or deleted: %q, %v", data, err)
	}
}

func TestIndexLockDoesNotRemoveAReplacement(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, indexLock)
	unlock, owner := acquireIndexLock(dir)
	if !owner {
		t.Fatal("could not acquire a fresh lock")
	}
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte("different owner's lock"), 0o644); err != nil {
		t.Fatal(err)
	}
	unlock()
	if data, err := os.ReadFile(name); err != nil || string(data) != "different owner's lock" {
		t.Fatalf("replacement lock was removed: %q, %v", data, err)
	}
}

func TestSharedPipelineCannotMutateSourceFiles(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "old")
	owner := recoveryTestPipeline(t, p)
	defer owner.Close()
	reg := NewImporterRegistry()
	if err := reg.Register(&fakeImporter{version: 1}); err != nil {
		t.Fatal(err)
	}
	shared, err := Open(p, reg, log.Nop())
	if err != nil {
		t.Fatal(err)
	}
	defer shared.Close()
	if !shared.Shared() {
		t.Fatal("second pipeline unexpectedly owns the index")
	}
	if _, _, err := shared.Scan(); err != nil {
		t.Fatal(err)
	}
	id, ok := shared.Lookup("assets/a.txt")
	if !ok {
		t.Fatal("shared pipeline could not read the source")
	}
	meta, err := os.ReadFile(file + MetaSuffix)
	if err != nil {
		t.Fatal(err)
	}
	if err := shared.Mkdir("assets/other"); err == nil {
		t.Error("shared pipeline created a folder")
	}
	if err := shared.Move(id, "assets/moved.txt"); err == nil {
		t.Error("shared pipeline moved a source")
	}
	if _, _, err := shared.Duplicate(id); err == nil {
		t.Error("shared pipeline duplicated a source")
	}
	if _, err := shared.Delete(id); err == nil {
		t.Error("shared pipeline deleted a source")
	}
	if _, _, err := shared.ImportExternal([]string{file}, "assets"); err == nil {
		t.Error("shared pipeline imported an external source")
	}
	if got, err := os.ReadFile(file); err != nil || string(got) != "old" {
		t.Fatalf("shared source changed: %q, %v", got, err)
	}
	if got, err := os.ReadFile(file + MetaSuffix); err != nil || !bytes.Equal(got, meta) {
		t.Fatalf("shared metadata changed: %q, %v", got, err)
	}
}

func TestSourceJournalIncompletePreparationIsNotDiscarded(t *testing.T) {
	p := newProject(t)
	dir := filepath.Join(p.Root, filepath.FromSlash(sourceJournalDir))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(dir, "before-0000")
	if err := os.WriteFile(backup, []byte("preserve me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SourceRecoveryStatus(p); err == nil {
		t.Fatal("incomplete preparation was silently accepted")
	}
	if err := RestoreSourceRecovery(p); err == nil {
		t.Fatal("incomplete preparation was silently discarded")
	}
	if got, err := os.ReadFile(backup); err != nil || string(got) != "preserve me" {
		t.Fatalf("incomplete recovery data was removed: %q, %v", got, err)
	}
}

func TestSourceJournalRecoveryRejectsMalformedCompleteEvent(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/a.txt", "old")
	pl := recoveryTestPipeline(t, p)
	pl.Close()
	killSourceJournalWorker(t, p, "published:assets/a.txt", false)
	journal := filepath.Join(p.Root, filepath.FromSlash(sourceJournalFile))
	f, err := os.OpenFile(journal, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{\"kind\":\"unexpected\"}\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(journal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SourceRecoveryStatus(p); err == nil {
		t.Fatal("status accepted an unknown complete recovery event")
	}
	if err := RestoreSourceRecovery(p); err == nil {
		t.Fatal("recovery accepted an unknown complete event")
	}
	if got, _ := os.ReadFile(file); string(got) != "candidate" {
		t.Fatalf("corrupt journal changed the source: %q", got)
	}
	if got, _ := os.ReadFile(journal); !bytes.Equal(got, before) {
		t.Fatal("corrupt journal was discarded or rewritten")
	}
}

func TestSourceJournalRecoveryDistinguishesEmptyAndMissing(t *testing.T) {
	p := newProject(t)
	file := write(t, p, "assets/empty.txt", "")
	root, err := os.OpenRoot(p.Root)
	if err != nil {
		t.Fatal(err)
	}
	_, err = startSourceJournal(root, []SourceEdit{{Path: "assets/empty.txt", Before: []byte{}, After: []byte("new")}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := replaceSource(root, "assets/empty.txt", []byte("new")); err != nil {
		t.Fatal(err)
	}
	root.Close()
	if err := RestoreSourceRecovery(p); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(file); err != nil || !info.Mode().IsRegular() || info.Size() != 0 {
		t.Fatalf("an existing empty source was deleted or changed: %v, %v", info, err)
	}
}
