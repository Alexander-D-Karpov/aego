package parser

import (
	"aego/script/ast"
	"aego/script/diag"
	"aego/script/lexer"
	"aego/script/source"
	"aego/script/token"
)

const maxNesting = 512

type parser struct {
	file      source.FileID
	src       []byte
	toks      []token.Token
	comments  []lexer.Comment
	pos       int
	tok       token.Token
	diags     []diag.Diagnostic
	errAt     uint32
	hasErr    bool
	exprLev   int
	depth     int
	deep      bool
	recovered bool
	typeGt    int
}

func ParseFile(sm *source.Manager, id source.FileID) (*ast.File, []diag.Diagnostic) {
	src := sm.Source(id)
	toks, comments, lexDiags := lexer.TokenizeAll(id, src, sm.Interner())
	p := &parser{file: id, src: src, toks: toks, comments: comments, tok: toks[0], typeGt: -1}
	p.diags = append(p.diags, lexDiags...)
	f := p.parseFile()
	diag.Sort(p.diags)
	return f, p.diags
}

// typeArgsEnd: a line break after a closing type-argument '>' ends the statement.
func (p *parser) typeArgsEnd() bool {
	if p.typeGt < 0 || p.typeGt != p.pos-1 {
		return false
	}
	for i := p.toks[p.typeGt].End; i < p.tok.Start && int(i) < len(p.src); i++ {
		if p.src[i] == '\n' {
			return true
		}
	}
	return false
}

func ParseSource(sm *source.Manager, path string, src []byte) (*ast.File, []diag.Diagnostic) {
	return ParseFile(sm, sm.Add(path, src))
}

func (p *parser) next() {
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	p.tok = p.toks[p.pos]
}

func (p *parser) prevEnd() uint32 {
	if p.pos == 0 {
		return p.tok.Start
	}
	return p.toks[p.pos-1].End
}

func (p *parser) loc(start uint32) ast.Loc {
	end := p.prevEnd()
	if end < start {
		end = start
	}
	return ast.Loc{Sp: source.Span{File: p.file, Start: start, End: end}}
}

func (p *parser) tokSpan() source.Span {
	return source.Span{File: p.file, Start: p.tok.Start, End: p.tok.End}
}

func (p *parser) tokLoc() ast.Loc {
	return ast.Loc{Sp: p.tokSpan()}
}

func (p *parser) errorf(code string, sp source.Span, format string, args ...any) {
	if p.hasErr && p.errAt == sp.Start {
		return
	}
	p.hasErr, p.errAt = true, sp.Start
	p.diags = append(p.diags, diag.Errorf(code, sp, format, args...))
}

func describe(t token.Token) string {
	switch {
	case t.IsAuto():
		return "newline"
	case t.Kind == token.EOF:
		return "end of file"
	case t.Kind == token.Ident:
		return "identifier " + t.Text
	case t.Kind.IsLiteral():
		return "literal " + t.Text
	case t.Kind.IsKeyword():
		return "keyword " + t.Text
	case t.Kind == token.Illegal:
		return "invalid token"
	}
	return "'" + t.Kind.String() + "'"
}

func (p *parser) errorExpected(what string) {
	if p.tok.Kind == token.Illegal {
		return
	}
	p.errorf(diag.UnexpectedToken, p.tokSpan(), "expected %s, found %s", what, describe(p.tok))
}

func (p *parser) expect(k token.Kind) bool {
	if p.tok.Kind != k {
		p.errorExpected("'" + k.String() + "'")
		return false
	}
	p.next()
	return true
}

func (p *parser) enter() bool {
	p.depth++
	if p.depth > maxNesting {
		if !p.deep {
			p.deep = true
			p.errorf(diag.TooDeep, p.tokSpan(), "nesting is deeper than %d levels", maxNesting)
		}
		return false
	}
	return true
}

func (p *parser) leave() {
	p.depth--
}

func isDeclStart(k token.Kind) bool {
	switch k {
	case token.Const, token.Type, token.Enum, token.Func, token.Script, token.Import, token.At, token.Var:
		return true
	}
	return false
}

func (p *parser) syncDecl() {
	p.recovered = true
	depth := 0
	for p.tok.Kind != token.EOF {
		switch p.tok.Kind {
		case token.LBrace, token.LParen, token.LBrack:
			depth++
		case token.RBrace, token.RParen, token.RBrack:
			if depth > 0 {
				depth--
			}
		case token.Semicolon:
			if depth == 0 {
				p.next()
				return
			}
		default:
			if depth == 0 && isDeclStart(p.tok.Kind) {
				return
			}
		}
		p.next()
	}
}

var yieldForms = map[string]ast.YieldKind{
	"wait":   ast.YieldWait,
	"frames": ast.YieldFrames,
	"fixed":  ast.YieldFixed,
}

func (p *parser) syncStmt() {
	depth := 0
	for p.tok.Kind != token.EOF {
		switch p.tok.Kind {
		case token.LBrace, token.LParen, token.LBrack:
			depth++
		case token.RParen, token.RBrack:
			if depth > 0 {
				depth--
			}
		case token.RBrace:
			if depth == 0 {
				return
			}
			depth--
		case token.Semicolon:
			if depth == 0 {
				return
			}
		case token.Var, token.If, token.For, token.Switch, token.Return, token.Break, token.Continue,
			token.Func, token.At, token.Yield, token.Signal:
			if depth == 0 {
				return
			}
		}
		p.next()
	}
}

func (p *parser) skipBraces() {
	if p.tok.Kind != token.LBrace {
		return
	}
	depth := 0
	for p.tok.Kind != token.EOF {
		switch p.tok.Kind {
		case token.LBrace:
			depth++
		case token.RBrace:
			depth--
			if depth == 0 {
				p.next()
				return
			}
		}
		p.next()
	}
}

func (p *parser) endDecl() {
	recovered := p.recovered
	p.recovered = false
	switch {
	case p.tok.Kind == token.Semicolon:
		p.next()
	case p.tok.Kind == token.EOF, p.typeArgsEnd(), recovered:
	default:
		p.errorExpected("';' or newline after declaration")
		p.syncDecl()
		p.recovered = false
	}
}

func (p *parser) sep(closing token.Kind, what string) {
	switch {
	case p.tok.Kind == token.Semicolon:
		p.next()
	case p.tok.Kind == closing, p.tok.Kind == token.EOF, p.typeArgsEnd():
	default:
		p.errorExpected("';' or newline after " + what)
		p.syncStmt()
		if p.tok.Kind == token.Semicolon {
			p.next()
		}
	}
}

func (p *parser) parseFile() *ast.File {
	f := &ast.File{ID: p.file}
	start := p.tok.Start
	if p.tok.Kind == token.Package {
		p.next()
		f.Package = p.parseIdent()
		p.endDecl()
	} else {
		p.errorf(diag.MissingPackage, p.tokSpan(), "file must start with a package clause")
	}
	for p.tok.Kind == token.Import {
		f.Imports = append(f.Imports, p.parseImport()...)
		p.endDecl()
	}
	for p.tok.Kind != token.EOF {
		before := p.pos
		if p.tok.Kind == token.Import {
			p.errorf(diag.LateImport, p.tokSpan(), "imports must come before declarations")
			f.Imports = append(f.Imports, p.parseImport()...)
		} else if d := p.parseTopDecl(); d != nil {
			f.Decls = append(f.Decls, d)
		}
		p.endDecl()
		if p.pos == before {
			p.next()
		}
	}
	f.Sp = source.Span{File: p.file, Start: start, End: p.tok.Start}
	return f
}

func (p *parser) parseImport() []*ast.Import {
	p.next()
	var out []*ast.Import
	one := func() bool {
		if p.tok.Kind != token.String {
			p.errorExpected("import path")
			return false
		}
		out = append(out, &ast.Import{Loc: p.tokLoc(), Path: p.tok.Value})
		p.next()
		return true
	}
	if p.tok.Kind != token.LParen {
		one()
		return out
	}
	p.next()
	for p.tok.Kind != token.RParen && p.tok.Kind != token.EOF {
		before := p.pos
		if !one() {
			p.syncStmt()
		}
		p.sep(token.RParen, "import")
		if p.pos == before {
			p.next()
		}
	}
	p.expect(token.RParen)
	return out
}

func (p *parser) misplaced(attrs []*ast.Attr) {
	if len(attrs) > 0 {
		p.errorf(diag.MisplacedAttr, attrs[0].Sp, "attributes are only allowed on scripts, script fields and functions")
	}
}

func (p *parser) parseTopDecl() ast.Decl {
	start := p.tok.Start
	doc := p.docBefore(start)
	attrs := p.parseAttrs()
	var d ast.Decl
	switch p.tok.Kind {
	case token.Script:
		d = p.parseScript(attrs, start)
	case token.Func:
		d = p.parseFunc(attrs, start)
	default:
		p.misplaced(attrs)
		switch p.tok.Kind {
		case token.Const:
			d = p.parseConst()
		case token.Type:
			d = p.parseStruct()
		case token.Enum:
			d = p.parseEnum()
		case token.Var:
			vs := p.tok.Start
			p.errorf(diag.PackageVar, p.tokSpan(), "package-level var is not allowed, use const or a script field")
			p.parseVarStmt()
			return &ast.BadDecl{Loc: p.loc(vs)}
		default:
			if len(attrs) > 0 && p.tok.Kind == token.EOF {
				return nil
			}
			bs := p.tok.Start
			p.errorExpected("declaration")
			p.next()
			p.syncDecl()
			return &ast.BadDecl{Loc: p.loc(bs)}
		}
	}
	setDoc(d, doc)
	return d
}

func (p *parser) parseAttrs() []*ast.Attr {
	var out []*ast.Attr
	for p.tok.Kind == token.At {
		start := p.tok.Start
		p.next()
		a := &ast.Attr{Name: p.parseIdent()}
		if p.tok.Kind == token.LParen {
			p.next()
			for p.tok.Kind != token.RParen && p.tok.Kind != token.EOF {
				a.Args = append(a.Args, p.parseAttrArg())
				if p.tok.Kind != token.Comma {
					break
				}
				p.next()
			}
			p.expect(token.RParen)
		}
		a.Loc = p.loc(start)
		out = append(out, a)
		if p.tok.IsAuto() {
			p.next()
		}
	}
	return out
}

func (p *parser) parseAttrArg() *ast.AttrArg {
	start := p.tok.Start
	arg := &ast.AttrArg{}
	if p.tok.Kind == token.Ident && p.peek(1).Kind == token.Assign {
		arg.Key = p.parseIdent()
		p.next()
	}
	arg.Value = p.parseUnary()
	if !isAttrLiteral(arg.Value) {
		p.errorf(diag.BadAttrArg, arg.Value.Span(), "attribute argument must be a literal")
		arg.Value = &ast.BadExpr{Loc: ast.Loc{Sp: arg.Value.Span()}}
	}
	arg.Loc = p.loc(start)
	return arg
}

func isAttrLiteral(x ast.Expr) bool {
	switch e := x.(type) {
	case *ast.BasicLit:
		return true
	case *ast.UnaryExpr:
		_, ok := e.X.(*ast.BasicLit)
		return ok && e.Op == token.Sub
	}
	return false
}

func (p *parser) peek(n int) token.Token {
	if i := p.pos + n; i < len(p.toks) {
		return p.toks[i]
	}
	return p.toks[len(p.toks)-1]
}

func (p *parser) parseIdent() *ast.Ident {
	if p.tok.Kind == token.Ident {
		id := &ast.Ident{Loc: p.tokLoc(), Name: p.tok.Text}
		p.next()
		return id
	}
	p.errorExpected("identifier")
	return &ast.Ident{Loc: ast.Loc{Sp: source.Span{File: p.file, Start: p.tok.Start, End: p.tok.Start}}, Name: "_"}
}

func (p *parser) parseConst() ast.Decl {
	start := p.tok.Start
	p.next()
	d := &ast.ConstDecl{Name: p.parseIdent()}
	if p.tok.Kind != token.Assign {
		d.Type = p.parseType()
	}
	p.expect(token.Assign)
	d.Value = p.parseExpr()
	d.Loc = p.loc(start)
	return d
}

func (p *parser) parseStruct() ast.Decl {
	start := p.tok.Start
	p.next()
	d := &ast.StructDecl{Name: p.parseIdent()}
	p.expect(token.Struct)
	p.expect(token.LBrace)
	for p.tok.Kind != token.RBrace && p.tok.Kind != token.EOF {
		before := p.pos
		fs := p.tok.Start
		doc := p.docBefore(fs)
		name := p.parseIdent()
		typ := p.parseType()
		d.Fields = append(d.Fields, &ast.Field{Loc: p.loc(fs), Doc: doc, Name: name, Type: typ})
		p.sep(token.RBrace, "field")
		if p.pos == before {
			p.next()
		}
	}
	p.expect(token.RBrace)
	d.Loc = p.loc(start)
	return d
}

func (p *parser) parseEnum() ast.Decl {
	start := p.tok.Start
	p.next()
	d := &ast.EnumDecl{Name: p.parseIdent()}
	p.expect(token.LBrace)
	for p.tok.Kind != token.RBrace && p.tok.Kind != token.EOF {
		before := p.pos
		d.Members = append(d.Members, p.parseIdent())
		p.sep(token.RBrace, "enum member")
		if p.pos == before {
			p.next()
		}
	}
	p.expect(token.RBrace)
	d.Loc = p.loc(start)
	return d
}

func (p *parser) parseFunc(attrs []*ast.Attr, start uint32) *ast.FuncDecl {
	p.next()
	d := &ast.FuncDecl{Attrs: attrs, Name: p.parseIdent()}
	d.Params, d.Result = p.parseSignature()
	if p.tok.Kind == token.Coroutine {
		d.Coroutine = true
		p.next()
	}
	if p.tok.Kind == token.LBrace {
		d.Body = p.parseBlock()
	} else {
		p.errorExpected("function body")
	}
	d.Loc = p.loc(start)
	return d
}

func (p *parser) parseSignature() ([]*ast.Param, ast.TypeExpr) {
	p.expect(token.LParen)
	var params []*ast.Param
	for p.tok.Kind != token.RParen && p.tok.Kind != token.EOF {
		start := p.tok.Start
		name := p.parseIdent()
		typ := p.parseType()
		params = append(params, &ast.Param{Loc: p.loc(start), Name: name, Type: typ})
		if p.tok.Kind != token.Comma {
			break
		}
		p.next()
	}
	p.expect(token.RParen)
	var result ast.TypeExpr
	if p.tok.Kind == token.Ident || p.tok.Kind == token.LBrack {
		result = p.parseType()
	}
	return params, result
}

func (p *parser) parseScript(attrs []*ast.Attr, start uint32) ast.Decl {
	p.next()
	d := &ast.ScriptDecl{Attrs: attrs, Name: p.parseIdent()}
	p.expect(token.LBrace)
	for p.tok.Kind != token.RBrace && p.tok.Kind != token.EOF {
		before := p.pos
		ms := p.tok.Start
		doc := p.docBefore(ms)
		mattrs := p.parseAttrs()
		switch p.tok.Kind {
		case token.Var:
			v := p.parseScriptVar(mattrs, ms)
			v.Doc = doc
			d.Vars = append(d.Vars, v)
		case token.Func:
			f := p.parseFunc(mattrs, ms)
			f.Doc = doc
			d.Funcs = append(d.Funcs, f)
		case token.Signal:
			p.misplaced(mattrs)
			s := p.parseSignal(ms)
			s.Doc = doc
			d.Signals = append(d.Signals, s)
		default:
			p.errorExpected("script field, signal or method")
			p.syncStmt()
			if p.tok.Kind == token.Semicolon {
				p.next()
			}
			if p.pos == before {
				p.next()
			}
			continue
		}
		p.sep(token.RBrace, "script member")
		if p.pos == before {
			p.next()
		}
	}
	p.expect(token.RBrace)
	d.Loc = p.loc(start)
	return d
}

func (p *parser) parseSignal(start uint32) *ast.SignalDecl {
	p.next()
	d := &ast.SignalDecl{Name: p.parseIdent()}
	var result ast.TypeExpr
	d.Params, result = p.parseSignature()
	if result != nil {
		p.errorf(diag.UnexpectedToken, result.Span(), "signals do not return a value")
	}
	d.Loc = p.loc(start)
	return d
}

func (p *parser) parseScriptVar(attrs []*ast.Attr, start uint32) *ast.ScriptVar {
	p.next()
	v := &ast.ScriptVar{Attrs: attrs, Name: p.parseIdent(), Type: p.parseType()}
	if p.tok.Kind == token.Assign {
		p.next()
		v.Value = p.parseExpr()
	}
	v.Loc = p.loc(start)
	return v
}

func (p *parser) parseType() ast.TypeExpr {
	start := p.tok.Start
	if !p.enter() {
		p.leave()
		return &ast.BadType{Loc: p.tokLoc()}
	}
	defer p.leave()
	switch p.tok.Kind {
	case token.LBrack:
		p.next()
		p.expect(token.RBrack)
		elem := p.parseType()
		return &ast.SliceType{Loc: p.loc(start), Elem: elem}
	case token.Ident:
		first := p.parseIdent()
		t := &ast.NamedType{Name: first}
		if p.tok.Kind == token.Period {
			p.next()
			t.Pkg = first
			t.Name = p.parseIdent()
		}
		if p.tok.Kind == token.Lt {
			p.next()
			for {
				t.Args = append(t.Args, p.parseType())
				if p.tok.Kind != token.Comma {
					break
				}
				p.next()
			}
			if p.expect(token.Gt) {
				p.typeGt = p.pos - 1
			}
		}
		t.Loc = p.loc(start)
		return t
	}
	p.errorExpected("type")
	return &ast.BadType{Loc: ast.Loc{Sp: source.Span{File: p.file, Start: p.tok.Start, End: p.tok.Start}}}
}

func (p *parser) parseBlock() *ast.BlockStmt {
	start := p.tok.Start
	if !p.enter() {
		p.leave()
		p.skipBraces()
		return &ast.BlockStmt{Loc: p.loc(start)}
	}
	defer p.leave()
	p.expect(token.LBrace)
	b := &ast.BlockStmt{Stmts: p.parseStmtList()}
	p.expect(token.RBrace)
	b.Loc = p.loc(start)
	return b
}

func (p *parser) parseStmtList() []ast.Stmt {
	var out []ast.Stmt
	for {
		switch p.tok.Kind {
		case token.RBrace, token.EOF, token.Case, token.Default:
			return out
		}
		before := p.pos
		if s := p.parseStmt(); s != nil {
			out = append(out, s)
		}
		switch k := p.tok.Kind; {
		case k == token.Semicolon:
			p.next()
		case k == token.RBrace, k == token.EOF, k == token.Case, k == token.Default, p.typeArgsEnd():
		default:
			p.errorExpected("';' or newline after statement")
			p.syncStmt()
			if p.tok.Kind == token.Semicolon {
				p.next()
			}
		}
		if p.pos == before {
			p.next()
		}
	}
}

func (p *parser) parseStmt() ast.Stmt {
	switch p.tok.Kind {
	case token.Var:
		return p.parseVarStmt()
	case token.Return:
		start := p.tok.Start
		p.next()
		r := &ast.ReturnStmt{}
		switch p.tok.Kind {
		case token.Semicolon, token.RBrace, token.Case, token.Default, token.EOF:
		default:
			r.Value = p.parseExpr()
		}
		r.Loc = p.loc(start)
		return r
	case token.Break, token.Continue:
		s := &ast.BranchStmt{Loc: p.tokLoc(), Tok: p.tok.Kind}
		p.next()
		return s
	case token.If:
		return p.parseIf()
	case token.For:
		return p.parseFor()
	case token.Switch:
		return p.parseSwitch()
	case token.LBrace:
		return p.parseBlock()
	case token.Semicolon:
		return nil
	case token.Func, token.Const, token.Type, token.Enum, token.Script:
		start := p.tok.Start
		p.errorf(diag.DeclInFunction, p.tokSpan(), "%s declarations are not allowed inside a function", p.tok.Text)
		p.next()
		p.syncStmt()
		if p.tok.Kind == token.LBrace {
			p.skipBraces()
		}
		return &ast.BadStmt{Loc: p.loc(start)}
	case token.Else:
		start := p.tok.Start
		p.errorf(diag.ElseOnNewLine, p.tokSpan(), "else must be on the same line as the closing brace of the if block")
		p.next()
		if p.tok.Kind == token.If {
			p.next()
			p.headExpr()
		}
		if p.tok.Kind == token.LBrace {
			p.parseBlock()
		}
		return &ast.BadStmt{Loc: p.loc(start)}
	case token.At:
		start := p.tok.Start
		p.errorf(diag.MisplacedAttr, p.tokSpan(), "attributes are not allowed inside a function body")
		p.parseAttrs()
		return &ast.BadStmt{Loc: p.loc(start)}
	case token.Yield:
		return p.parseYield()
	}
	return p.parseSimpleStmt()
}

func (p *parser) parseVarStmt() *ast.VarStmt {
	start := p.tok.Start
	p.next()
	s := &ast.VarStmt{Name: p.parseIdent(), Type: p.parseType()}
	if p.tok.Kind == token.Assign {
		p.next()
		s.Value = p.parseExpr()
	}
	s.Loc = p.loc(start)
	return s
}

func (p *parser) parseSimpleStmt() ast.Stmt {
	start := p.tok.Start
	x := p.parseExpr()
	if p.tok.Kind == token.Comma {
		p.errorf(diag.MultiAssign, p.tokSpan(), "multiple assignment is not supported, assign one value per statement")
		for p.tok.Kind == token.Comma {
			p.next()
			p.parseExpr()
		}
	}
	switch k := p.tok.Kind; {
	case k == token.Define:
		id, ok := x.(*ast.Ident)
		if !ok {
			p.errorf(diag.BadShortVar, x.Span(), "left side of := must be a single name")
			id = &ast.Ident{Loc: ast.Loc{Sp: x.Span()}, Name: "_"}
		}
		p.next()
		v := p.parseExpr()
		return &ast.ShortVarStmt{Loc: p.loc(start), Name: id, Value: v}
	case k.IsAssign():
		p.next()
		v := p.parseExpr()
		return &ast.AssignStmt{Loc: p.loc(start), Target: x, Op: k, Value: v}
	case k == token.Inc || k == token.Dec:
		p.next()
		return &ast.IncDecStmt{Loc: p.loc(start), X: x, Inc: k == token.Inc}
	}
	return &ast.ExprStmt{Loc: ast.Loc{Sp: x.Span()}, X: x}
}

func (p *parser) headExpr() ast.Expr {
	prev := p.exprLev
	p.exprLev = -1
	x := p.parseExpr()
	p.exprLev = prev
	return x
}

func (p *parser) parseIf() ast.Stmt {
	start := p.tok.Start
	if !p.enter() {
		p.leave()
		p.next()
		p.syncStmt()
		return &ast.BadStmt{Loc: p.loc(start)}
	}
	defer p.leave()
	p.next()
	s := &ast.IfStmt{Cond: p.headExpr()}
	s.Then = p.parseBlock()
	if p.tok.Kind == token.Else {
		p.next()
		switch p.tok.Kind {
		case token.If:
			s.Else = p.parseIf()
		case token.LBrace:
			s.Else = p.parseBlock()
		default:
			p.errorExpected("if or block after else")
		}
	}
	s.Loc = p.loc(start)
	return s
}

func (p *parser) parseFor() ast.Stmt {
	start := p.tok.Start
	p.next()
	s := &ast.ForStmt{}
	prev := p.exprLev
	p.exprLev = -1
	if p.tok.Kind != token.LBrace {
		var first ast.Stmt
		if p.tok.Kind != token.Semicolon {
			first = p.parseSimpleStmt()
		}
		if p.tok.Kind == token.Semicolon {
			p.next()
			s.Init = first
			if p.tok.Kind != token.Semicolon {
				s.Cond = p.parseExpr()
			}
			p.expect(token.Semicolon)
			if p.tok.Kind != token.LBrace {
				s.Post = p.parseSimpleStmt()
			}
		} else if es, ok := first.(*ast.ExprStmt); ok {
			s.Cond = es.X
		} else if first != nil {
			p.errorf(diag.BadForCondition, first.Span(), "expected a loop condition, found a statement")
		}
	}
	p.exprLev = prev
	s.Body = p.parseBlock()
	s.Loc = p.loc(start)
	return s
}

func (p *parser) parseSwitch() ast.Stmt {
	start := p.tok.Start
	p.next()
	s := &ast.SwitchStmt{Tag: p.headExpr()}
	p.expect(token.LBrace)
	for p.tok.Kind != token.RBrace && p.tok.Kind != token.EOF {
		before := p.pos
		switch p.tok.Kind {
		case token.Case, token.Default:
			cs := p.tok.Start
			c := &ast.CaseClause{Default: p.tok.Kind == token.Default}
			p.next()
			if !c.Default {
				c.Values = p.parseExprList()
			}
			p.expect(token.Colon)
			c.Body = p.parseStmtList()
			c.Loc = p.loc(cs)
			s.Cases = append(s.Cases, c)
		case token.Semicolon:
			p.next()
		default:
			p.errorExpected("case or default")
			p.syncStmt()
			if p.tok.Kind == token.Semicolon {
				p.next()
			}
		}
		if p.pos == before {
			p.next()
		}
	}
	p.expect(token.RBrace)
	s.Loc = p.loc(start)
	return s
}

func (p *parser) parseExprList() []ast.Expr {
	list := []ast.Expr{p.parseExpr()}
	for p.tok.Kind == token.Comma {
		p.next()
		list = append(list, p.parseExpr())
	}
	return list
}

func (p *parser) parseExpr() ast.Expr {
	return p.parseBinary(token.LowestPrec + 1)
}

func (p *parser) parseBinary(prec int) ast.Expr {
	x := p.parseUnary()
	for {
		op := p.tok.Kind
		oprec := op.Precedence()
		if oprec < prec {
			return x
		}
		p.next()
		y := p.parseBinary(oprec + 1)
		x = &ast.BinaryExpr{Loc: ast.Loc{Sp: x.Span().To(y.Span())}, Op: op, X: x, Y: y}
	}
}

func (p *parser) parseUnary() ast.Expr {
	if !p.enter() {
		p.leave()
		return &ast.BadExpr{Loc: p.tokLoc()}
	}
	defer p.leave()
	if k := p.tok.Kind; k == token.Sub || k == token.Not {
		start := p.tok.Start
		p.next()
		x := p.parseUnary()
		return &ast.UnaryExpr{Loc: p.loc(start), Op: k, X: x}
	}
	return p.parsePrimary()
}

func typeName(x ast.Expr) (ast.TypeExpr, bool) {
	switch e := x.(type) {
	case *ast.Ident:
		return &ast.NamedType{Loc: e.Loc, Name: e}, true
	case *ast.SelectorExpr:
		if pkg, ok := e.X.(*ast.Ident); ok {
			return &ast.NamedType{Loc: e.Loc, Pkg: pkg, Name: e.Sel}, true
		}
	}
	return nil, false
}

func (p *parser) looksLikeTypeArgs() bool {
	depth := 0
	for i := p.pos; i < len(p.toks); i++ {
		switch p.toks[i].Kind {
		case token.Lt:
			depth++
		case token.Gt:
			depth--
			if depth == 0 {
				return i+1 < len(p.toks) && p.toks[i+1].Kind == token.LParen
			}
		case token.Ident, token.Period, token.Comma, token.LBrack, token.RBrack:
		default:
			return false
		}
	}
	return false
}

func (p *parser) parsePrimary() ast.Expr {
	x := p.parseOperand()
	for {
		switch p.tok.Kind {
		case token.Period:
			p.next()
			sel := p.parseIdent()
			x = &ast.SelectorExpr{Loc: ast.Loc{Sp: x.Span().To(sel.Sp)}, X: x, Sel: sel}
		case token.LBrack:
			start := x.Span().Start
			p.next()
			p.exprLev++
			idx := p.parseExpr()
			p.exprLev--
			p.expect(token.RBrack)
			x = &ast.IndexExpr{Loc: p.loc(start), X: x, Index: idx}
		case token.LParen:
			x = p.parseCall(x, nil)
		case token.Lt:
			if _, ok := typeName(x); !ok || !p.looksLikeTypeArgs() {
				return x
			}
			p.next()
			var targs []ast.TypeExpr
			for {
				targs = append(targs, p.parseType())
				if p.tok.Kind != token.Comma {
					break
				}
				p.next()
			}
			p.expect(token.Gt)
			x = p.parseCall(x, targs)
		case token.LBrace:
			t, ok := typeName(x)
			if !ok || p.exprLev < 0 {
				return x
			}
			x = p.parseComposite(t, x.Span().Start)
		default:
			return x
		}
	}
}

func (p *parser) parseCall(fun ast.Expr, targs []ast.TypeExpr) ast.Expr {
	start := fun.Span().Start
	p.expect(token.LParen)
	p.exprLev++
	var args []ast.Expr
	for p.tok.Kind != token.RParen && p.tok.Kind != token.EOF {
		args = append(args, p.parseExpr())
		if p.tok.Kind != token.Comma {
			break
		}
		p.next()
	}
	p.exprLev--
	p.expect(token.RParen)
	return &ast.CallExpr{Loc: p.loc(start), Fun: fun, TypeArgs: targs, Args: args}
}

func (p *parser) parseComposite(t ast.TypeExpr, start uint32) ast.Expr {
	p.expect(token.LBrace)
	p.exprLev++
	lit := &ast.CompositeLit{Type: t}
	for p.tok.Kind != token.RBrace && p.tok.Kind != token.EOF {
		es := p.tok.Start
		el := &ast.Element{}
		x := p.parseExpr()
		if p.tok.Kind == token.Colon {
			p.next()
			id, ok := x.(*ast.Ident)
			if !ok {
				p.errorf(diag.BadCompositeKey, x.Span(), "composite literal key must be a field name")
				id = &ast.Ident{Loc: ast.Loc{Sp: x.Span()}, Name: "_"}
			}
			el.Key = id
			el.Value = p.parseExpr()
		} else {
			el.Value = x
		}
		el.Loc = p.loc(es)
		lit.Elems = append(lit.Elems, el)
		if p.tok.Kind != token.Comma {
			break
		}
		p.next()
	}
	p.exprLev--
	p.expect(token.RBrace)
	lit.Loc = p.loc(start)
	return lit
}

func (p *parser) parseYield() ast.Stmt {
	start := p.tok.Start
	p.next()
	s := &ast.YieldStmt{}
	if k, ok := yieldForms[p.tok.Text]; ok && p.tok.Kind == token.Ident && p.peek(1).Kind == token.LParen {
		s.Kind = k
		p.next()
		p.next()
		p.exprLev++
		s.Value = p.parseExpr()
		p.exprLev--
		p.expect(token.RParen)
	} else {
		switch p.tok.Kind {
		case token.Semicolon, token.RBrace, token.Case, token.Default, token.EOF:
		default:
			s.Kind = ast.YieldAwait
			s.Value = p.parseExpr()
		}
	}
	s.Loc = p.loc(start)
	return s
}

func (p *parser) parseStart() ast.Expr {
	start := p.tok.Start
	p.next()
	e := &ast.StartExpr{Name: p.parseIdent()}
	p.expect(token.LParen)
	p.exprLev++
	for p.tok.Kind != token.RParen && p.tok.Kind != token.EOF {
		e.Args = append(e.Args, p.parseExpr())
		if p.tok.Kind != token.Comma {
			break
		}
		p.next()
	}
	p.exprLev--
	p.expect(token.RParen)
	e.Loc = p.loc(start)
	return e
}

func (p *parser) parseOperand() ast.Expr {
	switch p.tok.Kind {
	case token.Ident:
		if p.tok.Text == "start" && p.peek(1).Kind == token.Ident && p.peek(2).Kind == token.LParen {
			return p.parseStart()
		}
		return p.parseIdent()
	case token.Int, token.Float, token.String, token.True, token.False, token.Nil:
		lit := &ast.BasicLit{Loc: p.tokLoc(), Kind: p.tok.Kind, Raw: p.tok.Text, Str: p.tok.Value}
		p.next()
		return lit
	case token.LParen:
		start := p.tok.Start
		p.next()
		p.exprLev++
		x := p.parseExpr()
		p.exprLev--
		p.expect(token.RParen)
		return &ast.ParenExpr{Loc: p.loc(start), X: x}
	case token.LBrack:
		start := p.tok.Start
		t := p.parseType()
		if p.tok.Kind == token.LBrace {
			return p.parseComposite(t, start)
		}
		p.errorf(diag.ExpectedComposit, t.Span(), "a slice type here must be followed by a composite literal")
		return &ast.BadExpr{Loc: p.loc(start)}
	}
	p.errorExpected("expression")
	return &ast.BadExpr{Loc: ast.Loc{Sp: source.Span{File: p.file, Start: p.tok.Start, End: p.tok.Start}}}
}
