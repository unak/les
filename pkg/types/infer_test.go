package types

import (
	"os"
	"path/filepath"
	"strings"
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

func TestAssignableUnknownAndNil(t *testing.T) {
	// Unknown is internal inference placeholder: compatible during inference
	if !IsAssignableTo(TypeInt, TypeUnknown) {
		t.Errorf("expected Int to be assignable to Unknown")
	}
	if !IsAssignableTo(TypeUnknown, TypeInt) {
		t.Errorf("expected Unknown to be assignable to Int")
	}
	if !IsAssignableTo(TypeNil, TypeUnknown) {
		t.Errorf("expected Nil to be assignable to Unknown")
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
	// 1. Both expr! and force expr keyword preserve inner expression types
	code := `
a: Int = (10 + 20)!
b: Int = force 30
c: String = ("hello")!
d: String = force "world"
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type error in force expression inference: %v", err)
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
w1: Int = Stdout.Write("text".Bytes())
w2: Int = Stderr.Write("err".Bytes())
Stdout.Flush()
Stderr.Flush()

l: String | Nil = Stdin.ReadLine()
s: String | Nil = Stdin.ReadString(":")
p: Bytes | Nil = Stdin.Peek(5)

# Test @io module
rawW: Int = @io.Stdout.Write("raw".Bytes())
rawR: Bytes | Nil = @io.Stdin.Read(10)
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

def readAll(r: @io.Reader): Bytes | Nil:
    r.Read(100)

struct CustomReader:
    def Read(n: Int): Bytes | Nil:
        "hello".Bytes()

cr = CustomReader()
res: Bytes | Nil = readAll(cr)

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

func TestDictTypeAnnotation(t *testing.T) {
	// 1. Basic typed variable declaration
	code1 := `
d: {String: Int} = {"count": 42}
`
	prog1 := parser.NewParser(lexer.NewLexer(code1)).ParseProgram()
	inf1 := NewInferencer()
	if err := inf1.InferProgram(prog1); err != nil {
		t.Fatalf("unexpected type error: %v", err)
	}

	// 2. Empty dict assignment
	code2 := `
d: {String: Int} = {}
`
	prog2 := parser.NewParser(lexer.NewLexer(code2)).ParseProgram()
	inf2 := NewInferencer()
	if err := inf2.InferProgram(prog2); err != nil {
		t.Fatalf("unexpected type error assigning empty dict: %v", err)
	}

	// 3. Function parameter and return type
	code3 := `
def lookup(table: {String: Int}, key: String): Int:
    table[key]

val = lookup({"a": 1}, "a")
`
	prog3 := parser.NewParser(lexer.NewLexer(code3)).ParseProgram()
	inf3 := NewInferencer()
	if err := inf3.InferProgram(prog3); err != nil {
		t.Fatalf("unexpected type error in function with dict: %v", err)
	}

	// 4. Nested dict type
	code4 := `
d: {String: {Int: Bool}} = {"nested": {1: true}}
`
	prog4 := parser.NewParser(lexer.NewLexer(code4)).ParseProgram()
	inf4 := NewInferencer()
	if err := inf4.InferProgram(prog4); err != nil {
		t.Fatalf("unexpected type error in nested dict: %v", err)
	}

	// 5. Dict in union
	code5 := `
d: {String: Int} | Nil = nil
`
	prog5 := parser.NewParser(lexer.NewLexer(code5)).ParseProgram()
	inf5 := NewInferencer()
	if err := inf5.InferProgram(prog5); err != nil {
		t.Fatalf("unexpected type error in union with dict: %v", err)
	}

	// 6. Dict with func value
	code6 := `
d: {String: () -> Int} = {"get": ||: 42}
`
	prog6 := parser.NewParser(lexer.NewLexer(code6)).ParseProgram()
	inf6 := NewInferencer()
	if err := inf6.InferProgram(prog6); err != nil {
		t.Fatalf("unexpected type error in dict with func value: %v", err)
	}

	// 7. Tuple containing dict
	code7 := `
t: ({String: Int}, Bool) = ({"a": 1}, true)
`
	prog7 := parser.NewParser(lexer.NewLexer(code7)).ParseProgram()
	inf7 := NewInferencer()
	if err := inf7.InferProgram(prog7); err != nil {
		t.Fatalf("unexpected type error in tuple containing dict: %v", err)
	}

	// 8. Negative: value type mismatch
	codeErr1 := `
d: {String: Int} = {"count": "forty-two"}
`
	progErr1 := parser.NewParser(lexer.NewLexer(codeErr1)).ParseProgram()
	infErr1 := NewInferencer()
	if err := infErr1.InferProgram(progErr1); err == nil {
		t.Fatalf("expected type error for dict value mismatch, got nil")
	}

	// 9. Negative: key type mismatch
	codeErr2 := `
d: {String: Int} = {123: 42}
`
	progErr2 := parser.NewParser(lexer.NewLexer(codeErr2)).ParseProgram()
	infErr2 := NewInferencer()
	if err := infErr2.InferProgram(progErr2); err == nil {
		t.Fatalf("expected type error for dict key mismatch, got nil")
	}

	// 10. Type string formatting
	dt := &DictType{Key: TypeString, Value: TypeInt}
	if dt.String() != "{String: Int}" {
		t.Fatalf("expected {String: Int}, got %s", dt.String())
	}

	// 11. Negative: mixed entry value types fail assignment to {String: Int}
	codeErr3 := `
z: {String: Int} = {"a": 1, "b": "x"}
`
	progErr3 := parser.NewParser(lexer.NewLexer(codeErr3)).ParseProgram()
	infErr3 := NewInferencer()
	if err := infErr3.InferProgram(progErr3); err == nil {
		t.Fatalf("expected type error for mixed entry value types, got nil")
	}

	// 12. Mixed entry succeeds when annotated with union
	code12 := `
z: {String: Int | String} = {"a": 1, "b": "x"}
`
	prog12 := parser.NewParser(lexer.NewLexer(code12)).ParseProgram()
	inf12 := NewInferencer()
	if err := inf12.InferProgram(prog12); err != nil {
		t.Fatalf("unexpected type error with union dict: %v", err)
	}

	// 13. Assigning dict with Unknown value (from unannotated function) is allowed
	code13 := `
def get_val(x):
    x

d: {String: Int} = {"a": get_val(42)}
`
	prog13 := parser.NewParser(lexer.NewLexer(code13)).ParseProgram()
	inf13 := NewInferencer()
	if err := inf13.InferProgram(prog13); err != nil {
		t.Fatalf("unexpected type error assigning Unknown-valued dict to typed dict: %v", err)
	}


	// 14. IsAssignableTo with TypeUnknown:
	emptyDict := &DictType{Key: TypeUnknown, Value: TypeUnknown}
	intDict := &DictType{Key: TypeString, Value: TypeInt}
	unknownValDict := &DictType{Key: TypeString, Value: TypeUnknown}

	if !IsAssignableTo(emptyDict, intDict) {
		t.Fatalf("expected empty dict to be assignable to {String: Int}")
	}
	if !IsAssignableTo(unknownValDict, intDict) {
		t.Fatalf("expected {String: Unknown} to be assignable to {String: Int}")
	}
	if !IsAssignableTo(intDict, unknownValDict) {
		t.Fatalf("expected {String: Int} to be assignable to {String: Unknown}")
	}
}

func TestUnknownTypeAnnotationError(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		expectedErr string
	}{
		{
			name:        "variable declaration unknown primitive typo",
			code:        `x: Strng = 1`,
			expectedErr: "unknown type 'Strng'",
		},
		{
			name:        "dict value type typo",
			code:        `d: {String: Intt} = {}`,
			expectedErr: "unknown type 'Intt'",
		},
		{
			name:        "dict key type typo",
			code:        `d: {Strng: Int} = {}`,
			expectedErr: "unknown type 'Strng'",
		},
		{
			name:        "function param type typo",
			code:        `def add(a: Integer, b: Int): Int: a + b`,
			expectedErr: "unknown type 'Integer'",
		},
		{
			name:        "function return type typo",
			code:        `def add(a: Int, b: Int): Integ: a + b`,
			expectedErr: "unknown type 'Integ'",
		},
		{
			name: "struct field type typo",
			code: `struct Point:
    x: Int
    y: Floatt
`,
			expectedErr: "unknown type 'Floatt'",
		},
		{
			name: "struct method param type typo",
			code: `struct Foo:
    def bar(self, a: BadType): Int: 1
`,
			expectedErr: "unknown type 'BadType'",
		},
		{
			name: "struct method return type typo",
			code: `struct Foo:
    def bar(self, a: Int): BadRet: 1
`,
			expectedErr: "unknown type 'BadRet'",
		},
		{
			name: "interface method param type typo",
			code: `interface Greeter:
    Greet(name: Stringg): String
`,
			expectedErr: "unknown type 'Stringg'",
		},
		{
			name: "interface method return type typo",
			code: `interface Greeter:
    Greet(name: String): Strng
`,
			expectedErr: "unknown type 'Strng'",
		},
		{
			name:        "is expression unknown type",
			code:        `b = 1 is Unk`,
			expectedErr: "unknown type 'Unk'",
		},
		{
			name:        "lambda parameter type typo",
			code:        `f = |a: BadType|: a`,
			expectedErr: "unknown type 'BadType'",
		},
		{
			name:        "tuple element type typo",
			code:        `t: (Int, BadType) = (1, 2)`,
			expectedErr: "unknown type 'BadType'",
		},
		{
			name:        "union member type typo",
			code:        `u: Int | BadType = 1`,
			expectedErr: "unknown type 'BadType'",
		},
		{
			name:        "function type in annotation typo",
			code:        `f: (BadType) -> Int = |x|: 1`,
			expectedErr: "unknown type 'BadType'",
		},
		{
			name:        "module type unknown",
			code:        `x: @unknownmod.Type = 1`,
			expectedErr: "unknown type '@unknownmod.Type'",
		},
		{
			name:        "prohibit Any type annotation",
			code:        `x: Any = 1`,
			expectedErr: "unknown type 'Any'",
		},
		{
			name:        "prohibit Unknown type annotation",
			code:        `x: Unknown = 1`,
			expectedErr: "unknown type 'Unknown'",
		},
		{
			name: "prohibit Any in match case",
			code: `
match 1:
    case Any:
        1
`,
			expectedErr: "unknown type 'Any'",
		},
		{
			name: "prohibit Unknown in match case",
			code: `
match 1:
    case Unknown:
        1
`,
			expectedErr: "unknown type 'Unknown'",
		},
	}


	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog := parser.NewParser(lexer.NewLexer(tt.code)).ParseProgram()
			inf := NewInferencer()
			err := inf.InferProgram(prog)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.expectedErr)
			}
			if !strings.Contains(err.Error(), tt.expectedErr) {
				t.Fatalf("expected error containing %q, got %q", tt.expectedErr, err.Error())
			}
		})
	}
}

func TestValidTypeAnnotations(t *testing.T) {
	validCodes := []string{
		`x: Int = 1`,
		`x: Float = 1.0`,
		`x: String = "abc"`,
		`x: Bytes = Bytes("abc")`,
		`x: Bool = true`,
		`x: Nil = nil`,
		`x: Tuple = (1, 2, 3)`,
		`x: Dict = {"a": 1}`,
		`x: {String: Int} = {"a": 1}`,
		`x: (Int, String) = (1, "a")`,
		`x: (Int, ...) = (1, 2, 3)`,
		`x: Int | String = 1`,
		`x: (Int) -> String = |a: Int|: "ok"`,
		`
struct Node:
    val: Int
    next: Node | Nil

def make_node(): Node:
    Node(val: 1, next: nil)
`,
		`
interface Cloneable:
    Clone(): Cloneable
`,
	}

	for i, code := range validCodes {
		prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
		inf := NewInferencer()
		if err := inf.InferProgram(prog); err != nil {
			t.Fatalf("case %d: unexpected error for valid code: %v\ncode:\n%s", i, err, code)
		}
	}
}

func TestStructInitTypeInference(t *testing.T) {
	validCode := `
struct Point:
    x: Int
    y: Int

    def init(r: Int, theta: Int):
        self.x = r
        self.y = theta

p: Point = Point(10, 20)
`
	prog := parser.NewParser(lexer.NewLexer(validCode)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type error: %v", err)
	}

	invalidCode := `
struct Point:
    x: Int
    y: Int

    def init(r: Int, theta: Int):
        self.x = r
        self.y = theta

p = Point("invalid", 20)
`
	progBad := parser.NewParser(lexer.NewLexer(invalidCode)).ParseProgram()
	infBad := NewInferencer()
	if err := infBad.InferProgram(progBad); err == nil {
		t.Fatalf("expected type error for string arg to int constructor, got nil")
	}
}

func TestStructInitVariadic(t *testing.T) {
	code := `
struct StringList:
    t: (String, ...)

    def init(*t: String):
        ;

sl0 = StringList()
sl1 = StringList("a")
sl2 = StringList("a", "b")
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type error: %v", err)
	}

	badCode := `
struct StringList:
    t: (String, ...)

    def init(*t: String):
        ;

sl = StringList(123)
`
	progBad := parser.NewParser(lexer.NewLexer(badCode)).ParseProgram()
	infBad := NewInferencer()
	if err := infBad.InferProgram(progBad); err == nil {
		t.Fatalf("expected type error for int arg to variadic string constructor, got nil")
	}
}

func TestStringIntAndIntStringTypes(t *testing.T) {
	code := `
a = "123".Int()
b = "123".Int(16)
c = 123.String()
d = 123.String(16)
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type error: %v", err)
	}
	if tA, ok := inf.scope.Lookup("a"); !ok || tA != TypeInt {
		t.Fatalf("expected a to be Int, got %v", tA)
	}
	if tB, ok := inf.scope.Lookup("b"); !ok || tB != TypeInt {
		t.Fatalf("expected b to be Int, got %v", tB)
	}
	if tC, ok := inf.scope.Lookup("c"); !ok || tC != TypeString {
		t.Fatalf("expected c to be String, got %v", tC)
	}
	if tD, ok := inf.scope.Lookup("d"); !ok || tD != TypeString {
		t.Fatalf("expected d to be String, got %v", tD)
	}

	badCode1 := `"123".Int("invalid")`
	progBad1 := parser.NewParser(lexer.NewLexer(badCode1)).ParseProgram()
	infBad1 := NewInferencer()
	if err := infBad1.InferProgram(progBad1); err == nil {
		t.Fatalf("expected type error for non-int radix, got nil")
	}

	badCode2 := `"123".Int(10, 20)`
	progBad2 := parser.NewParser(lexer.NewLexer(badCode2)).ParseProgram()
	infBad2 := NewInferencer()
	if err := infBad2.InferProgram(progBad2); err == nil {
		t.Fatalf("expected type error for 2 arguments to Int(), got nil")
	}
}

func TestFuncTypeBlockParamString(t *testing.T) {
	ft1 := &FuncType{
		Arg: &TupleType{Elems: []Type{}},
		BlockParam: &FuncType{
			Arg: &TupleType{Elems: []Type{TypeString}},
			Ret: &TupleType{Elems: []Type{}},
		},
		Ret: &TupleType{Elems: []Type{}},
	}
	expected1 := "((String) -> ()) -> ()"
	if ft1.String() != expected1 {
		t.Fatalf("expected %q, got %q", expected1, ft1.String())
	}

	ft2 := &FuncType{
		Arg: &TupleType{Elems: []Type{TypeInt}},
		BlockParam: &FuncType{
			Arg: &TupleType{Elems: []Type{TypeInt}},
			Ret: TypeString,
		},
		Ret: &TupleType{Elems: []Type{TypeString}, IsVariadic: true},
	}
	expected2 := "(Int, (Int) -> String) -> (String, ...)"
	if ft2.String() != expected2 {
		t.Fatalf("expected %q, got %q", expected2, ft2.String())
	}
}

func TestCustomInitReturnType(t *testing.T) {
	code := `
struct StringList:
    t: (String, ...)

    def init(*t: String):
        ()
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type error: %v", err)
	}

	stVal, ok := inf.scope.Lookup("StringList")
	if !ok {
		t.Fatalf("expected StringList in scope")
	}
	st, ok := stVal.(*StructType)
	if !ok {
		t.Fatalf("expected StructType, got %T", stVal)
	}
	initFn, ok := st.Methods["init"]
	if !ok {
		t.Fatalf("expected init method on StringList")
	}
	if !initFn.Ret.Equals(st) {
		t.Fatalf("expected init return type to be %s, got %s", st.Name, initFn.Ret)
	}

	// Verify BlockParam function type assignability and interface implementation
	ifaceCode := `
interface List:
    Each(block)

struct StringList:
    t: (String, ...)
    def init(*t: String):
        ()
    def Each(&block: (String) -> ()) -> ():
        ()

sl = StringList("a", "b")
l: List = sl
eachFn: ((String) -> ()) -> () = sl.Each
lType: String = l.Type()
`
	progIface := parser.NewParser(lexer.NewLexer(ifaceCode)).ParseProgram()
	infIface := NewInferencer()
	if err := infIface.InferProgram(progIface); err != nil {
		t.Fatalf("unexpected type error in block param interface/assignability test: %v", err)
	}
}

func TestOperatorMethodInference(t *testing.T) {
	code := `
struct Vec2:
    x: Int
    y: Int

    def +(other: Vec2):
        Vec2(x: x + other.x, y: y + other.y)

    def ==(other: Vec2):
        x == other.x && y == other.y

    def -():
        Vec2(x: -x, y: -y)

    def [](idx: Int):
        idx == 0 ? x : y

v1 = Vec2(x: 1, y: 2)
v2 = Vec2(x: 3, y: 4)

v3: Vec2 = v1 + v2
eq: Bool = v1 == v2
neq: Bool = v1 != v2
neg: Vec2 = -v1
elem: Int = v1[0]

eqObj: Equal = v1
cmpInt: Comparable = 42
cmpStr: Comparable = "abc"
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type inference error for operator methods: %v", err)
	}

	v3T, ok := inf.scope.Lookup("v3")
	if !ok || v3T.String() != "Vec2" {
		t.Fatalf("expected v3 to be Vec2, got %v", v3T)
	}
	eqT, ok := inf.scope.Lookup("eq")
	if !ok || eqT != TypeBool {
		t.Fatalf("expected eq to be Bool, got %v", eqT)
	}
	neqT, ok := inf.scope.Lookup("neq")
	if !ok || neqT != TypeBool {
		t.Fatalf("expected neq to be Bool, got %v", neqT)
	}
	negT, ok := inf.scope.Lookup("neg")
	if !ok || negT.String() != "Vec2" {
		t.Fatalf("expected neg to be Vec2, got %v", negT)
	}
	elemT, ok := inf.scope.Lookup("elem")
	if !ok || elemT != TypeInt {
		t.Fatalf("expected elem to be Int, got %v", elemT)
	}
}

func TestBitwiseTypeInference(t *testing.T) {
	code := `
a = 10
b = 2
c = ~a
d = a & b
e = a ^ b
f = a | b
g = a << b
h = a >> b

struct Bits:
    val: Int
    def ~(self) -> Bits:
        Bits(val: ~self.val)
    def &(self, other: Bits) -> Bits:
        Bits(val: self.val & other.val)

b1 = Bits(val: 1)
b2 = Bits(val: 2)
b3 = ~b1
b4 = b1 & b2
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type error: %v", err)
	}

	for _, v := range []string{"c", "d", "e", "f", "g", "h"} {
		ty, ok := inf.scope.Lookup(v)
		if !ok || ty != TypeInt {
			t.Fatalf("expected %s to be Int, got %v", v, ty)
		}
	}

	b3T, ok := inf.scope.Lookup("b3")
	if !ok || b3T.String() != "Bits" {
		t.Fatalf("expected b3 to be Bits, got %v", b3T)
	}
	b4T, ok := inf.scope.Lookup("b4")
	if !ok || b4T.String() != "Bits" {
		t.Fatalf("expected b4 to be Bits, got %v", b4T)
	}

	// Type errors on non-Int types
	badCodes := []string{
		`~"hello"`,
		`~1.5`,
		`1.5 & 2`,
		`"a" | "b"`,
		`true ^ false`,
		`1 << 1.5`,
		"struct S:\n    val: Int\n    def &(self, other: S) -> S: S(val: self.val & other.val)\ns = S(val: 1)\nres = s & \"bad\"",
		"struct S:\n    val: Int\n    def ~(self, extra: Int) -> S: S(val: ~self.val)\ns = S(val: 1)\nres = ~s",
	}
	for _, bad := range badCodes {
		p := parser.NewParser(lexer.NewLexer(bad)).ParseProgram()
		i := NewInferencer()
		if err := i.InferProgram(p); err == nil {
			t.Fatalf("expected type error for %q, got nil", bad)
		}
	}
	// Interface with explicit self and builtin operator method interfaces
	testIfaceCode := `
interface Negatable:
    -(): Int

struct S:
    val: Int
    def ~(self):
        S(val: ~self.val)
    def &(self, other: S):
        S(val: self.val & other.val)

interface ExplicitSelf:
    ~(self): S
    &(self, other: S): S

interface ExplicitSelfInt:
    ~(self): Int
    &(self, other: Int): Int

interface Adder:
    +(other: Int): Int

var neg: Negatable = 42
var add: Adder = 42
var es: ExplicitSelf = S(val: 10)
var esi: ExplicitSelfInt = 42
`
	progIface := parser.NewParser(lexer.NewLexer(testIfaceCode)).ParseProgram()
	infIface := NewInferencer()
	if err := infIface.InferProgram(progIface); err != nil {
		t.Fatalf("unexpected type error: %v", err)
	}

	badIfaceCodes := []string{
		"interface BadStringer:\n    String(x: String): String\nvar bs: BadStringer = 42",
		"interface StrBitAnd:\n    &(other: String): Int\nvar sba: StrBitAnd = 42",
	}
	for _, bad := range badIfaceCodes {
		p := parser.NewParser(lexer.NewLexer(bad)).ParseProgram()
		i := NewInferencer()
		if err := i.InferProgram(p); err == nil {
			t.Fatalf("expected type error for %q, got nil", bad)
		}
	}

	variadicOpCode := `
struct VarBit:
    v: Int
    def &(self, *others: VarBit):
        VarBit(v: self.v)

vb1 = VarBit(v: 1)
vb2 = VarBit(v: 2)
vb3 = vb1 & vb2
`
	progVar := parser.NewParser(lexer.NewLexer(variadicOpCode)).ParseProgram()
	infVar := NewInferencer()
	if err := infVar.InferProgram(progVar); err != nil {
		t.Fatalf("unexpected type error for variadic operator method: %v", err)
	}
}

func TestGenericTypeInference(t *testing.T) {
	code := `
def id[T](x: T) -> T:
    x

a = id(42)
b = id("hello")
c = id[Int](100)

struct Box[T]:
    val: T
    def get(self) -> T:
        self.val

box1 = Box(10)
box2 = Box[String]("world")
val1 = box1.get()
val2 = box2.get()

def add_numbers[T: Int | Float](x: T, y: T) -> T:
    x + y

sum1 = add_numbers(1, 2)
sum2 = add_numbers(1.5, 2.5)

interface Addable[T]:
    +(other: T): T

def add_gen[T: Addable[T]](x: T, y: T) -> T:
    x + y

sum3 = add_gen(10, 20)

struct Node[T]:
    val: T
    next: Node[T] | Nil

n1 = Node(val: 42, next: nil)

def my_map[T, U](items: (T, ...), fn: (T) -> U) -> (U, ...):
    (fn(items[0]), ...)

res = my_map((1, 2, 3), |x|: x * 2)
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type error in generic inference: %v", err)
	}

	scope := inf.Scope()
	// Check inferred types
	checkType := func(name string, expected string) {
		t.Helper()
		typ, ok := scope.Lookup(name)
		if !ok {
			t.Fatalf("variable %s not found in scope", name)
		}
		if typ.String() != expected {
			t.Errorf("expected %s to have type %s, got %s", name, expected, typ.String())
		}
	}

	checkType("a", "Int")
	checkType("b", "String")
	checkType("c", "Int")
	checkType("box1", "Box[Int]")
	checkType("box2", "Box[String]")
	checkType("val1", "Int")
	checkType("val2", "String")
	checkType("sum1", "Int")
	checkType("sum2", "Float")
	checkType("sum3", "Int")
	checkType("n1", "Node[Int]")
	checkType("res", "(Int, ...)")
}

func TestGenericConstraintErrors(t *testing.T) {
	badCases := []struct {
		code        string
		expectedErr string
	}{
		{
			code: `
def add_num[T: Int | Float](x: T, y: T) -> T:
    x + y

a = add_num("hello", "world")
`,
			expectedErr: "does not satisfy constraint",
		},
		{
			code: `
struct Box[T: Int]:
    val: T

b = Box[String]("hi")
`,
			expectedErr: "does not satisfy constraint",
		},
	}

	for _, tt := range badCases {
		prog := parser.NewParser(lexer.NewLexer(tt.code)).ParseProgram()
		inf := NewInferencer()
		err := inf.InferProgram(prog)
		if err == nil {
			t.Errorf("expected error for %s, got nil", tt.code)
			continue
		}
		if !strings.Contains(err.Error(), tt.expectedErr) {
			t.Errorf("expected error containing %q, got %v", tt.expectedErr, err)
		}
	}
}

func TestGenericMethodAndBlockInference(t *testing.T) {
	code := `
struct Box[T]:
    val: T
    def Map[U](fn: (T) -> U): Box[U]:
        Box(fn(self.val))

b = Box(10)
b2 = b.Map(|x|: x.ToString())

def mapBlock[T, U](item: T, &block: (T) -> U) -> U:
    block(item)

res = mapBlock(42) do |n|:
    n.ToString()
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type inference error: %v", err)
	}
}

func TestSelfAndGenericIndexInference(t *testing.T) {
	code := `
struct Box[T]:
    val: T
    def Clone(): Box[T]:
        self

struct List[T]:
    items: (T, ...)
    def [](i: Int) -> T:
        self.items[i]

l = List((10, 20, 30))
idx = 1
val = l[idx]

def getVal[T: Box[Int]](b: T) -> Int:
    b.val
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type inference error: %v", err)
	}

	// Verify Box[Int].Type() reflection
	bType, ok := inf.Scope().Lookup("Box")
	if !ok {
		t.Fatalf("Box not found in scope")
	}
	st := bType.(*StructType)
	instSt := InstantiateStruct(st, []Type{TypeInt})
	if initMethod, ok := instSt.Methods["init"]; ok {
		if !strings.Contains(initMethod.String(), "Box[Int]") {
			t.Fatalf("expected init return type to be Box[Int], got %s", initMethod.String())
		}
	}
}

func TestTupleOperationsInference(t *testing.T) {
	code := `
t = (1, "a")
t_plus = t + (true, 2.5)
t_shift = t << 99
t_app = t.Append(false)
t_prep = t.Prepend(nil)
t_concat = t.Concat((1,), ("b",))
t_slice1 = (1, 2, 3, 4).Slice(1, 3)

def accept_any_tuple(x: (...)) -> (...):
    x

def accept_any_dict(d: {...}) -> {...}:
    d
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type inference error: %v", err)
	}

	assertType := func(name string, expected string) {
		typ, ok := inf.Scope().Lookup(name)
		if !ok {
			t.Fatalf("%s not found in scope", name)
		}
		if typ.String() != expected {
			t.Errorf("expected %s to have type %s, got %s", name, expected, typ.String())
		}
	}

	assertType("t_plus", "(Int, String, Bool, Float)")
	assertType("t_shift", "(Int, String, Int)")
	assertType("t_app", "(Int, String, Bool)")
	assertType("t_prep", "(Nil, Int, String)")
	assertType("t_concat", "(Int, String, Int, String)")
	assertType("t_slice1", "(Int, Int)")

	// Generic constraint with Tuple
	genCode := `
def addElem[T: (...)](t: T) -> (Int, ...):
    t << 10

def joinTup[T: (...)](t1: T, t2: T) -> (...):
    t1 + t2

def appTup[T: (...)](t: T):
    t.Append("hello")
`
	progGen := parser.NewParser(lexer.NewLexer(genCode)).ParseProgram()
	infGen := NewInferencer()
	if err := infGen.InferProgram(progGen); err != nil {
		t.Fatalf("unexpected type inference error on generic tuple constraints: %v", err)
	}
}

func TestReviewFixes(t *testing.T) {
	code := `
# 1. Spread with variadic or generic tuple
def spread_mid[T: (...)](t: T):
    (0, ...t, 99)

def spread_untyped(t):
    (...t, 1)

# 2. Variadic tuple concatenation and append union
vt: (Int, ...) = (1, 2)
vt_app = vt << "str"
vt_plus = vt + ("str",)
vt_concat = vt.Concat(("str",))

# 3. Generic type instantiation in expr
def id[T](x: T) -> T:
    x

val1 = id[(...)]( (1, 2) )
val2 = id[{...}]( {"a": 1} )
val3 = id[(Int, ...)]( (1, 2, 3) )

# 4. Match case (...)
def match_tup(x: Int | (...)) -> String:
    match x:
        case (...):
            "is_tuple"
        case Int:
            "is_int"
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type inference error in TestReviewFixes: %v", err)
	}

	assertType := func(name string, expected string) {
		typ, ok := inf.Scope().Lookup(name)
		if !ok {
			t.Fatalf("%s not found in scope", name)
		}
		if typ.String() != expected {
			t.Errorf("expected %s to have type %s, got %s", name, expected, typ.String())
		}
	}

	assertType("vt_app", "(Int | String, ...)")
	assertType("vt_plus", "(Int | String, ...)")
	assertType("vt_concat", "(Int | String, ...)")

	// Negative test for invalid dot count
	invalidCode := `x = (.t, 1)`
	pInvalid := parser.NewParser(lexer.NewLexer(invalidCode))
	pInvalid.ParseProgram()
	if len(pInvalid.Errors()) == 0 {
		t.Errorf("expected error for '.t', got none")
	}
}
