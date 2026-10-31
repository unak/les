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
