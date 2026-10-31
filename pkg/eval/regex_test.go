package eval

import (
	"testing"

	"les/pkg/lexer"
	"les/pkg/parser"
)

func runEvalCode(t *testing.T, input string) (*Environment, *Evaluator) {
	t.Helper()
	l := lexer.NewLexer(input)
	p := parser.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	ev := NewEvaluator()
	_, err := ev.EvalProgram(prog)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	return ev.globalEnv, ev
}

func TestRegexAndMatchEvaluation(t *testing.T) {
	input := `
r = /foo/i
s = "FooBar"
m = s =~ r

p = r.Pattern()
f = r.Flags()
matched = r.Test("foo")
not_matched = r.Test("baz")

rep = /a/.Replace("banana", "o")
repAll = /a/.ReplaceAll("banana", "o")
parts = /,/.Split("1,2,3")
findall = /\w+/.FindAll("hello world")

t0 = m[0]
tText = m.Text()
tTarget = m.Target()
tSpan = m.Span(0)
`
	env, _ := runEvalCode(t, input)

	checkStr := func(varName, expected string) {
		val, ok := env.Get(varName)
		if !ok {
			t.Fatalf("%s not found in env", varName)
		}
		sv, ok := Force(val).(StringValue)
		if !ok || string(sv) != expected {
			t.Fatalf("%s: expected %q, got %v", varName, expected, val)
		}
	}

	checkBool := func(varName string, expected bool) {
		val, ok := env.Get(varName)
		if !ok {
			t.Fatalf("%s not found in env", varName)
		}
		bv, ok := Force(val).(BoolValue)
		if !ok || bool(bv) != expected {
			t.Fatalf("%s: expected %v, got %v", varName, expected, val)
		}
	}

	checkStr("p", "foo")
	checkStr("f", "i")
	checkBool("matched", true)
	checkBool("not_matched", false)
	checkStr("rep", "bonana")
	checkStr("repAll", "bonono")
	checkStr("t0", "Foo")
	checkStr("tText", "Foo")
	checkStr("tTarget", "FooBar")

	// parts
	partsVal, _ := env.Get("parts")
	partsTuple, ok := Force(partsVal).(*TupleValue)
	if !ok || len(partsTuple.Elements) != 3 {
		t.Fatalf("expected 3 parts, got %v", partsVal)
	}

	// findall
	faVal, _ := env.Get("findall")
	faTuple, ok := Force(faVal).(*TupleValue)
	if !ok || len(faTuple.Elements) != 2 {
		t.Fatalf("expected 2 matches in findall, got %v", faVal)
	}

	// span
	spanVal, _ := env.Get("tSpan")
	spanTuple, ok := Force(spanVal).(*TupleValue)
	if !ok || len(spanTuple.Elements) != 2 {
		t.Fatalf("expected span tuple (0, 3), got %v", spanVal)
	}
	s0, _ := Force(spanTuple.Elements[0]).(IntValue).ToInt()
	s1, _ := Force(spanTuple.Elements[1]).(IntValue).ToInt()
	if s0 != 0 || s1 != 3 {
		t.Fatalf("expected span (0, 3), got (%d, %d)", s0, s1)
	}
}

func TestRegexNamedGroupsAndPatternMatch(t *testing.T) {
	input := `
r = /(?P<year>\d{4})-(?P<month>\d{2})/
s = "2026-10"
m = s =~ r

year = m["year"]
month = m["month"]
groups = m.Groups()
named = m.NamedGroups()

res_y = match s:
    case /(?P<y>\d{4})-(?P<m>\d{2})/ as res:
        res["y"]
    case _:
        "fail"

is_match = "2026" is /^\d+$/
is_not_match = "abc" is /^\d+$/
`
	env, _ := runEvalCode(t, input)

	checkStr := func(varName, expected string) {
		val, ok := env.Get(varName)
		if !ok {
			t.Fatalf("%s not found in env", varName)
		}
		sv, ok := Force(val).(StringValue)
		if !ok || string(sv) != expected {
			t.Fatalf("%s: expected %q, got %v", varName, expected, val)
		}
	}

	checkBool := func(varName string, expected bool) {
		val, ok := env.Get(varName)
		if !ok {
			t.Fatalf("%s not found in env", varName)
		}
		bv, ok := Force(val).(BoolValue)
		if !ok || bool(bv) != expected {
			t.Fatalf("%s: expected %v, got %v", varName, expected, val)
		}
	}

	checkStr("year", "2026")
	checkStr("month", "10")
	checkStr("res_y", "2026")
	checkBool("is_match", true)
	checkBool("is_not_match", false)

	// groups
	gVal, _ := env.Get("groups")
	gTuple, ok := Force(gVal).(*TupleValue)
	if !ok || len(gTuple.Elements) != 2 {
		t.Fatalf("expected 2 capture groups in groups(), got %v", gVal)
	}

	// named
	nVal, _ := env.Get("named")
	nDict, ok := Force(nVal).(*DictValue)
	if !ok || len(nDict.Entries) != 2 {
		t.Fatalf("expected 2 entries in named, got %v", nVal)
	}
}

func TestRegexEdgeCasesAndReviewFixes(t *testing.T) {
	input := `
r = /(?P<year>\d{4})-(?P<month>\d{2})/
s = "2026-10"
m = s =~ r

empty_span = m.Span("")
neg_span = m.Span(-1)
neg_start = m.Start(-1)
neg_end = m.End(-1)

r_slash = /a\/b/
r_str = r_slash.Inspect()
is_escaped = "a/b" is /^a\/b$/

val = 0
match s:
    case /\d{4}/:
        val = 42
    case _:
        val = 99
`
	env, _ := runEvalCode(t, input)

	checkInt := func(varName string, expected int) {
		val, ok := env.Get(varName)
		if !ok {
			t.Fatalf("%s not found in env", varName)
		}
		iv, ok := Force(val).(IntValue)
		if !ok {
			t.Fatalf("%s not IntValue: %v", varName, val)
		}
		i64, _ := iv.ToInt()
		if i64 != expected {
			t.Fatalf("%s: expected %d, got %d", varName, expected, i64)
		}
	}

	esVal, _ := env.Get("empty_span")
	esTuple := Force(esVal).(*TupleValue)
	s0, _ := Force(esTuple.Elements[0]).(IntValue).ToInt()
	s1, _ := Force(esTuple.Elements[1]).(IntValue).ToInt()
	if s0 != -1 || s1 != -1 {
		t.Fatalf("expected (-1, -1), got (%d, %d)", s0, s1)
	}

	checkInt("neg_start", 5)
	checkInt("neg_end", 7)

	rStrVal, _ := env.Get("r_str")
	if string(Force(rStrVal).(StringValue)) != "/a\\/b/" {
		t.Fatalf("expected /a\\/b/, got %s", Force(rStrVal))
	}

	isEscVal, _ := env.Get("is_escaped")
	if !bool(Force(isEscVal).(BoolValue)) {
		t.Fatalf("expected is_escaped to be true")
	}

	checkInt("val", 42)
}

func TestRegexDuplicateNamedGroups(t *testing.T) {
	input := `
r = /(?P<x>\d+)|(?P<x>[a-z]+)/
m1 = "abc" =~ r
m2 = "123" =~ r

x1 = m1["x"]
span1 = m1.Span("x")
start1 = m1.Start("x")
end1 = m1.End("x")

x2 = m2["x"]
span2 = m2.Span("x")
start2 = m2.Start("x")
end2 = m2.End("x")
`
	env, _ := runEvalCode(t, input)

	checkStr := func(varName, expected string) {
		val, ok := env.Get(varName)
		if !ok {
			t.Fatalf("%s not found in env", varName)
		}
		sv, ok := Force(val).(StringValue)
		if !ok || string(sv) != expected {
			t.Fatalf("%s: expected %q, got %v", varName, expected, val)
		}
	}

	checkInt := func(varName string, expected int) {
		val, ok := env.Get(varName)
		if !ok {
			t.Fatalf("%s not found in env", varName)
		}
		iv, ok := Force(val).(IntValue)
		if !ok {
			t.Fatalf("%s not IntValue: %v", varName, val)
		}
		i, _ := iv.ToInt()
		if i != expected {
			t.Fatalf("%s: expected %d, got %d", varName, expected, i)
		}
	}

	checkStr("x1", "abc")
	checkInt("start1", 0)
	checkInt("end1", 3)

	checkStr("x2", "123")
	checkInt("start2", 0)
	checkInt("end2", 3)
}

func TestRegexInUnionIsExpr(t *testing.T) {
	input := `
b1 = "123" is /^\d+$/ | Int
b2 = 123 is /^\d+$/ | Int
b3 = "abc" is /^\d+$/ | Int
b4 = "hello" is (/^[a-z]+$/ | Int)
b5 = "xyz" is (/^a|b$/ | String)
`
	env, _ := runEvalCode(t, input)
	checkBool := func(varName string, expected bool) {
		val, ok := env.Get(varName)
		if !ok {
			t.Fatalf("%s not found in env", varName)
		}
		bv, ok := Force(val).(BoolValue)
		if !ok || bool(bv) != expected {
			t.Fatalf("%s: expected %v, got %v", varName, expected, val)
		}
	}
	checkBool("b1", true)
	checkBool("b2", true)
	checkBool("b3", false)
	checkBool("b4", true)
	checkBool("b5", true)
}

func TestRegexThunkMatching(t *testing.T) {
	input := `
lazy_bad = "abc" + "def"
lazy_good = "123" + "456"
b1 = lazy_bad is /^\d+$/
b2 = lazy_good is /^\d+$/
b3 = (lazy_bad, 1) is (/^\d+$/, Int)
b4 = (lazy_good, 1) is (/^\d+$/, Int)
`
	env, _ := runEvalCode(t, input)
	checkBool := func(varName string, expected bool) {
		val, ok := env.Get(varName)
		if !ok {
			t.Fatalf("%s not found in env", varName)
		}
		bv, ok := Force(val).(BoolValue)
		if !ok || bool(bv) != expected {
			t.Fatalf("%s: expected %v, got %v", varName, expected, val)
		}
	}
	checkBool("b1", false)
	checkBool("b2", true)
	checkBool("b3", false)
	checkBool("b4", true)
}

func TestMatchValueDictKeyDifferentSpans(t *testing.T) {
	input := `
s = "abc_abc"
matches = /abc/.FindAll(s)
m1 = matches[0]
m2 = matches[1]
d = {m1: 10, m2: 20}
v1 = d[m1]
v2 = d[m2]
size = d.Len()
`
	env, _ := runEvalCode(t, input)
	checkInt := func(varName string, expected int) {
		val, ok := env.Get(varName)
		if !ok {
			t.Fatalf("%s not found in env", varName)
		}
		iv, ok := Force(val).(IntValue)
		if !ok {
			t.Fatalf("%s not IntValue: %v", varName, val)
		}
		i, _ := iv.ToInt()
		if i != expected {
			t.Fatalf("%s: expected %d, got %d", varName, expected, i)
		}
	}
	checkInt("size", 2)
	checkInt("v1", 10)
	checkInt("v2", 20)
}

func TestMatchCaseUnionRegexExecution(t *testing.T) {
	input := `
def classify(x):
    match x:
        case /^\d+$/ | Int:
            return "number"
        case /[a/b]/ | String:
            return "string"
        case _:
            return "other"

r1 = classify("123")
r2 = classify(456)
r3 = classify("a/b")
r4 = classify("xyz")
r5 = classify(true)
`
	env, _ := runEvalCode(t, input)
	checkStr := func(varName, expected string) {
		val, ok := env.Get(varName)
		if !ok {
			t.Fatalf("%s not found in env", varName)
		}
		sv, ok := Force(val).(StringValue)
		if !ok || string(sv) != expected {
			t.Fatalf("%s: expected %q, got %v", varName, expected, val)
		}
	}
	checkStr("r1", "number")
	checkStr("r2", "number")
	checkStr("r3", "string")
	checkStr("r4", "string")
	checkStr("r5", "other")
}

func TestRegexDictPatternEvalAndIncompatibleThunkLaziness(t *testing.T) {
	input := `
d1 = {"foo": "abc"}
d2 = {"abc": 123}
m1 = d1 is {String: /(?i:abc)/}
m2 = d2 is {/(?i:abc)/: Int}
m3 = d1 is {String: /(?i:xyz)/}

lazy_incompat_dict = {"val": 1 / 0}
b_incompat = lazy_incompat_dict is {String: /^\d+$/}
lazy_incompat_tup = (1 / 0, "abc")
b_incompat2 = lazy_incompat_tup is (/^\d+$/, String)
`
	env, _ := runEvalCode(t, input)
	checkBool := func(varName string, expected bool) {
		val, ok := env.Get(varName)
		if !ok {
			t.Fatalf("%s not found in env", varName)
		}
		bv, ok := Force(val).(BoolValue)
		if !ok || bool(bv) != expected {
			t.Fatalf("%s: expected %v, got %v", varName, expected, val)
		}
	}
	checkBool("m1", true)
	checkBool("m2", true)
	checkBool("m3", false)
	checkBool("b_incompat", false)
	checkBool("b_incompat2", false)
}

func TestMatchCaseWildcardAndMultiParen(t *testing.T) {
	input := `
def classifyPair(p):
    match p:
        case (/^\d+$/, _):
            return "digit-pair"
        case {String: _}:
            return "any-string-dict"
        case ((/^[a-z]+$/)) as m:
            return m[0]
        case (true):
            return "true-val"
        case (...r) as allTup:
            return "fallback-tup"

r1 = classifyPair(("123", true))
r2 = classifyPair({"k": 42})
r3 = classifyPair("abc")
r4 = classifyPair(true)
r5 = classifyPair((1, 2, 3))
`
	env, _ := runEvalCode(t, input)
	checkStr := func(varName, expected string) {
		val, ok := env.Get(varName)
		if !ok {
			t.Fatalf("%s not found in env", varName)
		}
		sv, ok := Force(val).(StringValue)
		if !ok || string(sv) != expected {
			t.Fatalf("%s: expected %q, got %v", varName, expected, val)
		}
	}
	checkStr("r1", "digit-pair")
	checkStr("r2", "any-string-dict")
	checkStr("r3", "abc")
	checkStr("r4", "true-val")
	checkStr("r5", "fallback-tup")
}

func TestParseGenericTypeAnnotArrowAndMultipleBrackets(t *testing.T) {
	_, _, isGen1 := parseGenericTypeAnnot("Foo[Int] -> Bar[String]")
	if isGen1 {
		t.Fatalf("expected Foo[Int] -> Bar[String] not to be parsed as generic type")
	}

	base2, args2, isGen2 := parseGenericTypeAnnot("Pair[Int, String]")
	if !isGen2 || base2 != "Pair" || len(args2) != 2 || args2[0] != "Int" || args2[1] != "String" {
		t.Fatalf("expected Pair[Int, String] to be generic, got %v, %v, %v", isGen2, base2, args2)
	}

	_, _, isGen3 := parseGenericTypeAnnot("(Pair[Int, String])")
	if isGen3 {
		t.Fatalf("expected (Pair[Int, String]) not to be generic")
	}

	// Test DictKey uniqueness between Regex("a/b") and Regex("a\\\\/b")
	r1, err1 := CompileRegex("a/b", "")
	if err1 != nil {
		t.Fatalf("CompileRegex failed: %v", err1)
	}
	r2, err2 := CompileRegex("a\\\\/b", "")
	if err2 != nil {
		t.Fatalf("CompileRegex failed: %v", err2)
	}
	k1 := MakeDictKey(r1)
	k2 := MakeDictKey(r2)
	if k1 == k2 {
		t.Fatalf("expected distinct DictKeys for r1 and r2, got identical %v", k1)
	}
}

func TestThunkBooleanPatternMatching(t *testing.T) {
	input := `
p1 = (1 == 2, 10)
p2 = (1 == 1, 10)
b1 = p1 is (true, Int)
b2 = p2 is (true, Int)
b3 = p1 is (false, Int)
`
	env, _ := runEvalCode(t, input)
	checkBool := func(varName string, expected bool) {
		val, ok := env.Get(varName)
		if !ok {
			t.Fatalf("%s not found in env", varName)
		}
		bv, ok := Force(val).(BoolValue)
		if !ok || bool(bv) != expected {
			t.Fatalf("%s: expected %v, got %v", varName, expected, val)
		}
	}
	checkBool("b1", false)
	checkBool("b2", true)
	checkBool("b3", true)
}
