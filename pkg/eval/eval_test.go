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

func TestEvalUnaryPlus(t *testing.T) {
	code := `x = +42
y = +(-10)
print x, " ", y`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if out != "42 -10" {
		t.Fatalf("expected '42 -10', got %q", out)
	}
}

func TestArgsAndProgramName(t *testing.T) {
	code := `print ProgramName, " ", scriptName, " ", len(Args), " ", Args[0], " ", Args[1]`
	l := lexer.NewLexer(code)
	p := parser.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	ev := NewEvaluator()
	ev.SetProgramName("main.les")
	ev.SetScriptName("/path/to/main.les")
	ev.SetArgs([]string{"foo", "bar"})

	var out strings.Builder
	ev.SetOutput(&out)

	_, err := ev.EvalProgram(prog)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}

	expected := "main.les /path/to/main.les 2 foo bar"
	if out.String() != expected {
		t.Fatalf("expected %q, got %q", expected, out.String())
	}
}

func TestScriptLine(t *testing.T) {
	code := `
line1 = scriptLine
def foo():
    scriptLine

line4 = foo()
print line1, " ", line4
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	expected := "2 4"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

func TestIOModule(t *testing.T) {
	code := `import io

# Test ReadLine deferred action (only reads when demanded)
l1 = @io.ReadLine()
l2 = @io.ReadLine()

# At this point, l1 and l2 are thunks.
# Demand l1 first, then l2:
print "Line1: ", l1, "\n"
print "Line2: ", l2, "\n"

# Test ReadAll
allRest = @io.ReadAll()
print "Rest: ", allRest, "\n"
`
	l := lexer.NewLexer(code)
	p := parser.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	ev := NewEvaluator()
	fakeInput := strings.NewReader("hello\nworld\nremaining text")
	ev.SetInput(fakeInput)

	var out strings.Builder
	ev.SetOutput(&out)

	_, err := ev.EvalProgram(prog)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}

	expected := "Line1: hello\nLine2: world\nRest: remaining text\n"
	if out.String() != expected {
		t.Fatalf("expected %q, got %q", expected, out.String())
	}
}

func TestIOReaderInterface(t *testing.T) {
	code := `import io

stdin = @io.Stdin
isReader = stdin is @io.Reader
isCloser = stdin is @io.Closer
isReadCloser = stdin is @io.ReadCloser

print isReader, " ", isCloser, " ", isReadCloser
`
	l := lexer.NewLexer(code)
	p := parser.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	ev := NewEvaluator()
	var out strings.Builder
	ev.SetOutput(&out)

	_, err := ev.EvalProgram(prog)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}

	expected := "true true true"
	if out.String() != expected {
		t.Fatalf("expected %q, got %q", expected, out.String())
	}
}

func TestForLoopAndBreak(t *testing.T) {
	code := `
sum = 0
# Test infinite for with break and continue
for:
    sum = sum + 1
    if sum == 2:
        continue
    if sum >= 5:
        break
    print sum, " "
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	// sum 1 is printed, sum 2 continued, sum 3 and 4 printed, sum 5 breaks
	expected := "1 3 4"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

func TestForLoopWithCondition(t *testing.T) {
	code := `
i = 0
for i < 3:
    print i, " "
    i = i + 1
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	expected := "0 1 2"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

func TestForLoopEcho(t *testing.T) {
	code := `import io

for:
    line = @io.ReadLine()
    if line is Nil:
        break
    print line, "\n"
`
	l := lexer.NewLexer(code)
	p := parser.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	ev := NewEvaluator()
	ev.SetInput(strings.NewReader("line 1\nline 2\nline 3\n"))
	var out strings.Builder
	ev.SetOutput(&out)

	_, err := ev.EvalProgram(prog)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}

	expected := "line 1\nline 2\nline 3\n"
	if out.String() != expected {
		t.Fatalf("expected %q, got %q", expected, out.String())
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
classify true
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
x = nil
print x is Nil, x is Any, 42 is Any, 42 is Nil
print "\n"

match x:
    case nil:
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
	expected := "truetruetruefalse\ngot nil"
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
print type(true)
print "\n"
print type(nil)
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
			name:     "exit with true",
			code:     "exit true",
			wantCode: 0,
		},
		{
			name:     "exit with false",
			code:     "exit false",
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
			name:     "return true is exit 0",
			code:     "return true\n",
			wantCode: 0,
		},
		{
			name:     "return false is exit 1",
			code:     "return false\n",
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

func TestEvalStruct(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		wantVal string
	}{
		{
			name: "basic struct with named args and default value",
			code: `
struct Point:
    x: Int
    y: Int = 100

p = Point(x: 10)
(p.x, p.y)
`,
			wantVal: "(10, 100)",
		},
		{
			name: "struct with positional arguments",
			code: `
struct Point:
    x: Int
    y: Int

p = Point(10, 20)
(p.x, p.y)
`,
			wantVal: "(10, 20)",
		},
		{
			name: "struct method with implicit receiver and direct field access",
			code: `
struct Counter:
    count: Int

    def Inc(delta):
        count + delta

    def GetSelf():
        self.count

c = Counter(count: 42)
(c.Inc(8), c.GetSelf())
`,
			wantVal: "(50, 42)",
		},
		{
			name: "functional copy-on-write update",
			code: `
struct Point:
    x: Int
    y: Int

p1 = Point(1, 2)
p2 = Point(p1, x: 10)
(p1.x, p1.y, p2.x, p2.y)
`,
			wantVal: "(1, 2, 10, 2)",
		},
		{
			name: "struct embedding with promoted fields and methods",
			code: `
struct Person:
    name: String
    age: Int

    def Greet():
        "Hello " + name

struct Employee:
    Person
    role: String

    def Details():
        name + ": " + role

e = Employee(Person(name: "Alice", age: 30), role: "Engineer")
(e.name, e.age, e.role, e.Greet(), e.Details())
`,
			wantVal: `(Alice, 30, Engineer, Hello Alice, Alice: Engineer)`,
		},
		{
			name: "struct auto-instantiates default embedded struct",
			code: `
struct Tag:
    label: String = "default"

struct Item:
    Tag
    value: Int

it = Item(value: 99)
(it.label, it.value)
`,
			wantVal: "(default, 99)",
		},
		{
			name: "struct type test with is operator and subtyping via embedding",
			code: `
struct Animal:
    species: String

struct Dog:
    Animal
    bark: String

a = Animal(species: "Cat")
d = Dog(Animal(species: "Canine"), bark: "Woof")
(a is Animal, d is Dog, d is Animal, a is Dog)
`,
			wantVal: "(true, true, true, false)",
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

func TestEvalInterface(t *testing.T) {
	code := `
interface Stringer:
    ToString(): String

interface Greeter:
    Greet(): String

interface PersonProtocol:
    Stringer
    Greeter

struct Person:
    name: String

    def ToString():
        name

    def Greet():
        "hello, " + name

struct Anonymous:
    id: Int

    def ToString():
        "anon"

p = Person(name: "Bob")
anon = Anonymous(id: 1)

# Dynamic is checks
pIsStringer = p is Stringer
pIsGreeter = p is Greeter
pIsProtocol = p is PersonProtocol

anonIsStringer = anon is Stringer
anonIsGreeter = anon is Greeter

(pIsStringer, pIsGreeter, pIsProtocol, anonIsStringer, anonIsGreeter)
`
	l := lexer.NewLexer(code)
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

	expected := "(true, true, true, true, false)"
	if val.String() != expected {
		t.Fatalf("expected %q, got %q", expected, val.String())
	}
}

func TestEvalMultilineTupleAndDict(t *testing.T) {
	code := `
t = (
    10,
    20,
    30,
)

d = {
    "a": 1,
    "b": 2,
}

print t[0] + t[1] + t[2], " ", d["a"] + d["b"]
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("eval failed: %v", err)
	}
	expected := "60 3"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

func TestEvalSemicolonAndLineContinuation(t *testing.T) {
	code := `
a = 1; b = 2; c = 3

total = 10 + \
        20 + \
        30

def calc():
    x = 100; y = 200
    x + y

msg = \
    "hello " + \
    "world"

print a + b + c, " ", total, " ", calc(), " ", msg
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("eval failed: %v", err)
	}
	expected := "6 60 300 hello world"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

func TestEvalSemicolonAndLineContinuationVariants(t *testing.T) {
	code := `
;
x = 10;;
y = 20;

def f():
    return;
    999

def add(a: Int, \
        b: Int): Int:
    a + b

if 1 + \
   2 == 3:
    ans = add(x, y); print ans
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("eval failed: %v", err)
	}
	expected := "30"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

func TestEvalLineContinuationWithComments(t *testing.T) {
	code := `
val = 10 + \   # base value
      20 + \   # bonus value
      30       # final value

print val
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("eval failed: %v", err)
	}
	expected := "60"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

func TestEvalErrorHandling(t *testing.T) {
	code := `
# 1. Built-in Error constructor and methods
err = Error("something went wrong")
isErr = err is Error
msg = err.Message()

# 2. Function returning Union type (Int | Error)
def divide(a: Int, b: Int): Int | Error:
    if b == 0:
        Error("division by zero")
    else:
        a / b

res1 = divide(10, 2)
match1 = match res1:
    case Int:
        "ok"
    case Error:
        "err: " + res1.Message()

res2 = divide(10, 0)
match2 = match res2:
    case Int:
        "ok"
    case Error:
        "err: " + res2.Message()

# 3. Custom struct implementing Error duck-typing
struct MyCustomError:
    detail: String

    def Message():
        "custom: " + detail

    def ToString():
        "MyCustomError(custom: " + detail + ")"

custom = MyCustomError(detail: "not found")
customIsError = custom is Error

print isErr, " ", msg, " ", match1, " ", match2, " ", customIsError, " ", custom
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	expected := "true something went wrong ok err: division by zero true MyCustomError(custom: not found)"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

func TestBuiltinToStringAndShowInterface(t *testing.T) {
	code := `
# 1. Direct ToString on primitive literals & expressions
s_int = 42.ToString()
s_float = 3.14.ToString()
s_str = "hello".ToString()
s_bool1 = true.ToString()
s_bool2 = false.ToString()
s_nil = nil.ToString()
s_tuple = (1, 2).ToString()
s_list = [1, 2, 3].ToString()
s_dict = {"k": 99}.ToString()

# 2. Method tear-off (binding to variable)
f = 100.ToString
s_bound = f()

# 3. is Show interface satisfaction
intIsShow = 42 is Show
strIsShow = "les" is Show
nilIsShow = nil is Show
tupleIsShow = (1, 2) is Show
intIsError = 42 is Error

# 4. Custom struct with ToString satisfies Show
struct Person:
    name: String
    def ToString():
        "Person(" + name + ")"

p = Person(name: "Alice")
pIsShow = p is Show
pStr = p.ToString()

print s_int, "\n", s_float, "\n", s_str, "\n", s_bool1, "\n", s_bool2, "\n", s_nil, "\n", s_tuple, "\n", s_list, "\n", s_dict, "\n", s_bound, "\n", intIsShow, "\n", strIsShow, "\n", nilIsShow, "\n", tupleIsShow, "\n", intIsError, "\n", pIsShow, "\n", pStr
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	expected := "42\n3.14\nhello\ntrue\nfalse\nnil\n(1, 2)\n(1, 2, 3)\n{\"k\": 99}\n100\ntrue\ntrue\ntrue\ntrue\nfalse\ntrue\nPerson(Alice)"
	if strings.TrimSpace(out) != expected {
		t.Fatalf("expected:\n%s\ngot:\n%s", expected, out)
	}
}

func TestPrintRequiresShow(t *testing.T) {
	code := `
struct NoShow:
    x: Int

n = NoShow(x: 10)
print n
`
	_, err := runLesCode(code)
	if err == nil {
		t.Fatalf("expected error printing non-Show value, got nil")
	}
	if !strings.Contains(err.Error(), "does not implement Show interface") {
		t.Fatalf("expected Show interface error, got: %v", err)
	}
}

func TestNilEqualityAndDisallowCapitalLiterals(t *testing.T) {
	// 1. nil equality comparison
	code := `
a = nil == nil
b = nil != nil
c = 42 == nil
d = 42 != nil
print a, " ", b, " ", c, " ", d
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "true false false true"
	if strings.TrimSpace(out) != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}

	// 2. Disallow True / False / Nil as value literals (they are treated as undefined identifiers)
	badCodes := []string{
		"print True",
		"print False",
		"print Nil",
	}
	for _, bc := range badCodes {
		_, err := runLesCode(bc)
		if err == nil {
			t.Fatalf("expected error for %q, but got nil", bc)
		}
	}

	// 3. Match expression with true, false, nil value patterns
	matchCode := `
def check(v):
    match v:
        case true:
            "got-true"
        case false:
            "got-false"
        case nil:
            "got-nil"
        case _:
            "other"

print check(true), " ", check(false), " ", check(nil), " ", check(42)
`
	out2, err := runLesCode(matchCode)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected2 := "got-true got-false got-nil other"
	if strings.TrimSpace(out2) != expected2 {
		t.Fatalf("expected %q, got %q", expected2, out2)
	}
}

func TestForceBuiltinAndPostfixOperator(t *testing.T) {
	// 1. Basic calculation with force() and postfix !
	code1 := `
a = (10 + 20)!
b = force(30 + 40)
print a, " ", b
`
	out1, err := runLesCode(code1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected1 := "30 70"
	if strings.TrimSpace(out1) != expected1 {
		t.Fatalf("expected %q, got %q", expected1, out1)
	}

	// 2. Unused argument without force does NOT trigger division by zero
	codeLazy := `
def pick(a, b):
    a

print pick(42, 1 / 0)
`
	outLazy, err := runLesCode(codeLazy)
	if err != nil {
		t.Fatalf("lazy argument unexpectedly failed: %v", err)
	}
	if strings.TrimSpace(outLazy) != "42" {
		t.Fatalf("expected 42, got %q", outLazy)
	}

	// 3. Unused argument WITH postfix ! triggers division by zero immediately
	codeForcePostfix := `
def pick(a, b):
    a

print pick(42, (1 / 0)!)
`
	_, err = runLesCode(codeForcePostfix)
	if err == nil {
		t.Fatalf("expected division by zero error for postfix !, got nil")
	}
	if !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("expected division by zero, got: %v", err)
	}

	// 5. Tuple with postfix ! evaluates elements at construction
	codeTupleLazy := `
t = (42, 1 / 0)
print t[0]
`
	outTupleLazy, err := runLesCode(codeTupleLazy)
	if err != nil {
		t.Fatalf("lazy tuple unexpectedly failed: %v", err)
	}
	if strings.TrimSpace(outTupleLazy) != "42" {
		t.Fatalf("expected 42, got %q", outTupleLazy)
	}

	codeTupleForce := `
t = (42, (1 / 0)!)
print t[0]
`
	_, err = runLesCode(codeTupleForce)
	if err == nil {
		t.Fatalf("expected division by zero error for tuple force, got nil")
	}

	// 6. Dict with postfix ! evaluates values at construction
	codeDictLazy := `
d = {"a": 42, "b": 1 / 0}
print d["a"]
`
	outDictLazy, err := runLesCode(codeDictLazy)
	if err != nil {
		t.Fatalf("lazy dict unexpectedly failed: %v", err)
	}
	if strings.TrimSpace(outDictLazy) != "42" {
		t.Fatalf("expected 42, got %q", outDictLazy)
	}

	codeDictForce := `
d = {"a": 42, "b": force(1 / 0)}
print d["a"]
`
	_, err = runLesCode(codeDictForce)
	if err == nil {
		t.Fatalf("expected division by zero error for dict force, got nil")
	}

	// 7. Shadowed force does NOT trigger built-in force eager demand
	codeShadow := `
def run_shadowed(force):
    unused = force(1 / 0)
    42

cb = |x|: x
print run_shadowed(cb)
`
	outShadow, err := runLesCode(codeShadow)
	if err != nil {
		t.Fatalf("shadowed force unexpectedly failed: %v", err)
	}
	if strings.TrimSpace(outShadow) != "42" {
		t.Fatalf("expected 42, got %q", outShadow)
	}
}

