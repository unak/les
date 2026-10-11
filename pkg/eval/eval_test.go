package eval

import (
	"fmt"
	"strings"
	"testing"

	"les/pkg/lexer"
	"les/pkg/parser"
)

func runLesCode(code string) (string, error) {
	l := lexer.NewLexer(code)
	p := parser.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		return "", fmt.Errorf("%s", p.Errors()[0])
	}

	ev := NewEvaluator()
	var out strings.Builder
	ev.SetOutput(&out)
	_, err := ev.EvalProgram(prog)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

// MVP Test 1: Basic expressions & print driving demand
func TestMVP1_PrintAndExpressions(t *testing.T) {
	code := `x = 10 + 20
print x`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if out != "30" {
		t.Fatalf("expected '30', got %q", out)
	}
}

// MVP Test 2: Lazy Fibonacci stream and take n elements
func TestMVP2_LazyFibonacciStream(t *testing.T) {
	code := `def fib(a, b):
    (a, fib(b, a + b))

def take(n, s):
    n <= 0 ? () : (s[0], take(n - 1, s[1]))

print take(5, fib(0, 1))`

	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	// Expected: (0, (1, (1, (2, (3, ())))))
	expected := "(0, (1, (1, (2, (3, ())))))"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

// MVP Test 3: Lazy parameter evaluation (unused divergent/error arguments are never computed)
func TestMVP3_LazyUnusedArguments(t *testing.T) {
	code := `def choose(first, second):
    first

# 1 / 0 is a division by zero error if evaluated.
# Because second is unused, it must NOT evaluate or panic.
print choose(42, 1 / 0)`

	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if out != "42" {
		t.Fatalf("expected '42', got %q", out)
	}
}

func TestModuleAccessEvaluation(t *testing.T) {
	ev := NewEvaluator()
	var out strings.Builder
	ev.SetOutput(&out)

	// Register dummy module 'math' with public Add and private sub
	mathMod := &ModuleValue{
		Name: "math",
		Exports: map[string]Value{
			"Add": &BuiltinFunc{
				Name: "Add",
				Fn: func(args []Value) (Value, error) {
					a := Force(args[0]).(IntValue)
					b := Force(args[1]).(IntValue)
					return a + b, nil
				},
			},
			"sub": &BuiltinFunc{
				Name: "sub",
				Fn: func(args []Value) (Value, error) {
					return IntValue(0), nil
				},
			},
		},
	}
	ev.globalEnv.RegisterModule(mathMod)

	// 1. Access public member @math.Add
	code := `print @math.Add(10, 20)`
	l := lexer.NewLexer(code)
	p := parser.NewParser(l)
	prog := p.ParseProgram()
	_, err := ev.EvalProgram(prog)
	if err != nil {
		t.Fatalf("eval public module member failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != "30" {
		t.Fatalf("expected 30, got %q", out.String())
	}

	// 2. Access private member @math.sub should fail with error
	codeErr := `print @math.sub(10, 20)`
	l2 := lexer.NewLexer(codeErr)
	p2 := parser.NewParser(l2)
	prog2 := p2.ParseProgram()
	_, err2 := ev.EvalProgram(prog2)
	if err2 == nil {
		t.Fatalf("expected error accessing private member @math.sub, got nil")
	}
}

func TestEvalIsExpr(t *testing.T) {
	code := `
x = 42
y = "hello"
r1 = (x is Int) ? 1 : 0
r2 = (y is Int) ? 1 : 0
r3 = (x is Int | String) ? 1 : 0
print r1, r2, r3
`
	var out strings.Builder
	ev := NewEvaluator()
	ev.SetOutput(&out)

	l := lexer.NewLexer(code)
	p := parser.NewParser(l)
	prog := p.ParseProgram()
	_, err := ev.EvalProgram(prog)
	if err != nil {
		t.Fatalf("eval is expr failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != "1 0 1" {
		t.Fatalf("expected '1 0 1', got %q", out.String())
	}
}

func TestEvalMatchExpr(t *testing.T) {
	code := `
def classify(v):
    match v:
        case Int:
            print "number:", v
        case String:
            print "string:", v
        case _:
            print "other"

classify 100
classify "les"
classify True
`
	var out strings.Builder
	ev := NewEvaluator()
	ev.SetOutput(&out)

	l := lexer.NewLexer(code)
	p := parser.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	_, err := ev.EvalProgram(prog)
	if err != nil {
		t.Fatalf("eval match expr failed: %v", err)
	}
	expected := "number: 100\nstring: les\nother"
	if strings.TrimSpace(out.String()) != expected {
		t.Fatalf("expected %q, got %q", expected, out.String())
	}
}

func TestEvalNilAndAny(t *testing.T) {
	code := `
x = Nil
print x is Nil, x is Any, 42 is Any, 42 is Nil

match x:
    case Nil:
        print "got nil"
    case _:
        print "other"
`
	var out strings.Builder
	ev := NewEvaluator()
	ev.SetOutput(&out)

	l := lexer.NewLexer(code)
	p := parser.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	_, err := ev.EvalProgram(prog)
	if err != nil {
		t.Fatalf("eval nil/any failed: %v", err)
	}
	expected := "True True True False\ngot nil"
	if strings.TrimSpace(out.String()) != expected {
		t.Fatalf("expected %q, got %q", expected, out.String())
	}
}

