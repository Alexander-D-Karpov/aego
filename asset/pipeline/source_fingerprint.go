package pipeline

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// MaxSourceFingerprintBytes bounds a single optional streaming fingerprint.
// It does not raise the existing source transaction, importer or recovery
// limits, nor does it permit a caller to write the fingerprinted source.
const MaxSourceFingerprintBytes int64 = 256 << 20

// SourceFingerprint distinguishes a missing path from an existing empty file.
// It describes one observed byte sequence, not a lock or an atomic CAS.
type SourceFingerprint struct {
	Exists bool
	Size   int64
	SHA256 [sha256.Size]byte
}

// FingerprintSource streams a bounded project source without loading it into
// memory. It refuses pending recovery and symlinks, and makes best-effort
// identity and metadata checks for changes during the read. Those checks cannot
// detect every ABA or same-size rewrite with restored timestamps. Other
// processes do not share this mutex: a caller must still arrange a guarded
// publication and recheck all related inputs. Cancellation is cooperative and
// cannot interrupt a file read or mutex wait.
func (p *Pipeline) FingerprintSource(ctx context.Context, rel string, limit int64) (SourceFingerprint, error) {
	if p == nil || ctx == nil || limit < 0 || limit > MaxSourceFingerprintBytes {
		return SourceFingerprint{}, errors.New("source fingerprint needs a context and a bounded limit")
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if err := p.requireNoPendingSourceRecovery(); err != nil {
		return SourceFingerprint{}, err
	}
	root, err := os.OpenRoot(p.root)
	if err != nil {
		return SourceFingerprint{}, err
	}
	defer root.Close()
	return p.fingerprintSource(ctx, root, rel, limit, nil)
}

// The test checkpoint runs only after the stream has been read, allowing tests
// to exercise replacement detection deterministically; production passes nil.
func (p *Pipeline) fingerprintSource(ctx context.Context, root *os.Root, rel string, limit int64, checkpoint func()) (SourceFingerprint, error) {
	if ctx == nil || root == nil || limit < 0 || limit > MaxSourceFingerprintBytes {
		return SourceFingerprint{}, errors.New("source fingerprint needs a context and a bounded limit")
	}
	if err := ctx.Err(); err != nil {
		return SourceFingerprint{}, err
	}
	if !fs.ValidPath(rel) || !filepath.IsLocal(rel) || strings.ContainsAny(rel, "\\:#") || !p.inRoots(rel) {
		return SourceFingerprint{}, fmt.Errorf("not a project source path: %s", rel)
	}
	parts := strings.Split(rel, "/")
	infos := make([]os.FileInfo, 0, len(parts))
	current := ""
	for i, part := range parts {
		if strings.HasPrefix(part, ".") {
			return SourceFingerprint{}, fmt.Errorf("hidden asset path: %s", rel)
		}
		current = path.Join(current, part)
		info, err := root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return SourceFingerprint{}, nil
		}
		if err != nil {
			return SourceFingerprint{}, err
		}
		if info.Mode()&os.ModeSymlink != 0 || (i < len(parts)-1 && !info.IsDir()) || (i == len(parts)-1 && !info.Mode().IsRegular()) {
			return SourceFingerprint{}, fmt.Errorf("unsafe source path: %s", rel)
		}
		infos = append(infos, info)
	}
	initial := infos[len(infos)-1]
	if initial.Size() > limit {
		return SourceFingerprint{}, fmt.Errorf("%s exceeds %d bytes", rel, limit)
	}
	f, err := root.Open(rel)
	if err != nil {
		return SourceFingerprint{}, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !sameFingerprintFile(initial, opened) {
		return SourceFingerprint{}, fmt.Errorf("%s changed before fingerprinting", rel)
	}
	h := sha256.New()
	buf := make([]byte, 64<<10)
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return SourceFingerprint{}, err
		}
		n, readErr := f.Read(buf)
		if n > 0 {
			size += int64(n)
			if size > limit {
				return SourceFingerprint{}, fmt.Errorf("%s exceeds %d bytes", rel, limit)
			}
			_, _ = h.Write(buf[:n])
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return SourceFingerprint{}, readErr
		}
		if n == 0 {
			return SourceFingerprint{}, io.ErrNoProgress
		}
	}
	if checkpoint != nil {
		checkpoint()
	}
	if err := ctx.Err(); err != nil {
		return SourceFingerprint{}, err
	}
	after, err := f.Stat()
	if err != nil || size != initial.Size() || !sameFingerprintFile(initial, after) {
		return SourceFingerprint{}, fmt.Errorf("%s changed while fingerprinting", rel)
	}
	current = ""
	for i, part := range parts {
		current = path.Join(current, part)
		info, err := root.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !os.SameFile(infos[i], info) || infos[i].Mode() != info.Mode() {
			return SourceFingerprint{}, fmt.Errorf("%s changed while fingerprinting", rel)
		}
		if i == len(parts)-1 && !sameFingerprintFile(initial, info) {
			return SourceFingerprint{}, fmt.Errorf("%s changed while fingerprinting", rel)
		}
	}
	if err := ctx.Err(); err != nil {
		return SourceFingerprint{}, err
	}
	if err := f.Close(); err != nil {
		return SourceFingerprint{}, err
	}
	result := SourceFingerprint{Exists: true, Size: size}
	copy(result.SHA256[:], h.Sum(nil))
	return result, nil
}

func sameFingerprintFile(a, b os.FileInfo) bool {
	return a != nil && b != nil && a.Mode().IsRegular() && b.Mode().IsRegular() && os.SameFile(a, b) &&
		a.Mode() == b.Mode() && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}
