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

	if inf.env["x"] != TypeInt {
		t.Fatalf("expected x to be Int, got %v", inf.env["x"])
	}
	if inf.env["y"] != TypeInt {
		t.Fatalf("expected y to be Int, got %v", inf.env["y"])
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
