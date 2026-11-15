package remoteexample

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
)

// BuildPackage creates the existing, data-only ZIP/manifest distribution from
// explicit source bytes. It never reads a checkout implicitly and cannot put
// engine code, git state, native libraries or files outside this map in a ZIP.
// Files are sorted and stored without timestamps, so identical source bytes
// produce the same archive hash on every platform. Compression is unnecessary
// for the small examples and avoids publishing expansion-ratio bombs.
func BuildPackage(m Manifest, files map[string][]byte, manifestURL string, limits Limits) (Manifest, []byte, []byte, error) {
	l := normalizeLimits(limits)
	if len(files) == 0 || len(files) > l.Files {
		return m, nil, nil, fmt.Errorf("file count exceeds limits")
	}
	names := make([]string, 0, len(files))
	var total int64
	for name, data := range files {
		if err := validateResourcePath(name); err != nil {
			return m, nil, nil, err
		}
		size := int64(len(data))
		if size > l.FileBytes || size > l.ExpandedBytes-total {
			return m, nil, nil, fmt.Errorf("source resource budget exceeded: %s", name)
		}
		total += size
		names = append(names, name)
	}
	slices.Sort(names)
	m.Files = nil
	for _, name := range names {
		hash := sha256.Sum256(files[name])
		m.Files = append(m.Files, File{Path: name, Size: int64(len(files[name])), SHA256: hex.EncodeToString(hash[:])})
	}
	// Validate metadata before allocating the archive. The real digest and
	// length are filled after deterministic ZIP serialization.
	m.Archive.Size, m.Archive.SHA256 = 1, fmt.Sprintf("%064x", 0)
	if err := m.Validate(manifestURL, l); err != nil {
		return m, nil, nil, err
	}
	var out bytes.Buffer
	z := zip.NewWriter(&out)
	for _, name := range names {
		f, err := z.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			return m, nil, nil, err
		}
		if _, err := f.Write(files[name]); err != nil {
			return m, nil, nil, err
		}
		if int64(out.Len()) > l.ArchiveBytes {
			return m, nil, nil, fmt.Errorf("archive exceeds size limit")
		}
	}
	if err := z.Close(); err != nil {
		return m, nil, nil, err
	}
	hash := sha256.Sum256(out.Bytes())
	m.Archive.Size, m.Archive.SHA256 = int64(out.Len()), hex.EncodeToString(hash[:])
	if err := m.Validate(manifestURL, l); err != nil {
		return m, nil, nil, err
	}
	manifest, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return m, nil, nil, err
	}
	manifest = append(manifest, '\n')
	if int64(len(manifest)) > l.ManifestBytes {
		return m, nil, nil, fmt.Errorf("manifest exceeds size limit")
	}
	return m, manifest, out.Bytes(), nil
}
