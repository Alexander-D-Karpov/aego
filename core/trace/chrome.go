package trace

import (
	"bufio"
	"encoding/json"
	"io"
	"strconv"
)

func us(ns int64) string {
	return strconv.FormatFloat(float64(ns)/1e3, 'f', 3, 64)
}

func WriteChrome(w io.Writer, t *Tracer, frames []Frame) error {
	bw := bufio.NewWriter(w)
	bw.WriteString(`{"displayTimeUnit":"ms","traceEvents":[`)
	bw.WriteString(`{"name":"thread_name","ph":"M","pid":1,"tid":1,"args":{"name":"main"}}`)
	var names []string
	if t != nil {
		names = make([]string, len(t.names))
		for i, n := range t.names {
			q, _ := json.Marshal(n)
			names[i] = string(q)
		}
	}
	for i := range frames {
		f := &frames[i]
		bw.WriteString(`,{"name":"frame ` + strconv.FormatUint(f.Index, 10) + `","ph":"X","pid":1,"tid":1,"ts":` + us(f.Start) +
			`,"dur":` + us(f.Dur) + `,"args":{"gc":` + strconv.FormatUint(uint64(f.GC), 10) +
			`,"dropped":` + strconv.FormatUint(uint64(f.Dropped), 10) + `}}`)
		for _, e := range f.Events {
			name := `""`
			if int(e.Zone) < len(names) {
				name = names[e.Zone]
			}
			bw.WriteString(`,{"name":` + name + `,"ph":"X","pid":1,"tid":1,"ts":` +
				us(f.Start+int64(e.Start)) + `,"dur":` + us(int64(e.Dur)) + `}`)
		}
	}
	bw.WriteString("]}\n")
	return bw.Flush()
}
