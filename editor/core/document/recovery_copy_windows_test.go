package document

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"unsafe"

	"aego/asset"
	"aego/core/ids"
)

var (
	testRecoveryGetNamedSecurityInfo = recoveryAdvapi.NewProc("GetNamedSecurityInfoW")
	testRecoveryDescriptorToString   = recoveryAdvapi.NewProc("ConvertSecurityDescriptorToStringSecurityDescriptorW")
)

func currentRecoveryUser(t *testing.T) *syscall.Tokenuser {
	t.Helper()
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		t.Fatal(err)
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil || user == nil || user.User.Sid == nil {
		t.Fatalf("current Windows user: %v", err)
	}
	return user
}

// Inspect the named object's descriptor independently of the creation handle.
func recoveryDescriptorString(t *testing.T, name string) string {
	t.Helper()
	wide, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		t.Fatal(err)
	}
	var descriptor unsafe.Pointer
	code, _, _ := testRecoveryGetNamedSecurityInfo.Call(uintptr(unsafe.Pointer(wide)), 1, 0x00000001|0x00000004,
		0, 0, 0, 0, uintptr(unsafe.Pointer(&descriptor)))
	if code != 0 || descriptor == nil {
		t.Fatalf("read named recovery descriptor: %v", syscall.Errno(code))
	}
	defer syscall.LocalFree(syscall.Handle(uintptr(descriptor)))
	var text *uint16
	var length uint32
	ok, _, callErr := testRecoveryDescriptorToString.Call(uintptr(descriptor), 1, 0x00000001|0x00000004,
		uintptr(unsafe.Pointer(&text)), uintptr(unsafe.Pointer(&length)))
	if ok == 0 || text == nil || length == 0 {
		t.Fatalf("convert recovery descriptor: %v", callErr)
	}
	defer syscall.LocalFree(syscall.Handle(uintptr(unsafe.Pointer(text))))
	return syscall.UTF16ToString(unsafe.Slice(text, length))
}

func assertPrivateRecovery(t *testing.T, name string, directory bool) {
	t.Helper()
	p, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		t.Fatal(err)
	}
	h, err := syscall.CreateFile(p, 0x00020000, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS|syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		t.Fatalf("open recovery ACL: %v", err)
	}
	defer syscall.CloseHandle(h)
	if err := verifyRecoveryACL(h, currentRecoveryUser(t), directory); err != nil {
		t.Fatalf("recovery entry is not private: %s: %v", name, err)
	}
	sddl := recoveryDescriptorString(t, name)
	if !strings.Contains(sddl, "D:P") || strings.Count(sddl, "(") != 1 || !strings.Contains(sddl, "(A;") {
		t.Fatalf("unexpected named recovery descriptor on %s: %s", name, sddl)
	}
}

func TestWindowsRecoveryDoesNotInheritEveryoneAccess(t *testing.T) {
	parent := t.TempDir()
	cmd := exec.Command("icacls.exe", parent, "/grant", "*S-1-1-0:(OI)(CI)F")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("grant Everyone on the temporary parent: %s: %v", out, err)
	}
	parentSDDL := recoveryDescriptorString(t, parent)
	if !strings.Contains(parentSDDL, ";;;WD)") && !strings.Contains(parentSDDL, ";;;S-1-1-0)") {
		t.Fatalf("fixture has no Everyone grant: %s", parentSDDL)
	}
	m := NewManager(Options{Root: t.TempDir()})
	d, err := m.NewTransientAsset(ids.NewAssetID(), "drawing", "art", asset.ArtTexture, View|EditSource)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("private working drawing")
	d.Binding.Dirty = func() bool { return true }
	d.Binding.Recovery = func() ([]byte, error) { return bytes.Clone(want), nil }
	output, count, err := m.WriteRecoveryCopy(parent)
	if err != nil || count != 1 {
		t.Fatalf("create private copy in public parent: %q, %d, %v", output, count, err)
	}
	assertPrivateRecovery(t, output, true)
	entries, err := os.ReadDir(output)
	if err != nil || len(entries) != 2 {
		t.Fatalf("recovery entries: %v, %v", entries, err)
	}
	for _, entry := range entries {
		assertPrivateRecovery(t, filepath.Join(output, entry.Name()), false)
	}
	data, err := os.ReadFile(filepath.Join(output, filepath.Base(m.assetRecoveryPath(d))))
	if err != nil || !bytes.Equal(data, want) {
		t.Fatalf("recovery content: %q, %v", data, err)
	}
}

func createRecoveryACLFixture(t *testing.T, name, sddl string) {
	t.Helper()
	text, err := syscall.UTF16PtrFromString(sddl)
	if err != nil {
		t.Fatal(err)
	}
	var descriptor unsafe.Pointer
	ok, _, callErr := recoveryConvertDescriptor.Call(uintptr(unsafe.Pointer(text)), 1, uintptr(unsafe.Pointer(&descriptor)), 0)
	if ok == 0 {
		t.Fatalf("create fixture descriptor: %v", callErr)
	}
	defer syscall.LocalFree(syscall.Handle(uintptr(descriptor)))
	p, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		t.Fatal(err)
	}
	attrs := syscall.SecurityAttributes{SecurityDescriptor: uintptr(descriptor)}
	attrs.Length = uint32(unsafe.Sizeof(attrs))
	if err := syscall.CreateDirectory(p, &attrs); err != nil {
		t.Fatalf("create ACL fixture: %v", err)
	}
}

func createNullRecoveryACLFixture(t *testing.T, name string, user *syscall.Tokenuser) {
	t.Helper()
	var descriptor struct {
		revision, reserved uint8
		control            uint16
		owner, group       *syscall.SID
		sacl, dacl         unsafe.Pointer
	}
	initialize := recoveryAdvapi.NewProc("InitializeSecurityDescriptor")
	setOwner := recoveryAdvapi.NewProc("SetSecurityDescriptorOwner")
	setDACL := recoveryAdvapi.NewProc("SetSecurityDescriptorDacl")
	setControl := recoveryAdvapi.NewProc("SetSecurityDescriptorControl")
	if ok, _, err := initialize.Call(uintptr(unsafe.Pointer(&descriptor)), 1); ok == 0 {
		t.Fatalf("initialize null DACL fixture: %v", err)
	}
	if ok, _, err := setOwner.Call(uintptr(unsafe.Pointer(&descriptor)), uintptr(unsafe.Pointer(user.User.Sid)), 0); ok == 0 {
		t.Fatalf("set null DACL fixture owner: %v", err)
	}
	if ok, _, err := setDACL.Call(uintptr(unsafe.Pointer(&descriptor)), 1, 0, 0); ok == 0 {
		t.Fatalf("set null DACL fixture: %v", err)
	}
	if ok, _, err := setControl.Call(uintptr(unsafe.Pointer(&descriptor)), recoveryDACLProtected, recoveryDACLProtected); ok == 0 {
		t.Fatalf("protect null DACL fixture: %v", err)
	}
	p, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		t.Fatal(err)
	}
	attrs := syscall.SecurityAttributes{SecurityDescriptor: uintptr(unsafe.Pointer(&descriptor))}
	attrs.Length = uint32(unsafe.Sizeof(attrs))
	if err := syscall.CreateDirectory(p, &attrs); err != nil {
		t.Fatalf("create null DACL fixture: %v", err)
	}
	runtime.KeepAlive(user)
	runtime.KeepAlive(&descriptor)
}

func TestWindowsRecoveryVerifierRejectsPublicAndNullDACL(t *testing.T) {
	user := currentRecoveryUser(t)
	owner, err := user.User.Sid.String()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, dacl, want string }{
		{"everyone", "D:P(A;OICI;FA;;;WD)", "another account"},
		{"null", "", "missing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			name := filepath.Join(t.TempDir(), "fixture")
			if tc.dacl == "" {
				createNullRecoveryACLFixture(t, name, user)
			} else {
				createRecoveryACLFixture(t, name, "O:"+owner+tc.dacl)
			}
			p, err := syscall.UTF16PtrFromString(name)
			if err != nil {
				t.Fatal(err)
			}
			h, err := syscall.CreateFile(p, 0x00020000, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
				nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS|syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer syscall.CloseHandle(h)
			if err := verifyRecoveryACL(h, user, true); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unsafe ACL was accepted or rejected for the wrong reason: %v", err)
			}
		})
	}
}

func TestWindowsRecoveryFailsBeforeCreateWhenACLVolumeCannotBeVerified(t *testing.T) {
	parent := t.TempDir()
	root, err := os.OpenRoot(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	want := errors.New("filesystem cannot persist ACLs")
	dir, created, err := createPrivateRecoveryDirWithVolumeCheck(root, "copy", func(syscall.Handle) error { return want })
	if dir != nil || created || !errors.Is(err, want) {
		t.Fatalf("unverified ACL filesystem was accepted: %v, %v, %v", dir, created, err)
	}
	if entries, err := os.ReadDir(parent); err != nil || len(entries) != 0 {
		t.Fatalf("unverified ACL filesystem was modified: %v, %v", entries, err)
	}
}
