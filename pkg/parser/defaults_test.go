package parser

import (
	"strings"
	"testing"

	"les/pkg/ast"
	"les/pkg/lexer"
)

func TestParseTrailingDefaultsAndBlockParam(t *testing.T) {
	input := `
def greet(name: String, prefix: String = "Hello", &block = nil):
    pass

interface Greeter:
    def greet(name: String, prefix: String = "Hello", &block): String
`
	l := lexer.NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(prog.Body) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(prog.Body))
	}
	fn, ok := prog.Body[0].(*ast.FuncDef)
	if !ok {
		t.Fatalf("expected FuncDef, got %T", prog.Body[0])
	}
	if len(fn.Params) != 2 {
		t.Fatalf("expected 2 normal params, got %d", len(fn.Params))
	}
	if fn.Params[1].DefaultValue == nil {
		t.Fatalf("expected prefix to have default value")
	}
	if fn.BlockParam == nil || fn.BlockParam.Name != "block" {
		t.Fatalf("expected BlockParam 'block', got %+v", fn.BlockParam)
	}
	if fn.BlockParam.DefaultValue == nil {
		t.Fatalf("expected BlockParam default value nil")
	}

	iface, ok := prog.Body[1].(*ast.InterfaceDef)
	if !ok {
		t.Fatalf("expected InterfaceDef, got %T", prog.Body[1])
	}
	m := iface.Methods[0]
	if len(m.Params) != 2 {
		t.Fatalf("expected 2 params in interface method, got %d", len(m.Params))
	}
	if m.Params[1].DefaultValue == nil {
		t.Fatalf("expected default value for prefix in interface method")
	}
	if m.BlockParam == nil || m.BlockParam.Name != "block" {
		t.Fatalf("expected BlockParam 'block' in interface method, got %+v", m.BlockParam)
	}
	if m.BlockParam.DefaultValue != nil {
		t.Fatalf("expected no default value for block in interface method")
	}
}

func TestParseTrailingDefaultsErrors(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		errSub string
	}{
		{
			name:   "required after optional",
			input:  "def f(a = 1, b):\n    pass\n",
			errSub: "cannot follow default parameter",
		},
		{
			name:   "default for variadic",
			input:  "def f(*args = []):\n    pass\n",
			errSub: "cannot have a default value",
		},
		{
			name:   "param after block",
			input:  "def f(&block, a):\n    pass\n",
			errSub: "must be the last parameter",
		},
		{
			name:   "multiple block params",
			input:  "def f(&b1, &b2):\n    pass\n",
			errSub: "must be the last parameter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lexer.NewLexer(tt.input)
			p := NewParser(l)
			p.ParseProgram()
			if len(p.Errors()) == 0 {
				t.Fatalf("expected error containing %q, got none", tt.errSub)
			}
			found := false
			for _, err := range p.Errors() {
				if strings.Contains(err, tt.errSub) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected error containing %q, got errors: %v", tt.errSub, p.Errors())
			}
		})
	}
}
