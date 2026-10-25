package aetext

import "fmt"

type Diag struct {
	Name string
	Pos  Pos
	Msg  string
}

func (d *Diag) Error() string {
	return fmt.Sprintf("%s:%d:%d: %s", d.Name, d.Pos.Line, d.Pos.Col, d.Msg)
}

func (d *Diag) Line() int {
	return d.Pos.Line
}

func sprintf(format string, args ...any) string {
	if len(args) == 0 {
		return format
	}
	return fmt.Sprintf(format, args...)
}

type parser struct {
	lex  *lexer
	tok  token
	peek token
	has  bool
}

func Parse(src []byte, name string) (*Node, error) {
	p := &parser{lex: newLexer(src, name)}
	if err := p.advance(); err != nil {
		return nil, err
	}
	n, err := p.parseObject()
	if err != nil {
		return nil, err
	}
	if p.tok.kind != tokEOF {
		return nil, p.errorf("unexpected %s after top-level object", describe(p.tok))
	}
	return n, nil
}

func describe(t token) string {
	switch t.kind {
	case tokEOF:
		return "end of file"
	case tokIdent:
		return "identifier " + t.text
	case tokString:
		return "string"
	case tokInt, tokFloat:
		return "number " + t.text
	case tokColor:
		return "color"
	}
	return "'" + t.text + "'"
}

func (p *parser) errorf(format string, args ...any) *Diag {
	return &Diag{Name: p.lex.name, Pos: p.tok.pos, Msg: sprintf(format, args...)}
}

func (p *parser) advance() *Diag {
	if p.has {
		p.tok = p.peek
		p.has = false
		return nil
	}
	t, err := p.lex.next()
	if err != nil {
		return err
	}
	p.tok = t
	return nil
}

func (p *parser) lookahead() (token, *Diag) {
	if p.has {
		return p.peek, nil
	}
	t, err := p.lex.next()
	if err != nil {
		return token{}, err
	}
	p.peek = t
	p.has = true
	return t, nil
}

func (p *parser) isPunct(s string) bool {
	return p.tok.kind == tokPunct && p.tok.text == s
}

func (p *parser) expectPunct(s string) *Diag {
	if !p.isPunct(s) {
		return p.errorf("expected '%s', got %s", s, describe(p.tok))
	}
	return p.advance()
}

func (p *parser) parseObject() (*Node, *Diag) {
	if p.tok.kind != tokIdent {
		return nil, p.errorf("expected object name, got %s", describe(p.tok))
	}
	n := &Node{Kind: NodeObject, Head: p.tok.text, Pos: p.tok.pos}
	if err := p.advance(); err != nil {
		return nil, err
	}
	if p.tok.kind == tokString {
		n.Label = p.tok.text
		if err := p.advance(); err != nil {
			return nil, err
		}
	}
	for p.tok.kind == tokIdent {
		name := p.tok.text
		pos := p.tok.pos
		if err := p.advance(); err != nil {
			return nil, err
		}
		if err := p.expectPunct("="); err != nil {
			return nil, err
		}
		v, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		n.Attrs = append(n.Attrs, Field{Name: name, Value: v, Pos: pos})
	}
	if err := p.expectPunct("{"); err != nil {
		return nil, err
	}
	for {
		if p.isPunct("}") {
			return n, p.advance()
		}
		if p.tok.kind == tokEOF {
			return nil, p.errorf("unexpected end of file, expected '}'")
		}
		if p.tok.kind != tokIdent {
			return nil, p.errorf("expected field or block, got %s", describe(p.tok))
		}
		la, err := p.lookahead()
		if err != nil {
			return nil, err
		}
		if la.kind == tokPunct && la.text == "=" {
			name := p.tok.text
			pos := p.tok.pos
			if err := p.advance(); err != nil {
				return nil, err
			}
			if err := p.advance(); err != nil {
				return nil, err
			}
			v, err := p.parseValue()
			if err != nil {
				return nil, err
			}
			n.Fields = append(n.Fields, Field{Name: name, Value: v, Pos: pos})
			continue
		}
		block, err := p.parseObject()
		if err != nil {
			return nil, err
		}
		n.Blocks = append(n.Blocks, block)
	}
}

func (p *parser) parseValue() (Value, *Diag) {
	t := p.tok
	switch t.kind {
	case tokInt:
		return Int(t.i), p.advance()
	case tokFloat:
		return Float64(t.f), p.advance()
	case tokString:
		return String(t.text), p.advance()
	case tokColor:
		return Color(t.color), p.advance()
	case tokIdent:
		switch t.text {
		case "true":
			return Bool(true), p.advance()
		case "false":
			return Bool(false), p.advance()
		case "asset", "node":
			return p.parseRef(t.text)
		}
		return Ident(t.text), p.advance()
	case tokPunct:
		switch t.text {
		case "[":
			return p.parseList()
		case "{":
			return p.parseStruct()
		}
	}
	return Value{}, p.errorf("expected value, got %s", describe(t))
}

func (p *parser) parseRef(kind string) (Value, *Diag) {
	if err := p.advance(); err != nil {
		return Value{}, err
	}
	if err := p.expectPunct("("); err != nil {
		return Value{}, err
	}
	if p.tok.kind != tokString {
		return Value{}, p.errorf("%s reference needs a quoted identifier", kind)
	}
	id := p.tok.text
	if err := p.advance(); err != nil {
		return Value{}, err
	}
	if err := p.expectPunct(")"); err != nil {
		return Value{}, err
	}
	if kind == "asset" {
		return AssetRef(id), nil
	}
	return NodeRef(id), nil
}

func (p *parser) parseList() (Value, *Diag) {
	if err := p.advance(); err != nil {
		return Value{}, err
	}
	var items []Value
	if p.isPunct("]") {
		return Value{Kind: VList, List: items}, p.advance()
	}
	for {
		v, err := p.parseValue()
		if err != nil {
			return Value{}, err
		}
		items = append(items, v)
		if p.isPunct(",") {
			if err := p.advance(); err != nil {
				return Value{}, err
			}
			continue
		}
		if p.isPunct("]") {
			return Value{Kind: VList, List: items}, p.advance()
		}
		return Value{}, p.errorf("expected ',' or ']' in list, got %s", describe(p.tok))
	}
}

func (p *parser) parseStruct() (Value, *Diag) {
	if err := p.advance(); err != nil {
		return Value{}, err
	}
	var fields []Field
	for {
		if p.isPunct("}") {
			return Value{Kind: VStruct, Obj: fields}, p.advance()
		}
		if p.tok.kind != tokIdent {
			return Value{}, p.errorf("expected field name in struct, got %s", describe(p.tok))
		}
		name := p.tok.text
		pos := p.tok.pos
		if err := p.advance(); err != nil {
			return Value{}, err
		}
		if err := p.expectPunct("="); err != nil {
			return Value{}, err
		}
		v, err := p.parseValue()
		if err != nil {
			return Value{}, err
		}
		fields = append(fields, Field{Name: name, Value: v, Pos: pos})
	}
}

// ParseValue reads one value written in aetext syntax, such as 15, [0, -15], Default or "text".
func ParseValue(src []byte, name string) (Value, error) {
	p := &parser{lex: newLexer(src, name)}
	if err := p.advance(); err != nil {
		return Value{}, err
	}
	v, err := p.parseValue()
	if err != nil {
		return Value{}, err
	}
	if p.tok.kind != tokEOF {
		return Value{}, p.errorf("unexpected %s after the value", describe(p.tok))
	}
	return v, nil
}
