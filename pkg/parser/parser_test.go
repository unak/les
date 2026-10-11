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

func TestParseModuleAccessExpr(t *testing.T) {
	input := `res = @math.Add(1, 2)
print @math.Pi`

	l := lexer.NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(prog.Body) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(prog.Body))
	}

	varDecl, ok := prog.Body[0].(*ast.VarDecl)
	if !ok {
		t.Fatalf("expected VarDecl, got %T", prog.Body[0])
	}
	call, ok := varDecl.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr, got %T", varDecl.Value)
	}
	modAccess, ok := call.Callee.(*ast.ModuleAccessExpr)
	if !ok {
		t.Fatalf("expected ModuleAccessExpr as callee, got %T", call.Callee)
	}
	if modAccess.Module != "math" || modAccess.Member != "Add" {
		t.Fatalf("unexpected modAccess: %+v", modAccess)
	}
}

func TestParseImportStmt(t *testing.T) {
	input := `import math
import "utils/helpers"`

	l := lexer.NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(prog.Body) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(prog.Body))
	}

	imp1, ok := prog.Body[0].(*ast.ImportStmt)
	if !ok || imp1.Path != "math" {
		t.Fatalf("expected import math, got %+v", prog.Body[0])
	}
	imp2, ok := prog.Body[1].(*ast.ImportStmt)
	if !ok || imp2.Path != "utils/helpers" {
		t.Fatalf("expected import utils/helpers, got %+v", prog.Body[1])
	}
}

func TestParseIsExpr(t *testing.T) {
	input := `res = (x is Int) ? 1 : 0`
	l := lexer.NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	varDecl, ok := prog.Body[0].(*ast.VarDecl)
	if !ok {
		t.Fatalf("expected VarDecl, got %T", prog.Body[0])
	}
	tern, ok := varDecl.Value.(*ast.TernaryExpr)
	if !ok {
		t.Fatalf("expected TernaryExpr, got %T", varDecl.Value)
	}
	isExpr, ok := tern.Cond.(*ast.IsExpr)
	if !ok {
		t.Fatalf("expected IsExpr, got %T", tern.Cond)
	}
	if isExpr.TypeAnnot != "Int" {
		t.Fatalf("expected TypeAnnot Int, got %q", isExpr.TypeAnnot)
	}
}

func TestParseMatchExpr(t *testing.T) {
	input := `
res = match x:
    case Int:
        10
    case String:
        20
    case _:
        0
`
	l := lexer.NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	varDecl, ok := prog.Body[0].(*ast.VarDecl)
	if !ok {
		t.Fatalf("expected VarDecl, got %T", prog.Body[0])
	}
	matchExpr, ok := varDecl.Value.(*ast.MatchExpr)
	if !ok {
		t.Fatalf("expected MatchExpr, got %T", varDecl.Value)
	}
	if len(matchExpr.Cases) != 3 {
		t.Fatalf("expected 3 cases, got %d", len(matchExpr.Cases))
	}
	if matchExpr.Cases[0].Pattern != "Int" || matchExpr.Cases[1].Pattern != "String" || matchExpr.Cases[2].Pattern != "_" {
		t.Fatalf("unexpected cases: %+v", matchExpr.Cases)
	}
}

