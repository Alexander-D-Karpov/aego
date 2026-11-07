package remoteexample

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func grantEveryone(t *testing.T, name, rights string) {
	t.Helper()
	cmd := exec.Command("icacls.exe", name, "/grant", "*S-1-1-0:"+rights)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("grant test ACL to Everyone: %s: %v", out, err)
	}
}

func TestWindowsStoreACLRejectsUntrustedWriters(t *testing.T) {
	t.Run("read-only grant", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "store")
		if _, err := NewStore(root, Limits{}); err != nil {
			t.Fatal(err)
		}
		grantEveryone(t, root, "(R)")
		if _, err := NewStore(root, Limits{}); err != nil {
			t.Fatalf("read-only ACL was rejected: %v", err)
		}
	})
	t.Run("writable store", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "store")
		if _, err := NewStore(root, Limits{}); err != nil {
			t.Fatal(err)
		}
		grantEveryone(t, root, "(OI)(CI)M")
		if _, err := NewStore(root, Limits{}); err == nil || !strings.Contains(err.Error(), "untrusted write") {
			t.Fatalf("writable store ACL was accepted: %v", err)
		}
	})
	t.Run("changed cache", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "store")
		store, err := NewStore(root, Limits{})
		if err != nil {
			t.Fatal(err)
		}
		grantEveryone(t, filepath.Join(root, "cache"), "(OI)(CI)M")
		if _, err := store.Copies(); err == nil || !strings.Contains(err.Error(), "untrusted write") {
			t.Fatalf("changed cache ACL was not rechecked: %v", err)
		}
	})
	t.Run("editable scene", func(t *testing.T) {
		f := newFixture(t)
		c := f.clone(t)
		grantEveryone(t, c.Entry, "M")
		if _, err := f.store.Reopen(c.ID); err == nil || !strings.Contains(err.Error(), "untrusted write") {
			t.Fatalf("writable local scene was accepted: %v", err)
		}
	})
}

func TestWindowsNullDACLIsRejected(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	if _, err := NewStore(root, Limits{}); err != nil {
		t.Fatal(err)
	}
	name, err := syscall.UTF16PtrFromString(root)
	if err != nil {
		t.Fatal(err)
	}
	h, err := syscall.CreateFile(name, writeDAC, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS|syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		t.Fatal(err)
	}
	setSecurityInfo := securityDLL.NewProc("SetSecurityInfo")
	code, _, _ := setSecurityInfo.Call(uintptr(h), seFileObject, daclSecurityInfo, 0, 0, 0, 0)
	syscall.CloseHandle(h)
	if code != 0 {
		t.Fatalf("could not set null test DACL: %v", syscall.Errno(code))
	}
	if _, err := NewStore(root, Limits{}); err == nil {
		t.Fatal("null DACL (unrestricted access) was accepted")
	}
}

func TestWindowsJunctionIsRejected(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(parent, "junction")
	cmd := exec.Command("cmd.exe", "/c", "mklink", "/J", link, target)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create test junction: %s: %v", out, err)
	}
	if _, err := NewStore(filepath.Join(link, "store"), Limits{}); err == nil {
		t.Fatal("store followed a Windows junction")
	}
}
