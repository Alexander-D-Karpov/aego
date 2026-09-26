package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

func main() {
	maxChannel := flag.Int("max-channel", 4, "maximum allowed per-channel difference")
	maxPixels := flag.Float64("max-pixels", 0.5, "maximum percentage of differing pixels")
	diffOut := flag.String("diff", "", "write a diff image to this path when comparison fails")
	flag.Parse()
	if flag.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: golden [flags] <reference.png> <actual.png>")
		os.Exit(2)
	}
	ref, err := load(flag.Arg(0))
	if err != nil {
		fail(err)
	}
	act, err := load(flag.Arg(1))
	if err != nil {
		fail(err)
	}
	if ref.Bounds() != act.Bounds() {
		fail(fmt.Errorf("size mismatch: %v vs %v", ref.Bounds(), act.Bounds()))
	}
	b := ref.Bounds()
	total := b.Dx() * b.Dy()
	differing := 0
	worst := 0
	var diff *image.NRGBA
	if *diffOut != "" {
		diff = image.NewNRGBA(b)
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			d := channelDiff(ref.At(x, y), act.At(x, y))
			if d > worst {
				worst = d
			}
			if d > *maxChannel {
				differing++
				if diff != nil {
					diff.Set(x, y, color.NRGBA{R: 255, A: 255})
				}
				continue
			}
			if diff != nil {
				diff.Set(x, y, color.NRGBA{R: 40, G: 40, B: 40, A: 255})
			}
		}
	}
	pct := float64(differing) * 100 / float64(total)
	fmt.Printf("worst channel diff=%d differing=%d (%.3f%%) threshold=%d/%.3f%%\n",
		worst, differing, pct, *maxChannel, *maxPixels)
	if pct > *maxPixels {
		if diff != nil {
			if err := save(*diffOut, diff); err != nil {
				fail(err)
			}
			fmt.Fprintln(os.Stderr, "diff written to", *diffOut)
		}
		os.Exit(1)
	}
}

func channelDiff(a, b color.Color) int {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	d := 0
	for _, p := range [4][2]uint32{{ar, br}, {ag, bg}, {ab, bb}, {aa, ba}} {
		v := int(p[0]>>8) - int(p[1]>>8)
		if v < 0 {
			v = -v
		}
		if v > d {
			d = v
		}
	}
	return d
}

func load(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return png.Decode(f)
}

func save(path string, img image.Image) error {
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

func fail(err error) {
	fmt.Fprintln(os.Stderr, "golden:", err)
	os.Exit(2)
}
