package types

import (
	"les/pkg/lexer"
	"les/pkg/parser"
	"testing"
)

func TestRegexAndMatchTypes(t *testing.T) {
	input := `r = /foo/i
s = "foobar"
m = s =~ r
if m != nil:
    print m.Text()
    print m[0]
    print m[1]
    print m["group"]

match s:
    case /bar/ as m2:
        print m2.Len()
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

	rType, ok := inf.scope.Lookup("r")
	if !ok || rType != TypeRegex {
		t.Fatalf("expected r to be TypeRegex, got %v", rType)
	}

	mType, ok := inf.scope.Lookup("m")
	if !ok {
		t.Fatalf("m not found in scope")
	}
	expectedMUnion := NewUnionType(TypeMatch, TypeNil)
	if !mType.Equals(expectedMUnion) {
		t.Fatalf("expected m to be %v, got %v", expectedMUnion, mType)
	}
}

func TestRegexIsExprNarrowing(t *testing.T) {
	input := `
def check(x: String | Int):
    if x is /^[0-9]+$/:
        print x.Len()
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
}

func TestRegexMatchCaseNarrowing(t *testing.T) {
	input := `
def check(x: String | Int):
    match x:
        case /^[0-9]+$/:
            print x.Len()
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
}

func TestInvalidRegexTypeErrors(t *testing.T) {
	cases := []string{
		`r = /a(b/`,
		`b = "test" is /a(b/`,
	}
	for _, input := range cases {
		l := lexer.NewLexer(input)
		p := parser.NewParser(l)
		prog := p.ParseProgram()
		if len(p.Errors()) > 0 {
			continue
		}
		inf := NewInferencer()
		err := inf.InferProgram(prog)
		if err == nil {
			t.Fatalf("expected type error for invalid regex %q", input)
		}
	}
}

func TestRegexInUnionTypeAnnotation(t *testing.T) {
	input := `
def check(x: String | Int):
    if x is /^[0-9]+$/ | Int:
        print "matched"
    if x is (/^a|b$/ | Int):
        print "union with regex containing pipe"
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
}

func TestTypeParamRegexMatchMismatch(t *testing.T) {
	input := `
def check[T: String](x: T):
    m = x =~ 123
`
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	inf := NewInferencer()
	err := inf.InferProgram(prog)
	if err == nil {
		t.Fatalf("expected type error for x =~ 123 with T: String")
	}
}

func TestProhibitRegexInOrdinaryTypeAnnotation(t *testing.T) {
	cases := []string{
		`x: /foo/ = "bar"`,
		`def f(x: /^\d+$/): pass`,
		`struct S:
    x: /foo/`,
	}
	for _, input := range cases {
		l := lexer.NewLexer(input)
		p := parser.NewParser(l)
		prog := p.ParseProgram()
		if len(p.Errors()) > 0 {
			continue
		}
		inf := NewInferencer()
		err := inf.InferProgram(prog)
		if err == nil {
			t.Fatalf("expected type error for regex in ordinary type annotation %q", input)
		}
	}
}

func TestRegexMatchExhaustivenessNotCovered(t *testing.T) {
	input := `
def test(s: String): String:
    match s:
        case /^\d+$/:
            return "digit"
`
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	inf := NewInferencer()
	err := inf.InferProgram(prog)
	if err == nil {
		t.Fatalf("expected exhaustiveness/return mismatch error for regex case in String-returning function")
	}
}
