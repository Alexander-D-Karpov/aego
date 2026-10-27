package player

import (
	"path"
	"runtime"
	"sort"
	"strconv"
)

const (
	frameFunc    = "aego/engine.(*Engine).frame"
	snapshotFunc = "aego/runtime/player.(*Player).markWindow"
	siteDepth    = 32
	siteTop      = 30
)

type AllocSite struct {
	Stack     []string `json:"stack"`
	Objects   int64    `json:"objects"`
	Bytes     int64    `json:"bytes"`
	InFrame   bool     `json:"inFrame"`
	Truncated bool     `json:"truncated,omitempty"`
}

type AllocReport struct {
	Objects          int64       `json:"objects"`
	Bytes            int64       `json:"bytes"`
	FrameObjects     int64       `json:"frameObjects"`
	FrameBytes       int64       `json:"frameBytes"`
	TruncatedObjects int64       `json:"truncatedObjects"`
	Sites            []AllocSite `json:"sites"`
}

type memCount struct {
	objects, bytes int64
}

func memSnapshot() map[[32]uintptr]memCount {
	runtime.GC()
	n, _ := runtime.MemProfile(nil, true)
	var recs []runtime.MemProfileRecord
	for {
		recs = make([]runtime.MemProfileRecord, n+64)
		var ok bool
		if n, ok = runtime.MemProfile(recs, true); ok {
			break
		}
	}
	out := make(map[[32]uintptr]memCount, n)
	for _, r := range recs[:n] {
		out[r.Stack0] = memCount{r.AllocObjects, r.AllocBytes}
	}
	return out
}

func describe(stack [32]uintptr) (lines []string, inFrame, own, truncated bool) {
	pcs := stack[:]
	truncated = true
	for i, pc := range pcs {
		if pc == 0 {
			pcs, truncated = pcs[:i], false
			break
		}
	}
	frames := runtime.CallersFrames(pcs)
	for {
		f, more := frames.Next()
		switch f.Function {
		case frameFunc:
			inFrame = true
		case snapshotFunc:
			own = true
		}
		if len(lines) < siteDepth {
			lines = append(lines, f.Function+" "+path.Base(f.File)+":"+strconv.Itoa(f.Line))
		}
		if !more {
			return lines, inFrame || truncated, own, truncated && !inFrame
		}
	}
}

func memDiff(a, b map[[32]uintptr]memCount) AllocReport {
	var r AllocReport
	for k, e := range b {
		d := memCount{e.objects - a[k].objects, e.bytes - a[k].bytes}
		if d.objects <= 0 {
			continue
		}
		lines, inFrame, own, truncated := describe(k)
		if own {
			continue
		}
		r.Objects += d.objects
		r.Bytes += d.bytes
		if inFrame {
			r.FrameObjects += d.objects
			r.FrameBytes += d.bytes
		}
		if truncated {
			r.TruncatedObjects += d.objects
		}
		r.Sites = append(r.Sites, AllocSite{Stack: lines, Objects: d.objects, Bytes: d.bytes, InFrame: inFrame, Truncated: truncated})
	}
	sort.Slice(r.Sites, func(i, j int) bool {
		if r.Sites[i].InFrame != r.Sites[j].InFrame {
			return r.Sites[i].InFrame
		}
		return r.Sites[i].Objects > r.Sites[j].Objects
	})
	if len(r.Sites) > siteTop {
		r.Sites = r.Sites[:siteTop]
	}
	return r
}

func (p *Player) Allocs() *AllocReport {
	return p.allocs
}
