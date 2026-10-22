package macos

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"image"
	"os"
	"path/filepath"
	"strings"

	"aego/app/export/icon"
	"aego/core/errs"
)

type DocType struct {
	Name       string
	Extensions []string
	Role       string
}

type Info struct {
	Name       string
	Executable string
	ID         string
	Version    string
	Build      string
	MinOS      string
	Category   string
	Copyright  string
	Documents  []DocType
}

var icnsTypes = []struct {
	typ  string
	size int
}{
	{"icp4", 16}, {"icp5", 32}, {"icp6", 64}, {"ic07", 128}, {"ic08", 256}, {"ic09", 512}, {"ic10", 1024},
	{"ic11", 32}, {"ic12", 64}, {"ic13", 256}, {"ic14", 512},
}

func BundleID(module string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(module) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('.')
		}
	}
	id := strings.Trim(b.String(), ".")
	if id == "" {
		id = "game"
	}
	return "dev.aego." + id
}

func esc(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

func Plist(i Info, iconFile string) []byte {
	if i.Build == "" {
		i.Build = i.Version
	}
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n")
	b.WriteString("<plist version=\"1.0\">\n<dict>\n")
	str := func(k, v string) {
		if v != "" {
			b.WriteString("\t<key>" + k + "</key>\n\t<string>" + esc(v) + "</string>\n")
		}
	}
	str("CFBundleName", i.Name)
	str("CFBundleDisplayName", i.Name)
	str("CFBundleExecutable", i.Executable)
	str("CFBundleIdentifier", i.ID)
	str("CFBundleVersion", i.Build)
	str("CFBundleShortVersionString", i.Version)
	str("CFBundlePackageType", "APPL")
	str("CFBundleInfoDictionaryVersion", "6.0")
	str("CFBundleIconFile", iconFile)
	str("LSMinimumSystemVersion", i.MinOS)
	str("LSApplicationCategoryType", i.Category)
	str("NSHumanReadableCopyright", i.Copyright)
	if len(i.Documents) > 0 {
		b.WriteString("\t<key>CFBundleDocumentTypes</key>\n\t<array>\n")
		for _, d := range i.Documents {
			role := d.Role
			if role == "" {
				role = "Editor"
			}
			b.WriteString("\t\t<dict>\n\t\t\t<key>CFBundleTypeName</key>\n\t\t\t<string>" + esc(d.Name) + "</string>\n")
			b.WriteString("\t\t\t<key>CFBundleTypeRole</key>\n\t\t\t<string>" + esc(role) + "</string>\n")
			b.WriteString("\t\t\t<key>CFBundleTypeExtensions</key>\n\t\t\t<array>\n")
			for _, e := range d.Extensions {
				b.WriteString("\t\t\t\t<string>" + esc(e) + "</string>\n")
			}
			b.WriteString("\t\t\t</array>\n\t\t</dict>\n")
		}
		b.WriteString("\t</array>\n")
	}
	b.WriteString("\t<key>NSHighResolutionCapable</key>\n\t<true/>\n")
	b.WriteString("</dict>\n</plist>\n")
	return []byte(b.String())
}

func ICNS(src image.Image) ([]byte, error) {
	cache := map[int][]byte{}
	var body bytes.Buffer
	for _, t := range icnsTypes {
		data, ok := cache[t.size]
		if !ok {
			var err error
			if data, err = icon.PNG(icon.Sized(src, t.size)); err != nil {
				return nil, err
			}
			cache[t.size] = data
		}
		var h [8]byte
		copy(h[:4], t.typ)
		binary.BigEndian.PutUint32(h[4:], uint32(8+len(data)))
		body.Write(h[:])
		body.Write(data)
	}
	out := make([]byte, 8, 8+body.Len())
	copy(out, "icns")
	binary.BigEndian.PutUint32(out[4:], uint32(8+body.Len()))
	return append(out, body.Bytes()...), nil
}

func Fill(app string, i Info, img image.Image) error {
	contents := filepath.Join(app, "Contents")
	res := filepath.Join(contents, "Resources")
	if err := os.MkdirAll(res, 0o755); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot create the app bundle")
	}
	iconFile := ""
	if img != nil {
		data, err := ICNS(img)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(res, "icon.icns"), data, 0o644); err != nil {
			return errs.Wrap(err, errs.VFSIO, "cannot write the app icon")
		}
		iconFile = "icon"
	}
	if err := os.WriteFile(filepath.Join(contents, "Info.plist"), Plist(i, iconFile), 0o644); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot write Info.plist")
	}
	if err := os.WriteFile(filepath.Join(contents, "PkgInfo"), []byte("APPL????"), 0o644); err != nil {
		return errs.Wrap(err, errs.VFSIO, "cannot write PkgInfo")
	}
	return nil
}
