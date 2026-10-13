package types

import (
	"os"
	"path/filepath"
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

func TestSameScopeRebindingAllowed(t *testing.T) {
	input := `x = 10
x = 20`
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	prog := p.ParseProgram()

	inf := NewInferencer()
	err := inf.InferProgram(prog)
	if err != nil {
		t.Fatalf("expected same-type rebinding to succeed, got %v", err)
	}
}

func TestSameScopeTypeMismatchRebindingError(t *testing.T) {
	input := `x = 10
x = "string"`
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	prog := p.ParseProgram()

	inf := NewInferencer()
	err := inf.InferProgram(prog)
	if err == nil {
		t.Fatalf("expected type mismatch on rebinding error, got nil")
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
res = true ? 42 : "hello"
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

func TestInterfaceTypeInferenceAndDuckTyping(t *testing.T) {
	input := `
interface Reader:
    read(n: Int): String

interface Closer:
    close(): ()

interface ReadCloser:
    Reader
    Closer

struct File:
    path: String

    def read(n: Int):
        "data"

    def close():
        ()

f: File = File(path: "/tmp/foo")
# Structural assignability: File implements ReadCloser without 'implements' keyword
rc: ReadCloser = f
r: Reader = rc
res = r.read(10)
`
	prog := parser.NewParser(lexer.NewLexer(input)).ParseProgram()
	inf := NewInferencer()
	err := inf.InferProgram(prog)
	if err != nil {
		t.Fatalf("type inference error for interface duck typing: %v", err)
	}

	resT, ok := inf.scope.Lookup("res")
	if !ok || resT != TypeString {
		t.Fatalf("expected res to be String, got %v", resT)
	}
}

func TestInterfaceConflictingSignatures(t *testing.T) {
	inputConflict := `
interface A:
    test(): Int

interface B:
    test(): String

interface C:
    A
    B
`
	prog := parser.NewParser(lexer.NewLexer(inputConflict)).ParseProgram()
	inf := NewInferencer()
	err := inf.InferProgram(prog)
	if err == nil {
		t.Fatalf("expected conflict error when embedding interfaces with different signatures for test()")
	}
}

func TestFunctionTypeAnnotation(t *testing.T) {
	input := `
# 1. Variable with function type annotation
f: (Int) -> Int = |x|: x * 2

# 2. Higher-order function taking function parameter
def apply(fn: (Int) -> String, x: Int): String:
    fn(x)

# 3. Function returning a function
def multiplier(factor: Int): (Int) -> Int:
    |x: Int|: x * factor

# 4. Zero-argument function
getter: () -> Nil = ||: nil

# 5. Multi-argument function type
adder: (Int, Int) -> Int = |a, b|: a + b
`
	p := parser.NewParser(lexer.NewLexer(input))
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	inf := NewInferencer()
	err := inf.InferProgram(prog)
	if err != nil {
		t.Fatalf("type inference error for function type annotation: %v", err)
	}

	fT, ok := inf.scope.Lookup("f")
	if !ok {
		t.Fatalf("variable f not found")
	}
	fnType, ok := fT.(*FuncType)
	if !ok {
		t.Fatalf("expected FuncType, got %T", fT)
	}
	if fnType.String() != "(Int) -> Int" {
		t.Fatalf("expected (Int) -> Int, got %s", fnType.String())
	}
}

func TestFunctionTypeMismatch(t *testing.T) {
	inputMismatch := `
f: (Int) -> String = |x: Int|: x + 1
`
	prog := parser.NewParser(lexer.NewLexer(inputMismatch)).ParseProgram()
	inf := NewInferencer()
	err := inf.InferProgram(prog)
	if err == nil {
		t.Fatalf("expected type mismatch error assigning (Int) -> Int to (Int) -> String, got nil")
	}
}

func TestHigherOrderFunctionTypeInference(t *testing.T) {
	input := `
def map_int(fn: (Int) -> String, x: Int): String:
    fn(x)

def int_to_str(n: Int): String:
    "num"

res: String = map_int(int_to_str, 42)
`
	p := parser.NewParser(lexer.NewLexer(input))
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	inf := NewInferencer()
	err := inf.InferProgram(prog)
	if err != nil {
		t.Fatalf("type inference error: %v", err)
	}

	resT, ok := inf.scope.Lookup("res")
	if !ok || resT != TypeString {
		t.Fatalf("expected res to be String, got %v", resT)
	}
}

func TestModuleTypeChecking(t *testing.T) {
	inf := NewInferencer()
	inf.RegisterModule("math", &ModuleType{
		Name: "math",
		Exports: map[string]Type{
			"Add": &FuncType{
				Arg: &TupleType{Elems: []Type{TypeInt, TypeInt}},
				Ret: TypeInt,
			},
			"Pi": TypeFloat,
		},
	})

	// 1. Success matching
	inputValid := `
import math
res: Int = @math.Add(1, 2)
pi: Float = @math.Pi
`
	progValid := parser.NewParser(lexer.NewLexer(inputValid)).ParseProgram()
	if err := inf.InferProgram(progValid); err != nil {
		t.Fatalf("unexpected type error: %v", err)
	}

	// 2. Type mismatch with module return type
	inputMismatch := `
import math
res: String = @math.Add(1, 2)
`
	progMismatch := parser.NewParser(lexer.NewLexer(inputMismatch)).ParseProgram()
	infMismatch := NewInferencer()
	infMismatch.RegisterModule("math", inf.modules["math"])
	if err := infMismatch.InferProgram(progMismatch); err == nil {
		t.Fatalf("expected type mismatch error, got nil")
	}

	// 3. Undefined member in module
	inputUndefined := `
import math
res = @math.Subtract(10, 5)
`
	progUndefined := parser.NewParser(lexer.NewLexer(inputUndefined)).ParseProgram()
	infUndefined := NewInferencer()
	infUndefined.RegisterModule("math", inf.modules["math"])
	if err := infUndefined.InferProgram(progUndefined); err == nil {
		t.Fatalf("expected undefined member error, got nil")
	}
}

func TestFileModuleLoaderTypeChecking(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "les_mod_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	calcCode := `
module calc

def Add(a: Int, b: Int): Int:
    a + b

def helper(x: Int): Int:
    x * 2

Version = "1.0"
`
	calcPath := filepath.Join(tempDir, "calc.les")
	if err := os.WriteFile(calcPath, []byte(calcCode), 0644); err != nil {
		t.Fatalf("failed to write calc.les: %v", err)
	}

	// 1. Success type checking with FileModuleLoader
	loader := NewFileModuleLoader(tempDir)
	inf := NewInferencer()
	inf.SetModuleLoader(loader.Load)

	mainCode := `
import calc

ans: Int = @calc.Add(10, 20)
ver: String = @calc.Version
`
	prog := parser.NewParser(lexer.NewLexer(mainCode)).ParseProgram()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type error: %v", err)
	}

	// 2. Private member (lowercase 'helper') is not exported
	infPrivate := NewInferencer()
	infPrivate.SetModuleLoader(loader.Load)
	privateCode := `
import calc
x = @calc.helper(5)
`
	progPrivate := parser.NewParser(lexer.NewLexer(privateCode)).ParseProgram()
	if err := infPrivate.InferProgram(progPrivate); err == nil {
		t.Fatalf("expected error accessing unexported/private member 'helper', got nil")
	}

	// 3. Predeclared globals (Stdout, Stdin, Stderr, etc.) are not spuriously exported by user modules
	infBuiltins := NewInferencer()
	infBuiltins.SetModuleLoader(loader.Load)
	builtinLeakCode := `
import calc
s = @calc.Stdout
`
	progBuiltins := parser.NewParser(lexer.NewLexer(builtinLeakCode)).ParseProgram()
	if err := infBuiltins.InferProgram(progBuiltins); err == nil {
		t.Fatalf("expected error accessing unexported predeclared global 'Stdout' on user module, got nil")
	}
}

func TestBuiltinToStringAndShowInterfaceTypeInference(t *testing.T) {
	code := `
# 1. Primitives have ToString(): String
s1: String = 42.ToString()
s2: String = "hello".ToString()
s3: String = (1, 2).ToString()

# 2. Built-in types satisfy Show interface
def format_item(item: Show): String:
    item.ToString()

res1: String = format_item(42)
res2: String = format_item("test")
res3: String = format_item([1, 2, 3])

# 3. Custom struct with ToString satisfies Show
struct Item:
    id: Int
    def ToString():
        "Item"

item = Item(id: 1)
res4: String = format_item(item)
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type error: %v", err)
	}

	// 4. Int does NOT satisfy Error interface (lacks Message(): String)
	errCode := `
def fail_error(e: Error):
    e.Message()

fail_error(42)
`
	progErr := parser.NewParser(lexer.NewLexer(errCode)).ParseProgram()
	infErr := NewInferencer()
	if err := infErr.InferProgram(progErr); err == nil {
		t.Fatalf("expected type error assigning Int to Error interface, got nil")
	}
}

func TestPrintRequiresShowStaticTypeCheck(t *testing.T) {
	// 1. Valid print with Show types
	codeOk := `
struct Item:
    def ToString():
        "item"

print 42, "hello", (1, 2), Item()
`
	progOk := parser.NewParser(lexer.NewLexer(codeOk)).ParseProgram()
	infOk := NewInferencer()
	if err := infOk.InferProgram(progOk); err != nil {
		t.Fatalf("unexpected type error: %v", err)
	}

	// 2. Invalid print with non-Show type (struct without ToString)
	codeErr := `
struct NoShow:
    x: Int

print NoShow(x: 1)
`
	progErr := parser.NewParser(lexer.NewLexer(codeErr)).ParseProgram()
	infErr := NewInferencer()
	if err := infErr.InferProgram(progErr); err == nil {
		t.Fatalf("expected type error printing non-Show struct, got nil")
	}
}

func TestForceExprTypeInference(t *testing.T) {
	// 1. Both expr! and force(expr) preserve inner expression types
	code := `
a: Int = (10 + 20)!
b: Int = force(30)
c: String = ("hello")!
d: String = force("world")
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type error in force expression inference: %v", err)
	}

	// 2. Shadowed force parameter uses the parameter's type
	codeShadow := `
def run_callback(force: (Int) -> String):
    res: String = force(10)
    res
`
	progShadow := parser.NewParser(lexer.NewLexer(codeShadow)).ParseProgram()
	infShadow := NewInferencer()
	if err := infShadow.InferProgram(progShadow); err != nil {
		t.Fatalf("unexpected type error in shadowed force: %v", err)
	}
}

func TestStringOrdAndIntChrTypeInference(t *testing.T) {
	// 1. Valid Ord and Chr method calls
	codeOk := `
s: String = "A"
code: Int = s.Ord()
n: Int = 65
ch: String = n.Chr()
ch2: String = (97).Chr()
`
	progOk := parser.NewParser(lexer.NewLexer(codeOk)).ParseProgram()
	infOk := NewInferencer()
	if err := infOk.InferProgram(progOk); err != nil {
		t.Fatalf("unexpected type error: %v", err)
	}

	// 2. Calling Ord on Int should fail type check
	codeErr1 := `
n: Int = 65
x = n.Ord()
`
	progErr1 := parser.NewParser(lexer.NewLexer(codeErr1)).ParseProgram()
	infErr1 := NewInferencer()
	if err := infErr1.InferProgram(progErr1); err == nil {
		t.Fatalf("expected type error calling Ord on Int, got nil")
	}

	// 3. Calling Chr on String should fail type check
	codeErr2 := `
s: String = "A"
x = s.Chr()
`
	progErr2 := parser.NewParser(lexer.NewLexer(codeErr2)).ParseProgram()
	infErr2 := NewInferencer()
	if err := infErr2.InferProgram(progErr2); err == nil {
		t.Fatalf("expected type error calling Chr on String, got nil")
	}
}

func TestPipelineOperatorTypeInference(t *testing.T) {
	code := `
def inc(x: Int): Int:
    x + 1

def double(x: Int): Int:
    x * 2

def toString(x: Int): String:
    x.ToString()

a: Int = 10 |> inc |> double
b: String = 10 |> inc |> toString
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type error: %v", err)
	}

	codeMismatch := `
def inc(x: Int): Int:
    x + 1

a: String = 10 |> inc
`
	progMismatch := parser.NewParser(lexer.NewLexer(codeMismatch)).ParseProgram()
	infMismatch := NewInferencer()
	if err := infMismatch.InferProgram(progMismatch); err == nil {
		t.Fatalf("expected type mismatch error, got nil")
	}
}

func TestBuiltinGetsTypeInference(t *testing.T) {
	code := `
line: String | Nil = gets()
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type error: %v", err)
	}

	codeErr := `
line: Int = gets()
`
	progErr := parser.NewParser(lexer.NewLexer(codeErr)).ParseProgram()
	infErr := NewInferencer()
	if err := infErr.InferProgram(progErr); err == nil {
		t.Fatalf("expected type mismatch error for gets(), got nil")
	}

	codeArity := `
line = gets(1)
`
	progArity := parser.NewParser(lexer.NewLexer(codeArity)).ParseProgram()
	infArity := NewInferencer()
	if err := infArity.InferProgram(progArity); err == nil {
		t.Fatalf("expected arity mismatch error for gets(1), got nil")
	}
}

func TestFunctionArityMismatch(t *testing.T) {
	codeTooMany := `
def add(a: Int, b: Int): Int:
    a + b

res = add(1, 2, 3)
`
	progTooMany := parser.NewParser(lexer.NewLexer(codeTooMany)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(progTooMany); err == nil {
		t.Fatalf("expected arity mismatch error for extra arguments, got nil")
	}

	codeTooFew := `
def add(a: Int, b: Int): Int:
    a + b

res = add(1)
`
	progTooFew := parser.NewParser(lexer.NewLexer(codeTooFew)).ParseProgram()
	inf2 := NewInferencer()
	if err := inf2.InferProgram(progTooFew); err == nil {
		t.Fatalf("expected arity mismatch error for missing arguments, got nil")
	}
}

func TestIOAndBufioTypeInference(t *testing.T) {
	code := `
import io
import bufio

# Test globals
w1: Int = Stdout.Write("text")
w2: Int = Stderr.Write("err")
Stdout.Flush()
Stderr.Flush()

l: String | Nil = Stdin.ReadLine()
s: String | Nil = Stdin.ReadString(":")
p: String | Nil = Stdin.Peek(5)

# Test @io module
rawW: Int = @io.Stdout.Write("raw")
rawR: String | Nil = @io.Stdin.Read(10)
@io.Stdout.Flush()
@io.Stderr.Flush()

# Test @bufio module constructors
br = @bufio.Reader(@io.Stdin)
bw = @bufio.Writer(@io.Stdout)
l2: String | Nil = br.ReadLine()
bw.Flush()
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("type inference error for io/bufio: %v", err)
	}
}

func TestIOAndBufioTypeAnnotations(t *testing.T) {
	code := `
import io
import bufio

def readAll(r: @io.Reader): String | Nil:
    r.Read(100)

struct CustomReader:
    def Read(n: Int): String | Nil:
        "hello"

cr = CustomReader()
res: String | Nil = readAll(cr)

# Test @bufio.Reader and @bufio.Writer type annotations
def wrapBuf(br: @bufio.Reader, bw: @bufio.Writer): ():
    bw.Flush()

# Test assignability of global Stdin/Stdout to @bufio.Reader/@bufio.Writer
rGlobal: @bufio.Reader = Stdin
wGlobal: @bufio.Writer = Stdout
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("type inference error for qualified type annotations: %v", err)
	}
}

func TestOrdinaryExportedFunctionTypeAnnotationNotUnwrapped(t *testing.T) {
	tempDir := t.TempDir()

	mathCode := `
def Add(a: Int, b: Int): Int:
    a + b
`
	mathPath := filepath.Join(tempDir, "math.les")
	if err := os.WriteFile(mathPath, []byte(mathCode), 0644); err != nil {
		t.Fatalf("failed to write math.les: %v", err)
	}

	loader := NewFileModuleLoader(tempDir)
	inf := NewInferencer()
	inf.SetModuleLoader(loader.Load)

	// An annotation referencing @math.Add should be the function type (Int, Int) -> Int,
	// NOT unwrapped to its return type Int!
	testCode := `
import math

fn: @math.Add = @math.Add
res: Int = fn(10, 20)
`
	prog := parser.NewParser(lexer.NewLexer(testCode)).ParseProgram()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type error when using exported function type annotation: %v", err)
	}

	// Verify that annotating an Int with @math.Add fails because @math.Add is a function type, not Int
	testMismatchCode := `
import math

notInt: @math.Add = 42
`
	progMismatch := parser.NewParser(lexer.NewLexer(testMismatchCode)).ParseProgram()
	infMismatch := NewInferencer()
	infMismatch.SetModuleLoader(loader.Load)
	if err := infMismatch.InferProgram(progMismatch); err == nil {
		t.Fatalf("expected type mismatch annotating Int with @math.Add, but got nil")
	}
}

func TestBuiltinModuleTypeAnnotationsWithoutImport(t *testing.T) {
	inf := NewInferencer()
	testCode := `
def readStream(r: @bufio.Reader): String | Nil:
    r.ReadLine()

res: String | Nil = readStream(Stdin)
`
	prog := parser.NewParser(lexer.NewLexer(testCode)).ParseProgram()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type error when using @bufio.Reader without import: %v", err)
	}

	testMismatchCode := `
def readStream(r: @bufio.Reader): String | Nil:
    r.ReadLine()

bad: String | Nil = readStream(Stdout)
`
	inf2 := NewInferencer()
	progMismatch := parser.NewParser(lexer.NewLexer(testMismatchCode)).ParseProgram()
	if err := inf2.InferProgram(progMismatch); err == nil {
		t.Fatalf("expected type error passing Stdout to @bufio.Reader parameter, got nil")
	}
}

func TestBytesTypeInference(t *testing.T) {
	inf := NewInferencer()
	testCode := `
b1: Bytes = "hello".Bytes()
b2: Bytes = Bytes("world")
b3: Bytes = Bytes()
b4: Bytes = Bytes(b1)
b5: Bytes = b1.Bytes()
b_cat: Bytes = b1 + b2
first_byte: Int = b1[0]
text: String = b1.String()
valid: Bool = b1.IsValidString()
hex_str: String = b1.ToString()
is_eq: Bool = b1 == b2
is_lt: Bool = b1 < b2
length: Int = len(b1)
`
	prog := parser.NewParser(lexer.NewLexer(testCode)).ParseProgram()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type error in TestBytesTypeInference: %v", err)
	}

	// Negative test: indexing Bytes must return Int, not String
	testMismatchCode := `
b: Bytes = "hello".Bytes()
wrong: String = b[0]
`
	inf2 := NewInferencer()
	progMismatch := parser.NewParser(lexer.NewLexer(testMismatchCode)).ParseProgram()
	if err := inf2.InferProgram(progMismatch); err == nil {
		t.Fatalf("expected type error assigning Bytes index (Int) to String, got nil")
	}

	// Negative test: Bytes constructor with non-string/non-bytes argument
	testInvalidCtorCode := `
b = Bytes(123)
`
	inf3 := NewInferencer()
	progInvalidCtor := parser.NewParser(lexer.NewLexer(testInvalidCtorCode)).ParseProgram()
	if err := inf3.InferProgram(progInvalidCtor); err == nil {
		t.Fatalf("expected type error passing Int to Bytes constructor, got nil")
	}

	// Negative test: Bytes + String must fail type inference
	testMixedAddCode1 := `
b: Bytes = "hello".Bytes()
res = b + "world"
`
	inf4 := NewInferencer()
	progMixedAdd1 := parser.NewParser(lexer.NewLexer(testMixedAddCode1)).ParseProgram()
	if err := inf4.InferProgram(progMixedAdd1); err == nil {
		t.Fatalf("expected type error adding Bytes + String, got nil")
	}

	// Negative test: String + Bytes must fail type inference
	testMixedAddCode2 := `
b: Bytes = "hello".Bytes()
res = "hello" + b
`
	inf5 := NewInferencer()
	progMixedAdd2 := parser.NewParser(lexer.NewLexer(testMixedAddCode2)).ParseProgram()
	if err := inf5.InferProgram(progMixedAdd2); err == nil {
		t.Fatalf("expected type error adding String + Bytes, got nil")
	}
}


