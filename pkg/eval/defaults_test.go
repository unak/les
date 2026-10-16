package eval

import (
	"fmt"
	"strings"
	"testing"

	"les/pkg/lexer"
	"les/pkg/parser"
)

func runAndGetGlobalEnv(code string) (*Environment, error) {
	l := lexer.NewLexer(code)
	p := parser.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		return nil, fmt.Errorf("%s", p.Errors()[0])
	}

	ev := NewEvaluator()
	var out strings.Builder
	ev.SetOutput(&out)
	_, err := ev.EvalProgram(prog)
	if err != nil {
		return nil, err
	}
	return ev.globalEnv, nil
}

func TestEvalTrailingDefaults(t *testing.T) {
	input := `
def add(a, b = 10, c = 20):
    a + b + c

x1 = add(1)
x2 = add(1, 2)
x3 = add(1, 2, 3)
`
	env, err := runAndGetGlobalEnv(input)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	testInt(t, env, "x1", 31)
	testInt(t, env, "x2", 23)
	testInt(t, env, "x3", 6)
}

func TestEvalDefaultsReferencePrecedingParams(t *testing.T) {
	input := `
def range_box(start, stop = start + 10, step = 1):
    stop - start

r1 = range_box(5)
r2 = range_box(5, 30)
`
	env, err := runAndGetGlobalEnv(input)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	testInt(t, env, "r1", 10)
	testInt(t, env, "r2", 25)
}

func TestEvalBlockParam(t *testing.T) {
	input := `
def apply_twice(x, mult = 2, &block = nil):
    val = x * mult
    if block != nil:
        return block(val)
    val

r_noblock = apply_twice(10)
r_custom_mult = apply_twice(10, 3)
r_block_default = apply_twice(10) do |v|:
    v + 5
r_block_custom = apply_twice(10, 3) do |v|:
    v + 5
`
	env, err := runAndGetGlobalEnv(input)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	testInt(t, env, "r_noblock", 20)
	testInt(t, env, "r_custom_mult", 30)
	testInt(t, env, "r_block_default", 25)
	testInt(t, env, "r_block_custom", 35)
}

func TestEvalStructConstructorDefaults(t *testing.T) {
	input := `
struct Config:
    host: String
    port: Int = 8080
    timeout: Int = 30

c1 = Config("localhost")
c2 = Config("remote", 9000)
`
	env, err := runAndGetGlobalEnv(input)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}

	c1Val, _ := env.Get("c1")
	c1Inst := Force(c1Val).(*StructInstance)
	testStringVal(t, Force(c1Inst.Fields["host"]), "localhost")
	testIntVal(t, Force(c1Inst.Fields["port"]), 8080)
	testIntVal(t, Force(c1Inst.Fields["timeout"]), 30)

	c2Val, _ := env.Get("c2")
	c2Inst := Force(c2Val).(*StructInstance)
	testStringVal(t, Force(c2Inst.Fields["host"]), "remote")
	testIntVal(t, Force(c2Inst.Fields["port"]), 9000)
	testIntVal(t, Force(c2Inst.Fields["timeout"]), 30)
}

func testInt(t *testing.T, env *Environment, name string, expected int64) {
	t.Helper()
	v, ok := env.Get(name)
	if !ok {
		t.Fatalf("variable %s not found", name)
	}
	testIntVal(t, Force(v), expected)
}

func testIntVal(t *testing.T, val Value, expected int64) {
	t.Helper()
	if iv, ok := val.(IntValue); ok {
		if int64(iv) != expected {
			t.Fatalf("expected %d, got %d", expected, int64(iv))
		}
	} else {
		t.Fatalf("expected IntValue, got %T (%+v)", val, val)
	}
}

func testStringVal(t *testing.T, val Value, expected string) {
	t.Helper()
	if sv, ok := val.(StringValue); ok {
		if string(sv) != expected {
			t.Fatalf("expected %q, got %q", expected, string(sv))
		}
	} else {
		t.Fatalf("expected StringValue, got %T (%+v)", val, val)
	}
}
