package lexer

import (
	"aego/script/diag"
	"aego/script/source"
	"aego/script/token"
)

type Comment struct {
	Start uint32
	End   uint32
	Line  bool
}

func (l *Lexer) Comments() []Comment {
	return l.comments
}

func TokenizeAll(file source.FileID, src []byte, in *source.Interner) ([]token.Token, []Comment, []diag.Diagnostic) {
	l := New(file, src, in)
	out := make([]token.Token, 0, 16+len(src)/4)
	for {
		t := l.Next()
		out = append(out, t)
		if t.Kind == token.EOF {
			return out, l.comments, l.diags
		}
	}
}
