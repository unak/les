package types

import (
	"testing"

	"les/pkg/lexer"
	"les/pkg/parser"
)

func TestTypeInference(t *testing.T) {
	input := `x = 10
y: Int = 20
def add(a, b):
    a + b`

	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	prog := p.ParseProgram()

	inf := NewInferencer()
	err := inf.InferProgram(prog)
	if err != nil {
		t.Fatalf("type inference error: %v", err)
	}

	xType, ok := inf.scope.Lookup("x")
	if !ok || xType != TypeInt {
		t.Fatalf("expected x to be Int, got %v", xType)
	}
	yType, ok := inf.scope.Lookup("y")
	if !ok || yType != TypeInt {
		t.Fatalf("expected y to be Int, got %v", yType)
	}
}

func TestTypeMismatch(t *testing.T) {
	input := `x: Int = "string"`
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	prog := p.ParseProgram()

	inf := NewInferencer()
	err := inf.InferProgram(prog)
	if err == nil {
		t.Fatalf("expected type mismatch error, got nil")
	}
}

func TestSameScopeDuplicateDeclarationError(t *testing.T) {
	input := `x = 10
x = 20`
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	prog := p.ParseProgram()

	inf := NewInferencer()
	err := inf.InferProgram(prog)
	if err == nil {
		t.Fatalf("expected duplicate declaration error, got nil")
	}
}

func TestNestedScopeShadowingAllowed(t *testing.T) {
	input := `x = 10
def f():
    x = "shadowed"
    x`
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	prog := p.ParseProgram()

	inf := NewInferencer()
	err := inf.InferProgram(prog)
	if err != nil {
		t.Fatalf("expected nested shadowing to succeed, got error: %v", err)
	}
}

func TestInfiniteTupleTypeAnnotation(t *testing.T) {
	input := `t: (Int, ...) = (1, 2, 3)`
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	prog := p.ParseProgram()

	inf := NewInferencer()
	err := inf.InferProgram(prog)
	if err != nil {
		t.Fatalf("expected (Int, ...) annotation to succeed, got %v", err)
	}
	tt, ok := inf.scope.Lookup("t")
	if !ok {
		t.Fatalf("variable t not found")
	}
	tType, ok := tt.(*TupleType)
	if !ok || !tType.IsVariadic || len(tType.Elems) != 1 || tType.Elems[0] != TypeInt {
		t.Fatalf("expected variadic (Int, ...), got %v", tt)
	}
}

func TestUnionTypeAnnotation(t *testing.T) {
	input := `
x: Int | String = 42
y: Int | String = "hello"
`
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	prog := p.ParseProgram()

	inf := NewInferencer()
	err := inf.InferProgram(prog)
	if err != nil {
		t.Fatalf("expected Int | String annotation to succeed, got %v", err)
	}
}

func TestTernaryUnionInference(t *testing.T) {
	input := `
res = True ? 42 : "hello"
val: Int | String = res
`
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	prog := p.ParseProgram()

	inf := NewInferencer()
	err := inf.InferProgram(prog)
	if err != nil {
		t.Fatalf("expected ternary union inference to succeed, got %v", err)
	}
}

func TestAssignableAnyAndNil(t *testing.T) {
	// Any is a safe top type: anything is assignable to Any, but Any is not assignable to Int
	if !IsAssignableTo(TypeInt, TypeAny) {
		t.Errorf("expected Int to be assignable to Any")
	}
	if !IsAssignableTo(TypeNil, TypeAny) {
		t.Errorf("expected Nil to be assignable to Any")
	}
	if IsAssignableTo(TypeAny, TypeInt) {
		t.Errorf("Any should NOT be assignable to Int without narrowing")
	}

	// Nil with Union
	nilOrInt := NewUnionType(TypeInt, TypeNil)
	if !IsAssignableTo(TypeNil, nilOrInt) {
		t.Errorf("expected Nil to be assignable to Int | Nil")
	}
	if !IsAssignableTo(TypeInt, nilOrInt) {
		t.Errorf("expected Int to be assignable to Int | Nil")
	}
	if IsAssignableTo(TypeString, nilOrInt) {
		t.Errorf("expected String NOT to be assignable to Int | Nil")
	}
}

func TestLambdaTypeInference(t *testing.T) {
	input := `
f = |x: Int|: x + 1
res = f(10)
`
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	prog := p.ParseProgram()

	inf := NewInferencer()
	err := inf.InferProgram(prog)
	if err != nil {
		t.Fatalf("expected lambda type inference to succeed, got %v", err)
	}
}

func TestStructTypeInference(t *testing.T) {
	input := `
struct Person:
    name: String
    age: Int

    def Greet():
        "hello"

struct Employee:
    Person
    salary: Int

p: Person = Person(name: "Alice", age: 30)
pName = p.name
pGreet = p.Greet()

e: Employee = Employee(salary: 50000)
eName = e.name # promoted from Person
`
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	inf := NewInferencer()
	err := inf.InferProgram(prog)
	if err != nil {
		t.Fatalf("type inference error: %v", err)
	}

	pNameT, _ := inf.scope.Lookup("pName")
	if pNameT != TypeString {
		t.Fatalf("expected pName to be String, got %v", pNameT)
	}

	eNameT, _ := inf.scope.Lookup("eName")
	if eNameT != TypeString {
		t.Fatalf("expected eName to be String (promoted), got %v", eNameT)
	}
}

func TestAmbiguousSelectorStaticError(t *testing.T) {
	// Case 1: Ambiguous selector without override should fail statically!
	inputAmbiguous := `
struct A:
    def Talk():
        "from A"

struct B:
    def Talk():
        "from B"

struct C:
    A
    B

c: C = C()
res = c.Talk()
`
	pAmb := parser.NewParser(lexer.NewLexer(inputAmbiguous))
	progAmb := pAmb.ParseProgram()
	if len(pAmb.Errors()) > 0 {
		t.Fatalf("unexpected parse errors: %v", pAmb.Errors())
	}

	infAmb := NewInferencer()
	errAmb := infAmb.InferProgram(progAmb)
	if errAmb == nil {
		t.Fatalf("expected static error for ambiguous selector 'Talk', got nil")
	}

	// Case 2: Overridden selector in C should resolve cleanly without error!
	inputResolved := `
struct A:
    def Talk():
        "from A"

struct B:
    def Talk():
        "from B"

struct C:
    A
    B

    def Talk():
        "from C"

c: C = C()
res = c.Talk()
`
	pRes := parser.NewParser(lexer.NewLexer(inputResolved))
	progRes := pRes.ParseProgram()
	if len(pRes.Errors()) > 0 {
		t.Fatalf("unexpected parse errors: %v", pRes.Errors())
	}

	infRes := NewInferencer()
	errRes := infRes.InferProgram(progRes)
	if errRes != nil {
		t.Fatalf("expected override to resolve ambiguity cleanly, got error: %v", errRes)
	}
}


