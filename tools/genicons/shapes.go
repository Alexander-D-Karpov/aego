package main

import (
	"math"
	"strconv"
	"strings"
)

func ff(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}

func area(xy []float64) float64 {
	n := len(xy) / 2
	var s float64
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		s += xy[2*i]*xy[2*j+1] - xy[2*j]*xy[2*i+1]
	}
	return s
}

func poly(hole bool, xy ...float64) string {
	n := len(xy) / 2
	if n < 3 {
		return ""
	}
	reverse := (area(xy) < 0) != hole
	var b strings.Builder
	for i := 0; i < n; i++ {
		k := i
		if reverse {
			k = n - 1 - i
		}
		if i == 0 {
			b.WriteString("M")
		} else {
			b.WriteString(" L")
		}
		b.WriteString(ff(xy[2*k]) + " " + ff(xy[2*k+1]))
	}
	b.WriteString(" Z ")
	return b.String()
}

func rect(x, y, w, h float64, hole bool) string {
	return poly(hole, x, y, x+w, y, x+w, y+h, x, y+h)
}

func circle(cx, cy, r float64, hole bool) string {
	k := 0.5523 * r
	p := func(x, y float64) string { return ff(x) + " " + ff(y) }
	if !hole {
		return "M" + p(cx+r, cy) +
			" C" + p(cx+r, cy+k) + " " + p(cx+k, cy+r) + " " + p(cx, cy+r) +
			" C" + p(cx-k, cy+r) + " " + p(cx-r, cy+k) + " " + p(cx-r, cy) +
			" C" + p(cx-r, cy-k) + " " + p(cx-k, cy-r) + " " + p(cx, cy-r) +
			" C" + p(cx+k, cy-r) + " " + p(cx+r, cy-k) + " " + p(cx+r, cy) + " Z "
	}
	return "M" + p(cx+r, cy) +
		" C" + p(cx+r, cy-k) + " " + p(cx+k, cy-r) + " " + p(cx, cy-r) +
		" C" + p(cx-k, cy-r) + " " + p(cx-r, cy-k) + " " + p(cx-r, cy) +
		" C" + p(cx-r, cy+k) + " " + p(cx-k, cy+r) + " " + p(cx, cy+r) +
		" C" + p(cx+k, cy+r) + " " + p(cx+r, cy+k) + " " + p(cx+r, cy) + " Z "
}

func ring(cx, cy, r, w float64) string {
	return circle(cx, cy, r, false) + circle(cx, cy, r-w, true)
}

func bar(x1, y1, x2, y2, w float64) string {
	dx, dy := x2-x1, y2-y1
	l := math.Hypot(dx, dy)
	nx, ny := -dy/l*w/2, dx/l*w/2
	return poly(false, x1+nx, y1+ny, x2+nx, y2+ny, x2-nx, y2-ny, x1-nx, y1-ny)
}

func doc(paths ...string) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="` + strings.TrimSpace(strings.Join(paths, "")) + `"/></svg>` + "\n"
}

func placeholders() map[string]string {
	move := bar(12, 5, 12, 19, 2) + bar(5, 12, 19, 12, 2) +
		poly(false, 12, 1.5, 8.5, 5.5, 15.5, 5.5) + poly(false, 12, 22.5, 15.5, 18.5, 8.5, 18.5) +
		poly(false, 1.5, 12, 5.5, 8.5, 5.5, 15.5) + poly(false, 22.5, 12, 18.5, 15.5, 18.5, 8.5)
	script := poly(false, 9, 4, 10.5, 5.5, 8, 12, 10.5, 18.5, 9, 20, 6, 12) +
		poly(false, 15, 4, 18, 12, 15, 20, 13.5, 18.5, 16, 12, 13.5, 5.5)
	frame := rect(3, 4, 18, 16, false) + rect(5, 6, 14, 12, true)
	mountain := poly(false, 7, 16, 11, 10, 15, 16)
	search := ring(10, 10, 6, 2) + bar(14.2, 14.2, 20, 20, 2.6)
	return map[string]string{
		"tool.select":      doc(poly(false, 5, 3, 5, 19, 9.2, 15.2, 12, 21, 14.4, 20, 11.6, 14.2, 17, 14.2)),
		"tool.move":        doc(move),
		"tool.rotate":      doc(ring(12, 12, 8, 2.4), poly(false, 17, 2.5, 22, 7.5, 15, 8.5)),
		"tool.scale":       doc(rect(3, 9, 12, 12, false), rect(5, 11, 8, 8, true), poly(false, 13, 3, 21, 3, 21, 11), bar(10, 14, 19, 5, 2)),
		"view.grid":        doc(rect(4, 4, 7, 7, false), rect(13, 4, 7, 7, false), rect(4, 13, 7, 7, false), rect(13, 13, 7, 7, false)),
		"view.snap":        doc(rect(5, 4, 4, 10, false), rect(15, 4, 4, 10, false), rect(5, 14, 14, 5, false)),
		"view.zoom":        doc(search),
		"play.start":       doc(poly(false, 7, 4, 7, 20, 20, 12)),
		"play.pause":       doc(rect(6, 4, 4, 16, false), rect(14, 4, 4, 16, false)),
		"play.step":        doc(poly(false, 5, 4, 5, 20, 15, 12), rect(16, 4, 3, 16, false)),
		"play.stop":        doc(rect(5, 5, 14, 14, false)),
		"file.save":        doc(rect(4, 4, 16, 16, false), rect(7, 4.5, 9, 5, true), rect(7, 12, 10, 6, true)),
		"file.folder":      doc(poly(false, 3, 6, 10, 6, 12, 8, 21, 8, 21, 19, 3, 19)),
		"file.scene":       doc(frame, mountain),
		"file.texture":     doc(frame, circle(9, 9, 1.8, false), poly(false, 6, 17, 11, 11, 14, 14, 16, 12, 18, 17)),
		"file.script":      doc(script),
		"file.prefab":      doc(poly(false, 12, 3, 20, 7.5, 20, 16.5, 12, 21, 4, 16.5, 4, 7.5)),
		"cat.node":         doc(ring(12, 12, 7.5, 2.2), circle(12, 12, 2.6, false)),
		"cat.transform":    doc(move),
		"cat.rendering":    doc(frame, mountain),
		"cat.camera":       doc(rect(3, 7, 13, 11, false), poly(false, 16, 10.5, 21, 7, 21, 18, 16, 14.5)),
		"cat.scripts":      doc(script),
		"cat.physics":      doc(circle(12, 8, 4.5, false), rect(4, 16, 16, 3, false)),
		"cat.audio":        doc(poly(false, 3, 9, 7, 9, 12, 4, 12, 20, 7, 15, 3, 15), rect(15, 9, 2, 6, false), rect(19, 6, 2, 12, false)),
		"cat.game":         doc(rect(2, 7, 20, 10, false), rect(5, 11, 5, 2, true), circle(16, 10.5, 1.3, true), circle(18.5, 13.5, 1.3, true)),
		"ui.chevron_down":  doc(poly(false, 6, 9, 18, 9, 12, 16)),
		"ui.chevron_right": doc(poly(false, 9, 6, 16, 12, 9, 18)),
		"ui.close":         doc(bar(6, 6, 18, 18, 2.6), bar(18, 6, 6, 18, 2.6)),
		"ui.search":        doc(search),
		"ui.more":          doc(circle(5, 12, 2, false), circle(12, 12, 2, false), circle(19, 12, 2, false)),
		"ui.plus":          doc(rect(10.75, 5, 2.5, 14, false), rect(5, 10.75, 14, 2.5, false)),
		"ui.minus":         doc(rect(5, 10.75, 14, 2.5, false)),
		"ui.reset":         doc(ring(12, 13, 7, 2.2), poly(false, 8, 3, 14, 6, 8, 9)),
		"log.info":         doc(ring(12, 12, 9, 2), rect(11, 10.5, 2, 6.5, false), circle(12, 7.5, 1.3, false)),
		"log.warn":         doc(poly(false, 12, 3, 22, 20, 2, 20), rect(11, 9, 2, 6, true), rect(11, 16, 2, 2, true)),
		"log.error":        doc(ring(12, 12, 9, 2), bar(8.5, 8.5, 15.5, 15.5, 2), bar(15.5, 8.5, 8.5, 15.5, 2)),
		"build.ok":         doc(poly(false, 4, 12.5, 7, 9.5, 10, 12.5, 17, 5.5, 20, 8.5, 10, 18.5)),
		"build.busy":       doc(ring(12, 12, 8, 2.5), rect(11, 2, 2, 6, false)),
		"build.error":      doc(ring(12, 12, 9, 2), rect(11, 6.5, 2, 7.5, false), rect(11, 15.5, 2, 2, false)),
	}
}
