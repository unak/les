package parser

import (
	"strings"
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
c = !true
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

func TestParseForStmt(t *testing.T) {
	input := `
for x > 0:
    print x
    continue
    break
`
	l := lexer.NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(prog.Body) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Body))
	}
	forStmt, ok := prog.Body[0].(*ast.ForStmt)
	if !ok {
		t.Fatalf("expected ForStmt, got %T", prog.Body[0])
	}
	if forStmt.Cond == nil {
		t.Fatalf("expected non-nil Cond")
	}
	if len(forStmt.Body) != 3 {
		t.Fatalf("expected 3 body statements, got %d", len(forStmt.Body))
	}
}

func TestParseInfiniteForStmt(t *testing.T) {
	input := `
for:
    print "loop"
    break
`
	l := lexer.NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	forStmt, ok := prog.Body[0].(*ast.ForStmt)
	if !ok || forStmt.Cond != nil {
		t.Fatalf("expected infinite ForStmt with nil Cond, got %+v", prog.Body[0])
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

def puts(*args):
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

func TestParseMultilineTupleAndDict(t *testing.T) {
	input := `
t1 = (
    1,
    2,
    3,
)

t2 = (
    10,
    20
)

d1 = {
    "a": 1,
    "b": 2,
}

d2 = {
    "x": 100,
    "y": 200
}

empty_t = (
)

empty_d = {
}

t_comment = (
    # comment 1
    1,
    # comment 2
    2,
)
`
	l := lexer.NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(prog.Body) != 7 {
		t.Fatalf("expected 7 statements, got %d", len(prog.Body))
	}

	// t1
	vd1, ok := prog.Body[0].(*ast.VarDecl)
	if !ok || vd1.Name != "t1" {
		t.Fatalf("expected t1 VarDecl, got %+v", prog.Body[0])
	}
	tup1, ok := vd1.Value.(*ast.TupleLit)
	if !ok || len(tup1.Elements) != 3 {
		t.Fatalf("expected TupleLit with 3 elements, got %+v", vd1.Value)
	}

	// t2
	vd2, ok := prog.Body[1].(*ast.VarDecl)
	if !ok || vd2.Name != "t2" {
		t.Fatalf("expected t2 VarDecl, got %+v", prog.Body[1])
	}
	tup2, ok := vd2.Value.(*ast.TupleLit)
	if !ok || len(tup2.Elements) != 2 {
		t.Fatalf("expected TupleLit with 2 elements, got %+v", vd2.Value)
	}

	// d1
	vd3, ok := prog.Body[2].(*ast.VarDecl)
	if !ok || vd3.Name != "d1" {
		t.Fatalf("expected d1 VarDecl, got %+v", prog.Body[2])
	}
	dict1, ok := vd3.Value.(*ast.DictLit)
	if !ok || len(dict1.Entries) != 2 {
		t.Fatalf("expected DictLit with 2 entries, got %+v", vd3.Value)
	}

	// d2
	vd4, ok := prog.Body[3].(*ast.VarDecl)
	if !ok || vd4.Name != "d2" {
		t.Fatalf("expected d2 VarDecl, got %+v", prog.Body[3])
	}
	dict2, ok := vd4.Value.(*ast.DictLit)
	if !ok || len(dict2.Entries) != 2 {
		t.Fatalf("expected DictLit with 2 entries, got %+v", vd4.Value)
	}

	// empty_t
	vd5, ok := prog.Body[4].(*ast.VarDecl)
	if !ok || vd5.Name != "empty_t" {
		t.Fatalf("expected empty_t VarDecl, got %+v", prog.Body[4])
	}
	emptyTup, ok := vd5.Value.(*ast.TupleLit)
	if !ok || len(emptyTup.Elements) != 0 {
		t.Fatalf("expected empty TupleLit, got %+v", vd5.Value)
	}

	// empty_d
	vd6, ok := prog.Body[5].(*ast.VarDecl)
	if !ok || vd6.Name != "empty_d" {
		t.Fatalf("expected empty_d VarDecl, got %+v", prog.Body[5])
	}
	emptyDict, ok := vd6.Value.(*ast.DictLit)
	if !ok || len(emptyDict.Entries) != 0 {
		t.Fatalf("expected empty DictLit, got %+v", vd6.Value)
	}

	// t_comment
	vd7, ok := prog.Body[6].(*ast.VarDecl)
	if !ok || vd7.Name != "t_comment" {
		t.Fatalf("expected t_comment VarDecl, got %+v", prog.Body[6])
	}
	tupComment, ok := vd7.Value.(*ast.TupleLit)
	if !ok || len(tupComment.Elements) != 2 {
		t.Fatalf("expected TupleLit with 2 elements, got %+v", vd7.Value)
	}
}

func TestParseMultilineNestedAndCalls(t *testing.T) {
	input := `
nested = {
    "tuple": (
        1,
        (
            2,
            3,
        ),
    ),
    "single": (
        42,
    ),
}

def add(a: Int, b: Int): Int:
    a + b

res = add(
    10,
    20,
)
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
}

func TestParseSemicolonAndLineContinuation(t *testing.T) {
	input := `
a = 1; b = 2; c = 3

x = 10 + \
    20 + \
    30

struct Point:
    px: Int; py: Int
`
	l := lexer.NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(prog.Body) != 5 {
		t.Fatalf("expected 5 statements, got %d", len(prog.Body))
	}

	// Stmt 0, 1, 2 are a, b, c
	vdA, ok := prog.Body[0].(*ast.VarDecl)
	if !ok || vdA.Name != "a" {
		t.Fatalf("expected VarDecl a, got %+v", prog.Body[0])
	}
	vdB, ok := prog.Body[1].(*ast.VarDecl)
	if !ok || vdB.Name != "b" {
		t.Fatalf("expected VarDecl b, got %+v", prog.Body[1])
	}
	vdC, ok := prog.Body[2].(*ast.VarDecl)
	if !ok || vdC.Name != "c" {
		t.Fatalf("expected VarDecl c, got %+v", prog.Body[2])
	}

	// Stmt 3 is x with binary expression
	vdX, ok := prog.Body[3].(*ast.VarDecl)
	if !ok || vdX.Name != "x" {
		t.Fatalf("expected VarDecl x, got %+v", prog.Body[3])
	}

	// Stmt 4 is StructDef Point
	sd, ok := prog.Body[4].(*ast.StructDef)
	if !ok || sd.Name != "Point" || len(sd.Fields) != 2 {
		t.Fatalf("expected StructDef Point with 2 fields, got %+v", prog.Body[4])
	}
}

func TestParsePostfixForce(t *testing.T) {
	input := `
a = x!
b = f(1, 2)!
c = !y!
d = (1 + 2)! * 3
`
	l := lexer.NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(prog.Body) != 4 {
		t.Fatalf("expected 4 statements, got %d", len(prog.Body))
	}

	// a = x!
	vdA := prog.Body[0].(*ast.VarDecl)
	fA, ok := vdA.Value.(*ast.ForceExpr)
	if !ok {
		t.Fatalf("expected ForceExpr for a, got %T", vdA.Value)
	}
	idA, ok := fA.Expr.(*ast.Identifier)
	if !ok || idA.Name != "x" {
		t.Fatalf("expected identifier x in a!, got %+v", fA.Expr)
	}

	// b = f(1, 2)!
	vdB := prog.Body[1].(*ast.VarDecl)
	fB, ok := vdB.Value.(*ast.ForceExpr)
	if !ok {
		t.Fatalf("expected ForceExpr for b, got %T", vdB.Value)
	}
	if _, ok := fB.Expr.(*ast.CallExpr); !ok {
		t.Fatalf("expected CallExpr in b!, got %+v", fB.Expr)
	}

	// c = !y! -> UnaryExpr(!, ForceExpr(y))
	vdC := prog.Body[2].(*ast.VarDecl)
	uC, ok := vdC.Value.(*ast.UnaryExpr)
	if !ok || uC.Op != "!" {
		t.Fatalf("expected UnaryExpr ! for c, got %+v", vdC.Value)
	}
	fC, ok := uC.Expr.(*ast.ForceExpr)
	if !ok {
		t.Fatalf("expected ForceExpr inside !y!, got %T", uC.Expr)
	}
	idC, ok := fC.Expr.(*ast.Identifier)
	if !ok || idC.Name != "y" {
		t.Fatalf("expected identifier y inside !y!, got %+v", fC.Expr)
	}

	// d = (1 + 2)! * 3 -> BinaryExpr(*, ForceExpr(1 + 2), 3)
	vdD := prog.Body[3].(*ast.VarDecl)
	binD, ok := vdD.Value.(*ast.BinaryExpr)
	if !ok || binD.Op != "*" {
		t.Fatalf("expected BinaryExpr * for d, got %+v", vdD.Value)
	}
	if _, ok := binD.Left.(*ast.ForceExpr); !ok {
		t.Fatalf("expected ForceExpr on left of *, got %T", binD.Left)
	}
}

func TestParsePipelineOperator(t *testing.T) {
	input := `
res = x |> f
res2 = x |> f |> g
res3 = x |> add(10)
res4 = 5 |> (|n|: n * 2)
`
	l := lexer.NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(prog.Body) != 4 {
		t.Fatalf("expected 4 statements, got %d", len(prog.Body))
	}

	// 1. res = x |> f -> f(x)
	vd1 := prog.Body[0].(*ast.VarDecl)
	call1, ok := vd1.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr for res, got %T", vd1.Value)
	}
	if id, ok := call1.Callee.(*ast.Identifier); !ok || id.Name != "f" {
		t.Fatalf("expected callee f, got %+v", call1.Callee)
	}
	if len(call1.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(call1.Args))
	}
	if arg, ok := call1.Args[0].(*ast.Identifier); !ok || arg.Name != "x" {
		t.Fatalf("expected arg x, got %+v", call1.Args[0])
	}

	// 2. res2 = x |> f |> g -> g(f(x))
	vd2 := prog.Body[1].(*ast.VarDecl)
	call2, ok := vd2.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr for res2, got %T", vd2.Value)
	}
	if id, ok := call2.Callee.(*ast.Identifier); !ok || id.Name != "g" {
		t.Fatalf("expected callee g, got %+v", call2.Callee)
	}
	innerCall, ok := call2.Args[0].(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected inner CallExpr f(x), got %T", call2.Args[0])
	}
	if id, ok := innerCall.Callee.(*ast.Identifier); !ok || id.Name != "f" {
		t.Fatalf("expected inner callee f, got %+v", innerCall.Callee)
	}

	// 3. res3 = x |> add(10) -> add(x, 10)
	vd3 := prog.Body[2].(*ast.VarDecl)
	call3, ok := vd3.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr for res3, got %T", vd3.Value)
	}
	if id, ok := call3.Callee.(*ast.Identifier); !ok || id.Name != "add" {
		t.Fatalf("expected callee add, got %+v", call3.Callee)
	}
	if len(call3.Args) != 2 {
		t.Fatalf("expected 2 args (x, 10), got %d", len(call3.Args))
	}
	if id, ok := call3.Args[0].(*ast.Identifier); !ok || id.Name != "x" {
		t.Fatalf("expected first arg x, got %+v", call3.Args[0])
	}
	if num, ok := call3.Args[1].(*ast.IntLit); !ok || num.Value != 10 {
		t.Fatalf("expected second arg 10, got %+v", call3.Args[1])
	}

	// 4. res4 = 5 |> (|n|: n * 2) -> (|n|: n * 2)(5)
	vd4 := prog.Body[3].(*ast.VarDecl)
	call4, ok := vd4.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr for res4, got %T", vd4.Value)
	}
	if len(call4.Args) != 1 {
		t.Fatalf("expected 1 arg (5), got %d", len(call4.Args))
	}
	if num, ok := call4.Args[0].(*ast.IntLit); !ok || num.Value != 5 {
		t.Fatalf("expected arg 5, got %+v", call4.Args[0])
	}

	// 5. Parenthesis-omitted call greedily consumes pipeline: print x |> f -> print(f(x))
	inputPrint := `print x |> f`
	progPrint := NewParser(lexer.NewLexer(inputPrint)).ParseProgram()
	if len(progPrint.Body) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(progPrint.Body))
	}
	stmt, ok := progPrint.Body[0].(*ast.ExprStmt)
	if !ok {
		t.Fatalf("expected ExprStmt, got %T", progPrint.Body[0])
	}
	printCall, ok := stmt.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr for print, got %T", stmt.Value)
	}
	if id, ok := printCall.Callee.(*ast.Identifier); !ok || id.Name != "print" {
		t.Fatalf("expected callee print, got %+v", printCall.Callee)
	}
	if len(printCall.Args) != 1 {
		t.Fatalf("expected 1 arg to print, got %d", len(printCall.Args))
	}
	pipeCall, ok := printCall.Args[0].(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected argument to be CallExpr f(x), got %T", printCall.Args[0])
	}
	if id, ok := pipeCall.Callee.(*ast.Identifier); !ok || id.Name != "f" {
		t.Fatalf("expected inner callee f, got %+v", pipeCall.Callee)
	}

	// 6. Precedence below arithmetic: res = 1 + 2 |> double -> double(1 + 2)
	inputArith := `res = 1 + 2 |> double`
	progArith := NewParser(lexer.NewLexer(inputArith)).ParseProgram()
	if len(progArith.Body) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(progArith.Body))
	}
	vdArith := progArith.Body[0].(*ast.VarDecl)
	callArith, ok := vdArith.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr for res, got %T", vdArith.Value)
	}
	if id, ok := callArith.Callee.(*ast.Identifier); !ok || id.Name != "double" {
		t.Fatalf("expected callee double, got %+v", callArith.Callee)
	}
	if len(callArith.Args) != 1 {
		t.Fatalf("expected 1 arg to double, got %d", len(callArith.Args))
	}
	binArg, ok := callArith.Args[0].(*ast.BinaryExpr)
	if !ok || binArg.Op != "+" {
		t.Fatalf("expected binary expr 1 + 2 as arg, got %+v", callArith.Args[0])
	}

	// 7. Explicit call piped to next function: f(x) |> g -> g(f(x))
	inputCallPipe := `res = f(x) |> g`
	progCallPipe := NewParser(lexer.NewLexer(inputCallPipe)).ParseProgram()
	if len(progCallPipe.Body) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(progCallPipe.Body))
	}
	vdCallPipe := progCallPipe.Body[0].(*ast.VarDecl)
	callG, ok := vdCallPipe.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr for res, got %T", vdCallPipe.Value)
	}
	if id, ok := callG.Callee.(*ast.Identifier); !ok || id.Name != "g" {
		t.Fatalf("expected callee g, got %+v", callG.Callee)
	}
	innerF, ok := callG.Args[0].(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected inner CallExpr f(x), got %T", callG.Args[0])
	}
	if id, ok := innerF.Callee.(*ast.Identifier); !ok || id.Name != "f" {
		t.Fatalf("expected inner callee f, got %+v", innerF.Callee)
	}
}

func TestParseInvalidTypeAnnotations(t *testing.T) {
	invalidInputs := []string{
		`b: Dict[String, Int] = 5`,
		`b: {String} = 5`,
		`b: {} = 5`,
		`b: {: Int} = 5`,
		`b: {String:} = 5`,
		`b: {String: Int: Bool} = 5`,
		`x is Dict[String, Int]`,
		`x is {String}`,
		`x is {}`,
		"def f(x: Dict[String, Int]):\n    1",
		"def f(x: {String}):\n    1",
		`match v:
    case Dict[String, Int]:
        1`,
		`match v:
    case {String}:
        1`,
	}

	for _, input := range invalidInputs {
		l := lexer.NewLexer(input)
		p := NewParser(l)
		_ = p.ParseProgram()
		if len(p.Errors()) == 0 {
			t.Errorf("expected syntax error for %q, but got none", input)
		}
	}

	validInputs := []string{
		`b: {String: Int} = {"a": 1}`,
		`b: {String: {Int: Bool}} = {}`,
		`x is {String: Int}`,
		`x is {String: {Int: Bool}}`,
		"def f(x: {String: Int}) -> {String: Int}:\n    x",
		`match v:
    case {String: Int}:
        1`,
	}

	for _, input := range validInputs {
		l := lexer.NewLexer(input)
		p := NewParser(l)
		_ = p.ParseProgram()
		if len(p.Errors()) > 0 {
			t.Errorf("unexpected syntax error for %q: %v", input, p.Errors())
		}
	}
}

func TestNormalizedTypeAnnotationFormatting(t *testing.T) {
	tests := []struct {
		code     string
		expected string
	}{
		{`b: {String: Int} = {}`, `{String: Int}`},
		{`b: {String: {Int: Bool}} = {}`, `{String: {Int: Bool}}`},
		{`b: (Int, String) = (1, "a")`, `(Int, String)`},
		{`b: Int | String = 1`, `Int | String`},
		{`b: (Int) -> String = f`, `(Int) -> String`},
		{`b: @mod.Type = v`, `@mod.Type`},
	}

	for _, tt := range tests {
		l := lexer.NewLexer(tt.code)
		p := NewParser(l)
		prog := p.ParseProgram()
		if len(p.Errors()) > 0 {
			t.Fatalf("unexpected parser errors for %q: %v", tt.code, p.Errors())
		}
		if len(prog.Body) != 1 {
			t.Fatalf("expected 1 statement, got %d", len(prog.Body))
		}
		vd, ok := prog.Body[0].(*ast.VarDecl)
		if !ok {
			t.Fatalf("expected VarDecl, got %T", prog.Body[0])
		}
		if vd.TypeAnnot != tt.expected {
			t.Errorf("for %q, expected TypeAnnot %q, got %q", tt.code, tt.expected, vd.TypeAnnot)
		}
	}

	// Verify exact formatting in error messages
	errTests := []struct {
		code          string
		expectedError string
	}{
		{`b: Dict[String, Int] = 5`, `invalid type annotation 'Dict[String, Int]'`},
		{`b: {String} = 5`, `invalid dictionary type annotation in '{String}'`},
	}

	for _, tt := range errTests {
		l := lexer.NewLexer(tt.code)
		p := NewParser(l)
		_ = p.ParseProgram()
		if len(p.Errors()) == 0 {
			t.Errorf("expected error for %q, got none", tt.code)
			continue
		}
		found := false
		for _, err := range p.Errors() {
			if strings.Contains(err, tt.expectedError) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("for %q, expected error containing %q, got %v", tt.code, tt.expectedError, p.Errors())
		}
	}
}

func TestParseBlockLiteralFollowedByStatement(t *testing.T) {
	input := `
map(Args) do |s|:
  print s, "\n"

print "done1\n"
print "done2\n"
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

	// stmt 0: CallExpr with Block
	es0, ok := prog.Body[0].(*ast.ExprStmt)
	if !ok {
		t.Fatalf("expected ExprStmt for stmt 0, got %T", prog.Body[0])
	}
	call, ok := es0.Value.(*ast.CallExpr)
	if !ok || call.Block == nil {
		t.Fatalf("expected CallExpr with Block, got %+v", es0.Value)
	}

	// stmt 1: CallExpr print "done1\n"
	es1, ok := prog.Body[1].(*ast.ExprStmt)
	if !ok {
		t.Fatalf("expected ExprStmt for stmt 1, got %T", prog.Body[1])
	}
	call1, ok := es1.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr for stmt 1, got %T (%+v)", es1.Value, es1.Value)
	}
	if id, ok := call1.Callee.(*ast.Identifier); !ok || id.Name != "print" {
		t.Fatalf("expected callee print, got %+v", call1.Callee)
	}

	// stmt 2: CallExpr print "done2\n"
	es2, ok := prog.Body[2].(*ast.ExprStmt)
	if !ok {
		t.Fatalf("expected ExprStmt for stmt 2, got %T", prog.Body[2])
	}
	call2, ok := es2.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr for stmt 2, got %T (%+v)", es2.Value, es2.Value)
	}
	if id, ok := call2.Callee.(*ast.Identifier); !ok || id.Name != "print" {
		t.Fatalf("expected callee print, got %+v", call2.Callee)
	}
}

func TestParseMemberAssignStmt(t *testing.T) {
	input := `self.x = 100 + y`
	l := lexer.NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(prog.Body) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Body))
	}
	mas, ok := prog.Body[0].(*ast.MemberAssignStmt)
	if !ok {
		t.Fatalf("expected MemberAssignStmt, got %T", prog.Body[0])
	}
	id, ok := mas.Target.(*ast.Identifier)
	if !ok || id.Name != "self" {
		t.Fatalf("expected target 'self', got %+v", mas.Target)
	}
	if mas.Member != "x" {
		t.Fatalf("expected member 'x', got %s", mas.Member)
	}
	if _, ok := mas.Value.(*ast.BinaryExpr); !ok {
		t.Fatalf("expected BinaryExpr for Value, got %T", mas.Value)
	}
}

func TestParseStructWithInit(t *testing.T) {
	input := `
struct Point:
    x: Float
    y: Float

    def init(r: Float, theta: Float):
        self.x = r
        self.y = theta
`
	l := lexer.NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(prog.Body) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Body))
	}
	sd, ok := prog.Body[0].(*ast.StructDef)
	if !ok {
		t.Fatalf("expected StructDef, got %T", prog.Body[0])
	}
	if sd.Name != "Point" {
		t.Fatalf("expected Point, got %s", sd.Name)
	}
	if len(sd.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(sd.Fields))
	}
	if len(sd.Methods) != 1 {
		t.Fatalf("expected 1 method, got %d", len(sd.Methods))
	}
	m := sd.Methods[0]
	if m.Name != "init" {
		t.Fatalf("expected method init, got %s", m.Name)
	}
	if len(m.Params) != 2 {
		t.Fatalf("expected 2 params in init, got %d", len(m.Params))
	}
	if len(m.Body) != 2 {
		t.Fatalf("expected 2 stmts in init body, got %d", len(m.Body))
	}
	for i, stmt := range m.Body {
		if _, ok := stmt.(*ast.MemberAssignStmt); !ok {
			t.Fatalf("expected MemberAssignStmt at body[%d], got %T", i, stmt)
		}
	}
}

func TestParseEmptyBlock(t *testing.T) {
	input := `
def noop():

struct S:
    def init():

    def run():

noop()
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

	fn, ok := prog.Body[0].(*ast.FuncDef)
	if !ok || fn.Name != "noop" || len(fn.Body) != 0 {
		t.Fatalf("expected empty FuncDef, got %+v", prog.Body[0])
	}

	sd, ok := prog.Body[1].(*ast.StructDef)
	if !ok || len(sd.Methods) != 2 {
		t.Fatalf("expected StructDef with 2 methods, got %+v", prog.Body[1])
	}
	if len(sd.Methods[0].Body) != 0 || len(sd.Methods[1].Body) != 0 {
		t.Fatalf("expected empty method bodies, got %v and %v", sd.Methods[0].Body, sd.Methods[1].Body)
	}
}

func TestParseForceExpr(t *testing.T) {
	input := `
x = force 1 + 2
y = force(3 * 4)
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

	vd1, ok := prog.Body[0].(*ast.VarDecl)
	if !ok {
		t.Fatalf("expected VarDecl, got %T", prog.Body[0])
	}
	bin, ok := vd1.Value.(*ast.BinaryExpr)
	if !ok || bin.Op != "+" {
		t.Fatalf("expected BinaryExpr (+), got %T (%+v)", vd1.Value, vd1.Value)
	}
	f1, ok := bin.Left.(*ast.ForceExpr)
	if !ok {
		t.Fatalf("expected ForceExpr for left, got %T", bin.Left)
	}
	if _, ok := f1.Expr.(*ast.IntLit); !ok {
		t.Fatalf("expected IntLit in force, got %T", f1.Expr)
	}

	vd2, ok := prog.Body[1].(*ast.VarDecl)
	if !ok {
		t.Fatalf("expected VarDecl, got %T", prog.Body[1])
	}
	f2, ok := vd2.Value.(*ast.ForceExpr)
	if !ok {
		t.Fatalf("expected ForceExpr, got %T", vd2.Value)
	}
	if _, ok := f2.Expr.(*ast.BinaryExpr); !ok {
		t.Fatalf("expected BinaryExpr in force(...), got %T", f2.Expr)
	}
}

func TestParseBitwiseOperators(t *testing.T) {
	input := `
v1 = ~x
v2 = a & b
v3 = a ^ b
v4 = a | b
v5 = a << b
v6 = a >> b
prec1 = a | b ^ c & d << e + f
prec2 = a & b == c
prec3 = a | b is Int
check = x is Int | String
lam = |x, y|: x | y
val = x |> ~
call = x |> &(0xff)
`
	p := NewParser(lexer.NewLexer(input))
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("unexpected parser errors: %v", p.Errors())
	}

	// v1 = ~x
	vd1 := prog.Body[0].(*ast.VarDecl)
	un, ok := vd1.Value.(*ast.UnaryExpr)
	if !ok || un.Op != "~" {
		t.Fatalf("expected UnaryExpr (~), got %T (%+v)", vd1.Value, vd1.Value)
	}

	// check binary operators
	ops := []string{"&", "^", "|", "<<", ">>"}
	for i, op := range ops {
		vd := prog.Body[i+1].(*ast.VarDecl)
		bin, ok := vd.Value.(*ast.BinaryExpr)
		if !ok || bin.Op != op {
			t.Fatalf("[%d] expected BinaryExpr (%s), got %T (%+v)", i, op, vd.Value, vd.Value)
		}
	}

	// prec1: a | (b ^ (c & (d << (e + f))))
	vdPrec1 := prog.Body[6].(*ast.VarDecl)
	binPipe, ok := vdPrec1.Value.(*ast.BinaryExpr)
	if !ok || binPipe.Op != "|" {
		t.Fatalf("expected top-level | in prec1, got %T (%+v)", vdPrec1.Value, vdPrec1.Value)
	}
	binCaret, ok := binPipe.Right.(*ast.BinaryExpr)
	if !ok || binCaret.Op != "^" {
		t.Fatalf("expected ^ on RHS of |, got %T (%+v)", binPipe.Right, binPipe.Right)
	}
	binAmp, ok := binCaret.Right.(*ast.BinaryExpr)
	if !ok || binAmp.Op != "&" {
		t.Fatalf("expected & on RHS of ^, got %T (%+v)", binCaret.Right, binCaret.Right)
	}
	binShift, ok := binAmp.Right.(*ast.BinaryExpr)
	if !ok || binShift.Op != "<<" {
		t.Fatalf("expected << on RHS of &, got %T (%+v)", binAmp.Right, binAmp.Right)
	}
	binPlus, ok := binShift.Right.(*ast.BinaryExpr)
	if !ok || binPlus.Op != "+" {
		t.Fatalf("expected + on RHS of <<, got %T (%+v)", binShift.Right, binShift.Right)
	}

	// prec2: (a & b) == c
	vdPrec2 := prog.Body[7].(*ast.VarDecl)
	binEq, ok := vdPrec2.Value.(*ast.BinaryExpr)
	if !ok || binEq.Op != "==" {
		t.Fatalf("expected == at top of prec2, got %T (%+v)", vdPrec2.Value, vdPrec2.Value)
	}
	if leftAmp, ok := binEq.Left.(*ast.BinaryExpr); !ok || leftAmp.Op != "&" {
		t.Fatalf("expected & on LHS of ==, got %T", binEq.Left)
	}

	// prec3: (a | b) is Int
	vdPrec3 := prog.Body[8].(*ast.VarDecl)
	isExpr, ok := vdPrec3.Value.(*ast.IsExpr)
	if !ok {
		t.Fatalf("expected IsExpr in prec3, got %T", vdPrec3.Value)
	}
	if leftPipe, ok := isExpr.Expr.(*ast.BinaryExpr); !ok || leftPipe.Op != "|" {
		t.Fatalf("expected | on LHS of is, got %T", isExpr.Expr)
	}

	// check: x is Int | String
	vdCheck := prog.Body[9].(*ast.VarDecl)
	isCheck, ok := vdCheck.Value.(*ast.IsExpr)
	if !ok || isCheck.TypeAnnot != "Int | String" {
		t.Fatalf("expected IsExpr with 'Int | String', got %+v", vdCheck.Value)
	}

	// lam: |x, y|: x | y
	vdLam := prog.Body[10].(*ast.VarDecl)
	lamExpr, ok := vdLam.Value.(*ast.LambdaExpr)
	if !ok || len(lamExpr.Params) != 2 {
		t.Fatalf("expected LambdaExpr with 2 params, got %+v", vdLam.Value)
	}
	lamBin, ok := lamExpr.Expr.(*ast.BinaryExpr)
	if !ok || lamBin.Op != "|" {
		t.Fatalf("expected | body in lambda, got %+v", lamExpr.Expr)
	}

	// val: x |> ~
	vdPipeTilde := prog.Body[11].(*ast.VarDecl)
	pipeUn, ok := vdPipeTilde.Value.(*ast.UnaryExpr)
	if !ok || pipeUn.Op != "~" {
		t.Fatalf("expected UnaryExpr (~) from |> ~, got %+v", vdPipeTilde.Value)
	}

	// call: x |> &(0xff) desugars to x & 0xff
	vdPipeCall := prog.Body[12].(*ast.VarDecl)
	pipeCallBin, ok := vdPipeCall.Value.(*ast.BinaryExpr)
	if !ok || pipeCallBin.Op != "&" {
		t.Fatalf("expected BinaryExpr (&) from |> &(0xff), got %+v", vdPipeCall.Value)
	}
}

func TestParseBitwiseOperatorMethods(t *testing.T) {
	input := `
struct Flags:
    mask: Int
    def ~(self) -> Flags:
        Flags(mask: ~self.mask)
    def &(self, other: Flags) -> Flags:
        Flags(mask: self.mask & other.mask)
    def <<(self, n: Int) -> Flags:
        Flags(mask: self.mask << n)

interface Bitwise:
    ~(self): Bitwise
    &(self, other: Bitwise): Bitwise
    <<(self, n: Int): Bitwise
`
	p := NewParser(lexer.NewLexer(input))
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("unexpected parser errors: %v", p.Errors())
	}

	st, ok := prog.Body[0].(*ast.StructDef)
	if !ok {
		t.Fatalf("expected StructDef, got %T", prog.Body[0])
	}
	if len(st.Methods) != 3 {
		t.Fatalf("expected 3 methods, got %d", len(st.Methods))
	}
	if st.Methods[0].Name != "~" || st.Methods[1].Name != "&" || st.Methods[2].Name != "<<" {
		t.Fatalf("unexpected method names: %s, %s, %s", st.Methods[0].Name, st.Methods[1].Name, st.Methods[2].Name)
	}

	it, ok := prog.Body[1].(*ast.InterfaceDef)
	if !ok {
		t.Fatalf("expected InterfaceDef, got %T", prog.Body[1])
	}
	if len(it.Methods) != 3 {
		t.Fatalf("expected 3 interface methods, got %d", len(it.Methods))
	}
	if it.Methods[0].Name != "~" || it.Methods[1].Name != "&" || it.Methods[2].Name != "<<" {
		t.Fatalf("unexpected method names: %s, %s, %s", it.Methods[0].Name, it.Methods[1].Name, it.Methods[2].Name)
	}
}

func TestParsePipelineIntoLambda(t *testing.T) {
	input := `
v1 = x |> |v|: v + 1
v2 = x |> |v: Int|: v * 2
v3 = x |> ||: 42
v4 = x |> | |: 42
v5 = x |> |(0xff)
`
	p := NewParser(lexer.NewLexer(input))
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("unexpected parser errors: %v", p.Errors())
	}

	// v1 = x |> |v|: v + 1 => CallExpr with Lambda callee
	vd1 := prog.Body[0].(*ast.VarDecl)
	call1, ok := vd1.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr for v1, got %T", vd1.Value)
	}
	if _, ok := call1.Callee.(*ast.LambdaExpr); !ok {
		t.Fatalf("expected LambdaExpr callee for v1, got %T", call1.Callee)
	}

	// v2 = x |> |v: Int|: v * 2 => CallExpr with Lambda callee
	vd2 := prog.Body[1].(*ast.VarDecl)
	call2, ok := vd2.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr for v2, got %T", vd2.Value)
	}
	if lam2, ok := call2.Callee.(*ast.LambdaExpr); !ok || len(lam2.Params) != 1 || lam2.Params[0].TypeAnnot != "Int" {
		t.Fatalf("expected LambdaExpr with Typed param for v2, got %+v", call2.Callee)
	}

	// v5 = x |> |(0xff) => BinaryExpr with Op "|"
	vd5 := prog.Body[4].(*ast.VarDecl)
	bin5, ok := vd5.Value.(*ast.BinaryExpr)
	if !ok || bin5.Op != "|" {
		t.Fatalf("expected BinaryExpr (|) for v5, got %T (%+v)", vd5.Value, vd5.Value)
	}
}

func TestParseUserDefinedGenerics(t *testing.T) {
	code := `
def id[T](x: T) -> T:
    x

def pair[T: Int | String, U](a: T, b: U) -> (T, U):
    (a, b)

struct Box[T]:
    val: T
    def map[U](self, fn: (T) -> U) -> Box[U]:
        Box(fn(self.val))

interface Iterator[T]:
    Next(): T | Nil
    Map[U](fn: (T) -> U): Iterator[U]

struct Derived[T]:
    Box[T]
    tag: String

b1 = Box[Int](42)
p1 = pair[Int, String](10, "hello")
f1 = id[() -> Int]()
`
	l := lexer.NewLexer(code)
	p := NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("unexpected parser errors: %v", p.Errors())
	}

	// 1. def id[T](x: T) -> T
	f1, ok := prog.Body[0].(*ast.FuncDef)
	if !ok || f1.Name != "id" || len(f1.TypeParams) != 1 || f1.TypeParams[0].Name != "T" {
		t.Fatalf("expected generic func id[T], got %+v", prog.Body[0])
	}
	if f1.TypeParams[0].Constraint != "" {
		t.Errorf("expected empty constraint for T, got %q", f1.TypeParams[0].Constraint)
	}

	// 2. def pair[T: Int | String, U](...)
	f2, ok := prog.Body[1].(*ast.FuncDef)
	if !ok || f2.Name != "pair" || len(f2.TypeParams) != 2 {
		t.Fatalf("expected generic func pair[T, U], got %+v", prog.Body[1])
	}
	if f2.TypeParams[0].Name != "T" || f2.TypeParams[0].Constraint != "Int | String" {
		t.Errorf("expected T: Int | String, got %+v", f2.TypeParams[0])
	}
	if f2.TypeParams[1].Name != "U" || f2.TypeParams[1].Constraint != "" {
		t.Errorf("expected U unconstrained, got %+v", f2.TypeParams[1])
	}

	// 3. struct Box[T]
	s1, ok := prog.Body[2].(*ast.StructDef)
	if !ok || s1.Name != "Box" || len(s1.TypeParams) != 1 || s1.TypeParams[0].Name != "T" {
		t.Fatalf("expected generic struct Box[T], got %+v", prog.Body[2])
	}
	if len(s1.Methods) != 1 || len(s1.Methods[0].TypeParams) != 1 || s1.Methods[0].TypeParams[0].Name != "U" {
		t.Fatalf("expected method map[U] on Box, got %+v", s1.Methods)
	}

	// 4. interface Iterator[T]
	i1, ok := prog.Body[3].(*ast.InterfaceDef)
	if !ok || i1.Name != "Iterator" || len(i1.TypeParams) != 1 || i1.TypeParams[0].Name != "T" {
		t.Fatalf("expected generic interface Iterator[T], got %+v", prog.Body[3])
	}
	if len(i1.Methods) != 2 {
		t.Fatalf("expected 2 methods on Iterator, got %d", len(i1.Methods))
	}
	if i1.Methods[1].Name != "Map" || len(i1.Methods[1].TypeParams) != 1 || i1.Methods[1].TypeParams[0].Name != "U" {
		t.Fatalf("expected method Map[U] on Iterator, got %+v", i1.Methods[1])
	}

	// 5. struct Derived[T]: Box[T]
	s2, ok := prog.Body[4].(*ast.StructDef)
	if !ok || s2.Name != "Derived" || len(s2.Fields) != 2 {
		t.Fatalf("expected struct Derived[T], got %+v", prog.Body[4])
	}
	if !s2.Fields[0].IsEmbedded || s2.Fields[0].TypeAnnot != "Box[T]" {
		t.Errorf("expected embedded Box[T], got %+v", s2.Fields[0])
	}

	// 6. b1 = Box[Int](42) => CallExpr with IndexExpr callee (TypeArg = "Int")
	vd1, ok := prog.Body[5].(*ast.VarDecl)
	if !ok {
		t.Fatalf("expected VarDecl for b1, got %+v", prog.Body[5])
	}
	call1, ok := vd1.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr for b1, got %+v", vd1.Value)
	}
	idx1, ok := call1.Callee.(*ast.IndexExpr)
	if !ok || idx1.TypeArg != "Int" {
		t.Fatalf("expected IndexExpr callee with TypeArg 'Int', got %+v", call1.Callee)
	}

	// 7. p1 = pair[Int, String](10, "hello") => CallExpr with TypeInstExpr callee
	vd2, ok := prog.Body[6].(*ast.VarDecl)
	if !ok {
		t.Fatalf("expected VarDecl for p1, got %+v", prog.Body[6])
	}
	call2, ok := vd2.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr for p1, got %+v", vd2.Value)
	}
	inst2, ok := call2.Callee.(*ast.TypeInstExpr)
	if !ok || len(inst2.TypeArgs) != 2 || inst2.TypeArgs[0] != "Int" || inst2.TypeArgs[1] != "String" {
		t.Fatalf("expected TypeInstExpr callee with [Int, String], got %+v", call2.Callee)
	}

	// 8. f1 = id[() -> Int]() => CallExpr with TypeInstExpr callee (function type arg)
	vd3, ok := prog.Body[7].(*ast.VarDecl)
	if !ok {
		t.Fatalf("expected VarDecl for f1, got %+v", prog.Body[7])
	}
	call3, ok := vd3.Value.(*ast.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr for f1, got %+v", vd3.Value)
	}
	inst3, ok := call3.Callee.(*ast.TypeInstExpr)
	if !ok || len(inst3.TypeArgs) != 1 || inst3.TypeArgs[0] != "() -> Int" {
		t.Fatalf("expected TypeInstExpr callee with [() -> Int], got %+v", call3.Callee)
	}
}

func TestParseGenericErrors(t *testing.T) {
	errCases := []struct {
		code        string
		expectedErr string
	}{
		{"def f[](x): 1", "type parameter list cannot be empty"},
		{"struct Box[]:\n    x: Int", "type parameter list cannot be empty"},
		{"interface Iter[]:\n    Next(): Int", "type parameter list cannot be empty"},
		{"def f[T, T](x: T): x", "duplicate type parameter 'T'"},
		{"struct Box[T, T]:\n    x: T", "duplicate type parameter 'T'"},
		{"x = Box[]()", "bracket expression cannot be empty"},
	}

	for _, tt := range errCases {
		l := lexer.NewLexer(tt.code)
		p := NewParser(l)
		_ = p.ParseProgram()
		if len(p.Errors()) == 0 {
			t.Errorf("expected error for %q, got none", tt.code)
			continue
		}
		found := false
		for _, err := range p.Errors() {
			if strings.Contains(err, tt.expectedErr) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected error containing %q for %q, got %v", tt.expectedErr, tt.code, p.Errors())
		}
	}
}




