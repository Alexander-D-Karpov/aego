package remoteexample

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"unsafe"
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
	t.Run("replaceable parent", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "parent")
		if err := os.Mkdir(parent, 0o700); err != nil {
			t.Fatal(err)
		}
		grantEveryone(t, parent, "(DC)")
		if _, err := NewStore(filepath.Join(parent, "store"), Limits{}); err == nil || !strings.Contains(err.Error(), "untrusted write") {
			t.Fatalf("parent with DELETE_CHILD grant was accepted: %v", err)
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

func TestWindowsTrustedInstallerOwnerIsNotAnArbitraryService(t *testing.T) {
	trusted, err := storeTrustedSIDs()
	if err != nil {
		t.Fatal(err)
	}
	installer, err := syscall.StringToSid("S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464")
	if err != nil || !trusted.owner(installer) {
		t.Fatalf("Windows TrustedInstaller was not recognized: %v", err)
	}
	other, err := syscall.StringToSid("S-1-5-80-1-2-3-4-5")
	if err != nil || trusted.owner(other) {
		t.Fatalf("an arbitrary service SID was trusted: %v", err)
	}
}

func TestWindowsNullDACLIsRejected(t *testing.T) {
	root := filepath.Join(t.TempDir(), "store")
	name, err := syscall.UTF16PtrFromString(root)
	if err != nil {
		t.Fatal(err)
	}
	// Create the fixture with a null DACL from the outset. Attempting to
	// replace an inherited DACL can require extra privileges on hosted runners.
	var sd struct {
		revision, reserved uint8
		control            uint16
		owner, group       *syscall.SID
		sacl, dacl         unsafe.Pointer
	}
	initialize := securityDLL.NewProc("InitializeSecurityDescriptor")
	ok, _, callErr := initialize.Call(uintptr(unsafe.Pointer(&sd)), 1)
	if ok == 0 {
		t.Fatalf("initialize test descriptor: %v", callErr)
	}
	setDACL := securityDLL.NewProc("SetSecurityDescriptorDacl")
	ok, _, callErr = setDACL.Call(uintptr(unsafe.Pointer(&sd)), 1, 0, 0)
	if ok == 0 {
		t.Fatalf("set null test DACL: %v", callErr)
	}
	// Preserve the null DACL instead of inheriting ACEs from the test parent.
	setControl := securityDLL.NewProc("SetSecurityDescriptorControl")
	const daclProtected = 0x1000 // SE_DACL_PROTECTED
	ok, _, callErr = setControl.Call(uintptr(unsafe.Pointer(&sd)), daclProtected, daclProtected)
	if ok == 0 {
		t.Fatalf("protect null test DACL from inheritance: %v", callErr)
	}
	attrs := syscall.SecurityAttributes{SecurityDescriptor: uintptr(unsafe.Pointer(&sd))}
	attrs.Length = uint32(unsafe.Sizeof(attrs))
	if err := syscall.CreateDirectory(name, &attrs); err != nil {
		t.Fatalf("create directory with null DACL: %v", err)
	}
	if _, err := NewStore(root, Limits{}); err == nil || !strings.Contains(err.Error(), "null DACL") {
		t.Fatalf("store did not reject a null DACL (unrestricted access): %v", err)
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
