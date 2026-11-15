package document

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"
)

// Windows does not implement Unix permission bits as ACLs. Create each object
// with a protected DACL, rather than exposing it before a later ACL change.
// Handles keep all creates relative to the selected directory, including if it
// is renamed, and disallow deletion of open entries. A filesystem that cannot
// persist ACLs is rejected before writing. This is not encryption or protection
// from administrators, backup privileges, or a malicious filesystem server.
var (
	recoveryAdvapi               = syscall.NewLazyDLL("advapi32.dll")
	recoveryKernel               = syscall.NewLazyDLL("kernel32.dll")
	recoveryNT                   = syscall.NewLazyDLL("ntdll.dll")
	recoveryConvertDescriptor    = recoveryAdvapi.NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW")
	recoveryGetSecurityInfo      = recoveryAdvapi.NewProc("GetSecurityInfo")
	recoveryGetDescriptorControl = recoveryAdvapi.NewProc("GetSecurityDescriptorControl")
	recoveryGetACLInformation    = recoveryAdvapi.NewProc("GetAclInformation")
	recoveryGetACE               = recoveryAdvapi.NewProc("GetAce")
	recoveryIsValidACL           = recoveryAdvapi.NewProc("IsValidAcl")
	recoveryIsValidSID           = recoveryAdvapi.NewProc("IsValidSid")
	recoveryEqualSID             = recoveryAdvapi.NewProc("EqualSid")
	recoveryGetVolumeInformation = recoveryKernel.NewProc("GetVolumeInformationByHandleW")
	recoveryNtCreateFile         = recoveryNT.NewProc("NtCreateFile")
	recoveryNtStatusToDOS        = recoveryNT.NewProc("RtlNtStatusToDosError")
)

const (
	recoveryPersistentACLs   = 0x00000008
	recoveryDACLProtected    = 0x1000
	recoveryFileAllAccess    = 0x001f01ff
	recoveryFileGenericRead  = 0x00120089
	recoveryFileGenericWrite = 0x00120116
	recoveryFileCreate       = 2
	recoveryDirectoryFile    = 0x00000001
	recoveryNonDirectoryFile = 0x00000040
	recoverySynchronousIO    = 0x00000020
	recoveryOpenReparsePoint = 0x00200000
	recoveryCaseInsensitive  = 0x00000040
	recoveryDontReparse      = 0x00001000
)

type recoveryUnicodeString struct {
	Length, MaximumLength uint16
	Buffer                *uint16
}

type recoveryObjectAttributes struct {
	Length             uint32
	RootDirectory      syscall.Handle
	ObjectName         *recoveryUnicodeString
	Attributes         uint32
	SecurityDescriptor unsafe.Pointer
	SecurityQoS        unsafe.Pointer
}

type recoveryIOStatus struct {
	Status      uintptr
	Information uintptr
}

type recoveryACLSize struct{ aceCount, bytesInUse, bytesFree uint32 }
type recoveryACE struct {
	kind, flags uint8
	size        uint16
	mask        uint32
}

type privateRecoveryDir struct {
	file *os.File
	user *syscall.Tokenuser
}

// created is true even when subsequent validation fails, so callers can tell
// the user where an empty or partial directory was left for inspection.
func createPrivateRecoveryDir(root *os.Root, name string) (*privateRecoveryDir, bool, error) {
	return createPrivateRecoveryDirWithVolumeCheck(root, name, requireRecoveryACLVolume)
}

func createPrivateRecoveryDirWithVolumeCheck(root *os.Root, name string, checkVolume func(syscall.Handle) error) (*privateRecoveryDir, bool, error) {
	parent, err := root.Open(".")
	if err != nil {
		return nil, false, err
	}
	defer parent.Close()
	if err := checkVolume(syscall.Handle(parent.Fd())); err != nil {
		return nil, false, err
	}
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return nil, false, err
	}
	user, err := token.GetTokenUser()
	_ = token.Close()
	if err != nil {
		return nil, false, err
	}
	if user == nil || user.User.Sid == nil {
		return nil, false, fmt.Errorf("cannot identify the Windows recovery owner")
	}
	f, err := createRecoveryObject(parent, name, user, true)
	if err != nil {
		return nil, false, err
	}
	if err := verifyRecoveryACL(syscall.Handle(f.Fd()), user, true); err != nil {
		_ = f.Close()
		return nil, true, err
	}
	return &privateRecoveryDir{file: f, user: user}, true, nil
}

func (d *privateRecoveryDir) CreateFile(name string) (*os.File, error) {
	f, err := createRecoveryObject(d.file, name, d.user, false)
	if err != nil {
		return nil, err
	}
	if err := verifyRecoveryACL(syscall.Handle(f.Fd()), d.user, false); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

func (d *privateRecoveryDir) Close() error { return d.file.Close() }

func requireRecoveryACLVolume(h syscall.Handle) error {
	var flags uint32
	ok, _, callErr := recoveryGetVolumeInformation.Call(uintptr(h), 0, 0, 0, 0, uintptr(unsafe.Pointer(&flags)), 0, 0)
	if ok == 0 {
		return fmt.Errorf("cannot verify that the recovery filesystem enforces ACLs: %w", callErr)
	}
	if flags&recoveryPersistentACLs == 0 {
		return fmt.Errorf("the recovery filesystem does not enforce persistent ACLs")
	}
	return nil
}

// FILE_CREATE is exclusive and does not follow an existing reparse point. The
// descriptor is passed in the original creation call, not applied afterward.
func createRecoveryObject(parent *os.File, name string, user *syscall.Tokenuser, directory bool) (*os.File, error) {
	if !filepath.IsLocal(name) || name == "." || strings.ContainsAny(name, `\/:`) {
		return nil, fmt.Errorf("invalid recovery entry name %q", name)
	}
	owner, err := user.User.Sid.String()
	if err != nil {
		return nil, err
	}
	inherit := ""
	options := uint32(recoveryNonDirectoryFile | recoverySynchronousIO | recoveryOpenReparsePoint)
	if directory {
		inherit = "OICI"
		// FILE_CREATE already rejects an existing leaf, and OBJ_DONT_REPARSE
		// rejects reparse traversal. OPEN_REPARSE_POINT is not a documented
		// companion for FILE_DIRECTORY_FILE.
		options = recoveryDirectoryFile | recoverySynchronousIO
	}
	sddl, err := syscall.UTF16PtrFromString("O:" + owner + "D:P(A;" + inherit + ";FA;;;" + owner + ")")
	if err != nil {
		return nil, err
	}
	var descriptor unsafe.Pointer
	ok, _, callErr := recoveryConvertDescriptor.Call(uintptr(unsafe.Pointer(sddl)), 1, uintptr(unsafe.Pointer(&descriptor)), 0)
	if ok == 0 {
		return nil, fmt.Errorf("cannot prepare a private recovery ACL: %w", callErr)
	}
	defer syscall.LocalFree(syscall.Handle(uintptr(descriptor)))
	wide, err := syscall.UTF16FromString(name)
	if err != nil || len(wide)*2 > 65535 {
		return nil, fmt.Errorf("invalid recovery entry name %q", name)
	}
	unicode := recoveryUnicodeString{Length: uint16(len(wide)*2 - 2), MaximumLength: uint16(len(wide) * 2), Buffer: &wide[0]}
	attrs := recoveryObjectAttributes{
		RootDirectory:      syscall.Handle(parent.Fd()),
		ObjectName:         &unicode,
		Attributes:         recoveryCaseInsensitive | recoveryDontReparse,
		SecurityDescriptor: descriptor,
	}
	attrs.Length = uint32(unsafe.Sizeof(attrs))
	var handle syscall.Handle
	var status recoveryIOStatus
	ntstatus, _, _ := recoveryNtCreateFile.Call(
		uintptr(unsafe.Pointer(&handle)), recoveryFileGenericRead|recoveryFileGenericWrite,
		uintptr(unsafe.Pointer(&attrs)), uintptr(unsafe.Pointer(&status)), 0,
		syscall.FILE_ATTRIBUTE_NORMAL, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE,
		recoveryFileCreate, uintptr(options), 0, 0,
	)
	runtime.KeepAlive(parent)
	runtime.KeepAlive(user)
	if uint32(ntstatus) != 0 {
		code, _, _ := recoveryNtStatusToDOS.Call(ntstatus)
		return nil, fmt.Errorf("cannot create private recovery entry %q: %w", name, syscall.Errno(code))
	}
	return os.NewFile(uintptr(handle), name), nil
}

// Require precisely the current user as owner and as the only allowed SID.
// A null, inherited, changed or otherwise unrecognizable DACL fails closed.
func verifyRecoveryACL(h syscall.Handle, user *syscall.Tokenuser, directory bool) error {
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(h, &info); err != nil {
		return err
	}
	if info.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 || (info.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY != 0) != directory {
		return fmt.Errorf("private recovery entry is a reparse point or has changed type")
	}
	var owner *syscall.SID
	var acl, descriptor unsafe.Pointer
	code, _, _ := recoveryGetSecurityInfo.Call(uintptr(h), 1, 0x00000001|0x00000004,
		uintptr(unsafe.Pointer(&owner)), 0, uintptr(unsafe.Pointer(&acl)), 0, uintptr(unsafe.Pointer(&descriptor)))
	if code != 0 {
		return fmt.Errorf("cannot read recovery ACL: %w", syscall.Errno(code))
	}
	if descriptor != nil {
		defer syscall.LocalFree(syscall.Handle(uintptr(descriptor)))
	}
	if descriptor == nil || owner == nil || acl == nil {
		return fmt.Errorf("recovery ACL or owner is missing")
	}
	valid, _, _ := recoveryIsValidSID.Call(uintptr(unsafe.Pointer(owner)))
	if valid == 0 {
		return fmt.Errorf("recovery owner SID is invalid")
	}
	equal, _, _ := recoveryEqualSID.Call(uintptr(unsafe.Pointer(owner)), uintptr(unsafe.Pointer(user.User.Sid)))
	if equal == 0 {
		return fmt.Errorf("recovery owner is not the current user")
	}
	var control uint16
	var revision uint32
	ok, _, callErr := recoveryGetDescriptorControl.Call(uintptr(descriptor), uintptr(unsafe.Pointer(&control)), uintptr(unsafe.Pointer(&revision)))
	if ok == 0 {
		return fmt.Errorf("cannot inspect recovery ACL protection: %w", callErr)
	}
	valid, _, _ = recoveryIsValidACL.Call(uintptr(acl))
	if control&recoveryDACLProtected == 0 || valid == 0 {
		return fmt.Errorf("recovery ACL is not protected or is invalid")
	}
	var size recoveryACLSize
	ok, _, callErr = recoveryGetACLInformation.Call(uintptr(acl), uintptr(unsafe.Pointer(&size)), unsafe.Sizeof(size), 2)
	if ok == 0 {
		return fmt.Errorf("cannot inspect recovery ACL: %w", callErr)
	}
	if size.aceCount != 1 {
		return fmt.Errorf("recovery ACL has unexpected access entries")
	}
	var raw unsafe.Pointer
	ok, _, callErr = recoveryGetACE.Call(uintptr(acl), 0, uintptr(unsafe.Pointer(&raw)))
	if ok == 0 || raw == nil {
		return fmt.Errorf("cannot inspect recovery access entry: %w", callErr)
	}
	ace := (*recoveryACE)(raw)
	if ace.kind != 0 || ace.flags&0x18 != 0 || ace.size < 16 || ace.mask&recoveryFileAllAccess != recoveryFileAllAccess {
		return fmt.Errorf("recovery access entry is not a private full-access grant")
	}
	sid := (*syscall.SID)(unsafe.Add(raw, unsafe.Sizeof(*ace)))
	// A SID is 8 bytes followed by a count of 32-bit subauthorities.
	count := *(*uint8)(unsafe.Add(unsafe.Pointer(sid), 1))
	if 8+4*int(count) > int(ace.size)-8 {
		return fmt.Errorf("invalid recovery access entry SID")
	}
	valid, _, _ = recoveryIsValidSID.Call(uintptr(unsafe.Pointer(sid)))
	if valid == 0 {
		return fmt.Errorf("invalid recovery access entry SID")
	}
	equal, _, _ = recoveryEqualSID.Call(uintptr(unsafe.Pointer(sid)), uintptr(unsafe.Pointer(user.User.Sid)))
	runtime.KeepAlive(user)
	if equal == 0 {
		return fmt.Errorf("recovery ACL grants access to another account")
	}
	return nil
}
