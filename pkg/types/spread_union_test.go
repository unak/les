package types

import (
	"strings"
	"testing"

	"les/pkg/lexer"
	"les/pkg/parser"
)

func TestSpreadUnionOfTuples(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		varName string
		want    string
		wantErr string
	}{
		{
			name: "recursive take with unannotated stream",
			code: `
def take(k: Int, s):
    k <= 0 ? () : (s[0], ...take(k - 1, s[1]))
r = take(2, (1, (2, ())))
`,
		},
		{
			name: "variadic tuples of different element types",
			code: `
def pick(b: Bool) -> (Int, ...) | (String, ...):
    b ? (1,) : ("x",)
t: (...) = ()
t = (...pick(true), 2)
r = (...pick(true), 2)
`,
			varName: "r",
			want:    "(Int | String, ...)",
		},
		{
			name: "fixed-length tuples of different lengths",
			code: `
def pick(b: Bool) -> (Int,) | (String, Bool):
    b ? (1,) : ("x", true)
r = (0, ...pick(true), nil)
`,
			varName: "r",
			want:    "(Int, Int | String | Bool | Nil, ...)",
		},
		{
			name: "union of fixed tuples with an empty tuple",
			code: `
def pick(b: Bool) -> () | (Int, Int):
    b ? () : (1, 2)
r = (...pick(true))
`,
			varName: "r",
			want:    "(Int, ...)",
		},
		{
			name: "union containing Unknown element",
			code: `
def pick(b: Bool) -> (Int, ...) | (...):
    b ? (1,) : ("x",)
r = (...pick(true), 2)
`,
			varName: "r",
			want:    "(...)",
		},
		{
			name: "union of empty and variadic tuple",
			code: `
def pick(b: Bool) -> () | (Int, ...):
    b ? () : (1,)
r: (Int, ...) = (...pick(true))
`,
		},
		{
			name:    "union with Nil member",
			code:    "def f(b: Bool) -> (Int, ...) | Nil:\n    b ? (1,) : nil\nr = (...f(true))\n",
			wantErr: "cannot spread non-tuple value of type (Int, ...) | Nil",
		},
		{
			name:    "union with Int member",
			code:    "def f(b: Bool) -> (Int, ...) | Int:\n    b ? (1,) : 1\nr = (...f(true))\n",
			wantErr: "cannot spread non-tuple value of type (Int, ...) | Int",
		},
		{
			name:    "union of only non-tuples",
			code:    "def f(b: Bool) -> Int | String:\n    b ? 1 : \"x\"\nr = (...f(true))\n",
			wantErr: "cannot spread non-tuple value",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog := parser.NewParser(lexer.NewLexer(tt.code)).ParseProgram()
			inf := NewInferencer()
			err := inf.InferProgram(prog)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got none", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected inference to succeed, got %v", err)
			}
			if tt.varName == "" {
				return
			}
			got, ok := inf.scope.Lookup(tt.varName)
			if !ok {
				t.Fatalf("variable %q not found", tt.varName)
			}
			if got.String() != tt.want {
				t.Fatalf("expected %s to be %s, got %s", tt.varName, tt.want, got)
			}
		})
	}
}
