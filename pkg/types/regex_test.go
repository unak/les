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


func TestRegexTupleAndDictPatternInference(t *testing.T) {
	inf := NewInferencer()
	nTup, cTup, err := inf.ParsePatternTypeAnnot(`(/^\d+$/, Int)`)
	if err != nil {
		t.Fatalf(`failed to parse tuple pattern: %v`, err)
	}
	if nTup == nil || cTup != nil {
		t.Fatalf(`expected non-nil narrowType and nil coveredType for (/^\d+$/, Int)`)
	}

	nDict, cDict, err := inf.ParsePatternTypeAnnot(`{String: /(?i:abc)/}`)
	if err != nil {
		t.Fatalf(`failed to parse dict pattern: %v`, err)
	}
	if nDict == nil || cDict != nil {
		t.Fatalf(`expected non-nil narrowType and nil coveredType for {String: /(?i:abc)/}`)
	}

	input := `
def test(p: (String, Int)): String:
    match p:
        case (/^\d+$/, Int):
            return "digit-pair"
`
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf(`parser errors: %v`, p.Errors())
	}
	inf2 := NewInferencer()
	err2 := inf2.InferProgram(prog)
	if err2 == nil {
		t.Fatalf(`expected exhaustiveness error for tuple regex pattern`)
	}

	// Verify nested func type with enclosing parens is not treated as tuple
	nFunc, _, errFunc := inf.ParsePatternTypeAnnot(`((String) -> ()) -> ()`)
	if errFunc != nil {
		t.Fatalf(`failed to parse nested func type: %v`, errFunc)
	}
	if _, ok := nFunc.(*FuncType); !ok {
		t.Fatalf(`expected FuncType, got %T`, nFunc)
	}

	// Verify single regex literal trailing junk rejected
	_, _, errJunk := inf.ParsePatternTypeAnnot(`/abc/ extra`)
	if errJunk == nil {
		t.Fatalf(`expected error for regex with trailing junk`)
	}

	// Verify POSIX char class in pattern type annotation
	nPosix, _, errPosix := inf.ParsePatternTypeAnnot(`/[[:alpha:]/]/`)
	if errPosix != nil {
		t.Fatalf(`failed to parse POSIX char class regex: %v`, errPosix)
	}
	if nPosix != TypeString {
		t.Fatalf(`expected TypeString, got %v`, nPosix)
	}

	// Verify composite pattern with wildcard _
	nTupWild, cTupWild, errTupWild := inf.ParsePatternTypeAnnot(`(/^\d+$/, _)`)
	if errTupWild != nil {
		t.Fatalf(`failed to parse composite tuple with _: %v`, errTupWild)
	}
	if nTupWild == nil || cTupWild != nil {
		t.Fatalf(`expected non-nil narrowType and nil coveredType for (/^\d+$/, _)`)
	}

	nDictWild, cDictWild, errDictWild := inf.ParsePatternTypeAnnot(`{String: _}`)
	if errDictWild != nil {
		t.Fatalf(`failed to parse composite dict with _: %v`, errDictWild)
	}
	if nDictWild == nil || cDictWild == nil {
		t.Fatalf(`expected non-nil narrowType and non-nil coveredType for {String: _}`)
	}

	// Verify (...r) exhaustiveness
	codeRest := `
def testRest(t: (Int, String)): Int:
    match t:
        case (...r):
            return 42
`
	lRest := lexer.NewLexer(codeRest)
	pRest := parser.NewParser(lRest)
	progRest := pRest.ParseProgram()
	if len(pRest.Errors()) > 0 {
		t.Fatalf(`parser errors: %v`, pRest.Errors())
	}
	infRest := NewInferencer()
	if err := infRest.InferProgram(progRest); err != nil {
		t.Fatalf(`expected (...r) to be exhaustive, got %v`, err)
	}

	// Verify true | false union exhaustiveness in pattern
	_, cBoolUnion, errBoolUnion := inf.ParsePatternTypeAnnot(`true | false`)
	if errBoolUnion != nil {
		t.Fatalf(`failed to parse true | false union: %v`, errBoolUnion)
	}
	if cBoolUnion != TypeBool {
		t.Fatalf(`expected coveredType TypeBool, got %v`, cBoolUnion)
	}

	// Verify (true | false, Int) exhaustiveness
	codeBoolTup := `
def testBoolTup(p: (Bool, Int)): String:
    match p:
        case (true | false, Int):
            return ok
`
	lBoolTup := lexer.NewLexer(codeBoolTup)
	pBoolTup := parser.NewParser(lBoolTup)
	progBoolTup := pBoolTup.ParseProgram()
	if len(pBoolTup.Errors()) > 0 {
		t.Fatalf(`parser errors: %v`, pBoolTup.Errors())
	}
	infBoolTup := NewInferencer()
	if err := infBoolTup.InferProgram(progBoolTup); err != nil {
		t.Fatalf(`expected (true | false, Int) to be exhaustive, got %v`, err)
	}

	// Verify case _ as x and case true as x variable binding
	codeBindings := `
def testBindings(x: Int, b: Bool): Int:
    match x:
        case _ as anyVal:
            print anyVal
    match b:
        case true as t:
            print t
        case false as f:
            print f
    return 0
`
	lBind := lexer.NewLexer(codeBindings)
	pBind := parser.NewParser(lBind)
	progBind := pBind.ParseProgram()
	if len(pBind.Errors()) > 0 {
		t.Fatalf(`parser errors: %v`, pBind.Errors())
	}
	infBind := NewInferencer()
	if err := infBind.InferProgram(progBind); err != nil {
		t.Fatalf(`expected bindings to typecheck, got %v`, err)
	}
}
