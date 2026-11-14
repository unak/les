package types

import (
	"strings"
	"testing"

	"les/pkg/lexer"
	"les/pkg/parser"
)

func inferSource(t *testing.T, input string) error {
	t.Helper()
	p := parser.NewParser(lexer.NewLexer(input))
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("unexpected parse errors: %v", p.Errors())
	}
	return NewInferencer().InferProgram(prog)
}

func TestFieldAndMethodSameNameError(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "method after field",
			input: `
struct P:
    size: Int
    def size() -> Int:
        @size + 100
p = P(size: 1)
`,
			want: "4: field and method with the same name 'size' in struct 'P'",
		},
		{
			name: "function typed field",
			input: `
struct Shape:
    area: () -> Int
    def area() -> Int:
        1
`,
			want: "4: field and method with the same name 'area' in struct 'Shape'",
		},
		{
			name: "field after method",
			input: `
struct P:
    def size() -> Int:
        1
    size: Int
`,
			want: "5: field and method with the same name 'size' in struct 'P'",
		},
		{
			name: "generic struct",
			input: `
struct Box[T]:
    size: T
    def size() -> Int:
        1
`,
			want: "4: field and method with the same name 'size' in struct 'Box'",
		},
		{
			name: "init",
			input: `
struct P:
    init: Int
    def init(init: Int):
        @init = init
`,
			want: "4: field and method with the same name 'init' in struct 'P'",
		},
		{
			name: "method named like an embedded struct",
			input: `
struct Inner:
    k: Int
struct Outer:
    Inner
    def Inner() -> Int:
        1
`,
			want: "6: field and method with the same name 'Inner' in struct 'Outer'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := inferSource(t, tt.input)
			if err == nil {
				t.Fatalf("expected type error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected error containing %q, got: %v", tt.want, err)
			}
		})
	}
}

func TestFieldAndMethodSameNameValid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name: "function typed field without method",
			input: `
struct Shape:
    area: () -> Int
    def twice() -> Int:
        @area() * 2
s = Shape(area: ||: 2)
a = s.area()
b = s.twice()
`,
		},
		{
			name: "different case",
			input: `
struct P:
    size: Int
    def Size() -> Int:
        @size + 100
p = P(size: 1)
n = p.Size()
`,
		},
		{
			name: "generic struct with different names",
			input: `
struct Box[T]:
    value: T
    def Get() -> T:
        @value
`,
		},
		{
			name: "outer members shadow promoted members",
			input: `
struct Inner:
    v: Int
    def hello() -> String:
        "inner"
struct Outer:
    Inner
    v: String
    def hello() -> String:
        "outer"
o = Outer(v: "s")
a: String = o.v
b: String = o.hello()
`,
		},
		{
			name: "same method name in different structs",
			input: `
struct A:
    x: Int
    def x2() -> Int:
        1
struct B:
    x2: Int
    def x() -> Int:
        2
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := inferSource(t, tt.input); err != nil {
				t.Fatalf("unexpected type error: %v", err)
			}
		})
	}
}
