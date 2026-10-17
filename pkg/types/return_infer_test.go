package types

import (
	"testing"

	"les/pkg/lexer"
	"les/pkg/parser"
)

func TestReturnTypeInference(t *testing.T) {
	tests := []struct {
		name         string
		code         string
		funcName     string
		wantRetType  string
		wantWarnings []string
		wantErr      string
	}{
		{
			name: "empty function body returns unit with no warning",
			code: `def foo(*arg: Int):
`,
			funcName:     "foo",
			wantRetType:  "(Int, ...) -> ()",
			wantWarnings: nil,
		},
		{
			name: "assignment at end of function returns unit with no warning",
			code: `def f():
    x = 10
`,
			funcName:     "f",
			wantRetType:  "() -> ()",
			wantWarnings: nil,
		},
		{
			name: "loop at end of function returns unit with no warning",
			code: `def f(nums: (Int, ...)):
    i = 0
    for i < nums.Len():
        i = i + 1
`,
			funcName:     "f",
			wantRetType:  "((Int, ...)) -> ()",
			wantWarnings: nil,
		},
		{
			name: "loop with break value returns value union unit",
			code: `def find(nums: (Int, ...)):
    i = 0
    for i < nums.Len():
        if nums[i] > 10:
            break nums[i]
        i = i + 1
`,
			funcName:     "find",
			wantRetType:  "((Int, ...)) -> () | Int",
			wantWarnings: nil,
		},
		{
			name: "infinite loop with break value returns break type",
			code: `def nextVal():
    for:
        break 42
`,
			funcName:     "nextVal",
			wantRetType:  "() -> Int",
			wantWarnings: nil,
		},
		{
			name: "if else both returning same type",
			code: `def sign(x: Int):
    if x > 0:
        return 1
    else:
        return -1
`,
			funcName:     "sign",
			wantRetType:  "(Int) -> Int",
			wantWarnings: nil,
		},
		{
			name: "if else both returning different types produces union",
			code: `def classify(x: Int):
    if x > 0:
        return "positive"
    else:
        return 0
`,
			funcName:     "classify",
			wantRetType:  "(Int) -> String | Int",
			wantWarnings: nil,
		},
		{
			name: "if without else early return produces union with unit",
			code: `def maybe(x: Int):
    if x > 0:
        return x
`,
			funcName:     "maybe",
			wantRetType:  "(Int) -> Int | ()",
			wantWarnings: nil,
		},
		{
			name: "expression style if else at end of function",
			code: `def abs(x: Int):
    if x >= 0:
        x
    else:
        -x
`,
			funcName:     "abs",
			wantRetType:  "(Int) -> Int",
			wantWarnings: nil,
		},
		{
			name: "empty return statement produces unit",
			code: `def f():
    return
`,
			funcName:     "f",
			wantRetType:  "() -> ()",
			wantWarnings: nil,
		},
		{
			name: "tail-recursive function infers return type cleanly",
			code: `def count(n: Int):
    if n <= 0:
        return 0
    return count(n - 1)
`,
			funcName:     "count",
			wantRetType:  "(Int) -> Int",
			wantWarnings: nil,
		},
		{
			name: "expression-recursive function fib infers return type cleanly",
			code: `def fib(n: Int):
    if n <= 1:
        return n
    return fib(n - 1) + fib(n - 2)
`,
			funcName:     "fib",
			wantRetType:  "(Int) -> Int",
			wantWarnings: nil,
		},
		{
			name: "recursive function storing result in local variable infers cleanly",
			code: `def rep(n: Int):
    if n <= 0:
        return "a"
    s = rep(n - 1)
    return s + "b"
`,
			funcName:     "rep",
			wantRetType:  "(Int) -> String",
			wantWarnings: nil,
		},
		{
			name: "bare callable print auto-invocation returns unit",
			code: `def f():
    print
`,
			funcName:     "f",
			wantRetType:  "() -> ()",
			wantWarnings: nil,
		},
		{
			name: "call to exit does not fall through",
			code: `def mustPositive(x: Int):
    if x > 0:
        return x
    exit(1)
`,
			funcName:     "mustPositive",
			wantRetType:  "(Int) -> Int",
			wantWarnings: nil,
		},
		{
			name: "annotated return type mismatch caught on early return",
			code: `def f(x: Int) -> Int:
    if x > 0:
        return "bad"
    return 1
`,
			funcName: "f",
			wantErr:  "return type mismatch in function 'f': expected Int, got String | Int",
		},
		{
			name: "annotated return type mismatch caught on fall-through",
			code: `def f(x: Int) -> Int:
    if x > 0:
        return 1
`,
			funcName: "f",
			wantErr:  "return type mismatch in function 'f': expected Int, got Int | ()",
		},
		{
			name: "match expr with returns in cases",
			code: `def matchType(v: Int | String):
    match v:
        case Int:
            return 1
        case String:
            return "ok"
`,
			funcName:     "matchType",
			wantRetType:  "(Int | String) -> Int | String",
			wantWarnings: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog := parser.NewParser(lexer.NewLexer(tt.code)).ParseProgram()
			inf := NewInferencer()
			err := inf.InferProgram(prog)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if err.Error() != tt.wantErr && !containsSubstring(err.Error(), tt.wantErr) {
					t.Fatalf("expected error %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected type inference error: %v", err)
			}

			if tt.wantWarnings != nil {
				warnings := inf.Warnings()
				if len(warnings) != len(tt.wantWarnings) {
					t.Fatalf("expected %d warnings, got %d: %v", len(tt.wantWarnings), len(warnings), warnings)
				}
			} else {
				if len(inf.Warnings()) != 0 {
					t.Fatalf("expected 0 warnings, got %d: %v", len(inf.Warnings()), inf.Warnings())
				}
			}

			if tt.funcName != "" {
				fnVal, ok := inf.Scope().Lookup(tt.funcName)
				if !ok {
					t.Fatalf("function %s not found in scope", tt.funcName)
				}
				fnT, ok := fnVal.(*FuncType)
				if !ok {
					t.Fatalf("expected FuncType, got %T", fnVal)
				}
				if fnT.String() != tt.wantRetType {
					t.Fatalf("expected function type %q, got %q", tt.wantRetType, fnT.String())
				}
			}
		})
	}
}

func TestStructMethodReturnTypeInference(t *testing.T) {
	code := `
struct Service:
    def noop():
        ;

    def forLoop(items: (String, ...)):
        i = 0
        for i < items.Len():
            i = i + 1

    def compute(x: Int):
        if x > 0:
            return x
        return -x

    def helper(n: Int):
        if n <= 0:
            return 100
        return self.helper(n - 1)
`
	prog := parser.NewParser(lexer.NewLexer(code)).ParseProgram()
	inf := NewInferencer()
	if err := inf.InferProgram(prog); err != nil {
		t.Fatalf("unexpected type inference error: %v", err)
	}
	if len(inf.Warnings()) != 0 {
		t.Fatalf("expected 0 warnings, got %d: %v", len(inf.Warnings()), inf.Warnings())
	}

	stVal, ok := inf.Scope().Lookup("Service")
	if !ok {
		t.Fatalf("Service not found in scope")
	}
	st, ok := stVal.(*StructType)
	if !ok {
		t.Fatalf("expected StructType, got %T", stVal)
	}

	if noopM, ok := st.Methods["noop"]; !ok || noopM.String() != "() -> ()" {
		t.Fatalf("expected noop() -> (), got %v", noopM)
	}
	if loopM, ok := st.Methods["forLoop"]; !ok || loopM.String() != "((String, ...)) -> ()" {
		t.Fatalf("expected forLoop((String, ...)) -> (), got %v", loopM)
	}
	if compM, ok := st.Methods["compute"]; !ok || compM.String() != "(Int) -> Int" {
		t.Fatalf("expected compute(Int) -> Int, got %v", compM)
	}
	if helpM, ok := st.Methods["helper"]; !ok || helpM.String() != "(Int) -> Int" {
		t.Fatalf("expected helper(Int) -> Int, got %v", helpM)
	}
}

func containsSubstring(s, substr string) bool {
	return len(substr) <= len(s) && (s == substr || len(s) >= len(substr) && len(substr) > 0 && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
