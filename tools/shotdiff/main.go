// Command shotdiff compares regenerated documentation screenshots with the committed ones.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
)

func load(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func abs(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// differ returns the share of pixels whose channels move by more than 24 of 255.
func differ(a, b image.Image) (float64, error) {
	if a.Bounds().Size() != b.Bounds().Size() {
		return 1, fmt.Errorf("size %v, want %v", b.Bounds().Size(), a.Bounds().Size())
	}
	ra, rb := a.Bounds(), b.Bounds()
	bad := 0
	for y := 0; y < ra.Dy(); y++ {
		for x := 0; x < ra.Dx(); x++ {
			r1, g1, b1, _ := a.At(ra.Min.X+x, ra.Min.Y+y).RGBA()
			r2, g2, b2, _ := b.At(rb.Min.X+x, rb.Min.Y+y).RGBA()
			d := max(abs(int32(r1>>8)-int32(r2>>8)), abs(int32(g1>>8)-int32(g2>>8)), abs(int32(b1>>8)-int32(b2>>8)))
			if d > 24 {
				bad++
			}
		}
	}
	return float64(bad) / float64(ra.Dx()*ra.Dy()), nil
}

func main() {
	want := flag.String("want", "docs/shots", "committed screenshots")
	got := flag.String("got", ".cache/shots/out", "regenerated screenshots")
	limit := flag.Float64("limit", 0.03, "largest share of changed pixels")
	flag.Parse()
	names, err := filepath.Glob(filepath.Join(*want, "*.png"))
	if err != nil || len(names) == 0 {
		fmt.Fprintln(os.Stderr, "shotdiff: no committed screenshots in", *want)
		os.Exit(1)
	}
	sort.Strings(names)
	failed := false
	for _, w := range names {
		name := filepath.Base(w)
		a, err := load(w)
		if err == nil {
			var b image.Image
			if b, err = load(filepath.Join(*got, name)); err == nil {
				var share float64
				share, err = differ(a, b)
				if err == nil {
					fmt.Printf("%s: %.2f%% changed\n", name, share*100)
					if share > *limit {
						err = fmt.Errorf("%.2f%% of the pixels changed, more than %.2f%%", share*100, *limit*100)
					}
				}
			}
		}
		if err != nil {
			failed = true
			fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		}
	}
	if failed {
		fmt.Fprintln(os.Stderr, "shotdiff: screenshots are stale, run make shots and commit docs/shots")
		os.Exit(1)
	}
}
