package launcher

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aego/project"
)

type remoteCall struct {
	kind, source, origin, id string
	clone                    bool
}
type fakeRemote struct {
	calls chan remoteCall
	copy  RemoteCopy
	block bool
}

func (f *fakeRemote) List(context.Context) ([]RemoteCopy, error) {
	f.calls <- remoteCall{kind: "list"}
	return []RemoteCopy{f.copy}, nil
}
func (f *fakeRemote) Open(ctx context.Context, source, origin string, clone bool) (RemoteCopy, error) {
	f.calls <- remoteCall{kind: "open", source: source, origin: origin, clone: clone}
	if f.block {
		<-ctx.Done()
		return RemoteCopy{}, ctx.Err()
	}
	return f.copy, nil
}
func (f *fakeRemote) Reopen(_ context.Context, id string) (RemoteCopy, error) {
	f.calls <- remoteCall{kind: "reopen", id: id}
	return f.copy, nil
}
func (f *fakeRemote) Check(_ context.Context, id, origin string) (RemoteUpdate, error) {
	f.calls <- remoteCall{kind: "check", id: id, origin: origin}
	return RemoteUpdate{Available: true, Current: "1.0.0", Remote: "1.1.0"}, nil
}

func remoteTestCopy(t *testing.T, native bool) RemoteCopy {
	t.Helper()
	root := filepath.Join(t.TempDir(), "project")
	if _, err := project.Create(root, "Example", native); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(root, "scenes", "demo.aescene")
	if err := os.WriteFile(entry, []byte("scene version=1 {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return RemoteCopy{ID: "demo-000000000000000000000001", Name: "demo", Version: "1.0.0", Source: "https://example.com/demo.json", Directory: root, Entry: entry}
}

func waitRemote(t *testing.T, l *Launcher) {
	t.Helper()
	if l.remote.job == nil {
		t.Fatal("no remote operation started")
	}
	select {
	case reply := <-l.remote.job.done:
		l.remote.job.done <- reply
		l.pollRemote()
	case <-time.After(5 * time.Second):
		t.Fatal("remote worker did not finish")
	}
}

func TestRemoteConfirmationFreezesOriginAndOpensEntry(t *testing.T) {
	copy := remoteTestCopy(t, false)
	fake := &fakeRemote{calls: make(chan remoteCall, 4), copy: copy}
	l := New(Options{Remote: fake})
	defer l.Close()
	l.confirmRemote("open", "http://example.com/demo.json", "")
	if l.remote.pending != nil || l.err == "" {
		t.Fatal("insecure URL was accepted")
	}
	l.confirmRemote("open", copy.Source, "")
	if l.remote.pending == nil || l.remote.pending.origin != "https://example.com" {
		t.Fatal("confirmation did not show the exact origin")
	}
	select {
	case call := <-fake.calls:
		t.Fatalf("network started before confirmation: %+v", call)
	default:
	}
	request := *l.remote.pending
	l.remote.url = "https://different.example/other.json"
	l.remote.pending = nil
	l.startRemote(request)
	call := <-fake.calls
	if call.source != copy.Source || call.origin != "https://example.com" || call.clone {
		t.Fatalf("the confirmed request changed: %+v", call)
	}
	waitRemote(t, l)
	if root, ok := l.Result(); !ok || root != copy.Directory {
		t.Fatalf("launcher did not open the local copy: %q", root)
	}
	if entry, remote := l.ResultEntry(); !remote || entry != "scenes/demo.aescene" {
		t.Fatalf("entry scene not carried into the editor: %q remote=%v", entry, remote)
	}
	if err := l.Open(copy.Directory); err != nil {
		t.Fatal(err)
	}
	if entry, remote := l.ResultEntry(); remote || entry != "" {
		t.Fatal("a later local open retained remote launch state")
	}
}

func TestRemoteCancellationAndNativeProjectsFailClosed(t *testing.T) {
	copy := remoteTestCopy(t, false)
	fake := &fakeRemote{calls: make(chan remoteCall, 4), copy: copy, block: true}
	l := New(Options{Remote: fake})
	l.confirmRemote("clone", copy.Source, "")
	request := *l.remote.pending
	l.remote.pending = nil
	l.startRemote(request)
	<-fake.calls
	l.remote.job.canceled = true
	l.remote.job.cancel()
	waitRemote(t, l)
	if _, ok := l.Result(); ok || !strings.Contains(l.remote.status, "Canceled") {
		t.Fatal("a canceled remote request opened a project")
	}
	l.Close()
	l.startRemote(request)
	if l.remote.job != nil {
		t.Fatal("a closed launcher started another worker")
	}
	l = New(Options{})
	if err := l.acceptRemoteCopy(remoteTestCopy(t, true)); err == nil {
		t.Fatal("remote native Go project was accepted")
	}
	copy.Entry = filepath.Join(t.TempDir(), "outside.aescene")
	if err := os.WriteFile(copy.Entry, []byte("scene {}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := l.acceptRemoteCopy(copy); err == nil {
		t.Fatal("remote entry outside its project was accepted")
	}
}

func TestRemoteListReopenAndCheckStaySeparate(t *testing.T) {
	copy := remoteTestCopy(t, false)
	fake := &fakeRemote{calls: make(chan remoteCall, 4), copy: copy}
	l := New(Options{Remote: fake})
	defer l.Close()
	l.startRemote(remoteRequest{kind: "list"})
	if call := <-fake.calls; call.kind != "list" {
		t.Fatal(call)
	}
	waitRemote(t, l)
	if len(l.remote.copies) != 1 || l.remote.copies[0].ID != copy.ID {
		t.Fatal("saved copy was not listed")
	}
	l.confirmRemote("check", copy.Source, copy.ID)
	select {
	case <-fake.calls:
		t.Fatal("update check started before confirmation")
	default:
	}
	request := *l.remote.pending
	l.remote.pending = nil
	l.startRemote(request)
	if call := <-fake.calls; call.kind != "check" || call.origin != "https://example.com" {
		t.Fatal(call)
	}
	waitRemote(t, l)
	if !strings.Contains(l.remote.status, "1.1.0") || !strings.Contains(l.remote.status, "not changed") {
		t.Fatal("update check did not explicitly preserve the local copy")
	}
	l.startRemote(remoteRequest{kind: "reopen", id: copy.ID})
	if call := <-fake.calls; call.kind != "reopen" || call.id != copy.ID {
		t.Fatal(call)
	}
	waitRemote(t, l)
	if _, ok := l.Result(); !ok {
		t.Fatal("offline reopen did not open the local project")
	}
}

func TestChoosingLocalProjectCancelsPendingRemoteOpen(t *testing.T) {
	copy := remoteTestCopy(t, false)
	fake := &fakeRemote{calls: make(chan remoteCall, 1), copy: copy, block: true}
	l := New(Options{Remote: fake})
	l.startRemote(remoteRequest{kind: "open", source: copy.Source, origin: "https://example.com"})
	<-fake.calls
	local := remoteTestCopy(t, false)
	if err := l.Open(local.Directory); err != nil {
		t.Fatal(err)
	}
	waitRemote(t, l)
	if root, ok := l.Result(); !ok || root != local.Directory {
		t.Fatalf("late remote result replaced the chosen local project: %q", root)
	}
	if _, remote := l.ResultEntry(); remote {
		t.Fatal("late remote operation changed the local result type")
	}
}
