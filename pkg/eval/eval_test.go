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
	expected := "101"
	if strings.TrimSpace(out.String()) != expected {
		t.Fatalf("expected %q, got %q", expected, out.String())
	}
}

func TestEvalMatchExpr(t *testing.T) {
	code := `
def classify(v):
    match v:
        case Int:
            print "number:", v
            print "\n"
        case String:
            print "string:", v
            print "\n"
        case _:
            print "other"
            print "\n"

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
	expected := "number:100\nstring:les\nother"
	if strings.TrimSpace(out.String()) != expected {
		t.Fatalf("expected %q, got %q", expected, out.String())
	}
}

func TestEvalNilAndAny(t *testing.T) {
	code := `
x = Nil
print x is Nil, x is Any, 42 is Any, 42 is Nil
print "\n"

match x:
    case Nil:
        print "got nil"
        print "\n"
    case _:
        print "other"
        print "\n"
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
	expected := "TrueTrueTrueTrue\ngot nil"
	if strings.TrimSpace(out.String()) != expected {
		t.Fatalf("expected %q, got %q", expected, out.String())
	}
}

func TestEvalVariadicFunction(t *testing.T) {
	code := `
def my_print(prefix: String, *args: Any):
    print prefix, args
    print "\n"

my_print "zero:"
my_print "two:", 1, 2
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
		t.Fatalf("eval variadic func failed: %v", err)
	}
	expected := "zero:()\ntwo:(1, 2)"
	if strings.TrimSpace(out.String()) != expected {
		t.Fatalf("expected %q, got %q", expected, out.String())
	}
}

func TestEvalTuplePatternMatching(t *testing.T) {
	code := `
def fib(a: Int, b: Int):
    (a, fib(b, a + b))

s = fib(0, 1)

match s:
    case (first, rest):
        print "first:", first
        print "\n"
        match rest:
            case (second, rest2):
                print "second:", second
                print "\n"
                match rest2:
                    case (third, _):
                        print "third:", third
                        print "\n"

nums = (10, 20, 30, 40)
match nums:
    case (h, second, ...tail):
        print "head:", h, "second:", second, "tail:", tail
        print "\n"
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
		t.Fatalf("eval tuple pattern matching failed: %v", err)
	}
	expected := "first:0\nsecond:1\nthird:1\nhead:10second:20tail:(30, 40)"
	if strings.TrimSpace(out.String()) != expected {
		t.Fatalf("expected %q, got %q", expected, out.String())
	}
}

func TestEvalUnpackAssignment(t *testing.T) {
	code := `
def fib(a: Int, b: Int):
    (a, fib(b, a + b))

s = fib(0, 1)

# Paren unpacked assignment on stream
(f0, s1) = s
(f1, s2) = s1
(f2, _) = s2
print "fibs:", f0, f1, f2
print "\n"

# Splat unpack assignment without parens
h, ...tail = (100, 200, 300)
print "splat:", h, tail
print "\n"
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
		t.Fatalf("eval unpack assignment failed: %v", err)
	}
	expected := "fibs:011\nsplat:100(200, 300)"
	if strings.TrimSpace(out.String()) != expected {
		t.Fatalf("expected %q, got %q", expected, out.String())
	}
}

func TestEvalTypeBuiltin(t *testing.T) {
	code := `
print type(42)
print "\n"
print type((1,))
print "\n"
print type(1, 2)
print "\n"
print type((1, 2))
print "\n"
print type("les")
print "\n"
print type(True)
print "\n"
print type(Nil)
print "\n"
print type()
print "\n"
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
		t.Fatalf("eval type builtin failed: %v", err)
	}
	expected := "Int\n(Int)\n(Int, Int)\n(Int, Int)\nString\nBool\nNil\n()"
	if strings.TrimSpace(out.String()) != expected {
		t.Fatalf("expected %q, got %q", expected, out.String())
	}
}

func TestPutsFunction(t *testing.T) {
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
puts
puts "hello"
puts "already\n"
puts "a", "b\n", 42
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
		t.Fatalf("eval puts failed: %v", err)
	}
	expected := "\n\nhello\nalready\na\nb\n42\n"
	if out.String() != expected {
		t.Fatalf("expected %q, got %q", expected, out.String())
	}
}

func TestFirstClassAndHigherOrderFunctions(t *testing.T) {
	code := `
def double(x):
    x * 2

# Assign function to variable
f = double
res = f(21)
print "f(21)=", res, "\n"

# Higher-order function
def apply(fn, val):
    fn(val)

res2 = apply(f, 100)
print "apply=", res2, "\n"
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
		t.Fatalf("eval higher order failed: %v", err)
	}
	expected := "f(21)=42\napply=200\n"
	if out.String() != expected {
		t.Fatalf("expected %q, got %q", expected, out.String())
	}
}

func TestEvalLambdaExpr(t *testing.T) {
	code := `
# 1-line lambda
inc = |x|: x + 1
print inc(41), "\n"

# multi-line lambda block
complex_fn = |x|:
    y = x * 2
    y + 10
print complex_fn(5), "\n"

# 0-arg lambda
thunk = ||: "magic"
print thunk(), "\n"

# passed directly into higher-order function
def run(fn, a, b):
    fn(a, b)

print run(|x, y|: x * y, 6, 7), "\n"

# closure capturing outer variable
base = 100
adder = |x|: base + x
print adder(23), "\n"
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
		t.Fatalf("eval lambda failed: %v", err)
	}
	expected := "42\n20\nmagic\n42\n123\n"
	if out.String() != expected {
		t.Fatalf("expected %q, got %q", expected, out.String())
	}
}

func TestExitBuiltin(t *testing.T) {
	testCases := []struct {
		name     string
		code     string
		wantCode int
		wantOut  string
	}{
		{
			name:     "bare exit identifier",
			code:     "exit",
			wantCode: 0,
		},
		{
			name:     "exit call 0 args",
			code:     "exit()",
			wantCode: 0,
		},
		{
			name:     "exit with int",
			code:     "exit 42",
			wantCode: 42,
		},
		{
			name:     "exit with True",
			code:     "exit True",
			wantCode: 0,
		},
		{
			name:     "exit with False",
			code:     "exit False",
			wantCode: 1,
		},
		{
			name: "exit halts execution immediately",
			code: `
print "before\n"
exit 7
print "after\n"
`,
			wantCode: 7,
			wantOut:  "before\n",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			ev := NewEvaluator()
			ev.SetOutput(&out)

			l := lexer.NewLexer(tc.code)
			p := parser.NewParser(l)
			prog := p.ParseProgram()
			if len(p.Errors()) > 0 {
				t.Fatalf("parser errors: %v", p.Errors())
			}

			_, err := ev.EvalProgram(prog)
			if err == nil {
				t.Fatalf("expected ExitError, got nil")
			}
			exitErr, ok := err.(*ExitError)
			if !ok {
				t.Fatalf("expected *ExitError, got %T (%v)", err, err)
			}
			if exitErr.Code != tc.wantCode {
				t.Fatalf("expected exit code %d, got %d", tc.wantCode, exitErr.Code)
			}
			if tc.wantOut != "" && out.String() != tc.wantOut {
				t.Fatalf("expected output %q, got %q", tc.wantOut, out.String())
			}
		})
	}
}

func TestReturnInFunction(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		wantVal string
	}{
		{
			name: "return with value",
			code: `
def foo():
    return 42
    100
foo()
`,
			wantVal: "42",
		},
		{
			name: "return without value returns empty tuple",
			code: `
def bar():
    return
    100
bar()
`,
			wantVal: "()",
		},
		{
			name: "early return inside if",
			code: `
def check(x):
    if x > 0:
        return "positive"
    "non-positive"
(check(5), check(-2))
`,
			wantVal: `(positive, non-positive)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lexer.NewLexer(tt.code)
			p := parser.NewParser(l)
			prog := p.ParseProgram()
			if len(p.Errors()) > 0 {
				t.Fatalf("parser errors: %v", p.Errors())
			}

			ev := NewEvaluator()
			val, err := ev.EvalProgram(prog)
			if err != nil {
				t.Fatalf("eval error: %v", err)
			}
			if val.String() != tt.wantVal {
				t.Fatalf("expected %q, got %q", tt.wantVal, val.String())
			}
		})
	}
}

func TestBreakInClosure(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		wantVal string
	}{
		{
			name: "break with value in lambda",
			code: `
f = |x|:
    if x == 1:
        break 100
    200
(f(1), f(2))
`,
			wantVal: "(100, 200)",
		},
		{
			name: "break without value",
			code: `
f = ||:
    break
f()
`,
			wantVal: "()",
		},
		{
			name: "nested closures inner break",
			code: `
outer = ||:
    inner = ||:
        break "inner"
    (inner(), "outer")
outer()
`,
			wantVal: `(inner, outer)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lexer.NewLexer(tt.code)
			p := parser.NewParser(l)
			prog := p.ParseProgram()
			if len(p.Errors()) > 0 {
				t.Fatalf("parser errors: %v", p.Errors())
			}

			ev := NewEvaluator()
			val, err := ev.EvalProgram(prog)
			if err != nil {
				t.Fatalf("eval error: %v", err)
			}
			if val.String() != tt.wantVal {
				t.Fatalf("expected %q, got %q", tt.wantVal, val.String())
			}
		})
	}
}

func TestNonLocalReturn(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		wantVal string
	}{
		{
			name: "return inside closure exits enclosing def",
			code: `
def outer():
    fn = |x|:
        if x == 1:
            return 999
        0
    fn(1)
    100
outer()
`,
			wantVal: "999",
		},
		{
			name: "non-local return passes through higher-order function",
			code: `
def runTwice(f):
    f(1)
    f(2)
    10

def caller():
    runTwice() do |x|:
        if x == 1:
            return 777
        0
    20
caller()
`,
			wantVal: "777",
		},
		{
			name: "nested lambda non-local return to def",
			code: `
def test():
    f1 = ||:
        f2 = ||:
            return 42
        f2()
        1
    f1()
    2
test()
`,
			wantVal: "42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lexer.NewLexer(tt.code)
			p := parser.NewParser(l)
			prog := p.ParseProgram()
			if len(p.Errors()) > 0 {
				t.Fatalf("parser errors: %v", p.Errors())
			}

			ev := NewEvaluator()
			val, err := ev.EvalProgram(prog)
			if err != nil {
				t.Fatalf("eval error: %v", err)
			}
			if val.String() != tt.wantVal {
				t.Fatalf("expected %q, got %q", tt.wantVal, val.String())
			}
		})
	}
}

func TestTopLevelReturn(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		wantCode int
	}{
		{
			name:     "bare return is exit 0",
			code:     "return\n",
			wantCode: 0,
		},
		{
			name:     "return with int code",
			code:     "return 42\n",
			wantCode: 42,
		},
		{
			name:     "return True is exit 0",
			code:     "return True\n",
			wantCode: 0,
		},
		{
			name:     "return False is exit 1",
			code:     "return False\n",
			wantCode: 1,
		},
		{
			name: "top-level lambda non-local return acts as exit",
			code: `
f = ||:
    return 123
f()
`,
			wantCode: 123,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lexer.NewLexer(tt.code)
			p := parser.NewParser(l)
			prog := p.ParseProgram()
			if len(p.Errors()) > 0 {
				t.Fatalf("parser errors: %v", p.Errors())
			}

			ev := NewEvaluator()
			_, err := ev.EvalProgram(prog)
			if err == nil {
				t.Fatalf("expected ExitError, got nil")
			}
			exitErr, ok := err.(*ExitError)
			if !ok {
				t.Fatalf("expected *ExitError, got %T (%v)", err, err)
			}
			if exitErr.Code != tt.wantCode {
				t.Fatalf("expected exit code %d, got %d", tt.wantCode, exitErr.Code)
			}
		})
	}
}
