package remoteexample

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

// Go's synthesized Windows FileMode is not an ACL and reports writable
// directories as 0777. Inspect real NTFS security descriptors instead. This
// deliberately rejects unknown ACE forms and any untrusted write grant even
// when a deny ACE might make its effective access narrower.
var (
	securityDLL       = syscall.NewLazyDLL("advapi32.dll")
	getSecurityInfo   = securityDLL.NewProc("GetSecurityInfo")
	getACLInformation = securityDLL.NewProc("GetAclInformation")
	getACE            = securityDLL.NewProc("GetAce")
	isValidACL        = securityDLL.NewProc("IsValidAcl")
	isValidSID        = securityDLL.NewProc("IsValidSid")
	equalSID          = securityDLL.NewProc("EqualSid")
)

const (
	readControl          = 0x00020000
	ownerSecurityInfo    = 0x00000001
	daclSecurityInfo     = 0x00000004
	seFileObject         = 1
	aclSizeInformation   = 2
	accessAllowedACE     = 0
	accessDeniedACE      = 1
	inheritOnlyACE       = 0x08
	writeData            = 0x00000002
	appendData           = 0x00000004
	writeEA              = 0x00000010
	deleteChild          = 0x00000040
	writeAttributes      = 0x00000100
	deleteAccess         = 0x00010000
	writeDAC             = 0x00040000
	writeOwner           = 0x00080000
	maximumAllowed       = 0x02000000
	unsafeGenericAccess  = syscall.GENERIC_ALL | syscall.GENERIC_WRITE | maximumAllowed
	unsafeStoreAccess    = writeData | appendData | writeEA | deleteChild | writeAttributes | deleteAccess | writeDAC | writeOwner | unsafeGenericAccess
	unsafeAncestorAccess = deleteChild | writeAttributes | deleteAccess | writeDAC | writeOwner | unsafeGenericAccess
)

type aclSize struct{ aceCount, bytesInUse, bytesFree uint32 }
type aceHeader struct {
	kind, flags uint8
	size        uint16
}
type simpleACE struct {
	header aceHeader
	mask   uint32
}

func rejectReparse(name string) error {
	p, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	attributes, err := syscall.GetFileAttributes(p)
	if err != nil {
		return err
	}
	if attributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("reparse point rejected: %s", name)
	}
	return nil
}

type trustedSIDs struct {
	user, system, administrators, trustedInstaller, creatorOwner, ownerRights *syscall.SID
	tokenUser                                                                 *syscall.Tokenuser
}

func storeTrustedSIDs() (trustedSIDs, error) {
	var trusted trustedSIDs
	token, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return trusted, err
	}
	defer token.Close()
	trusted.tokenUser, err = token.GetTokenUser()
	if err != nil || trusted.tokenUser == nil || trusted.tokenUser.User.Sid == nil {
		return trusted, fmt.Errorf("cannot inspect current Windows user: %v", err)
	}
	trusted.user = trusted.tokenUser.User.Sid
	for _, item := range []struct {
		target **syscall.SID
		value  string
	}{
		{&trusted.system, "S-1-5-18"},
		{&trusted.administrators, "S-1-5-32-544"},
		// The Windows Modules Installer owns the system volume root on normal
		// installations. This exact service SID is not an arbitrary service.
		{&trusted.trustedInstaller, "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"},
		{&trusted.creatorOwner, "S-1-3-0"},
		{&trusted.ownerRights, "S-1-3-4"},
	} {
		*item.target, err = syscall.StringToSid(item.value)
		if err != nil {
			return trusted, err
		}
	}
	return trusted, nil
}

func sameSID(a, b *syscall.SID) bool {
	if a == nil || b == nil {
		return false
	}
	r, _, _ := equalSID.Call(uintptr(unsafe.Pointer(a)), uintptr(unsafe.Pointer(b)))
	return r != 0
}

func (t trustedSIDs) owner(sid *syscall.SID) bool {
	return sameSID(sid, t.user) || sameSID(sid, t.system) || sameSID(sid, t.administrators) || sameSID(sid, t.trustedInstaller)
}

func secureStoreDirectory(name string, _ os.FileInfo) error {
	trusted, err := storeTrustedSIDs()
	if err != nil {
		return err
	}
	// Check ancestors first: DELETE_CHILD or DACL/owner modification on a
	// parent can replace an otherwise private store directory. The final store
	// additionally forbids grants that could create or modify its children.
	paths := []string{name}
	for parent := filepath.Dir(name); parent != paths[len(paths)-1]; parent = filepath.Dir(parent) {
		paths = append(paths, parent)
	}
	for i := len(paths) - 1; i >= 0; i-- {
		if err := inspectACL(paths[i], trusted, i == 0, true); err != nil {
			return err
		}
	}
	runtime.KeepAlive(trusted.tokenUser)
	return nil
}

func newSecureTreeChecker() (func(string, os.FileInfo) error, error) {
	trusted, err := storeTrustedSIDs()
	if err != nil {
		return nil, err
	}
	return func(name string, info os.FileInfo) error {
		return inspectACL(name, trusted, true, info.IsDir())
	}, nil
}

func inspectACL(name string, trusted trustedSIDs, store, directory bool) error {
	p, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	h, err := syscall.CreateFile(p, readControl, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil,
		syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS|syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return fmt.Errorf("cannot inspect example store ACL for %s: %w", name, err)
	}
	defer syscall.CloseHandle(h)
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(h, &info); err != nil {
		return err
	}
	isDir := info.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY != 0
	if info.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 || isDir != directory {
		return fmt.Errorf("example store path is a reparse point or changed type: %s", name)
	}
	var owner *syscall.SID
	var acl, descriptor unsafe.Pointer
	code, _, _ := getSecurityInfo.Call(uintptr(h), seFileObject, ownerSecurityInfo|daclSecurityInfo,
		uintptr(unsafe.Pointer(&owner)), 0, uintptr(unsafe.Pointer(&acl)), 0, uintptr(unsafe.Pointer(&descriptor)))
	if code != 0 {
		return fmt.Errorf("cannot read example store ACL for %s: %w", name, syscall.Errno(code))
	}
	if descriptor != nil {
		defer syscall.LocalFree(syscall.Handle(uintptr(descriptor)))
	}
	if owner == nil || acl == nil || descriptor == nil {
		return fmt.Errorf("example store has a missing owner or null DACL: %s", name)
	}
	valid, _, _ := isValidSID.Call(uintptr(unsafe.Pointer(owner)))
	if valid == 0 || !trusted.owner(owner) {
		who := "invalid SID"
		if valid != 0 {
			who, _ = owner.String()
		}
		return fmt.Errorf("example store has an untrusted owner %s: %s", who, name)
	}
	valid, _, _ = isValidACL.Call(uintptr(acl))
	if valid == 0 {
		return fmt.Errorf("example store has an invalid DACL: %s", name)
	}
	var size aclSize
	ok, _, callErr := getACLInformation.Call(uintptr(acl), uintptr(unsafe.Pointer(&size)), unsafe.Sizeof(size), aclSizeInformation)
	if ok == 0 || size.aceCount == 0 || size.aceCount > 65535 {
		return fmt.Errorf("cannot inspect example store DACL for %s: %v", name, callErr)
	}
	mask := uint32(unsafeAncestorAccess)
	if store {
		mask = uint32(unsafeStoreAccess)
	}
	for i := uint32(0); i < size.aceCount; i++ {
		var raw unsafe.Pointer
		ok, _, callErr = getACE.Call(uintptr(acl), uintptr(i), uintptr(unsafe.Pointer(&raw)))
		if ok == 0 || raw == nil {
			return fmt.Errorf("cannot inspect example store ACE for %s: %v", name, callErr)
		}
		ace := (*simpleACE)(raw)
		switch ace.header.kind {
		case accessDeniedACE:
			continue
		case accessAllowedACE:
		default:
			return fmt.Errorf("unsupported example store ACE type %d: %s", ace.header.kind, name)
		}
		if ace.header.size < 16 {
			return fmt.Errorf("invalid example store ACE: %s", name)
		}
		sid := (*syscall.SID)(unsafe.Add(raw, 8))
		valid, _, _ = isValidSID.Call(uintptr(unsafe.Pointer(sid)))
		if valid == 0 || syscall.GetLengthSid(sid) > uint32(ace.header.size)-8 {
			return fmt.Errorf("invalid example store ACE SID: %s", name)
		}
		if trusted.owner(sid) {
			continue
		}
		if sameSID(sid, trusted.ownerRights) {
			continue // The actual owner was checked above.
		}
		if ace.header.flags&inheritOnlyACE != 0 {
			if !store || sameSID(sid, trusted.creatorOwner) {
				continue
			}
		}
		if ace.mask&mask != 0 {
			who, _ := sid.String()
			return fmt.Errorf("example store ACL grants untrusted write access to %s on %s", who, name)
		}
	}
	return nil
}
