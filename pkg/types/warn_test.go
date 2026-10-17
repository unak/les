package types

import (
	"testing"

	"les/pkg/lexer"
	"les/pkg/parser"
)

func TestWarnInferredUnknownType(t *testing.T) {
	tests := []struct {
		name         string
		code         string
		wantWarnings []string
	}{
		{
			name: "unannotated function param and return",
			code: `def f(x):
    return x
`,
			wantWarnings: []string{
				"1: Unknown type of parameter 'x' in function 'f' at col 7",
				"1: Unknown type of return value in function 'f' at col 1",
			},
		},
		{
			name: "fully annotated function has no warnings",
			code: `def f(x: Int): Int:
    return x
`,
			wantWarnings: nil,
		},
		{
			name: "unannotated block param",
			code: `def run(&block):
    return 10
`,
			wantWarnings: []string{
				"1: Unknown type of block parameter '&block' in function 'run' at col 10",
			},
		},
		{
			name: "unannotated lambda param",
			code: `lam = |x|: x + 1`,
			wantWarnings: []string{
				"1: Unknown type of parameter 'x' in lambda at col 8",
			},
		},
		{
			name: "empty dict and empty tuple are exempted",
			code: `d = {}
t = ()
x = 10
`,
			wantWarnings: nil,
		},
		{
			name: "unannotated var assigned unknown identifier",
			code: `y = undefined_ident`,
			wantWarnings: []string{
				"1: Unknown type of variable 'y' at col 1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := lexer.NewLexer(tt.code)
			p := parser.NewParser(l)
			prog := p.ParseProgram()
			if len(p.Errors()) > 0 {
				t.Fatalf("unexpected parser errors: %v", p.Errors())
			}

			inf := NewInferencer()
			_ = inf.InferProgram(prog)

			warnings := inf.Warnings()
			if len(warnings) != len(tt.wantWarnings) {
				t.Fatalf("expected %d warnings, got %d: %v", len(tt.wantWarnings), len(warnings), warnings)
			}
			for i, want := range tt.wantWarnings {
				got := warnings[i].String()
				if got != want {
					t.Errorf("warning[%d]: expected %q, got %q", i, want, got)
				}
			}
		})
	}
}

func TestWarnInferredUnknownTypeDisabled(t *testing.T) {
	code := `def f(x):
    return x
`
	l := lexer.NewLexer(code)
	p := parser.NewParser(l)
	prog := p.ParseProgram()

	inf := NewInferencer()
	inf.SetWarnUnknown(false)
	_ = inf.InferProgram(prog)

	if len(inf.Warnings()) != 0 {
		t.Fatalf("expected 0 warnings when disabled, got %d", len(inf.Warnings()))
	}
}
