package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/image/vector"
)

type pt struct {
	x, y float64
}

type seg struct {
	kind byte
	p    [3]pt
}

type icon struct {
	vb   [4]float64
	segs []seg
}

type iconJSON struct {
	ID   string `json:"id"`
	Size int    `json:"size"`
	X    int    `json:"x"`
	Y    int    `json:"y"`
	W    int    `json:"w"`
	H    int    `json:"h"`
}

type atlasJSON struct {
	Scale int        `json:"scale"`
	File  string     `json:"file"`
	Icons []iconJSON `json:"icons"`
}

type fileJSON struct {
	Atlases []atlasJSON `json:"atlases"`
}

func main() {
	src := flag.String("src", "../../engine/res/icons/src", "directory with <id>.svg sources")
	out := flag.String("out", "../../engine/res/icons", "output directory")
	initSrc := flag.Bool("init", false, "write placeholder svgs for ids that have no source yet")
	sizes := flag.String("sizes", "16,24", "icon sizes in logical pixels")
	scales := flag.String("scales", "1,2", "atlas scales")
	flag.Parse()

	if *initSrc {
		if err := writePlaceholders(*src); err != nil {
			fail(err)
		}
	}
	icons, err := loadDir(*src)
	if err != nil {
		fail(err)
	}
	if len(icons) == 0 {
		fail(errors.New("no svg sources found, run with -init"))
	}
	idsList := make([]string, 0, len(icons))
	for id := range icons {
		idsList = append(idsList, id)
	}
	sort.Strings(idsList)
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}
	var meta fileJSON
	for _, scale := range parseInts(*scales) {
		img, entries := bake(icons, idsList, parseInts(*sizes), scale)
		name := "icons@" + strconv.Itoa(scale) + ".png"
		if err := writePNG(filepath.Join(*out, name), img); err != nil {
			fail(err)
		}
		meta.Atlases = append(meta.Atlases, atlasJSON{Scale: scale, File: name, Icons: entries})
		fmt.Printf("wrote %s (%dx%d, %d icons)\n", name, img.Bounds().Dx(), img.Bounds().Dy(), len(idsList))
	}
	blob, err := json.Marshal(meta)
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(filepath.Join(*out, "icons.json"), append(blob, '\n'), 0o644); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "genicons:", err)
	os.Exit(1)
}

func parseInts(s string) []int {
	var out []int
	for _, part := range strings.Split(s, ",") {
		v, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || v <= 0 {
			fail(fmt.Errorf("bad number %q", part))
		}
		out = append(out, v)
	}
	return out
}

func loadDir(dir string) (map[string]icon, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.svg"))
	if err != nil {
		return nil, err
	}
	out := map[string]icon{}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		ic, err := parseSVG(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(f), err)
		}
		out[strings.TrimSuffix(filepath.Base(f), ".svg")] = ic
	}
	return out, nil
}

func parseSVG(data []byte) (icon, error) {
	ic := icon{vb: [4]float64{0, 0, 24, 24}}
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return ic, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		attr := func(n string) string {
			for _, a := range se.Attr {
				if a.Name.Local == n {
					return a.Value
				}
			}
			return ""
		}
		num := func(n string) float64 {
			v, _ := strconv.ParseFloat(strings.TrimSpace(attr(n)), 64)
			return v
		}
		switch se.Name.Local {
		case "svg":
			if vb := strings.Fields(strings.ReplaceAll(attr("viewBox"), ",", " ")); len(vb) == 4 {
				for i := range vb {
					ic.vb[i], _ = strconv.ParseFloat(vb[i], 64)
				}
			}
		case "path":
			segs, err := parsePath(attr("d"))
			if err != nil {
				return ic, err
			}
			ic.segs = append(ic.segs, segs...)
		case "rect":
			segs, _ := parsePath(rect(num("x"), num("y"), num("width"), num("height"), false))
			ic.segs = append(ic.segs, segs...)
		case "circle":
			segs, _ := parsePath(circle(num("cx"), num("cy"), num("r"), false))
			ic.segs = append(ic.segs, segs...)
		case "polygon":
			var xy []float64
			for _, f := range strings.Fields(strings.ReplaceAll(attr("points"), ",", " ")) {
				v, err := strconv.ParseFloat(f, 64)
				if err != nil {
					return ic, err
				}
				xy = append(xy, v)
			}
			segs, err := parsePath(poly(false, xy...))
			if err != nil {
				return ic, err
			}
			ic.segs = append(ic.segs, segs...)
		}
	}
	return ic, nil
}

func isCommand(ch byte) bool {
	return strings.IndexByte("MmLlHhVvCcQqZz", ch) >= 0
}

func parsePath(d string) ([]seg, error) {
	var out []seg
	var cur, start pt
	var cmd byte
	i := 0
	skip := func() {
		for i < len(d) && strings.IndexByte(" ,\t\n\r", d[i]) >= 0 {
			i++
		}
	}
	num := func() (float64, error) {
		skip()
		j := i
		if j < len(d) && (d[j] == '-' || d[j] == '+') {
			j++
		}
		dot := false
		for j < len(d) {
			ch := d[j]
			switch {
			case ch >= '0' && ch <= '9':
				j++
			case ch == '.' && !dot:
				dot = true
				j++
			case ch == 'e' || ch == 'E':
				j++
				if j < len(d) && (d[j] == '-' || d[j] == '+') {
					j++
				}
			default:
				goto done
			}
		}
	done:
		if j == i {
			return 0, fmt.Errorf("expected a number at %d", i)
		}
		v, err := strconv.ParseFloat(d[i:j], 64)
		i = j
		return v, err
	}
	point := func(rel bool) (pt, error) {
		x, err := num()
		if err != nil {
			return pt{}, err
		}
		y, err := num()
		if err != nil {
			return pt{}, err
		}
		if rel {
			return pt{cur.x + x, cur.y + y}, nil
		}
		return pt{x, y}, nil
	}
	for {
		skip()
		if i >= len(d) {
			break
		}
		if isCommand(d[i]) {
			cmd = d[i]
			i++
		} else if cmd == 0 {
			return nil, fmt.Errorf("path must start with a command")
		}
		rel := cmd >= 'a'
		switch cmd | 0x20 {
		case 'm':
			p, err := point(rel)
			if err != nil {
				return nil, err
			}
			out = append(out, seg{kind: 'M', p: [3]pt{p}})
			cur, start = p, p
			if rel {
				cmd = 'l'
			} else {
				cmd = 'L'
			}
		case 'l':
			p, err := point(rel)
			if err != nil {
				return nil, err
			}
			out = append(out, seg{kind: 'L', p: [3]pt{p}})
			cur = p
		case 'h':
			x, err := num()
			if err != nil {
				return nil, err
			}
			if rel {
				x += cur.x
			}
			cur = pt{x, cur.y}
			out = append(out, seg{kind: 'L', p: [3]pt{cur}})
		case 'v':
			y, err := num()
			if err != nil {
				return nil, err
			}
			if rel {
				y += cur.y
			}
			cur = pt{cur.x, y}
			out = append(out, seg{kind: 'L', p: [3]pt{cur}})
		case 'q':
			c, err := point(rel)
			if err != nil {
				return nil, err
			}
			p, err := point(rel)
			if err != nil {
				return nil, err
			}
			out = append(out, seg{kind: 'Q', p: [3]pt{c, p}})
			cur = p
		case 'c':
			c1, err := point(rel)
			if err != nil {
				return nil, err
			}
			c2, err := point(rel)
			if err != nil {
				return nil, err
			}
			p, err := point(rel)
			if err != nil {
				return nil, err
			}
			out = append(out, seg{kind: 'C', p: [3]pt{c1, c2, p}})
			cur = p
		case 'z':
			out = append(out, seg{kind: 'Z'})
			cur = start
			cmd = 0
		default:
			return nil, fmt.Errorf("unsupported path command %q", cmd)
		}
	}
	return out, nil
}

func raster(ic icon, px int) *image.Alpha {
	r := vector.NewRasterizer(px, px)
	sx := float64(px) / ic.vb[2]
	sy := float64(px) / ic.vb[3]
	tr := func(p pt) (float32, float32) {
		return float32((p.x - ic.vb[0]) * sx), float32((p.y - ic.vb[1]) * sy)
	}
	open := false
	for _, s := range ic.segs {
		switch s.kind {
		case 'M':
			if open {
				r.ClosePath()
			}
			x, y := tr(s.p[0])
			r.MoveTo(x, y)
			open = true
		case 'L':
			x, y := tr(s.p[0])
			r.LineTo(x, y)
		case 'Q':
			cx, cy := tr(s.p[0])
			x, y := tr(s.p[1])
			r.QuadTo(cx, cy, x, y)
		case 'C':
			ax, ay := tr(s.p[0])
			bx, by := tr(s.p[1])
			x, y := tr(s.p[2])
			r.CubeTo(ax, ay, bx, by, x, y)
		case 'Z':
			if open {
				r.ClosePath()
				open = false
			}
		}
	}
	if open {
		r.ClosePath()
	}
	dst := image.NewAlpha(image.Rect(0, 0, px, px))
	r.Draw(dst, dst.Bounds(), image.Opaque, image.Point{})
	return dst
}

type placement struct {
	id   string
	size int
	px   int
	x, y int
}

func bake(icons map[string]icon, ids []string, sizes []int, scale int) (*image.NRGBA, []iconJSON) {
	const pad = 1
	maxCell := sizes[len(sizes)-1]*scale + 2*pad
	width := nextPow2(maxCell * 16)
	var places []placement
	x, y, rowH := pad, pad, 0
	for _, size := range sizes {
		px := size * scale
		for _, id := range ids {
			if x+px+pad > width {
				x = pad
				y += rowH
				rowH = 0
			}
			places = append(places, placement{id: id, size: size, px: px, x: x, y: y})
			x += px + 2*pad
			rowH = max(rowH, px+2*pad)
		}
		x = pad
		y += rowH
		rowH = 0
	}
	height := nextPow2(y + pad)
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	entries := make([]iconJSON, 0, len(places))
	for _, p := range places {
		a := raster(icons[p.id], p.px)
		for yy := 0; yy < p.px; yy++ {
			for xx := 0; xx < p.px; xx++ {
				if v := a.AlphaAt(xx, yy).A; v > 0 {
					img.SetNRGBA(p.x+xx, p.y+yy, color.NRGBA{R: 255, G: 255, B: 255, A: v})
				}
			}
		}
		entries = append(entries, iconJSON{ID: p.id, Size: p.size, X: p.x, Y: p.y, W: p.px, H: p.px})
	}
	return img, entries
}

func nextPow2(v int) int {
	n := 1
	for n < v {
		n <<= 1
	}
	return n
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func writePlaceholders(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	written := 0
	for id, svg := range placeholders() {
		path := filepath.Join(dir, id+".svg")
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if err := os.WriteFile(path, []byte(svg), 0o644); err != nil {
			return err
		}
		written++
	}
	fmt.Printf("wrote %d placeholder svg files to %s\n", written, dir)
	return nil
}
