package main

import (
	"bytes"
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

type reader struct {
	b []byte
	i int
}

func (r *reader) byte() byte {
	if r.i >= len(r.b) {
		panic(errors.New("unexpected end of section"))
	}
	c := r.b[r.i]
	r.i++
	return c
}

func (r *reader) uleb() uint64 {
	var v uint64
	for shift := 0; ; shift += 7 {
		c := r.byte()
		v |= uint64(c&0x7f) << shift
		if c&0x80 == 0 {
			return v
		}
	}
}

func (r *reader) name() string {
	n := int(r.uleb())
	if r.i+n > len(r.b) {
		panic(errors.New("name runs past the section"))
	}
	s := string(r.b[r.i : r.i+n])
	r.i += n
	return s
}

func (r *reader) limits() {
	if r.byte()&1 != 0 {
		r.uleb()
		r.uleb()
		return
	}
	r.uleb()
}

type report struct {
	total  int
	gzip   int
	code   int
	data   int
	other  int
	funcs  int
	byPkg  map[string]int
	unname int
}

func countImports(r *reader) int {
	funcs := 0
	for n := r.uleb(); n > 0; n-- {
		r.name()
		r.name()
		switch r.byte() {
		case 0:
			r.uleb()
			funcs++
		case 1:
			r.byte()
			r.limits()
		case 2:
			r.limits()
		case 3:
			r.byte()
			r.byte()
		case 4:
			r.byte()
			r.uleb()
		}
	}
	return funcs
}

func parseNames(r *reader, out map[int]string) {
	for r.i < len(r.b) {
		sub := r.byte()
		end := r.i + int(r.uleb())
		if sub == 1 {
			for n := r.uleb(); n > 0; n-- {
				idx := int(r.uleb())
				out[idx] = r.name()
			}
		}
		r.i = end
	}
}

func pkgOf(name string, depth int) string {
	switch {
	case name == "":
		return "(unnamed)"
	case strings.HasPrefix(name, "type:"), strings.HasPrefix(name, "type_"):
		return "(types)"
	}
	slash := strings.LastIndex(name, "/")
	pkg := name
	if dot := strings.Index(name[slash+1:], "."); dot >= 0 {
		pkg = name[:slash+1+dot]
	}
	sep := "/"
	if slash < 0 {
		sep = "_"
	}
	if depth > 0 {
		if seg := strings.Split(pkg, sep); len(seg) > depth {
			pkg = strings.Join(seg[:depth], sep)
		}
	}
	return pkg
}

func analyze(b []byte, depth int) (rep *report, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("malformed wasm: %v", r)
		}
	}()
	if len(b) < 8 || !bytes.Equal(b[:4], []byte("\x00asm")) {
		return nil, errors.New("not a wasm module")
	}
	rep = &report{total: len(b), byPkg: map[string]int{}}
	r := &reader{b: b, i: 8}
	imported := 0
	var bodies []int
	names := map[int]string{}
	for r.i < len(b) {
		id := r.byte()
		size := int(r.uleb())
		end := r.i + size
		if end > len(b) {
			return nil, errors.New("section runs past the file")
		}
		sec := &reader{b: b[:end], i: r.i}
		switch id {
		case 0:
			if sec.name() == "name" {
				parseNames(sec, names)
			}
		case 2:
			imported = countImports(sec)
		case 10:
			rep.code = size
			for n := sec.uleb(); n > 0; n-- {
				s := int(sec.uleb())
				bodies = append(bodies, s)
				sec.i += s
			}
		case 11:
			rep.data = size
		default:
			rep.other += size
		}
		r.i = end
	}
	rep.funcs = len(bodies)
	for k, s := range bodies {
		n, ok := names[imported+k]
		if !ok {
			rep.unname += s
		}
		rep.byPkg[pkgOf(n, depth)] += s
	}
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	zw.Write(b)
	zw.Close()
	rep.gzip = buf.Len()
	return rep, nil
}

func mb(n int) string {
	return fmt.Sprintf("%6.2f MB", float64(n)/(1<<20))
}

func main() {
	top := flag.Int("top", 40, "packages to list")
	depth := flag.Int("depth", 0, "group packages by this many path segments, 0 keeps full paths")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: wasmsize [-top n] [-depth n] file.wasm")
		os.Exit(2)
	}
	data, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "wasmsize:", err)
		os.Exit(1)
	}
	rep, err := analyze(data, *depth)
	if err != nil {
		fmt.Fprintln(os.Stderr, "wasmsize:", err)
		os.Exit(1)
	}
	fmt.Printf("total %s, gzip %s, code %s in %d functions, data %s, other %s\n",
		mb(rep.total), mb(rep.gzip), mb(rep.code), rep.funcs, mb(rep.data), mb(rep.other))
	if rep.unname > rep.code/2 {
		fmt.Println("most functions have no names: build without -ldflags=-s")
	}
	type row struct {
		pkg  string
		size int
	}
	rows := make([]row, 0, len(rep.byPkg))
	for p, s := range rep.byPkg {
		rows = append(rows, row{p, s})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].size != rows[j].size {
			return rows[i].size > rows[j].size
		}
		return rows[i].pkg < rows[j].pkg
	})
	for i, r := range rows {
		if i == *top {
			break
		}
		fmt.Printf("%s %5.1f%%  %s\n", mb(r.size), float64(r.size)*100/float64(max(rep.code, 1)), r.pkg)
	}
}
