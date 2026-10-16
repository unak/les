package types

import (
	"testing"

	"les/pkg/lexer"
	"les/pkg/parser"
)

func TestTrailingDefaultsAndBlockParamInfer(t *testing.T) {
	code := `
def greet(name: String, prefix: String = "Hello", &block = nil): String:
    if block != nil:
        return block(name)
    prefix + " " + name

a = greet("Alice")
b = greet("Bob", "Hi")
c = greet("Carol") do |n|:
    "Hey " + n
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type error: %v", err)
	}

	badArity := `
def f(a: Int, b: Int = 10): Int:
    a + b

x = f()
`
	progBad := parser.NewParser(lexer.NewLexer(badArity)).ParseProgram()
	if err := NewInferencer().InferProgram(progBad); err == nil {
		t.Fatalf("expected arity error, got nil")
	}

	badBlock := `
def need_block(x: Int, &block): Int:
    block(x)

y = need_block(10)
`
	progBadBlock := parser.NewParser(lexer.NewLexer(badBlock)).ParseProgram()
	if err := NewInferencer().InferProgram(progBadBlock); err == nil {
		t.Fatalf("expected block required error, got nil")
	}

	badDefaultType := `
def invalid_default(x: Int = "not an int"): Int:
    x
`
	progBadDefault := parser.NewParser(lexer.NewLexer(badDefaultType)).ParseProgram()
	if err := NewInferencer().InferProgram(progBadDefault); err == nil {
		t.Fatalf("expected default value type error, got nil")
	}
}

func TestBuiltinBytesAndExitWithDefaults(t *testing.T) {
	code := `
b1 = Bytes()
b2 = Bytes("hello")
b3 = Bytes(b2)
exit()
exit(0)
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type error: %v", err)
	}

	badBytes := `b = Bytes(1, 2)`
	progBadBytes := parser.NewParser(lexer.NewLexer(badBytes)).ParseProgram()
	if err := NewInferencer().InferProgram(progBadBytes); err == nil {
		t.Fatalf("expected arity error for Bytes(1, 2), got nil")
	}

	badExit := `exit(1, 2)`
	progBadExit := parser.NewParser(lexer.NewLexer(badExit)).ParseProgram()
	if err := NewInferencer().InferProgram(progBadExit); err == nil {
		t.Fatalf("expected arity error for exit(1, 2), got nil")
	}
}

func TestStructConstructorDefaultsInfer(t *testing.T) {
	code := `
struct Config:
    host: String
    port: Int = 8080

c1 = Config("localhost")
c2 = Config("remote", 9000)
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type error: %v", err)
	}

	badArity := `
struct Config:
    host: String
    port: Int = 8080

c = Config()
`
	progBad := parser.NewParser(lexer.NewLexer(badArity)).ParseProgram()
	if err := NewInferencer().InferProgram(progBad); err == nil {
		t.Fatalf("expected struct constructor arity error, got nil")
	}
}

