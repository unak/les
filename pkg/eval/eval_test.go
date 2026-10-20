package eval

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	code := `print ProgramName, " ", scriptName, " ", Args.Len(), " ", Args[0], " ", Args[1]`
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

// nonCompReader implements io.Reader with a value receiver and a slice field, making the struct non-comparable.
type nonCompReader struct {
	data  []byte
	pos   *int
	dummy []int // makes struct non-comparable
}

func (r nonCompReader) Read(p []byte) (int, error) {
	if *r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[*r.pos:])
	*r.pos += n
	return n, nil
}

type countingReader struct {
	readCount int
}

func (r *countingReader) Read(p []byte) (int, error) {
	r.readCount++
	return 0, io.EOF
}

type partialErrReader struct {
	data []byte
	err  error
}

func (r *partialErrReader) Read(p []byte) (int, error) {
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, r.err
}

func TestBuiltinGets(t *testing.T) {
	code := `
# Test gets deferred action (only reads when demanded)
l1 = gets()
l2 = gets()

# At this point, l1 and l2 are thunks.
# Demand l1 first, then l2:
print "Line1: ", l1, "\n"
print "Line2: ", l2, "\n"

# EOF test
l3 = gets()
print "Line3 is Nil: ", l3 is Nil, "\n"
`
	l := lexer.NewLexer(code)
	p := parser.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	ev := NewEvaluator()
	fakeInput := strings.NewReader("hello\r\nworld\n")
	ev.SetInput(fakeInput)

	var out strings.Builder
	ev.SetOutput(&out)

	_, err := ev.EvalProgram(prog)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}

	expected := "Line1: hello\nLine2: world\nLine3 is Nil: true\n"
	if out.String() != expected {
		t.Fatalf("expected %q, got %q", expected, out.String())
	}

	// Test standalone CR is preserved
	ev2 := NewEvaluator()
	ev2.SetInput(strings.NewReader("value\r\nstandalone\r"))
	var out2 strings.Builder
	ev2.SetOutput(&out2)
	code2 := `
l1 = gets()
l2 = gets()
print l1, "|", l2
`
	prog2 := parser.NewParser(lexer.NewLexer(code2)).ParseProgram()
	if _, err := ev2.EvalProgram(prog2); err != nil {
		t.Fatalf("eval error with standalone CR: %v", err)
	}
	expected2 := "value|standalone\r"
	if out2.String() != expected2 {
		t.Fatalf("expected %q, got %q", expected2, out2.String())
	}

	// Test non-comparable reader (struct containing slice, passed as value) does not panic
	pos := 0
	ncReader := nonCompReader{
		data:  []byte("line from non-comp reader\n"),
		pos:   &pos,
		dummy: []int{1, 2, 3},
	}
	evNonComp := NewEvaluator()
	evNonComp.SetInput(ncReader)
	var outNonComp strings.Builder
	evNonComp.SetOutput(&outNonComp)
	codeNonComp := `print gets()`
	progNonComp := parser.NewParser(lexer.NewLexer(codeNonComp)).ParseProgram()
	if _, err := evNonComp.EvalProgram(progNonComp); err != nil {
		t.Fatalf("unexpected error with non-comparable reader: %v", err)
	}
	if outNonComp.String() != "line from non-comp reader" {
		t.Fatalf("expected %q, got %q", "line from non-comp reader", outNonComp.String())
	}

	// Test unused gets() performs zero reads (lazy demand-driven)
	cr := &countingReader{}
	evLazy := NewEvaluator()
	evLazy.SetInput(cr)
	codeLazy := `
x = gets()
y = gets()
`
	progLazy := parser.NewParser(lexer.NewLexer(codeLazy)).ParseProgram()
	if _, err := evLazy.EvalProgram(progLazy); err != nil {
		t.Fatalf("unexpected error with unused gets: %v", err)
	}
	if cr.readCount != 0 {
		t.Fatalf("expected 0 reads for undemanded gets(), got %d", cr.readCount)
	}

	// Test non-EOF read error propagation even with partial data
	pr := &partialErrReader{
		data: []byte("partial"),
		err:  errors.New("i/o error"),
	}
	evErr := NewEvaluator()
	evErr.SetInput(pr)
	codeErr := `x = gets(); print x`
	progErr := parser.NewParser(lexer.NewLexer(codeErr)).ParseProgram()
	if _, err := evErr.EvalProgram(progErr); err == nil {
		t.Fatalf("expected error from gets() with partial error reader, got nil")
	} else if !strings.Contains(err.Error(), "i/o error") {
		t.Fatalf("expected 'i/o error', got %v", err)
	}

	// Test shared buffer between gets() and @io.Stdin.Read
	codeMixed := `import io
l = gets()
rest = @io.Stdin.Read(5)
print l, "|", rest.String()
`
	evMixed := NewEvaluator()
	evMixed.SetInput(strings.NewReader("hello\nworld12345"))
	var outMixed strings.Builder
	evMixed.SetOutput(&outMixed)
	progMixed := parser.NewParser(lexer.NewLexer(codeMixed)).ParseProgram()
	if _, err := evMixed.EvalProgram(progMixed); err != nil {
		t.Fatalf("eval error mixed: %v", err)
	}
	expectedMixed := "hello|world"
	if outMixed.String() != expectedMixed {
		t.Fatalf("expected %q, got %q", expectedMixed, outMixed.String())
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
	code := `
for:
    line = gets()
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
					return a.Add(b), nil
				},
			},
			"sub": &BuiltinFunc{
				Name: "sub",
				Fn: func(args []Value) (Value, error) {
					return NewIntValue(0), nil
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

func TestEvalNil(t *testing.T) {
	code := `
x = nil
print x is Nil, 42 is Nil
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
		t.Fatalf("eval nil failed: %v", err)
	}
	expected := "truefalse\ngot nil"
	if strings.TrimSpace(out.String()) != expected {
		t.Fatalf("expected %q, got %q", expected, out.String())
	}
}

func TestEvalVariadicFunction(t *testing.T) {
	code := `
def my_print(prefix: String, *args):
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

func TestEvalTypeMethod(t *testing.T) {
	code := `
print (42).Type()
print "\n"
print (1,).Type()
print "\n"
print (1, 2).Type()
print "\n"
print "les".Type()
print "\n"
print true.Type()
print "\n"
print nil.Type()
print "\n"
print ().Type()
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
		t.Fatalf("eval type method failed: %v", err)
	}
	expected := "Int\n(Int)\n(Int, Int)\nString\nBool\nNil\n()"
	if strings.TrimSpace(out.String()) != expected {
		t.Fatalf("expected %q, got %q", expected, out.String())
	}

	// Non-forcing tuple type tests:
	codeNonForcing := `
def get_val(x):
    x

t1 = (1, 1 / 0)
print t1.Type()
print "\n"
t2 = (get_val(42),)
print t2.Type()
print "\n"
_ = t2[0]!
print t2.Type()
`
	var out2 strings.Builder
	ev2 := NewEvaluator()
	ev2.SetOutput(&out2)
	l2 := lexer.NewLexer(codeNonForcing)
	p2 := parser.NewParser(l2)
	prog2 := p2.ParseProgram()
	if len(p2.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p2.Errors())
	}
	_, err = ev2.EvalProgram(prog2)
	if err != nil {
		t.Fatalf("eval non-forcing tuple type failed: %v", err)
	}
	expected2 := "(Int, Int)\n(Unknown)\n(Int)"
	if strings.TrimSpace(out2.String()) != expected2 {
		t.Fatalf("expected %q, got %q", expected2, out2.String())
	}

	// Dict type tests:
	codeDictType := `
print ({}).Type()
print "\n"
print ({"key": "value"}).Type()
print "\n"
print ({1: 10, 2: 20}).Type()
print "\n"
print ({"a": 1, 2: "b"}).Type()
print "\n"
d_lazy = {"k": 1 / 0}
print d_lazy.Type()
print "\n"
def get_dyn(x):
    x
d_unk = {"a": get_dyn(10)}
print d_unk.Type()
print "\n"
_ = d_unk["a"]!
print d_unk.Type()
`
	var out3 strings.Builder
	ev3 := NewEvaluator()
	ev3.SetOutput(&out3)
	l3 := lexer.NewLexer(codeDictType)
	p3 := parser.NewParser(l3)
	prog3 := p3.ParseProgram()
	if len(p3.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p3.Errors())
	}
	_, err = ev3.EvalProgram(prog3)
	if err != nil {
		t.Fatalf("eval dict type failed: %v", err)
	}
	expected3 := "{Unknown: Unknown}\n{String: String}\n{Int: Int}\n{Int | String: Int | String}\n{String: Int}\n{String: Unknown}\n{String: Int}"
	if strings.TrimSpace(out3.String()) != expected3 {
		t.Fatalf("expected %q, got %q", expected3, out3.String())
	}

	// Direct argument nested dict and tuple type inference:
	codeDirectNested := `
print ({"outer": {"inner": 1}}).Type()
print "\n"
print (((1 / 0), (2 > 1) ? 1 : "a")).Type()
`
	var out4 strings.Builder
	ev4 := NewEvaluator()
	ev4.SetOutput(&out4)
	l4 := lexer.NewLexer(codeDirectNested)
	p4 := parser.NewParser(l4)
	prog4 := p4.ParseProgram()
	if len(p4.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p4.Errors())
	}
	_, err = ev4.EvalProgram(prog4)
	if err != nil {
		t.Fatalf("eval direct nested failed: %v", err)
	}
	expected4 := "{String: {String: Int}}\n(Int, Int | String)"
	if strings.TrimSpace(out4.String()) != expected4 {
		t.Fatalf("expected %q, got %q", expected4, out4.String())
	}
}

func TestPutsFunction(t *testing.T) {
	code := `
def puts_each(items):
    (head, ...tail) = items
    print head
    if head is String && head.Len() > 0 && head[-1] == "\n":
        ()
    else:
        print "\n"
    if tail.Len() > 0:
        puts_each(tail)

def puts(*args):
    if args.Len() == 0:
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
	// 1. Basic calculation with force keyword and postfix !
	code1 := `
a = (10 + 20)!
b = force (30 + 40)
c = force 50 + 60
print a, " ", b, " ", c
`
	out1, err := runLesCode(code1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected1 := "30 70 110"
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

	// 4. Unused argument WITH prefix force keyword triggers division by zero immediately
	codeForcePrefix := `
def pick(a, b):
    a

print pick(42, force (1 / 0))
`
	_, err = runLesCode(codeForcePrefix)
	if err == nil {
		t.Fatalf("expected division by zero error for prefix force, got nil")
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

	// 6. Dict with prefix force evaluates values at construction
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
d = {"a": 42, "b": force (1 / 0)}
print d["a"]
`
	_, err = runLesCode(codeDictForce)
	if err == nil {
		t.Fatalf("expected division by zero error for dict force, got nil")
	}
}

func TestStringOrdAndIntChr(t *testing.T) {
	// 1. Basic conversions
	code1 := `
print "A".Ord(), " ", "あ".Ord(), " ", 65.Chr(), " ", 12354.Chr()
`
	out1, err := runLesCode(code1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected1 := "65 12354 A あ"
	if strings.TrimSpace(out1) != expected1 {
		t.Fatalf("expected %q, got %q", expected1, out1)
	}

	// 2. Multicharacter string takes first character
	code2 := `
print "Hello".Ord()
`
	out2, err := runLesCode(code2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(out2) != "72" {
		t.Fatalf("expected 72, got %q", out2)
	}

	// 3. Empty string .Ord() raises runtime error
	codeEmpty := `
print "".Ord()
`
	_, err = runLesCode(codeEmpty)
	if err == nil {
		t.Fatalf("expected error on empty string .Ord(), got nil")
	}

	// 4. Out of range integer .Chr() raises runtime error (negative, > 0x10FFFF, surrogate)
	codeNegative := `
print (-1).Chr()
`
	_, err = runLesCode(codeNegative)
	if err == nil {
		t.Fatalf("expected error on negative .Chr(), got nil")
	}

	codeTooLarge := `
print (1114112).Chr()
`
	_, err = runLesCode(codeTooLarge)
	if err == nil {
		t.Fatalf("expected error on too large .Chr(), got nil")
	}

	codeSurrogate := `
print (55296).Chr()
`
	_, err = runLesCode(codeSurrogate)
	if err == nil {
		t.Fatalf("expected error on surrogate .Chr(), got nil")
	}

	// 5. Valid U+FFFD replacement character returns 65533
	codeReplacementChar := "print \"" + string(rune(0xFFFD)) + "\".Ord()\n"
	out, err := runLesCode(codeReplacementChar)
	if err != nil {
		t.Fatalf("unexpected error on valid U+FFFD .Ord(): %v", err)
	}
	if !strings.Contains(out, "65533") {
		t.Fatalf("expected 65533 for U+FFFD .Ord(), got %q", out)
	}

	// 6. Interface dynamic 'is' check with Ord and Chr signatures
	codeInterface := `
interface HasOrd:
    Ord(): Int

interface HasChr:
    Chr(): String

interface BadOrdParam:
    Ord(x: Int): Int

interface BadOrdRet:
    Ord(): String

interface BadChrRet:
    Chr(): Int

print "A" is HasOrd
print " "
print 65 is HasChr
print " "
print "A" is BadOrdParam
print " "
print "A" is BadOrdRet
print " "
print 65 is BadChrRet
`
	out, err = runLesCode(codeInterface)
	if err != nil {
		t.Fatalf("unexpected error on interface check: %v", err)
	}
	expectedOut := "true true false false false"
	if strings.TrimSpace(out) != expectedOut {
		t.Fatalf("expected %q, got %q", expectedOut, strings.TrimSpace(out))
	}
}

func TestPipelineOperator(t *testing.T) {
	code := `
def inc(x: Int): Int:
    x + 1

def double(x: Int): Int:
    x * 2

def add(x: Int, y: Int): Int:
    x + y

a = 5 |> inc |> double
b = 10 |> add(20)
c = 3 |> (|x|: x * 3) |> inc

print a
print " "
print b
print " "
print c
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("unexpected eval error for pipeline operator: %v", err)
	}
	expected := "12 30 10"
	if strings.TrimSpace(out) != expected {
		t.Fatalf("expected %q, got %q", expected, strings.TrimSpace(out))
	}

	// Verify parenthesis-omitted print consumes pipeline greedily: print 5 |> inc prints 6
	codePrint := `
def inc(x: Int): Int:
    x + 1

print 5 |> inc
`
	outPrint, err := runLesCode(codePrint)
	if err != nil {
		t.Fatalf("unexpected eval error for print with pipeline: %v", err)
	}
	if strings.TrimSpace(outPrint) != "6" {
		t.Fatalf("expected 6, got %q", strings.TrimSpace(outPrint))
	}

	// Verify arithmetic precedence and call result piping
	codeArithAndCall := `
def double(x: Int): Int:
    x * 2

def square(x: Int): Int:
    x * x

a = 1 + 2 |> double
b = double(3) |> square

print a
print " "
print b
`
	outArith, err := runLesCode(codeArithAndCall)
	if err != nil {
		t.Fatalf("unexpected eval error for arithmetic pipeline: %v", err)
	}
	if strings.TrimSpace(outArith) != "6 36" {
		t.Fatalf("expected '6 36', got %q", strings.TrimSpace(outArith))
	}
}

func TestGlobalStdoutAndStderr(t *testing.T) {
	ev := NewEvaluator()
	var stdout strings.Builder
	var stderr strings.Builder
	ev.SetOutput(&stdout)
	ev.SetErrorOutput(&stderr)

	code := `
Stdout.Write("stdout text\n".Bytes())
Stdout.Flush()
Stderr.Write("stderr text\n".Bytes())
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	if _, err := ev.EvalProgram(prog); err != nil {
		t.Fatalf("eval failed: %v", err)
	}

	if stdout.String() != "stdout text\n" {
		t.Fatalf("expected 'stdout text\\n', got %q", stdout.String())
	}
	// Stderr is unbuffered: text must be present immediately
	if stderr.String() != "stderr text\n" {
		t.Fatalf("expected 'stderr text\\n', got %q", stderr.String())
	}
}

func TestPrintRoutesToStdout(t *testing.T) {
	ev := NewEvaluator()
	var stdout strings.Builder
	ev.SetOutput(&stdout)

	code := `
print "hello", " ", 42, "\n"
Stdout.Write("direct write\n".Bytes())
Stdout.Flush()
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	if _, err := ev.EvalProgram(prog); err != nil {
		t.Fatalf("eval failed: %v", err)
	}

	expected := "hello 42\ndirect write\n"
	if stdout.String() != expected {
		t.Fatalf("expected %q, got %q", expected, stdout.String())
	}
}

func TestBufioReaderMethods(t *testing.T) {
	ev := NewEvaluator()
	inputData := "first:second\r\nthird\nfourth"
	ev.SetInput(strings.NewReader(inputData))

	code := `
# Test Peek
peeked = Stdin.Peek(5)
# Test ReadString with delimiter
part1 = Stdin.ReadString(":")
# Test ReadLine (strips \r\n)
line2 = Stdin.ReadLine()
# Test ReadLine (strips \n)
line3 = Stdin.ReadLine()
# Test ReadLine EOF
line4 = Stdin.ReadLine()
line5 = Stdin.ReadLine()

print peeked.String(), "|", part1, "|", line2, "|", line3, "|", line4, "|", line5 is Nil
`
	var stdout strings.Builder
	ev.SetOutput(&stdout)
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	if _, err := ev.EvalProgram(prog); err != nil {
		t.Fatalf("eval failed: %v", err)
	}

	expected := "first|first:|second|third|fourth|true"
	if stdout.String() != expected {
		t.Fatalf("expected %q, got %q", expected, stdout.String())
	}
}

func TestIOAndBufioModules(t *testing.T) {
	ev := NewEvaluator()
	var stdout strings.Builder
	var stderr strings.Builder
	ev.SetOutput(&stdout)
	ev.SetErrorOutput(&stderr)
	ev.SetInput(strings.NewReader("buffered input\n"))

	code := `
import io
import bufio

# Test @io raw streams
n1 = @io.Stdout.Write("raw out\n".Bytes())
n2 = @io.Stderr.Write("raw err\n".Bytes())

# Test @bufio.Reader constructor on @io.Stdin
br = @bufio.Reader(@io.Stdin)
line = br.ReadLine()

print line, " (", n1, ", ", n2, ")"
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	if _, err := ev.EvalProgram(prog); err != nil {
		t.Fatalf("eval failed: %v", err)
	}

	if stderr.String() != "raw err\n" {
		t.Fatalf("expected stderr 'raw err\\n', got %q", stderr.String())
	}
	expectedStdout := "raw out\nbuffered input (8, 8)"
	if stdout.String() != expectedStdout {
		t.Fatalf("expected stdout %q, got %q", expectedStdout, stdout.String())
	}
}

func TestUserStructBufioAdapters(t *testing.T) {
	ev := NewEvaluator()
	var stdout strings.Builder
	ev.SetOutput(&stdout)

	code := `
import bufio

struct InnerReader:
    def Read(n: Int):
        "embedded-data\n".Bytes()

struct MyReader:
    InnerReader

struct MyWriter:
    output: String = ""
    def Write(b: Bytes):
        print "[W:", b.String(), "]"
        b.Len()

mr = MyReader()
mw = MyWriter()

br = @bufio.Reader(mr)
bw = @bufio.Writer(mw)

line = br.ReadLine()
print "Read:", line, "\n"

bw.Write("hello".Bytes())
bw.Write(" world".Bytes())
bw.Flush()
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	if _, err := ev.EvalProgram(prog); err != nil {
		t.Fatalf("eval failed: %v", err)
	}

	expected := "Read:embedded-data\n[W:hello world]"
	if stdout.String() != expected {
		t.Fatalf("expected %q, got %q", expected, stdout.String())
	}
}

func TestBufioWriterAutoFlushAtExit(t *testing.T) {
	ev := NewEvaluator()
	var stdout strings.Builder
	ev.SetOutput(&stdout)

	code := `
import bufio

# Write without explicit Flush
Stdout.Write("buffered global\n".Bytes())

bw = @bufio.Writer(Stdout)
bw.Write("user writer\n".Bytes())
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	if _, err := ev.EvalProgram(prog); err != nil {
		t.Fatalf("eval failed: %v", err)
	}

	expected := "buffered global\nuser writer\n"
	if stdout.String() != expected {
		t.Fatalf("expected %q, got %q", expected, stdout.String())
	}
}

func TestStreamInterleavingOrder(t *testing.T) {
	ev := NewEvaluator()
	var stdout strings.Builder
	var stderr strings.Builder
	ev.SetOutput(&stdout)
	ev.SetErrorOutput(&stderr)

	code := `
import io

# Stdout is buffered, @io.Stdout.Write flushes Stdout first (C/Ruby style)
Stdout.Write("first-".Bytes())
@io.Stdout.Write("second-".Bytes())
Stdout.Write("third\n".Bytes())
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	if _, err := ev.EvalProgram(prog); err != nil {
		t.Fatalf("eval failed: %v", err)
	}

	expected := "first-second-third\n"
	if stdout.String() != expected {
		t.Fatalf("expected %q, got %q", expected, stdout.String())
	}
}

func TestBufioReadPeekEdgeCases(t *testing.T) {
	ev := NewEvaluator()
	ev.SetInput(strings.NewReader("hello world:::foo:::bar"))

	code := `
import bufio

r0 = Stdin.Read(0)
p0 = Stdin.Peek(0)
del1 = Stdin.ReadString(":::")
del2 = Stdin.ReadString(":::")

print r0 == Bytes(""), "|", p0 == Bytes(""), "|", del1, "|", del2
`
	var stdout strings.Builder
	ev.SetOutput(&stdout)
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	if _, err := ev.EvalProgram(prog); err != nil {
		t.Fatalf("eval failed: %v", err)
	}

	expected := "true|true|hello world:::|foo:::"
	if stdout.String() != expected {
		t.Fatalf("expected %q, got %q", expected, stdout.String())
	}
}

func TestEmptyStringCustomRead(t *testing.T) {
	ev := NewEvaluator()
	var stdout strings.Builder
	ev.SetOutput(&stdout)

	// Test custom reader that returns "" several times then EOF (Nil).
	// bufio.Reader handles io.ErrNoProgress without crash.
	code := `
import bufio

struct EmptyReader:
    def Read(n: Int):
        Bytes("")

er = EmptyReader()
br = @bufio.Reader(er)

# bufio.ReadLine on reader returning only empty Bytes should eventually see ErrNoProgress as EOF -> Nil
line = br.ReadLine()
print line is Nil
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	if _, err := ev.EvalProgram(prog); err != nil {
		t.Fatalf("eval failed: %v", err)
	}

	expected := "true"
	if stdout.String() != expected {
		t.Fatalf("expected %q, got %q", expected, stdout.String())
	}
}

func TestFlushPanicRecovery(t *testing.T) {
	ev := NewEvaluator()

	code := `
import bufio

struct FailingWriter:
    def Write(b: Bytes):
        b.Len() - 1000 # negative write count causes error

fw = FailingWriter()
bw = @bufio.Writer(fw)
bw.Write("trigger".Bytes())! # force demand point so write buffers
# Program exits, auto-flush triggers failure in Write
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	_, err := ev.EvalProgram(prog)
	if err == nil {
		t.Fatalf("expected error from failing flush, got nil")
	}
	if !strings.Contains(err.Error(), "negative count") {
		t.Fatalf("expected negative count error, got %v", err)
	}
}

func TestFlushDrainDependencies(t *testing.T) {
	ev := NewEvaluator()
	var stdout strings.Builder
	ev.SetOutput(&stdout)

	// Custom writer whose Write callback refills Stdout.Write
	// Tests that FlushAllBufioWriters drains all writers iteratively.
	code := `
import bufio

struct LoggingWriter:
    def Write(b: Bytes):
        Stdout.Write("logged:".Bytes() + b + "\n".Bytes())
        b.Len()

lw = LoggingWriter()
bw = @bufio.Writer(lw)

# Initial write to Stdout (registered before bw)
Stdout.Write("first\n".Bytes())
bw.Write("payload".Bytes())!
# Program exits, auto-flush flushes Stdout, then flushes bw which refills Stdout,
# and iterative drain loop flushes Stdout again.
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	if _, err := ev.EvalProgram(prog); err != nil {
		t.Fatalf("eval failed: %v", err)
	}

	expected := "first\nlogged:payload\n"
	if stdout.String() != expected {
		t.Fatalf("expected %q, got %q", expected, stdout.String())
	}
}

func TestRuntimeQualifiedTypeMatching(t *testing.T) {
	ev := NewEvaluator()
	var stdout strings.Builder
	ev.SetOutput(&stdout)

	// User struct named Reader must NOT match @bufio.Reader by name alone
	code := `
import io
import bufio

struct Reader:
    def Read(n: Int): "custom".Bytes()

userR = Reader()

print Stdin is @bufio.Reader, "|"
print Stdout is @bufio.Writer, "|"
print @io.Stdin is @io.Reader, "|"
print @io.Stdout is @io.Writer, "|"
print userR is @bufio.Reader # must be false (different identity)
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	if _, err := ev.EvalProgram(prog); err != nil {
		t.Fatalf("eval failed: %v", err)
	}

	expected := "true|true|true|true|false"
	if stdout.String() != expected {
		t.Fatalf("expected %q, got %q", expected, stdout.String())
	}
}

func TestWriterResultExceedingBuffer(t *testing.T) {
	ev := NewEvaluator()

	code := `
import bufio

struct LyingWriter:
    def Write(b: Bytes):
        999999 # claims to write way more than len(b)

lw = LyingWriter()
bw = @bufio.Writer(lw)
bw.Write("hello".Bytes())!
bw.Flush()!
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	_, err := ev.EvalProgram(prog)
	if err == nil {
		t.Fatalf("expected error from lying writer, got nil")
	}
	if !strings.Contains(err.Error(), "exceeding buffer length") {
		t.Fatalf("expected 'exceeding buffer length' error, got %v", err)
	}
}

func TestWriterResultNonInt(t *testing.T) {
	ev := NewEvaluator()

	code := `
import bufio

struct BadWriter:
    def Write(b: Bytes):
        "not an integer"

bw_custom = BadWriter()
bw = @bufio.Writer(bw_custom)
bw.Write("hello".Bytes())!
bw.Flush()!
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	_, err := ev.EvalProgram(prog)
	if err == nil {
		t.Fatalf("expected error from non-int write, got nil")
	}
	if !strings.Contains(err.Error(), "Write returned unexpected type") {
		t.Fatalf("expected 'Write returned unexpected type' error, got %v", err)
	}
}

func TestFileModuleLoaderAccessBuiltins(t *testing.T) {
	tempDir := t.TempDir()

	// Submodule uses built-in bufio, io, and Stdin / Stdout
	helperCode := `
import bufio
import io

def Greet(name: String): String:
    bw = @bufio.Writer(@io.Stdout)
    bw.Write("greeting: ".Bytes())!
    bw.Flush()!
    name
`
	helperPath := filepath.Join(tempDir, "helper.les")
	if err := os.WriteFile(helperPath, []byte(helperCode), 0644); err != nil {
		t.Fatalf("failed to write helper.les: %v", err)
	}

	ev := NewEvaluator()
	var out strings.Builder
	ev.SetOutput(&out)
	fml := NewFileModuleLoader(tempDir)
	ev.SetFileModuleLoader(fml)

	mainCode := `
import helper

msg = @helper.Greet("Les")
print msg
`
	prog := parser.NewParser(lexer.NewLexer(mainCode)).ParseProgram()
	if _, err := ev.EvalProgram(prog); err != nil {
		t.Fatalf("unexpected eval error with imported module: %v", err)
	}

	if out.String() != "greeting: Les" {
		t.Fatalf("expected 'greeting: Les', got %q", out.String())
	}
}

func TestCustomWriterExitRePanic(t *testing.T) {
	ev := NewEvaluator()

	code := `
import bufio

struct ExitingWriter:
    def Write(b: Bytes):
        exit(42)

ew = ExitingWriter()
bw = @bufio.Writer(ew)
bw.Write("trigger exit".Bytes())!
bw.Flush()!
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	_, err := ev.EvalProgram(prog)
	if err == nil {
		t.Fatalf("expected ExitError, got nil")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected error to be *ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != 42 {
		t.Fatalf("expected exit code 42, got %d", exitErr.Code)
	}
}

func TestModuleIsolationCallerShadowing(t *testing.T) {
	tempDir := t.TempDir()

	modCode := `
def UseStdout(): String:
    Stdout.Write("from module".Bytes())!
    Stdout.Flush()!
    "ok"
`
	modPath := filepath.Join(tempDir, "mod.les")
	if err := os.WriteFile(modPath, []byte(modCode), 0644); err != nil {
		t.Fatalf("failed to write mod.les: %v", err)
	}

	ev := NewEvaluator()
	var out strings.Builder
	ev.SetOutput(&out)
	fml := NewFileModuleLoader(tempDir)
	ev.SetFileModuleLoader(fml)

	// Caller script shadows Stdout with an Int
	callerCode := `
import mod

Stdout = 12345
res = @mod.UseStdout()
print res
`
	prog := parser.NewParser(lexer.NewLexer(callerCode)).ParseProgram()
	_, err := ev.EvalProgram(prog)
	if err != nil {
		t.Fatalf("unexpected eval error when caller shadows Stdout: %v", err)
	}
	if out.String() != "from moduleok" {
		t.Fatalf("expected 'from moduleok', got %q", out.String())
	}
}

func TestModuleInheritsProgramNameAndArgs(t *testing.T) {
	tempDir := t.TempDir()

	modCode := `
def Inspect(): String:
    ProgramName + ":" + Args[0] + ":" + scriptName
`
	modPath := filepath.Join(tempDir, "modinfo.les")
	if err := os.WriteFile(modPath, []byte(modCode), 0644); err != nil {
		t.Fatalf("failed to write modinfo.les: %v", err)
	}

	ev := NewEvaluator()
	ev.SetProgramName("main_app.les")
	ev.SetArgs([]string{"foo", "bar"})
	ev.SetScriptName("main_app.les")

	var out strings.Builder
	ev.SetOutput(&out)
	fml := NewFileModuleLoader(tempDir)
	ev.SetFileModuleLoader(fml)

	callerCode := `
import modinfo

info = @modinfo.Inspect()
print info, "|", scriptName
`
	prog := parser.NewParser(lexer.NewLexer(callerCode)).ParseProgram()
	_, err := ev.EvalProgram(prog)
	if err != nil {
		t.Fatalf("unexpected eval error: %v", err)
	}

	expectedPrefix := "main_app.les:foo:" + modPath + "|main_app.les"
	if out.String() != expectedPrefix {
		t.Fatalf("expected %q, got %q", expectedPrefix, out.String())
	}
}

func TestEmbeddedInterfaceOwningModuleResolution(t *testing.T) {
	tempDir := t.TempDir()

	// localmod exports its own 'Reader' interface with completely different signature: Read(n: String): Int
	localModCode := `
interface Reader:
    def Read(n: String): Int
`
	localModPath := filepath.Join(tempDir, "localmod.les")
	if err := os.WriteFile(localModPath, []byte(localModCode), 0644); err != nil {
		t.Fatalf("failed to write localmod.les: %v", err)
	}

	ev := NewEvaluator()
	var out strings.Builder
	ev.SetOutput(&out)
	fml := NewFileModuleLoader(tempDir)
	ev.SetFileModuleLoader(fml)

	// Even though localmod has a different Reader, @io.Stdin is @io.ReadCloser should resolve
	// ReadCloser's embedded Reader from the owning 'io' module, NOT localmod!
	callerCode := `
import io
import localmod

stdin = @io.Stdin
isRC = stdin is @io.ReadCloser
print isRC
`
	prog := parser.NewParser(lexer.NewLexer(callerCode)).ParseProgram()
	_, err := ev.EvalProgram(prog)
	if err != nil {
		t.Fatalf("unexpected eval error: %v", err)
	}
	if out.String() != "true" {
		t.Fatalf("expected 'true', got %q", out.String())
	}
}

func TestEvaluatorReuseAfterRuntimeErrorDoesNotFlushStaleBuffers(t *testing.T) {
	ev := NewEvaluator()
	var out strings.Builder
	ev.SetOutput(&out)

	// First run: writes to custom writer and Stdout, then panics with division by zero
	firstCode := `
import bufio

struct Buffer:
    content: Bytes
    def Write(b: Bytes): Int:
        content = content + b
        b.Len()

buf = Buffer(content: Bytes(""))
bw = @bufio.Writer(buf)
bw.Write("stale custom output".Bytes())!
Stdout.Write("stale stdout output".Bytes())!
(1 / 0)!
`
	prog1 := parser.NewParser(lexer.NewLexer(firstCode)).ParseProgram()
	_, err1 := ev.EvalProgram(prog1)
	if err1 == nil {
		t.Fatalf("expected runtime error, got nil")
	}
	if out.String() != "" {
		t.Fatalf("expected no output flushed on runtime error, got %q", out.String())
	}

	// Second run on the same evaluator: normal execution
	secondCode := `
Stdout.Write("clean output".Bytes())!
`
	prog2 := parser.NewParser(lexer.NewLexer(secondCode)).ParseProgram()
	_, err2 := ev.EvalProgram(prog2)
	if err2 != nil {
		t.Fatalf("unexpected eval error in second run: %v", err2)
	}
	if out.String() != "clean output" {
		t.Fatalf("expected %q, got %q", "clean output", out.String())
	}
}

func TestDrainedCustomBufioWritersCleanedUp(t *testing.T) {
	ev := NewEvaluator()
	var out strings.Builder
	ev.SetOutput(&out)

	code := `
import bufio

struct LoggingWriter:
    def Write(b: Bytes): Int:
        Stdout.Write("logged:".Bytes() + b)
        b.Len()

lw = LoggingWriter()
bw = @bufio.Writer(lw)
bw.Write("hello".Bytes())!
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	_, err := ev.EvalProgram(prog)
	if err != nil {
		t.Fatalf("unexpected eval error: %v", err)
	}
	if out.String() != "logged:hello" {
		t.Fatalf("expected 'logged:hello', got %q", out.String())
	}

	// After evaluation finishes and flushes, custom writer should be drained and removed from root ActiveBufioWriters
	for _, bw := range ev.builtinsEnv.ActiveBufioWriters {
		if bw != ev.builtinsEnv.StdoutWriter {
			t.Fatalf("expected no custom writers retained in ActiveBufioWriters, found %v", bw)
		}
	}

	out.Reset()
	// Now execute a Write on bw again in a subsequent run, verifying it gets re-registered and writes to lw
	code2 := `
bw.Write("world".Bytes())!
`
	prog2 := parser.NewParser(lexer.NewLexer(code2)).ParseProgram()
	_, err = ev.EvalProgram(prog2)
	if err != nil {
		t.Fatalf("unexpected eval error: %v", err)
	}
	for _, bw := range ev.builtinsEnv.ActiveBufioWriters {
		if bw != ev.builtinsEnv.StdoutWriter {
			t.Fatalf("expected no custom writers retained after second run, found %v", bw)
		}
	}
	if out.String() != "logged:world" {
		t.Fatalf("expected 'logged:world', got %q", out.String())
	}
}

func TestCustomWriterExitDuringShutdownPreservesExitCode(t *testing.T) {
	ev := NewEvaluator()
	var out strings.Builder
	ev.SetOutput(&out)

	code := `
import bufio

struct ExitingBuffer:
    def Write(b: Bytes): Int:
        exit(42)
        0

eb = ExitingBuffer()
bw = @bufio.Writer(eb)
bw.Write("trigger exit during flush".Bytes())!
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	_, err := ev.EvalProgram(prog)
	if err == nil {
		t.Fatalf("expected exit error, got nil")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected error to be *ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != 42 {
		t.Fatalf("expected exit code 42, got %d", exitErr.Code)
	}
}

func TestCustomWriterReusedAfterRuntimeErrorNotDetached(t *testing.T) {
	ev := NewEvaluator()
	var out strings.Builder
	ev.SetOutput(&out)

	firstCode := `
import bufio

struct LoggingWriter:
    def Write(b: Bytes): Int:
        Stdout.Write("logged:".Bytes() + b)
        b.Len()

lw = LoggingWriter()
bw = @bufio.Writer(lw)
bw.Write("abandoned".Bytes())!
(1 / 0)!
`
	prog1 := parser.NewParser(lexer.NewLexer(firstCode)).ParseProgram()
	_, err1 := ev.EvalProgram(prog1)
	if err1 == nil {
		t.Fatalf("expected runtime error, got nil")
	}
	if out.String() != "" {
		t.Fatalf("expected no output on error, got %q", out.String())
	}

	// Verify that bw can still write to lw in subsequent run and was not detached to io.Discard
	secondCode := `
bw.Write("valid".Bytes())!
`
	prog2 := parser.NewParser(lexer.NewLexer(secondCode)).ParseProgram()
	_, err2 := ev.EvalProgram(prog2)
	if err2 != nil {
		t.Fatalf("unexpected eval error in second run: %v", err2)
	}
	// "abandoned" was discarded on runtime error, "valid" was successfully written
	if out.String() != "logged:valid" {
		t.Fatalf("expected 'logged:valid', got %q", out.String())
	}
}

func TestShutdownFlushErrorResetsWriterState(t *testing.T) {
	ev := NewEvaluator()
	var out strings.Builder
	ev.SetOutput(&out)

	firstCode := `
import bufio

struct BrokenBuffer:
    def Write(b: Bytes): Int:
        (1 / 0)!
        0

bb = BrokenBuffer()
bw = @bufio.Writer(bb)
bw.Write("will fail on flush".Bytes())!
`
	prog1 := parser.NewParser(lexer.NewLexer(firstCode)).ParseProgram()
	_, err1 := ev.EvalProgram(prog1)
	if err1 == nil {
		t.Fatalf("expected flush error, got nil")
	}

	// On the next run with the same evaluator, the failed writer must have been reset and removed
	secondCode := `
Stdout.Write("recovered".Bytes())!
`
	prog2 := parser.NewParser(lexer.NewLexer(secondCode)).ParseProgram()
	_, err2 := ev.EvalProgram(prog2)
	if err2 != nil {
		t.Fatalf("unexpected error in second run: %v", err2)
	}
	if out.String() != "recovered" {
		t.Fatalf("expected 'recovered', got %q", out.String())
	}
}

func TestPeekTreatsNoProgressAsEOF(t *testing.T) {
	ev := NewEvaluator()
	var out strings.Builder
	ev.SetOutput(&out)

	code := `
import bufio

struct EmptyReader:
    def Read(n: Int): Bytes:
        Bytes("")

er = EmptyReader()
br = @bufio.Reader(er)
res = br.Peek(5)!
print res
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	_, err := ev.EvalProgram(prog)
	if err != nil {
		t.Fatalf("unexpected eval error in Peek: %v", err)
	}
	if out.String() != "nil" {
		t.Fatalf("expected nil representation 'nil', got %q", out.String())
	}
}

func TestEmbeddedInterfaceResolutionWhenImportPathDiffersFromModuleName(t *testing.T) {
	tempDir := t.TempDir()

	// Module declares "module mymod" but is imported as "pkg"
	modCode := `
module mymod

interface Inner:
    Foo(): Int

interface Outer:
    Inner
    Bar(): Int
`
	modPath := filepath.Join(tempDir, "pkg.les")
	if err := os.WriteFile(modPath, []byte(modCode), 0644); err != nil {
		t.Fatalf("failed to write pkg.les: %v", err)
	}

	// Another module in root dir with conflicting "Inner" interface:
	conflictingModCode := `
interface Inner:
    Unrelated(): String
`
	if err := os.WriteFile(filepath.Join(tempDir, "conflict.les"), []byte(conflictingModCode), 0644); err != nil {
		t.Fatalf("failed to write conflict.les: %v", err)
	}

	ev := NewEvaluator()
	var out strings.Builder
	ev.SetOutput(&out)
	fml := NewFileModuleLoader(tempDir)
	ev.SetFileModuleLoader(fml)

	callerCode := `
import conflict
import pkg

struct Implementer:
    def Foo(): Int: 1
    def Bar(): Int: 2

impl = Implementer()
res = impl is @pkg.Outer
print res
`
	prog := parser.NewParser(lexer.NewLexer(callerCode)).ParseProgram()
	_, err := ev.EvalProgram(prog)
	if err != nil {
		t.Fatalf("unexpected eval error: %v", err)
	}
	if out.String() != "true" {
		t.Fatalf("expected 'true', got %q", out.String())
	}
}

type errorOnFlushWriter struct{}

func (e *errorOnFlushWriter) Write(p []byte) (n int, err error) {
	return 0, fmt.Errorf("write error")
}

func TestSetOutputRemovesFailedOldWriter(t *testing.T) {
	ev := NewEvaluator()
	// Set output to a writer that fails
	ev.SetOutput(&errorOnFlushWriter{})

	// Buffer some data into Stdout
	code := `
Stdout.Write("buffered in bad output")!
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	_, _ = ev.EvalProgram(prog)

	// Switch output to clean buffer: old failing writer must be reset and removed from ActiveBufioWriters
	var cleanOut strings.Builder
	ev.SetOutput(&cleanOut)

	if len(ev.builtinsEnv.ActiveBufioWriters) != 1 || ev.builtinsEnv.ActiveBufioWriters[0] != ev.builtinsEnv.StdoutWriter {
		t.Fatalf("expected exactly 1 active writer (new StdoutWriter), got %d", len(ev.builtinsEnv.ActiveBufioWriters))
	}

	// Subsequent evaluation must succeed cleanly without encountering old failing writer
	code2 := `
Stdout.Write("hello clean".Bytes())!
`
	prog2 := parser.NewParser(lexer.NewLexer(code2)).ParseProgram()
	_, err := ev.EvalProgram(prog2)
	if err != nil {
		t.Fatalf("unexpected eval error after switching output: %v", err)
	}
	if cleanOut.String() != "hello clean" {
		t.Fatalf("expected 'hello clean', got %q", cleanOut.String())
	}
}

func TestStdoutFlushAndCloseFlushesBuffered(t *testing.T) {
	ev := NewEvaluator()
	var out strings.Builder
	ev.SetOutput(&out)

	code := `
Stdout.Write("buffered content".Bytes())!
@io.Stdout.Flush()
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	_, err := ev.EvalProgram(prog)
	if err != nil {
		t.Fatalf("unexpected eval error: %v", err)
	}
	if out.String() != "buffered content" {
		t.Fatalf("expected @io.Stdout.Flush() to flush buffered output, got %q", out.String())
	}

	out.Reset()
	codeClose := `
Stdout.Write("close content".Bytes())!
@io.Stdout.Close()
`
	progClose := parser.NewParser(lexer.NewLexer(codeClose)).ParseProgram()
	_, err = ev.EvalProgram(progClose)
	if err != nil {
		t.Fatalf("unexpected eval error: %v", err)
	}
	if out.String() != "close content" {
		t.Fatalf("expected @io.Stdout.Close() to flush buffered output, got %q", out.String())
	}
}

func TestCustomReaderPanicRecovery(t *testing.T) {
	ev := NewEvaluator()
	code := `
struct PanicReader:
    def Read(self, n: Int):
        1 / 0
r = @bufio.Reader(PanicReader())
r.ReadLine()!
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	_, err := ev.EvalProgram(prog)
	if err == nil {
		t.Fatalf("expected error from PanicReader, got nil")
	}
	if !strings.Contains(err.Error(), "Reader.Read") && !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("expected runtime error mentioning Reader.Read or division by zero, got %v", err)
	}
}

func TestCustomWriterShortWrite(t *testing.T) {
	ev := NewEvaluator()
	code := `
struct ShortWriter:
    def Write(self, b: Bytes):
        0
w = @bufio.Writer(ShortWriter())
w.Write("test".Bytes())!
w.Flush()!
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	_, err := ev.EvalProgram(prog)
	if err == nil {
		t.Fatalf("expected error from short write, got nil")
	}
}

func TestStdinReadZeroBytesAndEOF(t *testing.T) {
	ev := NewEvaluator()
	ev.SetInput(strings.NewReader("abc"))

	code := `
z = @io.Stdin.Read(0)!
a = @io.Stdin.Read(2)!
b = @io.Stdin.Read(2)!
c = @io.Stdin.Read(2)!
if z != Bytes(""):
    1 / 0
if a != Bytes("ab"):
    1 / 0
if b != Bytes("c"):
    1 / 0
if c != nil:
    1 / 0
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	_, err := ev.EvalProgram(prog)
	if err != nil {
		t.Fatalf("unexpected eval error in Stdin.Read tests: %v", err)
	}
}

func TestBytesBasicOperations(t *testing.T) {
	ev := NewEvaluator()
	var out strings.Builder
	ev.SetOutput(&out)

	code := `
b1 = "hello".Bytes()
b2 = Bytes("world")
b_empty = Bytes()

if b1.Len() != 5:
    1 / 0
if b_empty.Len() != 0:
    1 / 0
if b1[0] != 104:
    1 / 0
if b1[-1] != 111:
    1 / 0
if b1.String() != "hello":
    1 / 0
if b1.ToString() != "68656c6c6f":
    1 / 0
if b1.Bytes().ToString() != "68656c6c6f":
    1 / 0
if Bytes(b1).ToString() != "68656c6c6f":
    1 / 0
if !b1.IsValidString():
    1 / 0
if !(b1 is Bytes):
    1 / 0
if !(b1 is Show):
    1 / 0
if b1.Type() != "Bytes":
    1 / 0

b_cat = b1 + b2
if b_cat.String() != "helloworld":
    1 / 0
if !(b1 < b2):
    1 / 0
if !(b1 != b2):
    1 / 0
if !(b1 == Bytes("hello")):
    1 / 0

print b1
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	_, err := ev.EvalProgram(prog)
	if err != nil {
		t.Fatalf("unexpected eval error in TestBytesBasicOperations: %v", err)
	}
	if out.String() != "68656c6c6f" {
		t.Fatalf("expected print b1 to output hex '68656c6c6f', got %q", out.String())
	}
}

func TestBytesMultibyteRoundtrip(t *testing.T) {
	ev := NewEvaluator()
	code := `
orig = "こんにちは世界"
b = orig.Bytes()
if b.Len() != 21:
    1 / 0
if b.String() != orig:
    1 / 0
if !b.IsValidString():
    1 / 0
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	_, err := ev.EvalProgram(prog)
	if err != nil {
		t.Fatalf("unexpected eval error in TestBytesMultibyteRoundtrip: %v", err)
	}
}

func TestBytesIndexOutOfBounds(t *testing.T) {
	ev := NewEvaluator()
	codePositive := `
b = "abc".Bytes()
x = b[3]!
`
	progPos := parser.NewParser(lexer.NewLexer(codePositive)).ParseProgram()
	if _, err := ev.EvalProgram(progPos); err == nil {
		t.Fatalf("expected out of bounds error for positive index, got nil")
	}

	codeNegative := `
b = "abc".Bytes()
x = b[-4]!
`
	progNeg := parser.NewParser(lexer.NewLexer(codeNegative)).ParseProgram()
	if _, err := ev.EvalProgram(progNeg); err == nil {
		t.Fatalf("expected out of bounds error for negative index, got nil")
	}
}

func TestBytesInvalidUtf8(t *testing.T) {
	ev := NewEvaluator()
	// Inject invalid UTF-8 bytes: 0xFF, 0xFE, 0xFD
	ev.builtinsEnv.Set("bad", BytesValue("\xff\xfe\xfd"))

	code := `
if bad.IsValidString():
    1 / 0
if bad.Len() != 3:
    1 / 0
if bad[0] != 255:
    1 / 0
if bad[-1] != 253:
    1 / 0
if bad.ToString() != "fffefd":
    1 / 0
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	if _, err := ev.EvalProgram(prog); err != nil {
		t.Fatalf("unexpected error validating invalid UTF-8 bytes: %v", err)
	}

	codeDecode := `
s = bad.String()!
`
	progDecode := parser.NewParser(lexer.NewLexer(codeDecode)).ParseProgram()
	if _, err := ev.EvalProgram(progDecode); err == nil {
		t.Fatalf("expected error from bad.String() on invalid UTF-8, got nil")
	} else if !strings.Contains(err.Error(), "invalid UTF-8") {
		t.Fatalf("expected error message mentioning invalid UTF-8, got: %v", err)
	}
}

func TestBinaryStreamIO(t *testing.T) {
	ev := NewEvaluator()
	var buf strings.Builder
	ev.SetOutput(&buf)
	ev.builtinsEnv.Set("raw_bin", BytesValue("\xff\xfe\x00\x01"))

	code := `
import io
import bufio

# Write non-UTF-8 bytes (0xff, 0xfe, 0x00, 0x01)
Stdout.Write(raw_bin)!
Stdout.Flush()!
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	if _, err := ev.EvalProgram(prog); err != nil {
		t.Fatalf("unexpected eval error in binary output: %v", err)
	}

	if buf.String() != "\xff\xfe\x00\x01" {
		t.Fatalf("expected raw binary output \\xff\\xfe\\x00\\x01, got %q", buf.String())
	}

	// Read non-UTF-8 bytes back from Stdin
	ev2 := NewEvaluator()
	ev2.SetInput(strings.NewReader("\xff\xfe\x00\x01"))
	codeRead := `
import io
import bufio

read_bin = Stdin.Read(4)!
if read_bin.Len() != 4:
    1 / 0
if read_bin[0] != 255:
    1 / 0
if read_bin[1] != 254:
    1 / 0
if read_bin[2] != 0:
    1 / 0
if read_bin[3] != 1:
    1 / 0
if read_bin.ToString() != "fffe0001":
    1 / 0
`
	progRead := parser.NewParser(lexer.NewLexer(codeRead)).ParseProgram()
	if _, err := ev2.EvalProgram(progRead); err != nil {
		t.Fatalf("unexpected eval error in binary input: %v", err)
	}

	// Negative test: Passing String to Write must fail at runtime
	ev3 := NewEvaluator()
	codeInvalidWrite := `
Stdout.Write("hello")!
`
	progInvalid := parser.NewParser(lexer.NewLexer(codeInvalidWrite)).ParseProgram()
	if _, err := ev3.EvalProgram(progInvalid); err == nil {
		t.Fatalf("expected error passing String to Stdout.Write, got nil")
	} else if !strings.Contains(err.Error(), "Write expects Bytes") {
		t.Fatalf("expected 'Write expects Bytes' error, got %v", err)
	}
}

func TestBufioUtf8Validation(t *testing.T) {
	// Test ReadLine error on invalid UTF-8
	ev1 := NewEvaluator()
	ev1.SetInput(strings.NewReader("\xff\xfe\xfd\n"))
	codeReadLine := `
l = Stdin.ReadLine()!
`
	prog1 := parser.NewParser(lexer.NewLexer(codeReadLine)).ParseProgram()
	if _, err := ev1.EvalProgram(prog1); err == nil {
		t.Fatalf("expected error from ReadLine on invalid UTF-8, got nil")
	} else if !strings.Contains(err.Error(), "invalid UTF-8") {
		t.Fatalf("expected 'invalid UTF-8' error, got %v", err)
	}

	// Test ReadString error on invalid UTF-8
	ev2 := NewEvaluator()
	ev2.SetInput(strings.NewReader("\xff\xfe\xfd:"))
	codeReadString := `
s = Stdin.ReadString(":")!
`
	prog2 := parser.NewParser(lexer.NewLexer(codeReadString)).ParseProgram()
	if _, err := ev2.EvalProgram(prog2); err == nil {
		t.Fatalf("expected error from ReadString on invalid UTF-8, got nil")
	} else if !strings.Contains(err.Error(), "invalid UTF-8") {
		t.Fatalf("expected 'invalid UTF-8' error, got %v", err)
	}

	// Test valid UTF-8 multibyte strings work cleanly
	ev3 := NewEvaluator()
	ev3.SetInput(strings.NewReader("こんにちは\n世界:"))
	codeValid := `
l = Stdin.ReadLine()!
s = Stdin.ReadString(":")!
if l != "こんにちは":
    1 / 0
if s != "世界:":
    1 / 0
`
	prog3 := parser.NewParser(lexer.NewLexer(codeValid)).ParseProgram()
	if _, err := ev3.EvalProgram(prog3); err != nil {
		t.Fatalf("unexpected error on valid UTF-8: %v", err)
	}
}

func TestEvalDictTypeAnnotationAndIsExpr(t *testing.T) {
	code := `
d: {String: Int} = {"count": 42}
print d["count"]
print " "

print ({"a": 1!} is {String: Int})
print " "
print ({"a": "s"!} is {String: Int})
print " "
print ({"a": 1!} is {String: Int | Nil})
print " "
print (42 is {String: Int})
print " "

empty: {String: Int} = {}
print empty.Len()
print " "

lazyDict = {"a": 1, "b": 1 / 0}
print (lazyDict is {String: Int})
print " "

# Unevaluated value is not checked:
unevalDict = {"a": "not_int"}
print (unevalDict is {String: Int})
print " "
_ = unevalDict["a"]!
print (unevalDict is {String: Int})
print " "

def id(x):
    x

anyDict: {String: Int} = {"a": id(99)}
print anyDict["a"]
print " "

def lookup(table: {String: Int}, key: String): Int:
    table[key]

print lookup({"foo": 100}, "foo")
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("unexpected error evaluating code with dict type annot: %v", err)
	}
	expected := "42 true false true false 0 true false false 99 100"
	if strings.TrimSpace(out) != expected {
		t.Fatalf("expected output %q, got %q", expected, strings.TrimSpace(out))
	}
}

func TestEvalMatchWithDictPattern(t *testing.T) {
	code := `
def test_match(v):
    match v:
        case {String: Int}:
            "int-dict"
        case {String: String}:
            "str-dict"
        case _:
            "other"

print test_match({"a": 10!})
print " "
print test_match({"b": "hello"!})
print " "
print test_match(123)
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("unexpected error in match with dict pattern: %v", err)
	}
	expected := "int-dict str-dict other"
	if strings.TrimSpace(out) != expected {
		t.Fatalf("expected %q, got %q", expected, strings.TrimSpace(out))
	}
}

func TestEvalDictKeyTypeStrictness(t *testing.T) {
	// 1. Separate keys for 1 and "1", true and "true"
	code1 := `
d = {1: "int_one", "1": "str_one", true: "bool_true", "true": "str_true"}
print d.Len()
print " "
print d[1]
print " "
print d["1"]
print " "
print d[true]
print " "
print d["true"]
`
	out1, err := runLesCode(code1)
	if err != nil {
		t.Fatalf("unexpected error evaluating code1: %v", err)
	}
	expected1 := "4 int_one str_one bool_true str_true"
	if strings.TrimSpace(out1) != expected1 {
		t.Fatalf("expected %q, got %q", expected1, strings.TrimSpace(out1))
	}

	// 2. Cross-type lookup must fail with runtime error
	code2 := `
d = {1: "only_int"}
print d["1"]
`
	_, err2 := runLesCode(code2)
	if err2 == nil {
		t.Fatalf("expected error accessing d[\"1\"] when key is Int 1, but got nil")
	}
	if !strings.Contains(err2.Error(), "key not found in dict: 1") {
		t.Fatalf("expected 'key not found in dict: 1', got %v", err2)
	}

	code3 := `
d = {"true": "str_true"}
print d[true]
`
	_, err3 := runLesCode(code3)
	if err3 == nil {
		t.Fatalf("expected error accessing d[true] when key is String \"true\", but got nil")
	}
	if !strings.Contains(err3.Error(), "key not found in dict: true") {
		t.Fatalf("expected 'key not found in dict: true', got %v", err3)
	}

	// 3. 'is' check distinguishes key types
	code4 := `
intDict = {1: "val"}
print (intDict is {Int: String})
print " "
print (intDict is {String: String})
print " "

mixedDict = {1: "a", "1": "b"}
print (mixedDict is {Int | String: String})
print " "
print (mixedDict is {Int: String})
print " "
print (mixedDict is {String: String})
`
	out4, err := runLesCode(code4)
	if err != nil {
		t.Fatalf("unexpected error evaluating code4: %v", err)
	}
	expected4 := "true false true false false"
	if strings.TrimSpace(out4) != expected4 {
		t.Fatalf("expected %q, got %q", expected4, strings.TrimSpace(out4))
	}
}

func TestThunkTypeDetermination(t *testing.T) {
	// 1. Literal dict (1B: constant literal immediate evaluation)
	code1 := `
d = {"a": "hello"}
print (d is {String: Int})
print " "
print (d is {String: String})
`
	out1, err := runLesCode(code1)
	if err != nil {
		t.Fatalf("unexpected error in code1: %v", err)
	}
	if out1 != "false true" {
		t.Fatalf("expected 'false true', got %q", out1)
	}

	// 2. Expression dict (1A: inferred type propagated, non-forcing resolution)
	code2 := `
d = {"a": "he" + "llo"}
print (d is {String: Int})
print " "
print (d is {String: String})
`
	out2, err := runLesCode(code2)
	if err != nil {
		t.Fatalf("unexpected error in code2: %v", err)
	}
	if out2 != "false true" {
		t.Fatalf("expected 'false true', got %q", out2)
	}

	// 3. Delayed exception expression (1A: compatible inferred type skips force; incompatible rejects without force)
	code3 := `
d = {"a": 1 / 0}
print (d is {String: Int})
print " "
print (d is {String: String})
`
	out3, err := runLesCode(code3)
	if err != nil {
		t.Fatalf("unexpected error in code3 (should not force 1/0): %v", err)
	}
	if out3 != "true false" {
		t.Fatalf("expected 'true false', got %q", out3)
	}

	// 4. Dynamic unannotated function return (TypeUnknown preserves laziness until forced)
	code4 := `

def get_val(x):
    x

d = {"a": get_val("hello")}
print (d is {String: Int})
print " "
_ = d["a"]!
print (d is {String: Int})
`
	out4, err := runLesCode(code4)
	if err != nil {
		t.Fatalf("unexpected error in code4: %v", err)
	}
	if out4 != "true false" {
		t.Fatalf("expected 'true false', got %q", out4)
	}

	// 5. Tuple with constant literals, inferred expressions, and delayed exceptions
	code5 := `
t1 = (1, "hello")
print (t1 is (Int, Int))
print " "
print (t1 is (Int, String))
print " "

t2 = (1, "he" + "llo")
print (t2 is (Int, Int))
print " "
print (t2 is (Int, String))
print " "

t3 = (1, 1 / 0)
print (t3 is (Int, Int))
print " "
print (t3 is (Int, String))
`
	out5, err := runLesCode(code5)
	if err != nil {
		t.Fatalf("unexpected error in code5: %v", err)
	}
	if out5 != "false true false true true false" {
		t.Fatalf("expected 'false true false true true false', got %q", out5)
	}
}

func TestRadixIntegerLiteralsAndDigitSeparators(t *testing.T) {
	code := `
b1 = 0b1010
b2 = 0B1100_0011
o1 = 0o755
o2 = 0O644
d1 = 0d100
d2 = 0D1_000_000
x1 = 0xdead_beef
x2 = 0XFF
sep_int = 1_000_000
sep_float = 3.14159_26535
sep_float2 = 1_000.5_0

print b1, " "
print b2, " "
print o1, " "
print o2, " "
print d1, " "
print d2, " "
print x1, " "
print x2, " "
print sep_int, " "
print sep_float, " "
print sep_float2
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "10 195 493 420 100 1000000 3735928559 255 1000000 3.1415926535 1000.5"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

func TestInvalidNumericLiterals(t *testing.T) {
	invalidCases := []struct {
		name string
		code string
	}{
		{"consecutive dots", "x = 1.2.3\nprint x"},
		{"trailing underscore", "x = 1_\nprint x"},
		{"invalid prefix suffix", "x = 0a0\nprint x"},
		{"leading dot float without zero", "x = .1\nprint x"},
		{"trailing dot float without zero", "x = 1.\nprint x"},
		{"consecutive underscores", "x = 123__456\nprint x"},
		{"incomplete hex prefix", "x = 0x\nprint x"},
		{"incomplete binary prefix", "x = 0b\nprint x"},
		{"underscore immediately after prefix", "x = 0x_12\nprint x"},
		{"float with trailing underscore", "x = 3.14_\nprint x"},
		{"float with underscore after dot", "x = 3._14\nprint x"},
	}

	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := runLesCode(tc.code)
			if err == nil {
				t.Fatalf("expected syntax/eval error for %q, but succeeded with output %q", tc.code, out)
			}
		})
	}
}

func TestBlockLiteralFollowedByStatement(t *testing.T) {
	code := `
def mymap(t, fn):
  i = 0
  for i < t.Len():
    fn(t[i])
    i = i + 1

mymap(["a", "b"]) do |s|:
  print s, " "

print "done1 "
print "done2"
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "a b done1 done2"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

func TestStructInitAutoBindingAndValidation(t *testing.T) {
	code := `
struct Person:
    name: String
    age: Int
    city: String = "Tokyo"

    def init(name, age):
        if age < 0:
            error("Age cannot be negative")

p1 = Person("Alice", 30)
p2 = Person(name: "Bob", age: 25)
print p1.name, " ", p1.age, " ", p1.city, " | ", p2.name, " ", p2.age, " ", p2.city
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "Alice 30 Tokyo | Bob 25 Tokyo"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}

	// Validation failure check
	errCode := `
struct Person:
    name: String
    age: Int
    def init(name, age):
        if age < 0:
            exit(1)

Person("Invalid", -5)
`
	_, err = runLesCode(errCode)
	if err == nil {
		t.Fatalf("expected error for negative age, but succeeded")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("expected ExitError with code 1, got %v", err)
	}
}

func TestStructInitExplicitMemberAssign(t *testing.T) {
	code := `
struct Point:
    x: Int
    y: Int

    def init(multiplier: Int, base: Int):
        self.x = base * multiplier
        self.y = (base + 1) * multiplier

p = Point(10, 5)
print p.x, " ", p.y
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "50 60"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

func TestStructInitImmutabilityOutsideInit(t *testing.T) {
	code := `
struct Point:
    x: Int
    y: Int

    def init(x, y):
        self.x = x
        self.y = y

p = Point(1, 2)
p.x = 100
`
	_, err := runLesCode(code)
	if err == nil {
		t.Fatalf("expected error when mutating immutable struct field outside init, but succeeded")
	}
	if !strings.Contains(err.Error(), "cannot mutate field 'x' of immutable struct 'Point'") {
		t.Fatalf("expected immutable struct mutation error, got: %v", err)
	}
}

func TestStructInitWithCopyUpdate(t *testing.T) {
	code := `
struct Point:
    x: Int
    y: Int

    def init(x, y):
        self.x = x * 10
        self.y = y * 10

p1 = Point(1, 2)
# Copy-update bypasses init and preserves/overrides fields directly
p2 = Point(p1, x: 99)
print p1.x, " ", p1.y, " | ", p2.x, " ", p2.y
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "10 20 | 99 20"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

func TestStructInitVariadic(t *testing.T) {
	code := `
struct StringList:
    t: (String, ...)

    def init(*t: String):
        ;

    def join():
        res = ""
        i = 0
        for i < t.Len():
            if i > 0:
                res = res + ","
            res = res + t[i]
            i = i + 1
        res

sl0 = StringList()
sl1 = StringList("foo")
sl2 = StringList("foo", "bar")

print sl0.t.Len(), " ", sl1.join(), " ", sl2.join()
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "0 foo foo,bar"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

func TestEmptyBlockExecution(t *testing.T) {
	code := `
def noop():

struct Item:
    val: Int

    def init(val: Int):

    def clear():

item = Item(42)
r1 = noop()
r2 = item.clear()
print item.val, " ", r1, " ", r2
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "42 () ()"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

func TestStringIntAndIntString(t *testing.T) {
	code := `
# 1. String.Int() default radix 10
r1 = "123".Int()
r2 = "123a".Int()
r3 = "0x".Int()
r4 = "-42abc".Int()
r5 = "+99xyz".Int()
r6 = "".Int()
r7 = "xyz".Int()

# 2. String.Int(radix) with 2, 8, 16
r8 = "123a".Int(16)
r9 = "0x".Int(16)
r10 = "0b101".Int(2)
r11 = "101".Int(2)
r12 = "755".Int(8)
r13 = "-1010xyz".Int(2)

# 3. Int.String() default radix 10
s1 = 123.String()
s2 = (-42).String()
s3 = 0.String()

# 4. Int.String(radix) with 2, 8, 16
s4 = 255.String(16)
s5 = 10.String(2)
s6 = 493.String(8)
s7 = (-255).String(16)

print r1, " ", r2, " ", r3, " ", r4, " ", r5, " ", r6, " ", r7, " | ", \
      r8, " ", r9, " ", r10, " ", r11, " ", r12, " ", r13, " | ", \
      s1, " ", s2, " ", s3, " | ", \
      s4, " ", s5, " ", s6, " ", s7
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("unexpected eval error: %v", err)
	}
	expected := "123 123 0 -42 99 0 0 | 4666 0 0 5 493 -10 | 123 -42 0 | ff 1010 755 -ff"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}

	// 5. Unsupported radix errors
	badRadixCases := []string{
		`"123".Int(3)`,
		`"123".Int(0)`,
		`"123".Int(36)`,
		`123.String(5)`,
		`123.String(12)`,
	}
	for _, badCode := range badRadixCases {
		_, err := runLesCode(badCode)
		if err == nil {
			t.Fatalf("expected error for bad radix code: %s, got nil", badCode)
		}
		if !strings.Contains(err.Error(), "unsupported radix") {
			t.Fatalf("expected 'unsupported radix' error, got: %v", err)
		}
	}

	// 6. Arbitrary-precision large integer parsing & MinInt64
	codeOverflow := `
maxOverflow = "99999999999999999999999999999".Int()
minOverflow = "-99999999999999999999999999999".Int()
exactMin = "-9223372036854775808".Int()
exactMax = "9223372036854775807".Int()
print maxOverflow, " ", minOverflow, " ", exactMin, " ", exactMax
`
	outOvf, err := runLesCode(codeOverflow)
	if err != nil {
		t.Fatalf("unexpected eval error on overflow code: %v", err)
	}
	expectedOvf := "99999999999999999999999999999 -99999999999999999999999999999 -9223372036854775808 9223372036854775807"
	if outOvf != expectedOvf {
		t.Fatalf("expected %q, got %q", expectedOvf, outOvf)
	}
}

func TestFunctionTypeReflection(t *testing.T) {
	code := `
def add(a: Int, b: Int): Int:
    a + b

def inc(x: Int): Int:
    x + 1

def answer(): Int:
    42

def sum(*nums: Int): Int:
    0

f = |x: String|: x + "!"

struct Person:
    name: String
    def Greet(): String:
        "Hello"

p = Person("Alice")

print add.Type(), "|"
print inc.Type(), "|"
print answer.Type(), "|"
print sum.Type(), "|"
print f.Type(), "|"
print p.Greet.Type(), "|"
print print.Type(), "|"
print exit.Type(), "|"
print Bytes.Type(), "|"
print gets.Type(), "|"
s = "test"
print s.Ord.Type(), "|"
print s.Bytes.Type()
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("unexpected eval error in TestFunctionTypeReflection: %v", err)
	}

	expected := "(Int, Int) -> Int|(Int) -> Int|() -> Int|(Int, ...) -> Int|(String) -> String|() -> String|(Show, ...) -> ()|(Int) -> Nil|(String | Bytes) -> Bytes|() -> String | Nil|() -> Int|() -> Bytes"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}

	edgeCode := `
import bufio
import io

shadowed = 123

def outer(): String:
    def shadowed(s: String): String:
        s
    if true:
        def inIf(n: Int): Bool:
            n > 0
        return shadowed.Type() + "|" + inIf.Type()
    return ""

varLambda = |*nums: Int|: 0

def withBlock(&b: (Int) -> String): String:
    b.Type()

blkType = withBlock() do |x: Int|:
    "ok"

struct Base:
    def BaseVal(): Int:
        10

struct Sub:
    Base

sub = Sub()
errObj = Error("fail")

print outer(), "|"
print varLambda.Type(), "|"
print blkType, "|"
print sub.BaseVal.Type(), "|"
print Stdin.ReadLine.Type(), "|"
print Stdout.Write.Type(), "|"
print Stderr.Write.Type(), "|"
print (@bufio.Reader).Type(), "|"
print Error.Type(), "|"
print errObj.Message.Type(), "|"
print shadowed == 123, "|"
def topAdd(a: Int, b: Int): Int:
    a + b
print topAdd is (Int, Int) -> Int, "|"
print topAdd is (String) -> String, "|"
print topAdd is Function
`
	edgeOut, err := runLesCode(edgeCode)
	if err != nil {
		t.Fatalf("unexpected eval error in edgeCode: %v", err)
	}
	expectedEdge := "(String) -> String|(Int) -> Bool|(Int, ...) -> Int|(Int) -> String|() -> Int|() -> String | Nil|(Bytes) -> Int|(Bytes) -> Int|(io.Reader) -> bufio.Reader|(Show) -> Error|() -> String|true|true|false|true"
	if edgeOut != expectedEdge {
		t.Fatalf("expected %q, got %q", expectedEdge, edgeOut)
	}

	// Test StructDef constructor reflection and &block method reflection
	structReflectCode := `
interface List:
    Each(block)

struct StringList:
    t: (String, ...)

    def init(*t: String):
        ()

    def Each(&block: (String) -> ()) -> ():
        ()

    def Map(prefix: String, &block: (String) -> String) -> (String, ...):
        ()

sl = StringList("a", "b")
print StringList.Type(), "|"
print sl.Each.Type(), "|"
print sl.Map.Type(), "|"
print StringList is (String, ...) -> StringList, "|"
print StringList is Function, "|"
print sl.Each is ((String) -> ()) -> (), "|"
print sl is List
`
	structReflectOut, err := runLesCode(structReflectCode)
	if err != nil {
		t.Fatalf("unexpected eval error in structReflectCode: %v", err)
	}
	expectedStructReflect := "(String, ...) -> StringList|((String) -> ()) -> ()|(String, (String) -> String) -> (String, ...)|true|true|true|true"
	if structReflectOut != expectedStructReflect {
		t.Fatalf("expected %q, got %q", expectedStructReflect, structReflectOut)
	}
}

func TestOperatorMethodsAndEqualComparableInterfaces(t *testing.T) {
	code := `
struct Vec2:
    x: Int
    y: Int

    def +(other: Vec2):
        Vec2(x: x + other.x, y: y + other.y)

    def -(other: Vec2):
        Vec2(x: x - other.x, y: y - other.y)

    def -():
        Vec2(x: -x, y: -y)

    def +():
        self

    def ==(other: Vec2):
        x == other.x && y == other.y

    def <(other: Vec2):
        x < other.x

    def [](idx: Int):
        idx == 0 ? x : y

v1 = Vec2(x: 1, y: 2)
v2 = Vec2(x: 3, y: 4)

v3 = v1 + v2
print v3.x
print ", "
print v3.y
print "|"

v4 = v2 - v1
print v4.x
print ", "
print v4.y
print "|"

neg = -v1
print neg.x
print ", "
print neg.y
print "|"

pos = +v1
print pos.x
print ", "
print pos.y
print "|"

print (v1 == v2)
print "|"
print (v1 != v2)
print "|"
print (v1 == Vec2(x: 1, y: 2))
print "|"
print (v1 != Vec2(x: 1, y: 2))
print "|"
print (v1 < v2)
print "|"
print v1[0]
print ", "
print v1[1]
print "|"

# Test dot access on operators
print ((42).+(10))
print "|"
print ((42).-())
print "|"
print ((42).+())
print "|"
print (("hello").[](1))
print "|"

# Overload reflection on + and -
print (42).+.Type()
print "|"
print (42).-.Type()
print "|"

# Pipeline unary
print 42 |> -
print "|"
print (-5) |> +
print "|"

# Equal & Comparable interfaces
print (42 is Equal)
print "|"
print (42 is Comparable)
print "|"
print ("abc" is Equal)
print "|"
print ("abc" is Comparable)
print "|"
print ((Bytes("abc")) is Comparable)
print "|"
print (v1 is Equal)
print "|"
print (v1 is Comparable)
print "|"
print (v1 <= v2)
print "|"
print (v2 > v1)
print "|"
print (v2 >= v1)
print "|"
print ((|x|: x) is Equal)
print "|"
print ((1, 2).[].Type())
print "|"
print (10 |> - |> (|x|: x + 5))
print "|"
print (42 |> -(5))
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("unexpected eval error in TestOperatorMethodsAndEqualComparableInterfaces: %v", err)
	}

	expected := "4, 6|2, 2|-1, -2|1, 2|false|true|true|false|true|1, 2|52|-42|42|e|() -> Int | (Int) -> Int|() -> Int | (Int) -> Int|-42|-5|true|true|true|true|true|true|true|true|true|true|false|(Int) -> Unknown|-5|37"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

func TestInferredFunctionAndMethodTypeReflection(t *testing.T) {
	code := `
def noop():
    ;

def loopVoid(t: (String, ...)):
    i = 0
    for i < t.Len():
        i = i + 1

def branchInt(x: Int):
    if x > 0:
        return 1
    return 2

def branchUnion(x: Int):
    if x > 0:
        return "pos"
    return 0

def fib(n: Int):
    if n <= 1:
        return n
    return fib(n - 1) + fib(n - 2)

struct Calculator:
    def init():
        ;

    def clear():
        x = 0

    def abs(x: Int):
        if x >= 0:
            x
        else:
            -x

calc = Calculator()

print noop.Type()
print "|"
print loopVoid.Type()
print "|"
print branchInt.Type()
print "|"
print branchUnion.Type()
print "|"
print fib.Type()
print "|"
print Calculator.Type()
print "|"
print calc.clear.Type()
print "|"
print calc.abs.Type()
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("unexpected eval error in TestInferredFunctionAndMethodTypeReflection: %v", err)
	}

	expected := "() -> ()|((String, ...)) -> ()|(Int) -> Int|(Int) -> String | Int|(Int) -> Int|() -> Calculator|() -> ()|(Int) -> Int"
	if out != expected {
		t.Fatalf("expected %q, got %q", expected, out)
	}
}

func TestBitwiseOperatorsEval(t *testing.T) {
	code := `
print (~0), " "
print (~(-1)), " "
print (~5), " "
print (12 & 10), " "
print (12 | 10), " "
print (12 ^ 10), " "
print (1 << 3), " "
print (-1 << 2), " "
print (16 >> 2), " "
print (-1 >> 1), " "
print (1 << 64), " "
print (1 >> 64), " "
print (-1 >> 64), " "
print (1 << -1), " "
print (16 >> -2), " "
print (-8 >> 1), " "
print (1 | 2 ^ 2 & 2 == 1), " "
print (1 | 2 is Int), " "
print (0 |> ~), " "
print (15 |> &(10)), " "
print (1 |> <<(4)), " "
print (-8 |> >>(1))
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("unexpected eval error in TestBitwiseOperatorsEval: %v", err)
	}

	expected := "-1 0 -6 8 14 6 8 -4 4 -1 18446744073709551616 0 -1 0 64 -4 true true -1 10 16 -4"
	if out != expected {
		t.Fatalf("expected:\n%s\ngot:\n%s", expected, out)
	}

	// Custom struct operator methods and method tear-offs
	structCode := `
struct Flag:
    val: Int
    def ~(self) -> Flag:
        Flag(val: ~self.val)
    def &(self, other: Flag) -> Flag:
        Flag(val: self.val & other.val)
    def |(self, other: Flag) -> Flag:
        Flag(val: self.val | other.val)
    def ^(self, other: Flag) -> Flag:
        Flag(val: self.val ^ other.val)
    def <<(self, n: Int) -> Flag:
        Flag(val: self.val << n)
    def >>(self, n: Int) -> Flag:
        Flag(val: self.val >> n)

f1 = Flag(val: 12)
f2 = Flag(val: 10)
print (~f1).val, " "
print (f1 & f2).val, " "
print (f1 | f2).val, " "
print (f1 ^ f2).val, " "
print (f1 << 1).val, " "
print (f1 >> 1).val, " "

# Builtin Int operator method call
andFn = (12).&
print andFn(10), " "
tildeFn = (0).~
print tildeFn(), " "

# Pipeline into lambda
pipeLam = 5 |> |v|: v * 2 + 1
print pipeLam, " "

# Interface check with bitwise operator method
interface BitwiseNot:
    ~(): Int

interface BadNot:
    ~(x: Int): Int

print (42 is BitwiseNot), " "
print (42 is BadNot)
`
	outStruct, err := runLesCode(structCode)
	if err != nil {
		t.Fatalf("unexpected eval error for custom struct operators: %v", err)
	}
	expectedStruct := "-13 8 14 6 24 6 8 -1 11 true false"
	if outStruct != expectedStruct {
		t.Fatalf("expected struct:\n%s\ngot:\n%s", expectedStruct, outStruct)
	}
	// Interface with explicit self and builtin operator arity matching
	extraCode := `
interface Negatable:
    -(): Int

interface Adder:
    +(other: Int): Int

interface ExplicitSelfBitwise:
    ~(self): Int
    &(self, other: Int): Int

interface BadStringer:
    String(x: String): String

interface StrBitAnd:
    &(other: String): Int

interface MyShow:
    ToString(self): String

print (10 is Negatable), " "
print (10 is Adder), " "
print (10 is ExplicitSelfBitwise), " "
print (10 is BadStringer), " "
print (10 is StrBitAnd), " "
print (10 is MyShow)
`
	outExtra, err := runLesCode(extraCode)
	if err != nil {
		t.Fatalf("unexpected eval error: %v", err)
	}
	expectedExtra := "true true true false false true"
	if outExtra != expectedExtra {
		t.Fatalf("expected:\n%s\ngot:\n%s", expectedExtra, outExtra)
	}
}

func TestArbitraryPrecisionInt(t *testing.T) {
	code := `
a = 9223372036854775807
b = a + 1
c = b - 1
print (b.String()), " "
print (c.String()), " "

# Multiplication & division expansion and demotion
huge = 1 << 100
small = huge >> 90
print (small == 1024), " "

divSmall = huge / (1 << 90)
print (divSmall == 1024), " "

rem = huge % ((1 << 90) + 1)
print (rem > 0), " "

# MinInt64 negation
minVal = -9223372036854775808
negMin = -minVal
print (negMin == b), " "

# Comparisons between small and big Int
print (b > a), " "
print (a < b), " "
print (b == a + 1), " "
print (b != a), " "
print (-b == minVal), " "
print (-(b + 1) < minVal), " "

# Bitwise on big integers
print (b & a), " "
print (b | a == (1 << 64) - 1), " "
print (b ^ a == (1 << 64) - 1), " "
print (~b == -b - 1), " "

# Ruby shift semantics
print (100 << -2), " "
print (100 >> -2), " "
print (-100 >> 2), " "
print (-1 >> 100), " "
print (1 >> 100), " "

# String and radix formatting / parsing
hexStr = b.String(16)
print hexStr, " "
parsedHex = hexStr.Int(16)
print (parsedHex == b), " "

# Dict keying with big Int
d = {b: "huge", a: "max64"}
print d[b], " "
print d[a]
`
	out, err := runLesCode(code)
	if err != nil {
		t.Fatalf("unexpected eval error in TestArbitraryPrecisionInt: %v", err)
	}

	expected := "9223372036854775808 9223372036854775807 true true true true true true true true true true 0 true true true 25 400 -25 -1 0 8000000000000000 true huge max64"
	if out != expected {
		t.Fatalf("expected:\n%s\ngot:\n%s", expected, out)
	}

	// Verify huge shift limit safety
	_, err = runLesCode("1 << 100000000")
	if err == nil {
		t.Fatalf("expected error for excessive shift count, got nil")
	}
	if !strings.Contains(err.Error(), "shift count too large") {
		t.Fatalf("expected 'shift count too large' error, got %v", err)
	}

	// Verify chained shift limit safety
	_, err = runLesCode("(1 << 60000000) << 60000000")
	if err == nil {
		t.Fatalf("expected error for chained excessive shift count, got nil")
	}
	if !strings.Contains(err.Error(), "shift count too large") {
		t.Fatalf("expected 'shift count too large' error, got %v", err)
	}

	// Verify exit with big int preserves non-zero exit code
	ev := NewEvaluator()
	prog := parser.NewParser(lexer.NewLexer("exit(1 << 70)")).ParseProgram()
	_, err = ev.EvalProgram(prog)
	if exitErr, ok := err.(*ExitError); !ok || exitErr.Code == 0 {
		t.Fatalf("expected non-zero ExitError, got %v", err)
	}
}

func TestUserDefinedGenericsEval(t *testing.T) {
	// 1. Generic struct with call-site inference and explicit instantiation
	code1 := `
struct Box[T]:
    val: T

b1 = Box(10)
b2 = Box[String]("hello")
print b1.Type(), "\n"
print b1.val, "\n"
print (b1 is Box[Int]), "\n"
print (b1 is Box), "\n"
print (b1 is Box[String]), "\n"
print b2.Type(), "\n"
print b2.val, "\n"
print (b2 is Box[String]), "\n"
print (b2 is Box[Int]), "\n"
`
	out1, err := runLesCode(code1)
	if err != nil {
		t.Fatalf("code1 failed: %v", err)
	}
	expected1 := strings.Join([]string{
		"Box[Int]",
		"10",
		"true",
		"true",
		"false",
		"Box[String]",
		"hello",
		"true",
		"false",
	}, "\n")
	if out1 != expected1 {
		t.Fatalf("unexpected output for code1:\ngot:\n%s\nexpected:\n%s", out1, expected1)
	}

	// 2. Multi-parameter generic struct
	code2 := `
struct Pair[K, V]:
    first: K
    second: V

p = Pair("answer", 42)
print p.Type(), "\n"
print (p is Pair[String, Int]), "\n"
print (p is Pair[Int, String]), "\n"
print (p is Pair), "\n"
`
	out2, err := runLesCode(code2)
	if err != nil {
		t.Fatalf("code2 failed: %v", err)
	}
	expected2 := strings.Join([]string{
		"Pair[String, Int]",
		"true",
		"false",
		"true",
	}, "\n")
	if out2 != expected2 {
		t.Fatalf("unexpected output for code2:\ngot:\n%s\nexpected:\n%s", out2, expected2)
	}

	// 3. Generic function inference, explicit specialization, and constraints
	code3 := `
def id[T](x: T) -> T:
    x

print id(123), "\n"
print id[String]("abc"), "\n"

def add[T: Int | Float](a: T, b: T) -> T:
    a + b

print add(10, 20), "\n"
print add(1.5, 2.5), "\n"
`
	out3, err := runLesCode(code3)
	if err != nil {
		t.Fatalf("code3 failed: %v", err)
	}
	expected3 := strings.Join([]string{
		"123",
		"abc",
		"30",
		"4",
	}, "\n")
	if out3 != expected3 {
		t.Fatalf("unexpected output for code3:\ngot:\n%s\nexpected:\n%s", out3, expected3)
	}

	// 4. Pattern matching with generic types
	code4 := `
struct Container[T]:
    item: T

def inspect(c):
    match c:
        case Container[Int]:
            "container of int: " + c.item.ToString()
        case Container[String]:
            "container of string: " + c.item
        case _:
            "something else"

print inspect(Container(42)), "\n"
print inspect(Container("apple")), "\n"
print inspect(Container(true)), "\n"
`
	out4, err := runLesCode(code4)
	if err != nil {
		t.Fatalf("code4 failed: %v", err)
	}
	expected4 := strings.Join([]string{
		"container of int: 42",
		"container of string: apple",
		"something else",
	}, "\n")
	if out4 != expected4 {
		t.Fatalf("unexpected output for code4:\ngot:\n%s\nexpected:\n%s", out4, expected4)
	}

	// 5. Generic interface implementation check
	code5 := `
interface Iterator[T]:
    Next(): T

struct IntIter:
    val: Int
    def Next() -> Int:
        self.val

struct StrIter:
    val: String
    def Next() -> String:
        self.val

i = IntIter(100)
s = StrIter("stream")

print (i is Iterator[Int]), "\n"
print (i is Iterator[String]), "\n"
print (i is Iterator), "\n"
print (s is Iterator[String]), "\n"
print (s is Iterator[Int]), "\n"
`
	out5, err := runLesCode(code5)
	if err != nil {
		t.Fatalf("code5 failed: %v", err)
	}
	expected5 := strings.Join([]string{
		"true",
		"false",
		"true",
		"true",
		"false",
	}, "\n")
	if out5 != expected5 {
		t.Fatalf("unexpected output for code5:\ngot:\n%s\nexpected:\n%s", out5, expected5)
	}

	// 6. Generic struct laziness preservation
	code6 := `
struct LazyBox[T]:
    val: T

b = LazyBox(1 / 0)
print (b is LazyBox), "\n"
`
	out6, err := runLesCode(code6)
	if err != nil {
		t.Fatalf("lazy generic struct failed unexpectedly: %v", err)
	}
	if strings.TrimSpace(out6) != "true" {
		t.Fatalf("expected true, got %q", out6)
	}

	// 7. Generic function with self-referential / F-bounded constraint
	code7 := `
interface Ord[T]:
    <=(other: T): Bool

struct Item:
    num: Int
    def <=(other: Item) -> Bool:
        self.num <= other.num

def getMin[T: Ord[T]](a: T, b: T) -> T:
    if a <= b:
        a
    else:
        b

it1 = Item(10)
it2 = Item(20)
m = getMin(it1, it2)
print m.num, "\n"
`
	out7, err := runLesCode(code7)
	if err != nil {
		t.Fatalf("code7 failed: %v", err)
	}
	if strings.TrimSpace(out7) != "10" {
		t.Fatalf("expected 10, got %q", out7)
	}

	// 8. Inferred generic struct method reflection and interface satisfaction
	code8 := `
interface Getter[T]:
    Get(): T

struct Box[T]:
    val: T
    def Get(): T:
        self.val

b = Box(10)
print b.Get.Type(), "\n"
print (b is Getter[Int]), "\n"
print (b is Getter[String]), "\n"
`
	out8, err := runLesCode(code8)
	if err != nil {
		t.Fatalf("code8 failed: %v", err)
	}
	expected8 := strings.Join([]string{
		"() -> Int",
		"true",
		"false",
	}, "\n")
	if strings.TrimSpace(out8) != strings.TrimSpace(expected8) {
		t.Fatalf("unexpected output for code8:\ngot:\n%s\nexpected:\n%s", out8, expected8)
	}

	// 9. Embedded generic interface satisfaction
	code9 := `
interface Source[T]:
    Fetch(): T

interface Pipeline[T]:
    Source[T]
    Process(): T

struct DataStream:
    val: Int
    def Fetch() -> Int:
        self.val
    def Process() -> Int:
        self.val * 2

ds = DataStream(21)
print (ds is Pipeline[Int]), "\n"
print (ds is Pipeline[String]), "\n"
`
	out9, err := runLesCode(code9)
	if err != nil {
		t.Fatalf("code9 failed: %v", err)
	}
	expected9 := strings.Join([]string{
		"true",
		"false",
	}, "\n")
	if strings.TrimSpace(out9) != strings.TrimSpace(expected9) {
		t.Fatalf("unexpected output for code9:\ngot:\n%s\nexpected:\n%s", out9, expected9)
	}
}





func TestGenericBoxDictAndIndexing(t *testing.T) {
	code := `struct Box[T]:
    val: T

b = Box[{String: Int}]({ "a": 10 })
res1 = b.val["a"]

struct List[T]:
    items: (T, ...)
    def [](i: Int) -> T:
        self.items[i]

l = List((10, 20, 30))
idx = 1
res2 = l[idx]`
	out, err := runLesCode(code + "\nprint(res1)\nprint(res2)")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	expected := "1020"
	if strings.TrimSpace(out) != strings.TrimSpace(expected) {
		t.Fatalf("unexpected output:\ngot:\n%s\nexpected:\n%s", out, expected)
	}
}
