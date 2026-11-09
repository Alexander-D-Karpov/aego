//go:build linux

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
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// Holding the stable guard represents another process between inspecting a
// stale marker and removing it. The contender must not acquire the index lock
// in that interval, even when no PID marker exists yet.
func TestIndexGuardSerializesCompetingAcquisition(t *testing.T) {
	dir := t.TempDir()
	guard, err := os.OpenFile(filepath.Join(dir, "index.guard"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	if err := syscall.Flock(int(guard.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	defer syscall.Flock(int(guard.Fd()), syscall.LOCK_UN)
	unlock, owner := acquireIndexLock(dir)
	if owner {
		unlock()
		t.Fatal("acquired index while another writer held the guard")
	}
	if _, err := os.Lstat(filepath.Join(dir, indexLock)); !os.IsNotExist(err) {
		t.Fatalf("contender modified the PID marker: %v", err)
	}
}

func TestIndexGuardRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "external")
	if err := os.WriteFile(target, []byte("external data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "index.guard")); err != nil {
		t.Fatal(err)
	}
	unlock, owner := acquireIndexLock(dir)
	if owner {
		unlock()
		t.Fatal("a symlink was accepted as an index guard")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "external data" {
		t.Fatalf("external target changed: %q, %v", data, err)
	}
	if _, err := os.Lstat(filepath.Join(dir, indexLock)); !os.IsNotExist(err) {
		t.Fatalf("symlink guard created a PID marker: %v", err)
	}
}

func TestIndexGuardBlocksContenderWhenMarkerIsRemoved(t *testing.T) {
	dir := t.TempDir()
	unlock, owner := acquireIndexLock(dir)
	if !owner {
		t.Fatal("failed to acquire initial index lock")
	}
	defer unlock()
	if err := os.Remove(filepath.Join(dir, indexLock)); err != nil {
		t.Fatal(err)
	}
	if release, acquired := acquireIndexLock(dir); acquired {
		release()
		t.Fatal("second writer acquired index while the first guard was held")
	}
	unlock()
	release, acquired := acquireIndexLock(dir)
	if !acquired {
		t.Fatal("lock could not be acquired after releasing the guard")
	}
	defer release()
	// A repeated release by the old owner must not touch the new marker.
	unlock()
	if _, err := os.Lstat(filepath.Join(dir, indexLock)); err != nil {
		t.Fatalf("repeated release removed the new owner's marker: %v", err)
	}
}

func TestIndexGuardHasOneOwnerWithStaleMarker(t *testing.T) {
	dir := t.TempDir()
	dead := exec.Command("/bin/true")
	if err := dead.Start(); err != nil {
		t.Fatal(err)
	}
	pid := dead.Process.Pid
	if err := dead.Wait(); err != nil {
		t.Fatal(err)
	}
	if processAlive(pid) {
		t.Fatal("fixture PID is still active or has been reused")
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, indexLock), []byte(fmt.Sprintf("%d\n%s\n", pid, host)), 0600); err != nil {
		t.Fatal(err)
	}
	const contenders = 24
	start, release := make(chan struct{}), make(chan struct{})
	result := make(chan bool, contenders)
	var workers sync.WaitGroup
	for range contenders {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			unlock, owner := acquireIndexLock(dir)
			result <- owner
			<-release
			unlock()
		}()
	}
	close(start)
	owners := 0
	for range contenders {
		if <-result {
			owners++
		}
	}
	close(release)
	workers.Wait()
	if owners != 1 {
		t.Fatalf("concurrent owners = %d, want 1", owners)
	}
	unlock, owner := acquireIndexLock(dir)
	defer unlock()
	if !owner {
		t.Fatal("index remained locked after all contenders exited")
	}
}

func TestIndexGuardChild(t *testing.T) {
	dir := os.Getenv("AEGO_TEST_INDEX_GUARD_DIR")
	if dir == "" {
		return
	}
	unlock, owner := acquireIndexLock(dir)
	if !owner {
		t.Fatal("child failed to acquire index lock")
	}
	defer unlock()
	fmt.Println("guard-child-ready")
	_, _ = io.Copy(io.Discard, os.Stdin)
}

func TestIndexGuardRealProcessDeathAndExplicitReacquire(t *testing.T) {
	dir := t.TempDir()
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, bin, "-test.run=^TestIndexGuardChild$")
	child.Env = append(os.Environ(), "AEGO_TEST_INDEX_GUARD_DIR="+dir)
	stdin, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	child.Stderr = os.Stderr
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	}()
	ready, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || strings.TrimSpace(ready) != "guard-child-ready" {
		t.Fatalf("child did not acquire the lock: %q, %v", ready, err)
	}
	guardPath := filepath.Join(dir, "index.guard")
	guardInfo, err := os.Lstat(guardPath)
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, indexLock)
	before, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if unlock, owner := acquireIndexLock(dir); owner {
		unlock()
		t.Fatal("parent acquired a live child's index lock")
	}
	if after, err := os.ReadFile(marker); err != nil || !bytes.Equal(before, after) {
		t.Fatalf("live marker changed: %q, %v", after, err)
	}
	if err := child.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err == nil {
		t.Fatal("SIGKILL child unexpectedly exited successfully")
	}
	waited = true
	if after, err := os.ReadFile(marker); err != nil || !bytes.Equal(before, after) {
		t.Fatalf("marker unexpectedly disappeared after SIGKILL: %q, %v", after, err)
	}
	unlock, owner := acquireIndexLock(dir)
	if !owner {
		t.Fatal("could not explicitly reacquire after child process died")
	}
	if current, err := os.Lstat(guardPath); err != nil || !os.SameFile(guardInfo, current) {
		t.Fatalf("guard inode was replaced after SIGKILL: %v", err)
	}
	unlock()
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatalf("owned marker remained after release: %v", err)
	}
	if _, err := os.Lstat(guardPath); err != nil {
		t.Fatalf("persistent guard was removed: %v", err)
	}
}
