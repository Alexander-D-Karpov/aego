package parser

import (
	"testing"

	"aego/script/diag"
)

func TestSyntaxCodes(t *testing.T) {
	cases := map[string]string{
		diag.DeclInFunction:   "package p\nfunc f() {\n\tconst X = 1\n}\n",
		diag.BadShortVar:      "package p\nfunc f() {\n\ta.b := 1\n}\n",
		diag.BadCompositeKey:  "package p\nfunc f() {\n\tx := P{a.b: 1}\n}\n",
		diag.BadForCondition:  "package p\nfunc f() {\n\tfor x := 1 {\n\t}\n}\n",
		diag.ExpectedComposit: "package p\nfunc f() {\n\tx := []int\n}\n",
	}
	for code, src := range cases {
		if _, ds, _ := parse(t, src); !hasCode(ds, code) {
			t.Errorf("%s not reported for:\n%s\ngot %v", code, src, ds)
		}
	}
}
