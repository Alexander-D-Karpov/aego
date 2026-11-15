package document

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"aego/core/log"
	"aego/scene"
	"aego/scene/sceneio"
	"aego/schema"
)

type Health struct {
	Max float32 `aego:"max"`
}

func newManager(t *testing.T) *Manager {
	t.Helper()
	types := scene.NewTypes(schema.NewRegistry())
	if err := scene.RegisterComponent[Health](types, schema.Name("test.Health")); err != nil {
		t.Fatal(err)
	}
	if err := sceneio.RegisterUnknown(types); err != nil {
		t.Fatal(err)
	}
	if err := types.Schema().Freeze(); err != nil {
		t.Fatal(err)
	}
	return NewManager(Options{Root: t.TempDir(), Types: types, Log: log.Nop()})
}

func sampleScene(m *Manager, max float32) *scene.Scene {
	s := scene.New(m.opt.Types, nil, scene.Options{})
	a := s.Create(s.Root(), "A")
	scene.Add[Health](s, a).Max = max
	s.Flush()
	return s
}

func TestSaveCloseReopen(t *testing.T) {
	m := newManager(t)
	d := m.New("scenes/a.aescene", sampleScene(m, 5))
	if !d.Dirty() {
		t.Fatal("new document must be dirty")
	}
	if err := m.Save(d.ID); err != nil {
		t.Fatal(err)
	}
	if d.Dirty() {
		t.Fatal("saved document is dirty")
	}
	data, _ := os.ReadFile(m.Abs(d.Path))
	if string(data) != string(sceneio.Save(d.Scene())) {
		t.Fatal("saved file is not the canonical form")
	}
	if err := m.Close(d.ID, false); err != nil {
		t.Fatal(err)
	}
	back, err := m.Open("scenes/a.aescene")
	if err != nil {
		t.Fatal(err)
	}
	a := back.Scene().Find(back.Scene().Root(), "A")
	if h, ok := scene.Get[Health](back.Scene(), a); !ok || h.Max != 5 {
		t.Fatal("component lost")
	}
	if back.Dirty() {
		t.Fatal("freshly opened document is dirty")
	}
}

func TestCloseRefusesDirty(t *testing.T) {
	m := newManager(t)
	d := m.New("scenes/a.aescene", sampleScene(m, 1))
	if err := m.Close(d.ID, false); err == nil {
		t.Fatal("dirty close accepted")
	}
	if err := m.Close(d.ID, true); err != nil || m.Active() != nil {
		t.Fatal("forced close failed")
	}
}

func touchLater(t *testing.T, abs string, data []byte) {
	t.Helper()
	if err := os.WriteFile(abs, data, 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	os.Chtimes(abs, later, later)
}

func TestExternalChange(t *testing.T) {
	m := newManager(t)
	d := m.New("scenes/a.aescene", sampleScene(m, 1))
	m.Save(d.ID)

	touchLater(t, m.Abs(d.Path), sceneio.Save(sampleScene(m, 9)))
	reloaded, conflicts := m.CheckExternal()
	if len(reloaded) != 1 || len(conflicts) != 0 {
		t.Fatalf("clean document: reloaded=%v conflicts=%v", reloaded, conflicts)
	}
	a := d.Scene().Find(d.Scene().Root(), "A")
	if h, _ := scene.Get[Health](d.Scene(), a); h.Max != 9 {
		t.Fatal("reload did not pick up the new content")
	}

	d.MarkDirty()
	touchLater(t, m.Abs(d.Path), append(sceneio.Save(sampleScene(m, 3)), '\n'))
	reloaded, conflicts = m.CheckExternal()
	if len(reloaded) != 0 || len(conflicts) != 1 {
		t.Fatalf("dirty document: reloaded=%v conflicts=%v", reloaded, conflicts)
	}
	if _, again := m.CheckExternal(); len(again) != 0 {
		t.Fatal("the same change must be reported once")
	}
}

func TestRecovery(t *testing.T) {
	m := newManager(t)
	d := m.New("scenes/untitled.aescene", sampleScene(m, 4))
	if n, err := m.WriteRecovery(); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	fresh := NewManager(m.opt)
	found := fresh.FindRecovery()
	if len(found) != 1 || found[0].Path != d.Path {
		t.Fatalf("found = %+v", found)
	}
	back, err := fresh.RestoreRecovery(found[0])
	if err != nil {
		t.Fatal(err)
	}
	if !back.Dirty() || back.Scene().Find(back.Scene().Root(), "A").IsZero() {
		t.Fatal("restored document must be dirty and complete")
	}
	if err := fresh.Save(back.ID); err != nil {
		t.Fatal(err)
	}
	if len(NewManager(m.opt).FindRecovery()) != 0 {
		t.Fatal("save must drop the recovery file")
	}
}

func TestRecoveryDoesNotSwitchAnExistingActiveScene(t *testing.T) {
	previous := newManager(t)
	selected := previous.New("scenes/selected.aescene", sampleScene(previous, 5))
	if err := previous.Save(selected.ID); err != nil {
		t.Fatal(err)
	}
	other := previous.New("scenes/unknown.aescene", sampleScene(previous, 9))
	if n, err := previous.WriteRecovery(); err != nil || n != 1 {
		t.Fatalf("cannot create unrelated scene recovery: n=%d, %v", n, err)
	}
	for _, dirty := range []bool{false, true} {
		name := "clean active scene"
		if dirty {
			name = "dirty active scene"
		}
		t.Run(name, func(t *testing.T) {
			fresh := NewManager(previous.opt)
			active, err := fresh.Open(selected.Path)
			if err != nil {
				t.Fatal(err)
			}
			if dirty {
				active.Scene().Create(active.Scene().Root(), "Unsaved node")
				active.Scene().Flush()
				active.MarkDirty()
			}
			before := sceneio.Save(active.Scene())
			recoveries := fresh.FindRecovery()
			if len(recoveries) != 1 || recoveries[0].Path != other.Path {
				t.Fatalf("found recovery = %+v", recoveries)
			}
			recoveryBytes, err := os.ReadFile(recoveries[0].File)
			if err != nil {
				t.Fatal(err)
			}
			var events []Event
			fresh.Subscribe(func(e Event) { events = append(events, e) })
			restored, err := fresh.RestoreRecovery(recoveries[0])
			if err != nil {
				t.Fatal(err)
			}
			if fresh.Active() != active || !bytes.Equal(sceneio.Save(active.Scene()), before) || active.Dirty() != dirty {
				t.Fatal("restoring another scene changed the previously active scene")
			}
			if fresh.Find(other.Path) != restored || !restored.Dirty() || restored.Scene().Find(restored.Scene().Root(), "A").IsZero() {
				t.Fatal("the unrelated recovery did not remain open and dirty")
			}
			for _, e := range events {
				if e.Kind == EvActivated {
					t.Fatalf("recovery temporarily activated another scene: %+v", events)
				}
			}
			if got, err := os.ReadFile(recoveries[0].File); err != nil || !bytes.Equal(got, recoveryBytes) {
				t.Fatal("restoring the scene changed the recovery copy", err)
			}
		})
	}
	// With no previously selected scene, the explicitly recovered scene is
	// still activated so it does not disappear into an empty workspace.
	empty := NewManager(previous.opt)
	recoveries := empty.FindRecovery()
	if len(recoveries) != 1 {
		t.Fatalf("found recovery = %+v", recoveries)
	}
	first, err := empty.RestoreRecovery(recoveries[0])
	if err != nil || empty.Active() != first {
		t.Fatalf("first recovered scene was not activated: %v", err)
	}
}

func TestDiscardOpenRecoveryPreservesUnrelatedCopies(t *testing.T) {
	m := newManager(t)
	d := m.New("scenes/open.aescene", sampleScene(m, 4))
	if err := os.MkdirAll(m.Abs("scripts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(m.Abs("scripts/open.aes"), []byte("package old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	script, err := m.OpenScript("scripts/open.aes")
	if err != nil {
		t.Fatal(err)
	}
	script.Code.ReplaceSnapshot([]byte("package changed\n"), false)
	if n, err := m.WriteRecovery(); err != nil || n != 2 {
		t.Fatalf("cannot create an open document recovery: %d, %v", n, err)
	}
	unrelated := filepath.Join(m.recoveryDir(), "asset-unopened.recovery")
	if err := os.WriteFile(unrelated, []byte("unrelated recovery"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.DiscardOpenRecovery(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(m.recoveryFile(d.Path)); !os.IsNotExist(err) {
		t.Fatalf("discarded open document retained its recovery: %v", err)
	}
	if _, err := os.Stat(m.scriptRecoveryFile(script.Path)); !os.IsNotExist(err) {
		t.Fatalf("discarded open script retained its recovery: %v", err)
	}
	if got, err := os.ReadFile(unrelated); err != nil || string(got) != "unrelated recovery" {
		t.Fatal("discarding open documents erased an unrelated recovery")
	}
}

func TestDiscardOpenRecoveryPreservesUnownedOrChangedCopies(t *testing.T) {
	for _, changed := range []bool{false, true} {
		name := "preexisting"
		if changed {
			name = "changed after write"
		}
		t.Run(name, func(t *testing.T) {
			m := newManager(t)
			d := m.New("scenes/open.aescene", sampleScene(m, 4))
			if changed {
				if _, err := m.WriteRecovery(); err != nil {
					t.Fatal(err)
				}
			} else if err := os.MkdirAll(m.recoveryDir(), 0755); err != nil {
				t.Fatal(err)
			}
			file := m.recoveryFile(d.Path)
			if err := os.WriteFile(file, []byte("other recovery"), 0600); err != nil {
				t.Fatal(err)
			}
			err := m.DiscardOpenRecovery()
			if (!changed && err != nil) || (changed && err == nil) {
				t.Fatalf("unexpected conflict result: %v", err)
			}
			if got, err := os.ReadFile(file); err != nil || string(got) != "other recovery" {
				t.Fatal("discard removed a recovery not owned by this editor session")
			}
		})
	}
}

func TestPeriodicRecoveryDoesNotReplaceEarlierOrChangedCopies(t *testing.T) {
	for _, changed := range []bool{false, true} {
		name := "earlier session"
		if changed {
			name = "external modification"
		}
		t.Run(name, func(t *testing.T) {
			m := newManager(t)
			d := m.New("scenes/open.aescene", sampleScene(m, 4))
			if changed {
				if _, err := m.WriteRecovery(); err != nil {
					t.Fatal(err)
				}
			} else if err := os.MkdirAll(m.recoveryDir(), 0755); err != nil {
				t.Fatal(err)
			}
			file := m.recoveryFile(d.Path)
			if err := os.WriteFile(file, []byte("other recovery"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := m.WriteRecovery(); err == nil {
				t.Fatal("periodic write did not report the recovery conflict")
			}
			if got, err := os.ReadFile(file); err != nil || string(got) != "other recovery" {
				t.Fatal("periodic recovery replaced another copy")
			}
		})
	}
}

func TestDiscardOpenRecoveryDoesNotFollowReplacedSymlink(t *testing.T) {
	m := newManager(t)
	d := m.New("scenes/open.aescene", sampleScene(m, 4))
	if _, err := m.WriteRecovery(); err != nil {
		t.Fatal(err)
	}
	file := m.recoveryFile(d.Path)
	target := m.Abs("outside")
	if err := os.WriteFile(target, []byte("outside bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, file); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := m.DiscardOpenRecovery(); err == nil {
		t.Fatal("discard did not report the replaced recovery")
	}
	if st, err := os.Lstat(file); err != nil || st.Mode()&os.ModeSymlink == 0 {
		t.Fatal("discard removed the replacement symlink")
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "outside bytes" {
		t.Fatal("discard changed a symlink target")
	}
}

func TestUniquePath(t *testing.T) {
	m := newManager(t)
	m.New("scenes/untitled.aescene", sampleScene(m, 1))
	if p := m.UniquePath("scenes/untitled.aescene"); p != "scenes/untitled-2.aescene" {
		t.Fatalf("path = %s", p)
	}
}

func TestLock(t *testing.T) {
	root := t.TempDir()
	release, other, err := AcquireLock(root)
	if err != nil || other != nil {
		t.Fatalf("first lock: %v %v", other, err)
	}
	info, ok := ReadLock(root)
	if !ok || info.PID != os.Getpid() {
		t.Fatal("lock not written")
	}
	release()
	if _, ok := ReadLock(root); ok {
		t.Fatal("release did not remove the lock")
	}
}

func TestRenamedMovesScripts(t *testing.T) {
	m := newManager(t)
	if err := WriteAtomic(m.Abs("scripts/a/enemy.aes"), []byte("package a\n")); err != nil {
		t.Fatal(err)
	}
	d, err := m.OpenScript("scripts/a/enemy.aes")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(m.Abs("scripts/b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(m.Abs("scripts/a"), m.Abs("scripts/b/a")); err != nil {
		t.Fatal(err)
	}
	m.Renamed("scripts/a", "scripts/b/a")
	if d.Path != "scripts/b/a/enemy.aes" || m.Script("scripts/b/a/enemy.aes") != d {
		t.Fatalf("script path = %s", d.Path)
	}
	if _, conflicts := m.CheckExternalScripts(); len(conflicts) != 0 {
		t.Fatal("a moved script must not report as deleted")
	}
}
