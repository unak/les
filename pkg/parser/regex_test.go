package parser

import (
	"les/pkg/ast"
	"les/pkg/lexer"
	"testing"
)

func TestParseRegexLiteralAndMatchExpr(t *testing.T) {
	input := `r = /foo/i
b = s =~ /bar/
match val:
    case /abc/ as m:
        m[0]
    case Int:
        42
`
	l := lexer.NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("unexpected parser errors: %v", p.Errors())
	}
	if len(prog.Body) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(prog.Body))
	}

	// 1. VarDecl r = /foo/i
	v1, ok := prog.Body[0].(*ast.VarDecl)
	if !ok {
		t.Fatalf("expected VarDecl, got %T", prog.Body[0])
	}
	rlit, ok := v1.Value.(*ast.RegexLit)
	if !ok {
		t.Fatalf("expected RegexLit, got %T", v1.Value)
	}
	if rlit.Pattern != "foo" || rlit.Flags != "i" {
		t.Fatalf("expected pattern 'foo' and flags 'i', got %q and %q", rlit.Pattern, rlit.Flags)
	}

	// 2. VarDecl b = s =~ /bar/
	v2, ok := prog.Body[1].(*ast.VarDecl)
	if !ok {
		t.Fatalf("expected VarDecl, got %T", prog.Body[1])
	}
	bin, ok := v2.Value.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", v2.Value)
	}
	if bin.Op != "=~" {
		t.Fatalf("expected '=~', got %q", bin.Op)
	}

	// 3. MatchExpr
	es, ok := prog.Body[2].(*ast.ExprStmt)
	if !ok {
		t.Fatalf("expected ExprStmt, got %T", prog.Body[2])
	}
	me, ok := es.Value.(*ast.MatchExpr)
	if !ok {
		t.Fatalf("expected MatchExpr, got %T", es.Value)
	}
	if len(me.Cases) != 2 {
		t.Fatalf("expected 2 cases, got %d", len(me.Cases))
	}
	if me.Cases[0].RegexPattern == nil || me.Cases[0].RegexPattern.Pattern != "abc" {
		t.Fatalf("expected RegexPattern 'abc', got %+v", me.Cases[0].RegexPattern)
	}
	if me.Cases[0].AsName != "m" {
		t.Fatalf("expected AsName 'm', got %q", me.Cases[0].AsName)
	}
	if me.Cases[1].Pattern != "Int" {
		t.Fatalf("expected Pattern 'Int', got %q", me.Cases[1].Pattern)
	}
}

func TestParseRegexEscapedSlashAndBackslash(t *testing.T) {
	input := `r = /a\\\/b\/c/`
	l := lexer.NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("unexpected parser errors: %v", p.Errors())
	}
	v := prog.Body[0].(*ast.VarDecl)
	rlit := v.Value.(*ast.RegexLit)
	expected := `a\\/b/c`
	if rlit.Pattern != expected {
		t.Fatalf("expected pattern %q, got %q", expected, rlit.Pattern)
	}
}

func TestParseMatchCaseMissingAsIdentError(t *testing.T) {
	input := `
match s:
    case /foo/ as:
        print 1
`
	l := lexer.NewLexer(input)
	p := NewParser(l)
	p.ParseProgram()
	if len(p.Errors()) == 0 {
		t.Fatalf("expected parser error for missing identifier after as")
	}
}
