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


