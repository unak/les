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
