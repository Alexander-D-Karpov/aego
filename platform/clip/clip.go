package clip

import (
	"bytes"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"aego/platform"
)

const (
	MimePNG       = "image/png"
	MimeBMP       = "image/bmp"
	MimeXBMP      = "image/x-bmp"
	MimeURIList   = "text/uri-list"
	MimeGnome     = "x-special/gnome-copied-files"
	MimeText      = "text/plain;charset=utf-8"
	MimeTextPlain = "text/plain"
	maxTextPaths  = 64
	maxPathText   = 1 << 16
)

type Item = platform.ClipItem

type Reader interface {
	Text() string
	Data(mime string) ([]byte, bool)
}

type Writer interface {
	SetText(s string)
	SetData(mime string, data []byte, text string) bool
}

type Prober interface {
	Has(mime string) bool
}

type MultiWriter interface {
	SetItems(items []Item) bool
}

var imageMimes = [...]string{MimePNG, MimeBMP, MimeXBMP}

func Has(r Reader, mime string) bool {
	if r == nil {
		return false
	}
	if p, ok := r.(Prober); ok {
		return p.Has(mime)
	}
	data, ok := r.Data(mime)
	return ok && len(data) > 0
}

func maybe(r Reader, mime string) bool {
	if p, ok := r.(Prober); ok {
		return p.Has(mime)
	}
	return true
}

func HasImage(r Reader) bool {
	for _, m := range imageMimes {
		if Has(r, m) {
			return true
		}
	}
	return false
}

func HasFiles(r Reader) bool {
	return Has(r, MimeURIList) || Has(r, MimeGnome)
}

func DecodeImage(data []byte) (*image.NRGBA, error) {
	if len(data) >= 2 && data[0] == 'B' && data[1] == 'M' {
		return DecodeBMP(data)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return ToNRGBA(img), nil
}

func ToNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok && n.Rect.Min == (image.Point{}) {
		return n
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Rect, img, b.Min, draw.Src)
	return out
}

func EncodePNG(img image.Image) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil
	}
	return buf.Bytes()
}

func Image(r Reader) (*image.NRGBA, bool) {
	if r == nil {
		return nil, false
	}
	for _, m := range imageMimes {
		if !maybe(r, m) {
			continue
		}
		data, ok := r.Data(m)
		if !ok || len(data) == 0 {
			continue
		}
		if img, err := DecodeImage(data); err == nil {
			return img, true
		}
	}
	return nil, false
}

func Files(r Reader) ([]string, bool) {
	if r == nil {
		return nil, false
	}
	for _, m := range [...]string{MimeURIList, MimeGnome} {
		if !maybe(r, m) {
			continue
		}
		if data, ok := r.Data(m); ok {
			if files := ParseURIList(data); len(files) > 0 {
				return files, true
			}
		}
	}
	return textPaths(r.Text())
}

func textPaths(s string) ([]string, bool) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > maxPathText {
		return nil, false
	}
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.Trim(strings.TrimSpace(line), `"'`)
		if line == "" {
			continue
		}
		if p, ok := fromURI(line); ok {
			line = p
		}
		if !filepath.IsAbs(line) || len(out) == maxTextPaths {
			return nil, false
		}
		if _, err := os.Stat(line); err != nil {
			return nil, false
		}
		out = append(out, filepath.Clean(line))
	}
	return out, len(out) > 0
}

func fromURI(s string) (string, bool) {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "file" || (u.Host != "" && u.Host != "localhost") {
		return "", false
	}
	p := u.Path
	if runtime.GOOS == "windows" && len(p) > 2 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	if p == "" {
		return "", false
	}
	return filepath.FromSlash(p), true
}

func toURI(p string) string {
	s := filepath.ToSlash(p)
	if !strings.HasPrefix(s, "/") {
		s = "/" + s
	}
	return (&url.URL{Scheme: "file", Path: s}).String()
}

func ParseURIList(data []byte) []string {
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		if p, ok := fromURI(line); ok {
			out = append(out, p)
		}
	}
	return out
}

func URIList(paths []string) []byte {
	var b strings.Builder
	for _, p := range paths {
		b.WriteString(toURI(p))
		b.WriteString("\r\n")
	}
	return []byte(b.String())
}

func GnomeFiles(paths []string) []byte {
	parts := []string{"copy"}
	for _, p := range paths {
		parts = append(parts, toURI(p))
	}
	return []byte(strings.Join(parts, "\n"))
}

func fileItems(paths []string) []Item {
	if len(paths) == 0 {
		return nil
	}
	text := []byte(strings.Join(paths, "\n"))
	return []Item{
		{Mime: MimeURIList, Data: URIList(paths)},
		{Mime: MimeGnome, Data: GnomeFiles(paths)},
		{Mime: MimeText, Data: text},
		{Mime: MimeTextPlain, Data: text},
	}
}

func put(w Writer, items []Item, text string) bool {
	if w == nil || len(items) == 0 {
		return false
	}
	if m, ok := w.(MultiWriter); ok {
		return m.SetItems(items)
	}
	return w.SetData(items[0].Mime, items[0].Data, text)
}

func SetFiles(w Writer, paths []string) bool {
	return put(w, fileItems(paths), strings.Join(paths, "\n"))
}

func SetImagePNG(w Writer, data []byte, paths []string) bool {
	if len(data) == 0 {
		return false
	}
	items := append([]Item{{Mime: MimePNG, Data: data}}, fileItems(paths)...)
	return put(w, items, strings.Join(paths, "\n"))
}

func SetImage(w Writer, img image.Image, paths []string) bool {
	return SetImagePNG(w, EncodePNG(img), paths)
}
