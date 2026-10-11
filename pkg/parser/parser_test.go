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

func TestParseUnaryOperators(t *testing.T) {
	input := `
a = +10
b = -20
c = !True
`
	l := lexer.NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(prog.Body) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(prog.Body))
	}
	vd1 := prog.Body[0].(*ast.VarDecl)
	u1, ok := vd1.Value.(*ast.UnaryExpr)
	if !ok || u1.Op != "+" {
		t.Fatalf("expected unary +, got %+v", vd1.Value)
	}

	vd2 := prog.Body[1].(*ast.VarDecl)
	u2, ok := vd2.Value.(*ast.UnaryExpr)
	if !ok || u2.Op != "-" {
		t.Fatalf("expected unary -, got %+v", vd2.Value)
	}

	vd3 := prog.Body[2].(*ast.VarDecl)
	u3, ok := vd3.Value.(*ast.UnaryExpr)
	if !ok || u3.Op != "!" {
		t.Fatalf("expected unary !, got %+v", vd3.Value)
	}
}

func TestParseScriptLine(t *testing.T) {
	input := `
x = scriptLine
y = scriptLine
`
	l := lexer.NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(prog.Body) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(prog.Body))
	}
	vd1 := prog.Body[0].(*ast.VarDecl)
	i1, ok := vd1.Value.(*ast.IntLit)
	if !ok || i1.Value != 2 {
		t.Fatalf("expected line 2 IntLit, got %+v", vd1.Value)
	}

	vd2 := prog.Body[1].(*ast.VarDecl)
	i2, ok := vd2.Value.(*ast.IntLit)
	if !ok || i2.Value != 3 {
		t.Fatalf("expected line 3 IntLit, got %+v", vd2.Value)
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

func TestPutsParser(t *testing.T) {
	code := `
def puts_each(items):
    (head, ...tail) = items
    print head
    if head is String && len(head) > 0 && head[-1] == "\n":
        ()
    else:
        print "\n"
    if len(tail) > 0:
        puts_each(tail)

def puts(*args: Any):
    if len(args) == 0:
        print "\n"
    else:
        puts_each(args)

puts()
puts "hello"
puts "already\n"
puts "a", "b\n", 42
`
	l := lexer.NewLexer(code)
	p := NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(prog.Body) == 0 {
		t.Fatalf("no statements parsed")
	}
}

func TestParseLambdaExpr(t *testing.T) {
	code := `
f1 = |x|: x * 2
f2 = |x, y|: x + y
f3 = ||: 42
f4 = |x: Int|: x + 1
f5 = |x|:
    y = x * 2
    y + 1
`
	l := lexer.NewLexer(code)
	p := NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(prog.Body) != 5 {
		t.Fatalf("expected 5 statements, got %d", len(prog.Body))
	}

	// Verify f1
	vd1, ok := prog.Body[0].(*ast.VarDecl)
	if !ok {
		t.Fatalf("expected VarDecl, got %T", prog.Body[0])
	}
	lam1, ok := vd1.Value.(*ast.LambdaExpr)
	if !ok {
		t.Fatalf("expected LambdaExpr, got %T", vd1.Value)
	}
	if len(lam1.Params) != 1 || lam1.Params[0].Name != "x" {
		t.Fatalf("unexpected params for lam1: %+v", lam1.Params)
	}

	// Verify f3 (0-arg)
	vd3 := prog.Body[2].(*ast.VarDecl)
	lam3 := vd3.Value.(*ast.LambdaExpr)
	if len(lam3.Params) != 0 {
		t.Fatalf("expected 0 params for f3, got %d", len(lam3.Params))
	}

	// Verify f5 (multi-line)
	vd5 := prog.Body[4].(*ast.VarDecl)
	lam5 := vd5.Value.(*ast.LambdaExpr)
	if len(lam5.Params) != 1 {
		t.Fatalf("expected 1 param for f5, got %d", len(lam5.Params))
	}
	if len(lam5.Body) != 2 {
		t.Fatalf("expected 2 statements in f5 body, got %d", len(lam5.Body))
	}
}

func TestParseReturnAndBreak(t *testing.T) {
	// Top-level break should fail with parser error
	inputErr := `break 10`
	pErr := NewParser(lexer.NewLexer(inputErr))
	pErr.ParseProgram()
	if len(pErr.Errors()) == 0 {
		t.Fatalf("expected parse error for break outside closure, got none")
	}

	// Break in def without closure should also fail
	inputDefErr := "def foo():\n    break 10\n"
	pDefErr := NewParser(lexer.NewLexer(inputDefErr))
	pDefErr.ParseProgram()
	if len(pDefErr.Errors()) == 0 {
		t.Fatalf("expected parse error for break inside def (outside closure), got none")
	}

	// Break inside lambda should succeed
	inputOk := "f = |x|:\n    if x > 0:\n        break x\n    0\n"
	pOk := NewParser(lexer.NewLexer(inputOk))
	progOk := pOk.ParseProgram()
	if len(pOk.Errors()) > 0 {
		t.Fatalf("unexpected parser errors: %v", pOk.Errors())
	}
	if len(progOk.Body) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(progOk.Body))
	}

	// Return inside def and top-level return
	inputRet := "return 1\ndef bar():\n    return 2\n"
	pRet := NewParser(lexer.NewLexer(inputRet))
	progRet := pRet.ParseProgram()
	if len(pRet.Errors()) > 0 {
		t.Fatalf("unexpected parser errors for return: %v", pRet.Errors())
	}
	if len(progRet.Body) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(progRet.Body))
	}
}

func TestParseStructDefAndMemberAccess(t *testing.T) {
	code := `
struct Person:
    name: String
    age: Int = 0

    def Greet():
        "Hello"

struct Employee:
    Person
    salary: Int

p = Person(name: "Alice", age: 30)
p2 = Person(p, age: 31)
print p.name
`
	l := lexer.NewLexer(code)
	p := NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	if len(prog.Body) != 5 {
		t.Fatalf("expected 5 statements, got %d", len(prog.Body))
	}

	// 1: Person
	sd1, ok := prog.Body[0].(*ast.StructDef)
	if !ok {
		t.Fatalf("expected StructDef, got %T", prog.Body[0])
	}
	if sd1.Name != "Person" || len(sd1.Fields) != 2 || len(sd1.Methods) != 1 {
		t.Fatalf("unexpected StructDef Person: %+v", sd1)
	}
	if sd1.Fields[0].Name != "name" || sd1.Fields[0].TypeAnnot != "String" {
		t.Fatalf("unexpected field 0: %+v", sd1.Fields[0])
	}
	if sd1.Fields[1].Name != "age" || sd1.Fields[1].DefaultValue == nil {
		t.Fatalf("unexpected field 1: %+v", sd1.Fields[1])
	}

	// 2: Employee (embedded Person)
	sd2, ok := prog.Body[1].(*ast.StructDef)
	if !ok {
		t.Fatalf("expected StructDef, got %T", prog.Body[1])
	}
	if sd2.Name != "Employee" || len(sd2.Fields) != 2 {
		t.Fatalf("unexpected StructDef Employee: %+v", sd2)
	}
	if !sd2.Fields[0].IsEmbedded || sd2.Fields[0].Name != "Person" {
		t.Fatalf("expected embedded Person field, got %+v", sd2.Fields[0])
	}

	// 3: Named args call
	vd1, ok := prog.Body[2].(*ast.VarDecl)
	if !ok {
		t.Fatalf("expected VarDecl, got %T", prog.Body[2])
	}
	call1, ok := vd1.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr, got %T", vd1.Value)
	}
	if len(call1.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(call1.Args))
	}
	named1, ok := call1.Args[0].(*ast.NamedArgExpr)
	if !ok || named1.Name != "name" {
		t.Fatalf("expected named arg 'name', got %+v", call1.Args[0])
	}

	// 4: Copy update
	vd2, ok := prog.Body[3].(*ast.VarDecl)
	if !ok {
		t.Fatalf("expected VarDecl, got %T", prog.Body[3])
	}
	call2, ok := vd2.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr, got %T", vd2.Value)
	}
	if len(call2.Args) != 2 {
		t.Fatalf("expected 2 args for copy update, got %d", len(call2.Args))
	}

	// 5: Member access p.name
	es, ok := prog.Body[4].(*ast.ExprStmt)
	if !ok {
		t.Fatalf("expected ExprStmt, got %T", prog.Body[4])
	}
	call3, ok := es.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr for print, got %T", es.Value)
	}
	ma, ok := call3.Args[0].(*ast.MemberAccessExpr)
	if !ok || ma.Member != "name" {
		t.Fatalf("expected MemberAccessExpr 'name', got %+v", call3.Args[0])
	}
}

func TestParseInterfaceDef(t *testing.T) {
	input := `
interface Reader:
    read(n: Int): String

interface Closer:
    close(): ()

interface ReadCloser:
    Reader
    Closer
    reset(): Bool
`
	l := lexer.NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(prog.Body) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(prog.Body))
	}

	i1, ok := prog.Body[0].(*ast.InterfaceDef)
	if !ok || i1.Name != "Reader" || len(i1.Methods) != 1 {
		t.Fatalf("unexpected Reader interface: %+v", prog.Body[0])
	}
	if i1.Methods[0].Name != "read" || len(i1.Methods[0].Params) != 1 || i1.Methods[0].ReturnType != "String" {
		t.Fatalf("unexpected method signature: %+v", i1.Methods[0])
	}

	i3, ok := prog.Body[2].(*ast.InterfaceDef)
	if !ok || i3.Name != "ReadCloser" {
		t.Fatalf("unexpected ReadCloser interface: %+v", prog.Body[2])
	}
	if len(i3.Embedded) != 2 || i3.Embedded[0] != "Reader" || i3.Embedded[1] != "Closer" {
		t.Fatalf("unexpected embedded interfaces: %v", i3.Embedded)
	}
	if len(i3.Methods) != 1 || i3.Methods[0].Name != "reset" || i3.Methods[0].ReturnType != "Bool" {
		t.Fatalf("unexpected ReadCloser methods: %+v", i3.Methods)
	}
}

