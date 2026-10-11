package parser

import (
	"testing"

	"les/pkg/ast"
	"les/pkg/lexer"
)

func TestParseParenOmissionCall(t *testing.T) {
	input := `print 1 + 2`
	l := lexer.NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(prog.Body) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Body))
	}

	exprStmt, ok := prog.Body[0].(*ast.ExprStmt)
	if !ok {
		t.Fatalf("expected ExprStmt, got %T", prog.Body[0])
	}
	call, ok := exprStmt.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr, got %T", exprStmt.Value)
	}
	if len(call.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(call.Args))
	}
	// Arg should be (1 + 2) binary expr
	bin, ok := call.Args[0].(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr inside call arg, got %T", call.Args[0])
	}
	if bin.Op != "+" {
		t.Fatalf("expected op +, got %s", bin.Op)
	}
}

func TestParseTernary(t *testing.T) {
	input := `res = x > 0 ? 1 : 0`
	l := lexer.NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(prog.Body) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Body))
	}
	varDecl, ok := prog.Body[0].(*ast.VarDecl)
	if !ok {
		t.Fatalf("expected VarDecl, got %T", prog.Body[0])
	}
	tern, ok := varDecl.Value.(*ast.TernaryExpr)
	if !ok {
		t.Fatalf("expected TernaryExpr, got %T", varDecl.Value)
	}
	if tern.Cond == nil || tern.Then == nil || tern.Else == nil {
		t.Fatalf("incomplete ternary: %+v", tern)
	}
}

func TestParseBlockArgument(t *testing.T) {
	input := `each items do |x|:
    print x`

	l := lexer.NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(prog.Body) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Body))
	}
	exprStmt, ok := prog.Body[0].(*ast.ExprStmt)
	if !ok {
		t.Fatalf("expected ExprStmt, got %T", prog.Body[0])
	}
	call, ok := exprStmt.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr, got %T", exprStmt.Value)
	}
	if call.Block == nil {
		t.Fatalf("expected non-nil Block in CallExpr")
	}
	if len(call.Block.Params) != 1 || call.Block.Params[0].Name != "x" {
		t.Fatalf("expected block param x, got %+v", call.Block.Params)
	}
}

func TestParseModuleDeclaration(t *testing.T) {
	input := `module math

def Add(a, b):
    a + b`

	l := lexer.NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if prog.ModuleName != "math" {
		t.Fatalf("expected moduleName 'math', got %q", prog.ModuleName)
	}
}
