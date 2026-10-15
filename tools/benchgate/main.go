package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"aego/tools/bench/stat"
)

type series struct {
	ns     []float64
	allocs []float64
}

func trimProcs(name string) string {
	if i := strings.LastIndexByte(name, '-'); i > 0 {
		if _, err := strconv.Atoi(name[i+1:]); err == nil {
			return name[:i]
		}
	}
	return name
}

func parse(path string) (map[string]*series, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]*series{}
	pkg := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if p, ok := strings.CutPrefix(line, "pkg: "); ok {
			pkg = strings.TrimSpace(p)
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 || !strings.HasPrefix(fields[0], "Benchmark") {
			continue
		}
		name := pkg + "." + trimProcs(fields[0])
		s := out[name]
		if s == nil {
			s = &series{}
			out[name] = s
		}
		for i := 2; i+1 < len(fields); i += 2 {
			v, err := strconv.ParseFloat(fields[i], 64)
			if err != nil {
				continue
			}
			switch fields[i+1] {
			case "ns/op":
				s.ns = append(s.ns, v)
			case "allocs/op":
				s.allocs = append(s.allocs, v)
			}
		}
	}
	return out, sc.Err()
}

func main() {
	basePath := flag.String("base", "", "baseline go test -bench output")
	headPath := flag.String("head", "", "current go test -bench output")
	alpha := flag.Float64("alpha", 0.05, "significance level")
	tolerance := flag.Float64("tolerance", 10, "allowed regression of the median in percent")
	timing := flag.Bool("time", true, "compare timings; false checks allocations only")
	flag.Parse()
	head, err := parse(*headPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "benchgate:", err)
		os.Exit(2)
	}
	base, err := parse(*basePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "benchgate: no baseline at %s, run make bench-baseline\n", *basePath)
		return
	}
	names := make([]string, 0, len(head))
	for name := range head {
		names = append(names, name)
	}
	sort.Strings(names)
	failed := false
	for _, name := range names {
		h := head[name]
		b, ok := base[name]
		if !ok {
			fmt.Printf("%-60s (new)\n", name)
			continue
		}
		mark := "ok"
		if len(h.allocs) > 0 && len(b.allocs) > 0 {
			ha, ba := math.Round(stat.Median(h.allocs)), math.Round(stat.Median(b.allocs))
			if ha != ba {
				fmt.Printf("%-60s FAIL allocs/op %.0f, baseline %.0f\n", name, ha, ba)
				failed = true
				continue
			}
		}
		if !*timing {
			fmt.Printf("%-60s allocs %s\n", name, mark)
			continue
		}
		v := stat.Compare(b.ns, h.ns, *alpha, *tolerance)
		if v.Regressed {
			mark = "FAIL"
			failed = true
		}
		fmt.Printf("%-60s %12.0f -> %12.0f ns/op  %+6.1f%%  p=%.3f  %s\n", name, v.Base, v.Head, v.Delta, v.P, mark)
	}
	for name := range base {
		if _, ok := head[name]; !ok {
			fmt.Printf("%-60s missing in the current run\n", name)
		}
	}
	if failed {
		os.Exit(1)
	}
	fmt.Println("benchgate: ok")
}
