package harness

import (
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"aego/script/bind"
	"aego/script/build"
	"aego/script/vm"
)

type Options struct {
	VM     vm.Options
	Filter string
}

type Result struct {
	Package string
	Name    string
	Fault   *vm.Fault
	Elapsed time.Duration
}

type Report struct {
	Results []Result
	Passed  int
	Failed  int
	Elapsed time.Duration
}

func IsTestFunc(f *vm.Func) bool {
	if f.Script >= 0 || f.Params != 0 || f.Results != 0 || !build.IsTest(f.File) {
		return false
	}
	rest, ok := strings.CutPrefix(f.Name, "Test")
	if !ok {
		return false
	}
	if rest == "" {
		return true
	}
	r, _ := utf8.DecodeRuneInString(rest)
	return !unicode.IsLower(r)
}

func Run(res *build.Result, reg *bind.Registry, ctx *bind.Ctx, opt Options) (*Report, error) {
	if opt.VM.Budget == 0 {
		opt.VM.Budget = 10_000_000
	}
	m := vm.New(opt.VM)
	m.Env = ctx
	mods := make([]*vm.Module, 0, len(res.Order))
	for _, p := range res.Order {
		lm, err := m.Load(res.Modules[p], reg.Resolve)
		if err != nil {
			return nil, err
		}
		mods = append(mods, lm)
	}
	if err := vm.Link(mods); err != nil {
		return nil, err
	}
	rep := &Report{}
	start := time.Now()
	for _, lm := range mods {
		for _, f := range lm.Funcs {
			if !IsTestFunc(f) || (opt.Filter != "" && !strings.Contains(f.Name, opt.Filter)) {
				continue
			}
			t0 := time.Now()
			_, fl := m.Call(f, nil, nil)
			rep.Results = append(rep.Results, Result{Package: lm.Package, Name: f.Name, Fault: fl, Elapsed: time.Since(t0)})
			if fl == nil {
				rep.Passed++
			} else {
				rep.Failed++
			}
			m.Collect(nil)
		}
	}
	rep.Elapsed = time.Since(start)
	return rep, nil
}

func (r *Report) Print(w io.Writer, verbose bool) {
	var b strings.Builder
	for _, t := range r.Results {
		switch {
		case t.Fault != nil:
			fmt.Fprintf(&b, "--- FAIL: %s.%s (%.3fs)\n", t.Package, t.Name, t.Elapsed.Seconds())
			where := ""
			if len(t.Fault.Stack) > 0 {
				top := t.Fault.Stack[0]
				where = fmt.Sprintf("%s:%d: ", top.File, top.Line)
			}
			fmt.Fprintf(&b, "    %s%s [%s]\n", where, t.Fault.Message, t.Fault.Code)
			for _, fr := range t.Fault.Stack[min(1, len(t.Fault.Stack)):] {
				fmt.Fprintf(&b, "        %s %s:%d\n", fr.Function, fr.File, fr.Line)
			}
		case verbose:
			fmt.Fprintf(&b, "--- PASS: %s.%s (%.3fs)\n", t.Package, t.Name, t.Elapsed.Seconds())
		}
	}
	if r.Failed > 0 {
		fmt.Fprintf(&b, "FAIL\t%d passed, %d failed (%.3fs)\n", r.Passed, r.Failed, r.Elapsed.Seconds())
	} else {
		fmt.Fprintf(&b, "ok\t%d tests (%.3fs)\n", r.Passed, r.Elapsed.Seconds())
	}
	io.WriteString(w, b.String())
}
